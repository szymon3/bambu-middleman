package printer

import (
	"encoding/json"
	"log/slog"
	"testing"
)

// mockMQTTMessage implements mqtt.Message for testing handleMessage directly.
type mockMQTTMessage struct {
	payload []byte
}

func (m *mockMQTTMessage) Duplicate() bool   { return false }
func (m *mockMQTTMessage) Qos() byte         { return 0 }
func (m *mockMQTTMessage) Retained() bool    { return false }
func (m *mockMQTTMessage) Topic() string     { return "" }
func (m *mockMQTTMessage) MessageID() uint16 { return 0 }
func (m *mockMQTTMessage) Payload() []byte   { return m.payload }
func (m *mockMQTTMessage) Ack()              {}

func newTestClient() *MQTTClient {
	return &MQTTClient{
		log:               slog.Default(),
		events:            make(chan PrintEvent, eventBufferSize),
		filamentLoads:     make(chan struct{}, eventBufferSize),
		lastHWSwitchState: -1,
	}
}

func expectFilamentLoad(t *testing.T, c *MQTTClient) {
	t.Helper()
	select {
	case _, ok := <-c.filamentLoads:
		if !ok {
			t.Fatal("filamentLoads channel closed unexpectedly")
		}
	default:
		t.Fatal("expected a filament load event but channel was empty")
	}
}

func expectNoFilamentLoad(t *testing.T, c *MQTTClient) {
	t.Helper()
	select {
	case <-c.filamentLoads:
		t.Fatal("expected no filament load event but got one")
	default:
	}
}

func sendMessage(c *MQTTClient, payload string) {
	c.handleMessage(nil, &mockMQTTMessage{payload: []byte(payload)})
}

func expectEvent(t *testing.T, c *MQTTClient) PrintEvent {
	t.Helper()
	select {
	case ev := <-c.events:
		return ev
	default:
		t.Fatal("expected an event but channel was empty")
		return PrintEvent{}
	}
}

func expectNoEvent(t *testing.T, c *MQTTClient) {
	t.Helper()
	select {
	case ev := <-c.events:
		t.Fatalf("expected no event but got %+v", ev)
	default:
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		state PrintState
		want  bool
	}{
		{StateFinish, true},
		{StateFailed, true},
		{StateIdle, false},
		{StatePrepare, false},
		{StateRunning, false},
		{StatePause, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := IsTerminal(tt.state); got != tt.want {
				t.Errorf("IsTerminal(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestMQTTReportUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantState   PrintState
		wantFile    string
		wantSubtask string
	}{
		{
			name: "finish state",
			payload: `{
				"print": {
					"gcode_state": "FINISH",
					"gcode_file": "my_model.gcode",
					"subtask_name": "my_model",
					"mc_percent": 100,
					"sequence_id": "0"
				}
			}`,
			wantState:   StateFinish,
			wantFile:    "my_model.gcode",
			wantSubtask: "my_model",
		},
		{
			name: "failed state",
			payload: `{
				"print": {
					"gcode_state": "FAILED",
					"gcode_file": "partial_print.gcode",
					"subtask_name": "partial_print",
					"mc_percent": 42,
					"sequence_id": "1"
				}
			}`,
			wantState:   StateFailed,
			wantFile:    "partial_print.gcode",
			wantSubtask: "partial_print",
		},
		{
			name: "running state",
			payload: `{
				"print": {
					"gcode_state": "RUNNING",
					"gcode_file": "in_progress.gcode",
					"subtask_name": "in_progress",
					"mc_percent": 55
				}
			}`,
			wantState:   StateRunning,
			wantFile:    "in_progress.gcode",
			wantSubtask: "in_progress",
		},
		{
			name:        "idle state with no file",
			payload:     `{"print": {"gcode_state": "IDLE"}}`,
			wantState:   StateIdle,
			wantFile:    "",
			wantSubtask: "",
		},
		{
			name:        "missing print key",
			payload:     `{"system": {"some": "data"}}`,
			wantState:   "",
			wantFile:    "",
			wantSubtask: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var report MQTTReport
			if err := json.Unmarshal([]byte(tt.payload), &report); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if report.Print.GCodeState != tt.wantState {
				t.Errorf("GCodeState = %q, want %q", report.Print.GCodeState, tt.wantState)
			}
			if report.Print.GCodeFile != tt.wantFile {
				t.Errorf("GCodeFile = %q, want %q", report.Print.GCodeFile, tt.wantFile)
			}
			if report.Print.SubtaskName != tt.wantSubtask {
				t.Errorf("SubtaskName = %q, want %q", report.Print.SubtaskName, tt.wantSubtask)
			}
		})
	}
}

func TestPrintEventGCodeFTPSPath(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"my_model.gcode", "/cache/my_model.gcode"},
		{"complex-name_v2.gcode", "/cache/complex-name_v2.gcode"},
	}
	for _, tt := range tests {
		e := PrintEvent{GCodeFile: tt.file}
		if got := e.GCodeFTPSPath(); got != tt.want {
			t.Errorf("GCodeFTPSPath() = %q, want %q", got, tt.want)
		}
	}
}

