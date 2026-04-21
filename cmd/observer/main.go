package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/szymon3/bambu-middleman/auditlog"
	"github.com/szymon3/bambu-middleman/printer"
	"github.com/szymon3/bambu-middleman/spoolman"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	cfg, err := printer.LoadFromEnv()
	if err != nil {
		log.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	log.Info("starting observer",
		"printer_ip", cfg.PrinterIP,
		"serial", cfg.Serial,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var spoolClient *spoolman.Client
	if u := os.Getenv("SPOOLMAN_URL"); u != "" {
		spoolClient = spoolman.New(u)
		log.Info("spoolman integration enabled", "url", u)
	}

	var auditLogger *auditlog.Logger
	if dbPath := os.Getenv("AUDIT_DB_PATH"); dbPath != "" {
		var err error
		auditLogger, err = auditlog.Open(dbPath, log)
		if err != nil {
			log.Error("audit log disabled", "err", err)
		} else {
			log.Info("audit log enabled", "path", dbPath)
			defer auditLogger.Close()
		}
	}

	mqttClient := printer.NewMQTTClient(cfg, log)

	// Run MQTT connection loop in background; closes Events() channel on return.
	go mqttClient.Run(ctx)

	obs := NewObserver(cfg, mqttClient, log, spoolClient, auditLogger)
	obs.Run(ctx)

	log.Info("observer stopped")
}
