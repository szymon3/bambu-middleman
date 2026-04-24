# bambu-middleman

Automatic filament tracking for Bambu Lab printers.

[![CI](https://github.com/szymon3/bambu-middleman/actions/workflows/ci.yml/badge.svg)](https://github.com/szymon3/bambu-middleman/actions/workflows/ci.yml)
[![License: GPL-3.0](https://img.shields.io/github/license/szymon3/bambu-middleman)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/szymon3/bambu-middleman)](https://github.com/szymon3/bambu-middleman/releases/latest)

bambu-middleman runs as a background daemon on your local network, listens to your printer over MQTT, and tracks exactly how much filament each print consumes -- then syncs it to [Spoolman](https://github.com/Donkie/Spoolman) automatically.

**Why use it:**

- **Fully automatic** -- runs in the background, detects print start and finish via MQTT. No manual logging, no forgetting to update your spool inventory.
- **Accurate for failed and cancelled prints** -- filament usage is computed to the exact last layer printed, not the full file. If your print fails at layer 50 of 300, only what was actually extruded gets recorded.
- **Spoolman sync** -- filament consumption is sent to Spoolman on every print completion, keeping your spool inventory accurate without any extra steps.
- **NFC and QR spool activation** -- tap an NFC sticker or scan a QR code to tell bambu-middleman which spool is loaded. Print labels with QR codes directly from the built-in web UI.
- **Audit log** -- every print is recorded in a local SQLite database with full metadata: filament type, weight, layers, Spoolman result, timestamps.

## How it works

The observer daemon connects to your printer's local MQTT broker and waits for print events. When a print finishes (or fails), it downloads the gcode file from the printer via FTPS, parses it to compute per-layer filament consumption, resolves which Spoolman spool to charge, and sends the usage update. The entire flow is automatic -- once configured, it runs unattended.

## Quick start

Install on a Linux machine on the same network as your printer (Raspberry Pi, home server, etc.):

```bash
curl -fsSL https://raw.githubusercontent.com/szymon3/bambu-middleman/master/install.sh | sudo bash
```

Edit the configuration:

```bash
sudo nano /etc/bambu-observer/env
```

Set the three required variables (see [Configuration](#configuration) below), then start the service:

```bash
sudo systemctl restart bambu-observer
sudo journalctl -u bambu-observer -f   # watch the logs
```

See [Installation](docs/installation.md) for build-from-source instructions and upgrade details.

## Configuration

All settings are environment variables. When using the install script, they live in `/etc/bambu-observer/env`.

| Variable | Required | Description |
|---|---|---|
| `PRINTER_IP` | yes | Local IP address of the printer |
| `PRINTER_SERIAL` | yes | Printer serial number (used in MQTT topic) |
| `PRINTER_ACCESS_CODE` | yes | 8-digit access code shown on the printer screen |
| `LOG_LEVEL` | no | `DEBUG` for verbose output; default `INFO` |
| `SPOOLMAN_URL` | no | Base URL of a Spoolman instance -- enables automatic spool-usage updates |
| `AUDIT_DB_PATH` | no | Filesystem path for the audit log SQLite database |
| `WEBUI_ADDR` | no | Address to bind the built-in HTTP server, e.g. `:8080` -- enables active spool tracking |
| `WEBUI_BASE_URL` | no | Externally reachable base URL of the HTTP server -- baked into generated QR codes, e.g. `http://192.168.1.10:8080` |
| `SPOOLMAN_SOURCE` | no | Ordered list of spool ID sources; see [Spoolman Integration](docs/spoolman-integration.md). Default: `api,notes` |

A template configuration file is included at [`packaging/env.example`](packaging/env.example).

## Documentation

| Topic | Description |
|-------|-------------|
| [Installation](docs/installation.md) | Install script, upgrades, systemd service, building from source |
| [Active Spool Tracking](docs/active-spool-tracking.md) | NFC/QR spool activation, label printing, automatic clearing |
| [Spoolman Integration](docs/spoolman-integration.md) | Automatic filament sync, spool ID resolution, filament notes tagging |
| [Audit Log](docs/audit-log.md) | SQLite print history, what's recorded, schema |
| [HTTP API Reference](docs/http-api.md) | Endpoint table, request/response examples |
| [GCode Parser](docs/gcode-parser.md) | Go library for parsing OrcaSlicer gcode (standalone, zero external deps) |
| [mqttdump](docs/mqttdump.md) | Diagnostic tool for capturing raw MQTT messages |

## Releases

Pre-built `bambu-observer` binaries for `linux/amd64` and `linux/arm64` are attached to each [GitHub Release](https://github.com/szymon3/bambu-middleman/releases) along with a `sha256sums.txt` checksum file. Release binaries include build provenance attestations.

## Compatibility & help wanted

bambu-middleman is currently tested on a **Bambu Lab P1S** with **OrcaSlicer**. The author only has access to this specific hardware, so expanding support depends on community testing and contributions.

If you have different hardware and are willing to help, here's what would be most valuable:

- **Other Bambu printers** (A1, A1 Mini, X1C, X1, P1P) -- the MQTT message format may differ. Running [mqttdump](docs/mqttdump.md) during a print and sharing the output would help immensely.
- **AMS (Automatic Material System)** -- multi-spool AMS support is not yet implemented. Testing with AMS tray changes and sharing mqttdump captures would be a great starting point.
- **Multi-filament prints** -- currently rejected at parse time. Contributions toward multi-filament gcode parsing are welcome.
- **Other slicers** (BambuStudio, PrusaSlicer, Cura) -- the gcode parser expects OrcaSlicer comment format. If you use a different slicer, a sample gcode file would help add support.

Open an [issue](https://github.com/szymon3/bambu-middleman/issues) or submit a PR -- all contributions are welcome.

## License

[GNU General Public License v3.0](LICENSE)