func TestBackoffWait(t *testing.T) {
	tests := []struct {
		attempt int
		want    string
	}{
		{0, "3s"},
		{1, "6s"},
		{2, "12s"},
		{3, "24s"},
		{4, "48s"},
		{5, "1m0s"}, // 96s > cap, clamped to 60s
		{6, "1m0s"},
		{10, "1m0s"},
	}
	for _, tt := range tests {
		got := backoffWait(tt.attempt)
		if got.String() != tt.want {
			t.Errorf("backoffWait(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestGCodeFileTracking(t *testing.T) {
	t.Run("FINISH without prior RUNNING is ignored", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		expectNoEvent(t, c)
	})

	t.Run("RUNNING with file then FINISH without file emits event", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"49248-Disc.gcode","subtask_name":"49248-Disc"}}`)
		expectNoEvent(t, c) // RUNNING should not emit
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		ev := expectEvent(t, c)
		if ev.GCodeFile != "49248-Disc.gcode" {
			t.Errorf("GCodeFile = %q, want %q", ev.GCodeFile, "49248-Disc.gcode")
		}
		if ev.SubtaskName != "49248-Disc" {
			t.Errorf("SubtaskName = %q, want %q", ev.SubtaskName, "49248-Disc")
		}
		if ev.State != StateFinish {
			t.Errorf("State = %q, want %q", ev.State, StateFinish)
		}
	})

	t.Run("duplicate FINISH after emit does not re-trigger", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"model.gcode"}}`)
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		expectEvent(t, c) // consume the first event
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		expectNoEvent(t, c) // second FINISH must be suppressed
	})

	t.Run("PREPARE activates print lifecycle", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"PREPARE","gcode_file":"prep.gcode","subtask_name":"prep"}}`)
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		ev := expectEvent(t, c)
		if ev.GCodeFile != "prep.gcode" {
			t.Errorf("GCodeFile = %q, want %q", ev.GCodeFile, "prep.gcode")
		}
	})

	t.Run("FAILED state also triggers event after RUNNING", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"fail.gcode"}}`)
		sendMessage(c, `{"print":{"gcode_state":"FAILED"}}`)
		ev := expectEvent(t, c)
		if ev.State != StateFailed {
			t.Errorf("State = %q, want %q", ev.State, StateFailed)
		}
		if ev.GCodeFile != "fail.gcode" {
			t.Errorf("GCodeFile = %q, want %q", ev.GCodeFile, "fail.gcode")
		}
	})
}

func TestHWSwitchState(t *testing.T) {
	t.Run("0→1 emits filament load", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"hw_switch_state":0}}`) // establishes baseline at 0
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // 0→1 transition
		expectFilamentLoad(t, c)
	})

	t.Run("1→1 does not emit", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // baseline at 1 (from -1 sentinel)
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // 1→1, no transition
		expectNoFilamentLoad(t, c)
	})

	t.Run("absent field does not affect state or emit", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"hw_switch_state":0}}`) // baseline 0
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"f.gcode"}}`) // no hw_switch_state
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // 0→1: should still emit
		expectFilamentLoad(t, c)
	})

	t.Run("sentinel: first hw_switch_state=1 (from -1) does not emit", func(t *testing.T) {
		c := newTestClient()
		// On observer restart, printer sends hw_switch_state:1 in first full dump.
		// lastHWSwitchState starts at -1 (sentinel), so this is -1→1, not 0→1.
		sendMessage(c, `{"print":{"hw_switch_state":1}}`)
		expectNoFilamentLoad(t, c)
	})

	t.Run("sentinel: first 0 then 1 does emit", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"hw_switch_state":0}}`) // -1→0, sets baseline
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // 0→1
		expectFilamentLoad(t, c)
	})

	t.Run("1→0→1 emits once on second 0→1", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // sentinel, no emit
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":0}}`) // 1→0, no emit
		expectNoFilamentLoad(t, c)
		sendMessage(c, `{"print":{"hw_switch_state":1}}`) // 0→1, emit
		expectFilamentLoad(t, c)
	})
}

func TestLayerTracking(t *testing.T) {
	t.Run("layer_num updates are accumulated", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"model.gcode","layer_num":3}}`)
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","layer_num":7}}`)
		sendMessage(c, `{"print":{"gcode_state":"FAILED"}}`)
		ev := expectEvent(t, c)
		if ev.LastLayerNum != 7 {
			t.Errorf("LastLayerNum = %d, want 7", ev.LastLayerNum)
		}
	})

	t.Run("FINISH event carries last layer", func(t *testing.T) {
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"model.gcode","layer_num":5}}`)
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		ev := expectEvent(t, c)
		if ev.LastLayerNum != 5 {
			t.Errorf("LastLayerNum = %d, want 5", ev.LastLayerNum)
		}
	})

	t.Run("layer_num reset after terminal event", func(t *testing.T) {
		c := newTestClient()
		// First print
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"a.gcode","layer_num":10}}`)
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		expectEvent(t, c)

		// Second print — starts with layer_num=0 (omitted), should not carry over first print's layer
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"b.gcode"}}`)
		sendMessage(c, `{"print":{"gcode_state":"FINISH"}}`)
		ev := expectEvent(t, c)
		if ev.LastLayerNum != 0 {
			t.Errorf("LastLayerNum = %d after reset, want 0", ev.LastLayerNum)
		}
	})

	t.Run("state-less layer_num messages are captured mid-print", func(t *testing.T) {
		// Real-world pattern: dedicated layer-transition messages arrive with
		// only layer_num set and no gcode_state (field absent → empty string).
		c := newTestClient()
		sendMessage(c, `{"print":{"gcode_state":"RUNNING","gcode_file":"model.gcode"}}`)
		sendMessage(c, `{"print":{"layer_num":1}}`) // no gcode_state
		sendMessage(c, `{"print":{"layer_num":2}}`)
		sendMessage(c, `{"print":{"layer_num":3}}`)
		sendMessage(c, `{"print":{"gcode_state":"FAILED"}}`) // FAILED has no layer_num
		ev := expectEvent(t, c)
		if ev.LastLayerNum != 3 {
			t.Errorf("LastLayerNum = %d, want 3", ev.LastLayerNum)
		}
	})
}
