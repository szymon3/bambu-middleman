package gcode

import (
	"fmt"
	"time"
)

// ParseStatus indicates how completely the file was parsed.
type ParseStatus int

const (
	ParseOK      ParseStatus = iota // all sections parsed successfully
	ParsePartial                    // file parsed but some sections missing or malformed
)

// PrintFile holds all data extracted from a parsed gcode file.
type PrintFile struct {
	Status       ParseStatus
	Metadata     PrintMetadata
	Config       PrintConfig
	Footer       FooterSummary // slicer-reported totals from file tail
	StartupUsage FilamentUsage // purge line, nozzle load — pre-layer-1
	Layers       []LayerUsage  // per-layer computed usage (index 0 = layer 1)
}

// ComputedUsage returns model-only filament usage summed up to upToLayer (1-indexed, inclusive).
// upToLayer == 0 means all layers.
// Returns an error if upToLayer is negative or exceeds the number of parsed layers.
func (p *PrintFile) ComputedUsage(upToLayer int) (FilamentUsage, error) {
	if upToLayer < 0 {
		return FilamentUsage{}, fmt.Errorf("upToLayer must be >= 0, got %d", upToLayer)
	}
	if upToLayer > len(p.Layers) {
		return FilamentUsage{}, fmt.Errorf("upToLayer %d exceeds parsed layer count %d", upToLayer, len(p.Layers))
	}
	end := len(p.Layers)
	if upToLayer > 0 {
		end = upToLayer
	}
	var total FilamentUsage
	for i := 0; i < end; i++ {
		total = addUsage(total, p.Layers[i].Usage)
	}
	return total, nil
}

// TotalUsage returns the total filament consumed from the spool:
// StartupUsage plus all layer usage.
func (p *PrintFile) TotalUsage() FilamentUsage {
	layerTotal, _ := p.ComputedUsage(0)
	return addUsage(p.StartupUsage, layerTotal)
}

func addUsage(a, b FilamentUsage) FilamentUsage {
	return FilamentUsage{
		LengthMM:  a.LengthMM + b.LengthMM,
		VolumeCM3: a.VolumeCM3 + b.VolumeCM3,
		WeightG:   a.WeightG + b.WeightG,
	}
}

// PrintMetadata holds header-level information about the print.
type PrintMetadata struct {
	SlicerName         string
	SlicerVersion      string
	GeneratedAt        time.Time
	TotalLayers        int
	MaxZHeight         float64
	FilamentDiameter   float64 // mm
	FilamentDensity    float64 // g/cm³
	ModelPrintTime     time.Duration
	TotalEstimatedTime time.Duration
	FirstLayerTime     time.Duration
}

// PrintConfig holds slicer configuration values from the config block.
type PrintConfig struct {
	FilamentType           string // PLA, PETG, ABS, …
	FilamentColour         string // hex e.g. #000000
	FilamentVendor         string
	NozzleTemp             int // °C
	NozzleInitialLayerTemp int
}

// FilamentUsage holds computed or reported filament consumption values.
type FilamentUsage struct {
	LengthMM  float64
	VolumeCM3 float64
	WeightG   float64
}

// LayerUsage holds filament usage for a single layer.
type LayerUsage struct {
	Number int
	Usage  FilamentUsage
}

// FooterSummary holds slicer-reported totals from the gcode file tail.
type FooterSummary struct {
	FilamentUsage FilamentUsage
	FilamentCost  float64 // slicer-reported cost (currency unit not tracked — depends on slicer config)
}
