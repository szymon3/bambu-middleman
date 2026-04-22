# bambu-middleman

A Go library for parsing OrcaSlicer gcode files targeting Bambu Lab P1S printers.

## What it does

The library reads a gcode file and extracts three categories of information:

- **Metadata** — slicer name and version, total layer count, filament diameter and density, estimated print times
- **Config** — filament type, vendor, colour, nozzle temperatures
- **Filament usage** — per-layer consumption computed from E-axis motion commands, plus the slicer-reported footer totals

Startup filament (purge line, nozzle priming before layer 1) is tracked separately from model filament so callers can report either or both.

## Supported formats

Single-filament OrcaSlicer gcode for Bambu Lab P1S. Multi-filament files are rejected at parse time with an error.

## Installation

```
go get github.com/szymon3/bambu-middleman
```

## Quick start

Parse a file and read the results from the returned `*PrintFile`. Check `pf.Status` to determine whether parsing was complete (`ParseOK`) or partial (`ParsePartial`). Call `pf.TotalUsage()` for total spool consumption or `pf.ComputedUsage(n)` to sum usage up to a specific layer. Register a layer hook via `gcode.New(gcode.WithLayerHook(...))` to receive per-layer callbacks as the file is scanned.

## Project status

Core parsing is stable. HTTP middleware, webhook triggers, and Bambu printer API integration are planned for future milestones.

---

## Running the tools

Two binaries live under `cmd/`. Both are configured entirely via environment variables and shut down gracefully on SIGINT/SIGTERM.

### observer

Connects to the printer via MQTT, waits for print completions, downloads and parses the gcode via FTPS, and emits structured JSON logs. Optionally updates Spoolman spool usage and writes an audit database.

```bash
go run ./cmd/observer
# or build first:
go build -o observer ./cmd/observer && ./observer
```

### mqttdump

Diagnostic tool that subscribes to the printer's MQTT report topic and writes every message (with a UTC timestamp) to a JSONL file for offline inspection.

```bash
go run ./cmd/mqttdump
# or build first:
go build -o mqttdump ./cmd/mqttdump && ./mqttdump
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `PRINTER_IP` | yes | Local IP address of the printer |
| `PRINTER_SERIAL` | yes | Printer serial number (used in MQTT topic) |
| `PRINTER_ACCESS_CODE` | yes | 8-digit access code shown on the printer screen |
| `LOG_LEVEL` | no | `DEBUG` for verbose output; default `INFO` |
| `SPOOLMAN_URL` | no (observer) | Base URL of a Spoolman instance — enables automatic spool-usage updates |
| `AUDIT_DB_PATH` | no (observer) | Filesystem path for the audit log SQLite database |
| `DUMP_FILE` | no (mqttdump) | Output path for the JSONL dump; defaults to `mqtt_dump_YYYYMMDD_HHMMSS.jsonl` |

Minimal setup:

```sh
export PRINTER_IP=192.168.1.x
export PRINTER_SERIAL=YOUR_PRINTER_SERIAL
export PRINTER_ACCESS_CODE=41012572
source .env && go run ./cmd/observer
```
