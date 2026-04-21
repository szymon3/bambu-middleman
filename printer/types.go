package printer

// PrintState represents the gcode_state values reported by the Bambu P1S over MQTT.
type PrintState string

const (
	StateIdle    PrintState = "IDLE"
	StatePrepare PrintState = "PREPARE"
	StateRunning PrintState = "RUNNING"
	StatePause   PrintState = "PAUSE"
	StateFinish  PrintState = "FINISH"
	StateFailed  PrintState = "FAILED"
)

// IsTerminal returns true if the state indicates a print has ended (successfully or not).
func IsTerminal(s PrintState) bool {
	return s == StateFinish || s == StateFailed
}

// MQTTReport is the top-level JSON structure of messages published on the printer report topic.
type MQTTReport struct {
	Print PrintPayload `json:"print"`
}

// PrintPayload holds the fields relevant to print status within an MQTT report message.
type PrintPayload struct {
	GCodeState  PrintState `json:"gcode_state"`
	GCodeFile   string     `json:"gcode_file"`
	SubtaskName string     `json:"subtask_name"`
	Progress    int        `json:"mc_percent"`
	SequenceID  string     `json:"sequence_id"`
}

// PrintEvent is emitted when a print reaches a terminal state.
type PrintEvent struct {
	State       PrintState
	GCodeFile   string // bare filename, e.g. "my_model.gcode"
	SubtaskName string
}

// GCodeFTPSPath returns the FTPS path for the gcode file on the printer.
// Bambu Lab stores print files under /cache/ on the built-in FTP server.
func (e PrintEvent) GCodeFTPSPath() string {
	return "/cache/" + e.GCodeFile
}
