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
