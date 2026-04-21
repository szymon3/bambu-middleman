package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/szymon3/bambu-middleman/printer"
)

func main() {
	cfg, err := printer.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	outPath := os.Getenv("DUMP_FILE")
	if outPath == "" {
		outPath = "mqtt_dump_" + time.Now().Format("20060102_150405") + ".jsonl"
	}

	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create dump file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Fprintf(os.Stderr, "Connecting to %s...\n", cfg.MQTTBrokerAddr())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var count int
	connLost := make(chan struct{}, 1)

	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tls://%s", cfg.MQTTBrokerAddr())).
		SetClientID("bambu-mqttdump").
		SetUsername("bblp").
		SetPassword(cfg.AccessCode).
		SetKeepAlive(30 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetAutoReconnect(false).
		SetTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // printer uses a self-signed certificate
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			fmt.Fprintf(os.Stderr, "Connection lost: %v\n", err)
			select {
			case connLost <- struct{}{}:
			default:
			}
		})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		fmt.Fprintf(os.Stderr, "Connection timed out\n")
		os.Exit(1)
	}
	if err := token.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		os.Exit(1)
	}

	handler := func(_ mqtt.Client, msg mqtt.Message) {
		// Parse into a raw JSON value so we embed it unescaped.
		var raw json.RawMessage
		if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
			// Fallback: store as a JSON string.
			raw, _ = json.Marshal(string(msg.Payload()))
		}

		line, _ := json.Marshal(struct {
			TS      string          `json:"ts"`
			Payload json.RawMessage `json:"payload"`
		}{
			TS:      time.Now().UTC().Format(time.RFC3339Nano),
			Payload: raw,
		})
		fmt.Fprintf(f, "%s\n", line)

		count++
		fmt.Fprintf(os.Stderr, "[%s] message #%d\n", time.Now().Format("15:04:05"), count)
	}

	subToken := client.Subscribe(cfg.ReportTopic(), 0, handler)
	if !subToken.WaitTimeout(10 * time.Second) {
		fmt.Fprintf(os.Stderr, "Subscribe timed out\n")
		os.Exit(1)
	}
	if err := subToken.Error(); err != nil {
		fmt.Fprintf(os.Stderr, "Subscribe failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Logging to %s (Ctrl+C to stop)\n", outPath)

	select {
	case <-ctx.Done():
	case <-connLost:
	}

	client.Disconnect(500)
	fmt.Fprintf(os.Stderr, "Wrote %d messages. Done.\n", count)
}
