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

Core parsing is stable. Active spool tracking via built-in HTTP server is stable. Webhook triggers and further Bambu printer API integration are planned for future milestones.

---

## Releases

Pre-built `observer` binaries for `linux/amd64` and `linux/arm64` are attached to each [GitHub Release](https://github.com/szymon3/bambu-middleman/releases) along with a `sha256sums.txt` checksum file. `mqttdump` is a developer diagnostic tool and is not included in release assets.

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
| `WEBUI_ADDR` | no (observer) | Address to bind the built-in HTTP server, e.g. `:8080` — enables active spool tracking |
| `WEBUI_BASE_URL` | no (observer) | Externally reachable base URL of the HTTP server — baked into generated QR codes, e.g. `http://192.168.1.10:8080` |
| `SPOOLMAN_SOURCE` | no (observer) | Ordered list of spool ID sources; see [Active Spool Tracking](#active-spool-tracking). Default: `api,notes` |

Minimal setup:

```sh
export PRINTER_IP=192.168.1.x
export PRINTER_SERIAL=YOUR_PRINTER_SERIAL
export PRINTER_ACCESS_CODE=41012572
source .env && go run ./cmd/observer
```

---

## Active Spool Tracking

> **AMS not supported.** AMS multi-spool setups are not currently supported. Only single external spool (vt_tray) is handled. AMS tray changes are ignored.

Active spool tracking lets you associate a physical filament spool with a Spoolman spool ID by tapping an NFC sticker or scanning a QR code. When a print finishes, bambu-middleman uses the active spool to record filament consumption in Spoolman.

### Enabling

Set `WEBUI_ADDR` to start the built-in HTTP server alongside the observer:

```
WEBUI_ADDR=:8080
```

If unset, no HTTP server starts and active spool tracking is unavailable.

Also set `WEBUI_BASE_URL` to the externally reachable address of the server — this is baked into generated QR codes:

```
WEBUI_BASE_URL=http://192.168.1.10:8080
```

### Setting up a spool

1. Find your spool's ID in Spoolman (visible in the UI or API).
2. Print a label: open `http://<server>/spool/<id>/label` in a browser and print the page. The label contains a QR code and the filament details (manufacturer, name, material). Add `?orientation=horizontal` for a wide label instead of the default tall one.
3. Alternatively, program an NFC sticker with the URL `http://<server>/spool/<id>/activate`.

> The raw QR code PNG is still available at `/spool/<id>/qr` if you need to embed it elsewhere.

### Using it

Tap the NFC sticker or scan the QR code. A confirmation page opens in the browser — tap **Activate** to set the spool as active. This prevents accidental switches from unintentional taps.

To clear the active spool without loading new filament, open `http://<server>/spool/clear` in the browser and confirm.

### When the active spool sets and clears

```
[you load spool #42 into printer]
→ open /spool/42/activate, tap Activate
→ active spool: #42

[print starts and finishes]
→ spool #42 charged in Spoolman
→ active spool: #42  (still set — spool is still loaded)

[another print finishes]
→ spool #42 charged again
→ active spool: #42

[you unload spool #42 and load spool #7]
→ printer emits filament load event
→ active spool: cleared automatically

[open /spool/7/activate, tap Activate]
→ active spool: #7
```

### Automatic clearing

bambu-middleman listens to the printer's MQTT feed. When a filament load is detected — the moment filament passes through the runout switch — the active spool is cleared automatically. Tap the new spool's NFC or QR code after loading to set the next active spool.

This only applies to the external spool slot (vt_tray). AMS tray changes are ignored.

You can also clear the active spool manually at any time by opening `http://<server>/spool/clear` in a browser and confirming.

### Spool ID resolution

The `SPOOLMAN_SOURCE` env var controls how bambu-middleman finds the spool ID when a print finishes. The value is an ordered list — sources are tried left to right and the first match wins.

| `SPOOLMAN_SOURCE` | active spool set, notes has ID | active spool set, no notes ID | notes has ID, no active spool | neither |
|---|---|---|---|---|
| `api,notes` *(default)* | active spool | active spool | notes | skipped |
| `notes,api` | notes | active spool | notes | skipped |
| `api` | active spool | active spool | skipped | skipped |
| `notes` | notes | skipped | notes | skipped |

**Filament notes** — if you always print with the same filament profile for a given spool, you can embed the Spoolman ID directly in the profile so no tapping is required. In Orca Slicer, open the filament profile and add to the **Notes** field:

```
spoolman#42
```

The tag is case-insensitive and can appear anywhere in the notes alongside other text:

```
Bought 2025-01. SPOOLMAN#42 leftover spool.
```

### HTTP endpoints reference

| Method | Path | Response | Notes |
|--------|------|----------|-------|
| `GET` | `/spool/active` | `application/json` | `{"spool_id": 42, "activated_at": "2026-04-22T20:31:00.000Z"}` or `{"spool_id": null}` when none set |
| `GET` | `/spool/{id}/activate` | `text/html` | Confirmation page — safe to embed in NFC/QR |
| `POST` | `/spool/{id}/activate` | `text/html` | Sets spool `id` as active; submitted by the confirmation form |
| `GET` | `/spool/clear` | `text/html` | Confirmation page showing current active spool |
| `POST` | `/spool/clear` | `text/html` | Clears the active spool; submitted by the confirmation form |
| `GET` | `/spool/{id}/qr` | `image/png` | QR code encoding `WEBUI_BASE_URL/spool/{id}/activate`; cached 24 h |
| `GET` | `/spool/{id}/label` | `text/html` | Print-ready label page: QR code + filament details (manufacturer, name, material). Add `?orientation=horizontal` for side-by-side layout (default: vertical). Requires `WEBUI_BASE_URL`. |

Valid spool IDs are integers in the range 1–999999. Requests outside that range return `400 Bad Request`.

The activate and clear flows use a GET → POST two-step deliberately. NFC tags and QR codes can only trigger GET requests (that is all a phone browser does when it reads them), so the GET endpoints serve confirmation pages rather than performing the action directly. The actual state change happens on the subsequent POST submitted by the HTML form. This also prevents accidental activations from an unintentional tap.
