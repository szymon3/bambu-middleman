# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go test ./...                                # all tests (all packages)
go test ./gcode/...                          # gcode package tests
go test ./printer/...                        # printer package tests (MQTT, 3MF)
go test ./... -v                             # verbose
go test ./gcode/... -run TestParseMetadata   # single test
go test -race ./...                          # race detector
go build ./...                               # build all binaries
```

## Architecture

### Project structure
- **`gcode/`** — stateless gcode parser library (`types.go` + `parser.go`). No external deps.
- **`printer/`** — printer connectivity: MQTT state machine (`mqtt.go`), FTPS download (`ftps.go`), 3MF extraction (`3mf.go`), config from env (`config.go`), shared types (`types.go`).
- **`cmd/observer/`** — main binary: wires MQTT events → FTPS download → gcode parse → structured log output.
- **`cmd/mqttdump/`** — diagnostic tool: dumps raw MQTT messages to a JSONL file for debugging.

### Gcode parser internals
- **Two files**: `types.go` — all public types + `PrintFile` methods; `parser.go` — state machine + helper parsers (header, config, footer).
- **State flow**: `HEADER → IDLE → CONFIG → IDLE → STARTUP → LAYER(n)… → FOOTER`. Startup zone (purge/priming before layer 1) accumulates into `StartupUsage`; each `; layer num/total_layer_count: N/M` comment flushes the accumulator and starts a new `LayerUsage`.
- **E-axis accounting**: net algebraic accumulation — retraction subtracts. Only G0/G1/G2/G3 with an `E` param count; inline comments are stripped before `E` extraction; `G92 E<val>` resets position without accumulating.
- **Filament conversion**: `radius = d/2`, `cross = π×r²`, `vol_mm3 = len×cross`, `vol_cm3 = vol_mm3/1000`, `weight_g = vol_cm3×density`.

### MQTT state machine (`printer/mqtt.go`)
- Tracks print lifecycle: `PREPARE`/`RUNNING` activate tracking, `FINISH`/`FAILED` emit a `PrintEvent` and reset. Spurious terminal states (printer sends at startup/idle) are ignored if no print is active.
- Layer transitions arrive as standalone messages with `layer_num` set but no `gcode_state`.
- Reconnects with exponential backoff (3s base, 60s cap).

## Constraints

- Multi-filament detection: any `filament_*` config key with `;` in value → parse error, unless value starts with `"` (quoted gcode snippet).
- T-code policy: T1–T9 (single-digit) → error; T255/T1000 (multi-digit Bambu preset codes) → silently ignored.
- `ParseOK` requires: header present, config present, `len(Layers)==TotalLayers`, footer present, diameter > 0, density > 0. Anything missing → `ParsePartial`.
- Test fixture: `gcode/testdata/sample_print.gcode` — 318 layers, PLA, ~22 903 mm filament.
- External dependencies: `paho.mqtt.golang` (MQTT), `jlaffaye/ftp` (FTPS). The `gcode` package has zero external deps.
- Module: `github.com/szymon3/bambu-middleman`, go 1.26.
