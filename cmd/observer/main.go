package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/szymon3/bambu-middleman/auditlog"
	"github.com/szymon3/bambu-middleman/printer"
	"github.com/szymon3/bambu-middleman/spoolman"
	"github.com/szymon3/bambu-middleman/webui"
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

	webuiAddr := os.Getenv("WEBUI_ADDR")
	webuiBaseURL := strings.TrimRight(os.Getenv("WEBUI_BASE_URL"), "/")
	dbPath := os.Getenv("AUDIT_DB_PATH")
	spoolmanURL := os.Getenv("SPOOLMAN_URL")

	// Fail fast: active spool tracking requires the audit DB.
	if webuiAddr != "" && dbPath == "" {
		log.Error("WEBUI_ADDR is set but AUDIT_DB_PATH is not — active spool tracking requires the SQLite database")
		os.Exit(1)
	}

	log.Info("starting observer",
		"printer_ip", cfg.PrinterIP,
		"serial", cfg.Serial,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var spoolClient *spoolman.Client
	if spoolmanURL != "" {
		spoolClient = spoolman.New(spoolmanURL)
		log.Info("spoolman integration enabled", "url", spoolmanURL)
	}

	sources := parseSpoolmanSources(os.Getenv("SPOOLMAN_SOURCE"))

	var auditLogger *auditlog.Logger
	if dbPath != "" {
		var err error
		auditLogger, err = auditlog.Open(dbPath, log)
		if err != nil {
			log.Error("audit log disabled", "err", err)
		} else {
			log.Info("audit log enabled", "path", dbPath)
			defer auditLogger.Close() // runs LAST (registered first — LIFO)
		}
	}

	// Start the HTTP server when WEBUI_ADDR is configured.
	// The shutdown defer is registered AFTER auditLogger.Close(), so it runs
	// FIRST (LIFO) — ensuring in-flight handlers finish before the DB closes.
	if webuiAddr != "" {
		srv := &http.Server{
			Addr:         webuiAddr,
			Handler:      webui.New(auditLogger, webuiBaseURL),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		go func() {
			log.Info("webui listening", "addr", webuiAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("webui server error", "err", err)
			}
		}()
		defer func() { // runs FIRST (registered after auditLogger.Close() — LIFO)
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			if err := srv.Shutdown(shutCtx); err != nil {
				log.Error("webui shutdown error", "err", err)
			}
		}()
		log.Info("webui enabled", "addr", webuiAddr, "base_url", webuiBaseURL)
	}

	mqttClient := printer.NewMQTTClient(cfg, log)

	// Run MQTT connection loop in background; closes Events() and FilamentLoads() on return.
	go mqttClient.Run(ctx)

	obs := NewObserver(cfg, mqttClient, log, spoolClient, auditLogger, sources)
	obs.Run(ctx)

	log.Info("observer stopped")
}

// parseSpoolmanSources parses the SPOOLMAN_SOURCE env var into an ordered slice
// of valid source tokens ("api", "notes"). Invalid tokens are silently filtered.
// Returns the default ["api", "notes"] if the input is empty or all tokens are invalid.
func parseSpoolmanSources(s string) []string {
	if s == "" {
		return []string{"api", "notes"}
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "api" || p == "notes" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return []string{"api", "notes"}
	}
	return result
}
