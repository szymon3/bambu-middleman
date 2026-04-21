package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/szymon3/bambu-middleman/gcode"
	"github.com/szymon3/bambu-middleman/printer"
)

// Observer consumes PrintEvents from the MQTT client, downloads the GCode file
// via FTPS, parses it, and logs structured results.
type Observer struct {
	cfg  printer.Config
	mqtt *printer.MQTTClient
	log  *slog.Logger
}

// NewObserver creates an Observer wired to the given MQTT client.
func NewObserver(cfg printer.Config, mqttClient *printer.MQTTClient, log *slog.Logger) *Observer {
	return &Observer{
		cfg:  cfg,
		mqtt: mqttClient,
		log:  log,
	}
}

// Run consumes events from the MQTT client until ctx is cancelled or the events
// channel is closed. Each event triggers a download-and-parse cycle.
func (o *Observer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-o.mqtt.Events():
			if !ok {
				return
			}
			o.handleEvent(ctx, event)
		}
	}
}

func (o *Observer) handleEvent(ctx context.Context, event printer.PrintEvent) {
	log := o.log.With(
		"subtask", event.SubtaskName,
		"file", event.GCodeFile,
		"state", event.State,
	)

	log.Info("print ended, downloading gcode")

	rc, err := printer.DownloadGCode(o.cfg, event.GCodeFTPSPath())
	if err != nil {
		log.Error("ftps download failed", "err", err)
		return
	}
	defer rc.Close()

	var reader io.ReadCloser = rc
	var meta printer.ThreeMFInfo
	if strings.HasSuffix(strings.ToLower(event.GCodeFile), ".3mf") {
		extracted, info, err := printer.ExtractFromThreeMF(rc)
		if err != nil {
			log.Error("3mf extraction failed", "err", err)
			return
		}
		defer extracted.Close()
		reader = extracted
		meta = info
	}

	result, err := gcode.Parse(reader)
	if err != nil {
		log.Error("gcode parse failed", "err", err)
		return
	}

	o.logResult(log, result, meta, event)
}

func (o *Observer) logResult(log *slog.Logger, result *gcode.PrintFile, meta printer.ThreeMFInfo, event printer.PrintEvent) {
	// For completed prints use all layers. For failed/cancelled prints only
	// count layers that were actually extruded, using the last layer_num from MQTT.
	var layerUsage gcode.FilamentUsage
	var layersPrinted int
	if event.State == printer.StateFailed && event.LastLayerNum > 0 {
		// layer_num is 1-indexed: layer_num=4 means layer 4 was the last printed.
		upTo := event.LastLayerNum
		if upTo > len(result.Layers) {
			upTo = len(result.Layers)
		}
		var err error
		layerUsage, err = result.ComputedUsage(upTo)
		if err != nil {
			log.Warn("layer usage computation failed, falling back to full", "err", err)
			layerUsage, _ = result.ComputedUsage(0)
		}
		layersPrinted = upTo
	} else {
		if event.State == printer.StateFailed {
			log.Warn("failed print with unknown last layer, reporting full gcode usage")
		}
		layerUsage, _ = result.ComputedUsage(0)
		layersPrinted = len(result.Layers)
	}

	total := addUsage(result.StartupUsage, layerUsage)

	statusStr := parseStatusString(result.Status)

	// Parse Spoolman spool ID from filament notes if present.
	var spoolmanID int
	if len(meta.FilamentNotes) > 0 && meta.FilamentNotes[0] != "" {
		if id, ok := printer.ParseSpoolmanID(meta.FilamentNotes[0]); ok {
			log.Debug("spoolman tag found in filament notes", "spoolman_id", id)
			spoolmanID = id
		} else {
			log.Debug("no spoolman tag in filament notes")
		}
	}

	args := []any{
		"parse_status", statusStr,
		"layers", len(result.Layers),
		"layers_printed", layersPrinted,
		"total_weight_g", round2(total.WeightG),
		"total_length_mm", round2(total.LengthMM),
		"total_volume_cm3", round2(total.VolumeCM3),
		"layer_weight_g", round2(layerUsage.WeightG),
		"startup_weight_g", round2(result.StartupUsage.WeightG),
		"footer_weight_g", round2(result.Footer.FilamentUsage.WeightG),
		"filament_type", result.Config.FilamentType,
		"filament_vendor", result.Config.FilamentVendor,
		"slicer", fmt.Sprintf("%s %s", result.Metadata.SlicerName, result.Metadata.SlicerVersion),
	}
	if spoolmanID != 0 {
		args = append(args, "spoolman_id", spoolmanID)
	}
	log.Info("print parsed", args...)
}

func parseStatusString(s gcode.ParseStatus) string {
	switch s {
	case gcode.ParseOK:
		return "OK"
	case gcode.ParsePartial:
		return "PARTIAL"
	default:
		return "FAILED"
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func addUsage(a, b gcode.FilamentUsage) gcode.FilamentUsage {
	return gcode.FilamentUsage{
		LengthMM:  a.LengthMM + b.LengthMM,
		VolumeCM3: a.VolumeCM3 + b.VolumeCM3,
		WeightG:   a.WeightG + b.WeightG,
	}
}
