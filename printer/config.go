package printer

import (
	"errors"
	"os"
)

// Config holds the connection parameters for a single Bambu Lab P1S printer.
// All fields are required and loaded from environment variables.
type Config struct {
	PrinterIP  string // local IP address of the printer
	Serial     string // printer serial number (used in MQTT topic)
	AccessCode string // 8-digit access code shown on printer screen
}

// LoadFromEnv reads printer configuration from environment variables.
// Returns an error if any required variable is missing or empty.
func LoadFromEnv() (Config, error) {
	cfg := Config{
		PrinterIP:  os.Getenv("PRINTER_IP"),
		Serial:     os.Getenv("PRINTER_SERIAL"),
		AccessCode: os.Getenv("PRINTER_ACCESS_CODE"),
	}

	var errs []error
	if cfg.PrinterIP == "" {
		errs = append(errs, errors.New("PRINTER_IP is required"))
	}
	if cfg.Serial == "" {
		errs = append(errs, errors.New("PRINTER_SERIAL is required"))
	}
	if cfg.AccessCode == "" {
		errs = append(errs, errors.New("PRINTER_ACCESS_CODE is required"))
	}

	return cfg, errors.Join(errs...)
}

// MQTTBrokerAddr returns the MQTT broker address for the printer.
func (c Config) MQTTBrokerAddr() string {
	return c.PrinterIP + ":8883"
}

// FTPSAddr returns the FTPS server address for the printer.
func (c Config) FTPSAddr() string {
	return c.PrinterIP + ":990"
}

// ReportTopic returns the MQTT topic to subscribe to for print status reports.
func (c Config) ReportTopic() string {
	return "device/" + c.Serial + "/report"
}
