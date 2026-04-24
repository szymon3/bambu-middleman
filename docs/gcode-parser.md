# GCode Parser

The `gcode` package is a standalone Go library for parsing OrcaSlicer gcode files. It has zero external dependencies and can be imported independently of the rest of bambu-middleman.

## Installation

```
go get github.com/szymon3/bambu-middleman/gcode
```

## Usage

```go
import "github.com/szymon3/bambu-middleman/gcode"

// Parse from an io.Reader
result, err := gcode.Parse(reader)
if err != nil {
    log.Fatal(err)
}

// Check parse completeness
if result.Status == gcode.ParseOK {
    // all sections present and valid
}

// Total filament consumed (startup + all layers)
total := result.TotalUsage()
fmt.Printf("%.2f g, %.2f mm\n", total.WeightG, total.LengthMM)

// Filament used up to layer N (1-indexed, inclusive)
partial, err := result.ComputedUsage(50)

// Per-layer callback during parsing
parser := gcode.New(gcode.WithLayerHook(func(layer int, usage gcode.FilamentUsage) {
    fmt.Printf("Layer %d: %.2f g\n", layer, usage.WeightG)
}))
result, err = parser.ParseFile("path/to/file.gcode")
```

## What's extracted

### Metadata

Slicer name and version, generation timestamp, total layer count, max Z height, filament diameter and density, estimated print times (model, first layer, total).

### Configuration

Filament type (PLA, PETG, ABS, ...), colour, vendor, nozzle temperature (normal and initial layer).

### Filament usage

- **Per-layer consumption** -- computed from E-axis motion commands (G0/G1/G2/G3)
- **Startup usage** -- purge line and nozzle priming before layer 1, tracked separately
- **Footer totals** -- slicer-reported values from the file tail (length, volume, weight, cost)

Filament is converted from extrusion length to volume and weight using the diameter and density from the file header:

```
volume_mm3 = length_mm * pi * (diameter/2)^2
weight_g   = (volume_mm3 / 1000) * density
```

## Parse status

| Status | Meaning |
|--------|---------|
| `ParseOK` | Header, config, all declared layers, footer present. Diameter > 0, density > 0. |
| `ParsePartial` | File parsed but some sections missing or incomplete (truncated file, missing density, etc.). |

## Supported formats

- **Single-filament OrcaSlicer gcode** -- files must start with `; HEADER_BLOCK_START`
- **3MF archives** -- the `printer` package can extract embedded gcode from OrcaSlicer `.3mf` files (see `printer.ExtractFromThreeMF`)

## Limitations

- **Multi-filament files are rejected** at parse time. Detection: any `filament_*` config key with `;` in the value (indicating multiple filaments) triggers an error, unless the value starts with `"` (quoted gcode snippet).
- **Tool changes**: T1--T9 (single-digit) are rejected as multi-filament indicators. T0 is allowed. Multi-digit Bambu preset codes (T255, T1000) are silently ignored.
- **OrcaSlicer only** -- other slicers produce different gcode comment formats and are not currently supported. If you use a different slicer with a Bambu printer and would like to help add support, see [Compatibility & help wanted](../README.md#compatibility--help-wanted).

## Layer hook

The `WithLayerHook` option registers a callback that fires after each layer is fully parsed:

```go
gcode.New(gcode.WithLayerHook(func(layer int, usage gcode.FilamentUsage) {
    // layer is 1-indexed
    // runs synchronously on the parser goroutine -- keep it fast
}))
```
