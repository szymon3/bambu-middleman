package printer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	eventBufferSize   = 16
	reconnectBaseWait = 3 * time.Second
	reconnectMaxWait  = 60 * time.Second
	keepAlive         = 30 * time.Second
	connectTimeout    = 10 * time.Second
)

// MQTTClient connects to the printer's local MQTT broker and emits PrintEvents
// for terminal print states (FINISH, FAILED).
type MQTTClient struct {
	cfg    Config
	log    *slog.Logger
	events chan PrintEvent

	// mu protects the print lifecycle state below. paho may invoke the message
	// handler from multiple goroutines.
	mu              sync.Mutex
	printActive     bool   // true after PREPARE/RUNNING, reset on terminal event
	lastGCodeFile   string // captured from any non-empty gcode_file in flight
	lastSubtaskName string // captured from any non-empty subtask_name in flight
}

// NewMQTTClient creates a new MQTTClient. Call Run to start the connection loop.
func NewMQTTClient(cfg Config, log *slog.Logger) *MQTTClient {
	return &MQTTClient{
		cfg:    cfg,
		log:    log,
		events: make(chan PrintEvent, eventBufferSize),
	}
}

// Events returns a read-only channel that receives a PrintEvent whenever a print
// reaches a terminal state (FINISH or FAILED).
func (c *MQTTClient) Events() <-chan PrintEvent {
	return c.events
}

// Run starts the connection loop. It connects to the printer MQTT broker, subscribes
// to the report topic, and reconnects with exponential backoff on disconnection.
// Blocks until ctx is cancelled; closes the Events channel on return.
func (c *MQTTClient) Run(ctx context.Context) {
	defer close(c.events)

	attempt := 0
	for {
		connected, err := c.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			wait := backoffWait(attempt)
			c.log.Error("mqtt connect failed", "err", err, "retry_in", wait.String(), "attempt", attempt)
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}

		if !connected {
			// ctx was cancelled before we could connect.
			return
		}

		// Connection established; reset backoff.
		attempt = 0
		c.log.Info("mqtt connected", "broker", c.cfg.MQTTBrokerAddr())

		// connect blocks until context is cancelled or connection drops.
		// It returns nil in both cases; we distinguish via ctx.Err().
		if ctx.Err() != nil {
			return
		}
		// Connection was lost; wait briefly then reconnect.
		wait := backoffWait(attempt)
		c.log.Info("mqtt reconnecting after connection loss", "retry_in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// connect dials the broker, subscribes, and blocks until the context is cancelled
// or the connection is lost. Returns (true, nil) on success, (false, nil) if ctx
// was already done, or (false, err) if the connection or subscribe failed.
func (c *MQTTClient) connect(ctx context.Context) (bool, error) {
	if ctx.Err() != nil {
		return false, nil
	}

	connLost := make(chan error, 1)

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tls://%s", c.cfg.MQTTBrokerAddr())).
		SetClientID("bambu-middleman").
		SetUsername("bblp").
		SetPassword(c.cfg.AccessCode).
		SetKeepAlive(keepAlive).
		SetConnectTimeout(connectTimeout).
		SetAutoReconnect(false). // we manage reconnect ourselves
		SetTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // printer uses a self-signed certificate
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			c.log.Warn("mqtt connection lost", "err", err)
			select {
			case connLost <- err:
			default:
			}
		})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(connectTimeout) {
		return false, fmt.Errorf("connection timed out")
	}
	if err := token.Error(); err != nil {
		return false, fmt.Errorf("connect: %w", err)
	}

	subToken := client.Subscribe(c.cfg.ReportTopic(), 0, c.handleMessage)
	if !subToken.WaitTimeout(connectTimeout) {
		client.Disconnect(250)
		return false, fmt.Errorf("subscribe timed out")
	}
	if err := subToken.Error(); err != nil {
		client.Disconnect(250)
		return false, fmt.Errorf("subscribe: %w", err)
	}

	c.log.Info("mqtt subscribed", "topic", c.cfg.ReportTopic())

	select {
	case <-ctx.Done():
		client.Disconnect(250)
	case <-connLost:
		// Outer Run loop will handle reconnect.
	}
	return true, nil
}

func (c *MQTTClient) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	var report MQTTReport
	if err := json.Unmarshal(msg.Payload(), &report); err != nil {
		c.log.Warn("mqtt message parse error", "err", err)
		return
	}

	p := report.Print
	if p.GCodeState == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Always capture non-empty file/subtask fields as they are present in
	// early messages but absent from the terminal FINISH/FAILED message.
	if p.GCodeFile != "" {
		c.lastGCodeFile = p.GCodeFile
	}
	if p.SubtaskName != "" {
		c.lastSubtaskName = p.SubtaskName
	}

	switch p.GCodeState {
	case StatePrepare, StateRunning:
		c.printActive = true
	case StateIdle:
		c.printActive = false
	case StateFinish, StateFailed:
		if !c.printActive {
			// Spurious terminal state — printer sends FINISH at startup/idle.
			c.log.Debug("terminal state with no active print, ignoring", "state", p.GCodeState)
			return
		}

		file := c.lastGCodeFile
		subtask := c.lastSubtaskName

		// Always reset so subsequent duplicate FINISH messages are ignored.
		c.printActive = false
		c.lastGCodeFile = ""
		c.lastSubtaskName = ""

		if file == "" {
			c.log.Warn("terminal state with no gcode_file tracked, skipping", "state", p.GCodeState)
			return
		}

		event := PrintEvent{
			State:       p.GCodeState,
			GCodeFile:   file,
			SubtaskName: subtask,
		}
		select {
		case c.events <- event:
		default:
			c.log.Warn("event channel full, dropping print event", "state", p.GCodeState, "file", file)
		}
	}
}

// backoffWait returns the exponential backoff duration for the given attempt (0-indexed),
// capped at reconnectMaxWait.
func backoffWait(attempt int) time.Duration {
	wait := reconnectBaseWait
	for i := 0; i < attempt; i++ {
		wait *= 2
		if wait >= reconnectMaxWait {
			return reconnectMaxWait
		}
	}
	return wait
}
