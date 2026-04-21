# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go test ./gcode/...                          # all tests
go test ./gcode/... -v                       # verbose
go test ./gcode/... -run TestParseMetadata   # single test
go test -race ./gcode/...                    # race detector
```

## Architecture

- **Two files**: `types.go` — all public types + `PrintFile` methods; `parser.go` — state machine + helper parsers (header, config, footer). No other source files.
- **State flow**: `HEADER → IDLE → CONFIG → IDLE → STARTUP → LAYER(n)… → FOOTER`. Startup zone (purge/priming before layer 1) accumulates into `StartupUsage`; each `; layer num/total_layer_count: N/M` comment flushes the accumulator and starts a new `LayerUsage`.
- **E-axis accounting**: net algebraic accumulation — retraction subtracts. Only G0/G1/G2/G3 with an `E` param count; inline comments are stripped before `E` extraction; `G92 E<val>` resets position without accumulating.
- **Filament conversion**: `radius = d/2`, `cross = π×r²`, `vol_mm3 = len×cross`, `vol_cm3 = vol_mm3/1000`, `weight_g = vol_cm3×density`.

## Constraints

- Multi-filament detection: any `filament_*` config key with `;` in value → parse error, unless value starts with `"` (quoted gcode snippet).
- T-code policy: T1–T9 (single-digit) → error; T255/T1000 (multi-digit Bambu preset codes) → silently ignored.
- `ParseOK` requires: header present, config present, `len(Layers)==TotalLayers`, footer present, diameter > 0, density > 0. Anything missing → `ParsePartial`.
- Test fixture: `sample_print.gcode` at repo root, referenced as `../sample_print.gcode` from the `gcode/` package. 318 layers, PLA, ~22 903 mm filament.
- No external dependencies. Module: `github.com/szymon3/bambu-middleman`, go 1.26.
