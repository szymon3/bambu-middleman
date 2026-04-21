package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/szymon3/bambu-middleman/printer"
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

	mqttClient := printer.NewMQTTClient(cfg, log)

	// Run MQTT connection loop in background; closes Events() channel on return.
	go mqttClient.Run(ctx)

	obs := NewObserver(cfg, mqttClient, log)
	obs.Run(ctx)

	log.Info("observer stopped")
}
