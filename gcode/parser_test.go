package gcode

import (
	"errors"
	"io"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

const samplePath = "../sample_print.gcode"

func mustParse(t *testing.T) *PrintFile {
	t.Helper()
	pf, err := ParseFile(samplePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return pf
}

func TestParseStatus(t *testing.T) {
	pf := mustParse(t)
	if pf.Status != ParseOK {
		t.Errorf("expected ParseOK, got %d", pf.Status)
	}

	// Truncated: first 1000 lines should lack layers and footer
	lines := readFirstNLines(t, samplePath, 1000)
	pfTrunc, err := Parse(strings.NewReader(lines))
	if err != nil {
		t.Fatalf("Parse (truncated): %v", err)
	}
	if pfTrunc.Status != ParsePartial {
		t.Errorf("expected ParsePartial for truncated file, got %d", pfTrunc.Status)
	}
}

func TestParseMetadata(t *testing.T) {
	pf := mustParse(t)
	m := pf.Metadata
	if m.TotalLayers != 318 {
		t.Errorf("TotalLayers: got %d, want 318", m.TotalLayers)
	}
	if m.FilamentDiameter != 1.75 {
		t.Errorf("FilamentDiameter: got %v, want 1.75", m.FilamentDiameter)
	}
	if m.FilamentDensity != 1.24 {
		t.Errorf("FilamentDensity: got %v, want 1.24", m.FilamentDensity)
	}
	if m.SlicerName != "OrcaSlicer" {
		t.Errorf("SlicerName: got %q, want OrcaSlicer", m.SlicerName)
	}
	if m.SlicerVersion != "2.3.2" {
		t.Errorf("SlicerVersion: got %q, want 2.3.2", m.SlicerVersion)
	}
	wantTime := time.Date(2026, 4, 20, 22, 17, 12, 0, time.UTC)
	if !m.GeneratedAt.Equal(wantTime) {
		t.Errorf("GeneratedAt: got %v, want %v", m.GeneratedAt, wantTime)
	}
}

func TestParseConfig(t *testing.T) {
	pf := mustParse(t)
	c := pf.Config
	if c.FilamentType != "PLA" {
		t.Errorf("FilamentType: got %q, want PLA", c.FilamentType)
	}
	if c.FilamentVendor != "Jayo" {
		t.Errorf("FilamentVendor: got %q, want Jayo", c.FilamentVendor)
	}
	if c.NozzleTemp != 220 {
		t.Errorf("NozzleTemp: got %d, want 220", c.NozzleTemp)
	}
	if c.NozzleInitialLayerTemp != 225 {
		t.Errorf("NozzleInitialLayerTemp: got %d, want 225", c.NozzleInitialLayerTemp)
	}
}

func TestFooterSummary(t *testing.T) {
	pf := mustParse(t)
	f := pf.Footer
	assertApprox(t, "LengthMM", f.FilamentUsage.LengthMM, 22903.93, 0.001)
	assertApprox(t, "VolumeCM3", f.FilamentUsage.VolumeCM3, 55.09, 0.001)
	assertApprox(t, "WeightG", f.FilamentUsage.WeightG, 68.31, 0.001)
	assertApprox(t, "FilamentCost", f.FilamentCost, 2.80, 0.001)
}

func TestStartupAndTotalUsage(t *testing.T) {
	pf := mustParse(t)

	// Startup should be ~136.8 mm net
	assertApprox(t, "StartupUsage.LengthMM", pf.StartupUsage.LengthMM, 136.8, 0.05)

	// TotalUsage == StartupUsage + ComputedUsage(0)
	cu, err := pf.ComputedUsage(0)
	if err != nil {
		t.Fatalf("ComputedUsage(0): %v", err)
	}
	tu := pf.TotalUsage()
	wantLen := pf.StartupUsage.LengthMM + cu.LengthMM
	wantVol := pf.StartupUsage.VolumeCM3 + cu.VolumeCM3
	wantWt := pf.StartupUsage.WeightG + cu.WeightG

	assertApprox(t, "TotalUsage.LengthMM", tu.LengthMM, wantLen, 1e-9)
	assertApprox(t, "TotalUsage.VolumeCM3", tu.VolumeCM3, wantVol, 1e-9)
	assertApprox(t, "TotalUsage.WeightG", tu.WeightG, wantWt, 1e-9)
}

func TestFilamentUsageAllLayers(t *testing.T) {
	pf := mustParse(t)
	cu, err := pf.ComputedUsage(0)
	if err != nil {
		t.Fatalf("ComputedUsage(0): %v", err)
	}
	want := 22903.93
	pct := math.Abs(cu.LengthMM-want) / want
	if pct > 0.005 {
		t.Errorf("ComputedUsage(0).LengthMM = %.2f, want within 0.5%% of %.2f (got %.4f%%)", cu.LengthMM, want, pct*100)
	}
}

func TestFilamentUsageUpToLayer159(t *testing.T) {
	pf := mustParse(t)
	cu, err := pf.ComputedUsage(159)
	if err != nil {
		t.Fatalf("ComputedUsage(159): %v", err)
	}
	want := 11096.035
	pct := math.Abs(cu.LengthMM-want) / want
	if pct > 0.001 {
		t.Errorf("ComputedUsage(159).LengthMM = %.3f, want within 0.1%% of %.3f (got %.4f%%)", cu.LengthMM, want, pct*100)
	}
}

func TestFilamentUsageOutOfRange(t *testing.T) {
	pf := mustParse(t)
	if _, err := pf.ComputedUsage(-1); err == nil {
		t.Error("expected error for upToLayer=-1")
	}
	if _, err := pf.ComputedUsage(319); err == nil {
		t.Error("expected error for upToLayer=319")
	}
	// Boundary: upToLayer == len(Layers) must succeed and equal ComputedUsage(0).
	all, err := pf.ComputedUsage(0)
	if err != nil {
		t.Fatalf("ComputedUsage(0): %v", err)
	}
	bound, err := pf.ComputedUsage(len(pf.Layers))
	if err != nil {
		t.Fatalf("ComputedUsage(%d): %v", len(pf.Layers), err)
	}
	if bound != all {
		t.Errorf("ComputedUsage(len)=%+v, want ComputedUsage(0)=%+v", bound, all)
	}
}

func TestWithLayerHook(t *testing.T) {
	var calls []int
	pf, err := New(WithLayerHook(func(layer int, _ FilamentUsage) {
		calls = append(calls, layer)
	})).ParseFile(samplePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(calls) != pf.Metadata.TotalLayers {
		t.Errorf("hook fired %d times, want %d", len(calls), pf.Metadata.TotalLayers)
	}
	for i := 1; i < len(calls); i++ {
		if calls[i] <= calls[i-1] {
			t.Errorf("layer numbers not monotonically increasing: calls[%d]=%d, calls[%d]=%d", i-1, calls[i-1], i, calls[i])
		}
	}
}

func TestParseMultiFilamentUnsupported(t *testing.T) {
	base := `; HEADER_BLOCK_START
; generated by OrcaSlicer 2.3.2 on 2026-04-20 at 22:17:12
; total layer number: 1
; filament_diameter: 1.75
; filament_density: 1.24
; HEADER_BLOCK_END
; CONFIG_BLOCK_START
%s
; CONFIG_BLOCK_END
`
	cases := []struct {
		name string
		line string
	}{
		{"type", "; filament_type = PLA;PETG"},
		{"colour", "; filament_colour = #000000;#FFFFFF"},
		{"vendor", "; filament_vendor = Jayo;Generic"},
		{"settings_id", "; filament_settings_id = A;B"},
		{"ids", "; filament_ids = GFL99;GFL00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(base, "%s", tc.line, 1)
			if _, err := Parse(strings.NewReader(src)); err == nil {
				t.Errorf("expected error for multi-value %s", tc.name)
			}
		})
	}

	// Quoted gcode snippets that legitimately contain ';' must NOT trigger.
	okSrc := strings.Replace(base, "%s",
		`; filament_end_gcode = "; filament end gcode \n\n"`+"\n"+
			`; filament_type = PLA`, 1)
	if _, err := Parse(strings.NewReader(okSrc)); err != nil {
		t.Errorf("quoted gcode snippet should not trigger multi-filament error, got: %v", err)
	}
}

func TestInlineCommentEIgnored(t *testing.T) {
	src := `; HEADER_BLOCK_START
; generated by OrcaSlicer 2.3.2 on 2026-04-20 at 22:17:12
; total layer number: 1
; filament_diameter: 1.75
; filament_density: 1.24
; HEADER_BLOCK_END
; CONFIG_BLOCK_START
; filament_type = PLA
; CONFIG_BLOCK_END
; EXECUTABLE_BLOCK_START
M83
; layer num/total_layer_count: 1/1
G1 X10 Y10 ; retract E-99 hint comment
G1 X20 Y20 ;E50 another comment
; EXECUTABLE_BLOCK_END
; filament used [mm] = 0
`
	pf, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Layers) != 1 {
		t.Fatalf("layers: got %d, want 1", len(pf.Layers))
	}
	if pf.Layers[0].Usage.LengthMM != 0 {
		t.Errorf("LengthMM: got %v, want 0 (comment E must be ignored)", pf.Layers[0].Usage.LengthMM)
	}
}

func TestUnsupportedToolChange(t *testing.T) {
	src := `; HEADER_BLOCK_START
; generated by OrcaSlicer 2.3.2 on 2026-04-20 at 22:17:12
; total layer number: 1
; filament_diameter: 1.75
; filament_density: 1.24
; HEADER_BLOCK_END
; CONFIG_BLOCK_START
; filament_type = PLA
; CONFIG_BLOCK_END
; EXECUTABLE_BLOCK_START
M83
; layer num/total_layer_count: 1/1
T1
; EXECUTABLE_BLOCK_END
`
	if _, err := Parse(strings.NewReader(src)); err == nil {
		t.Error("expected error for T1 tool change")
	}

	// T0 (default tool) must be allowed.
	okSrc := strings.Replace(src, "T1", "T0", 1)
	if _, err := Parse(strings.NewReader(okSrc)); err != nil {
		t.Errorf("T0 should be allowed, got: %v", err)
	}
}

func TestBambuPresetTCodesAllowed(t *testing.T) {
	// sample_print.gcode contains T1000 and T255 — must parse without error.
	if _, err := ParseFile(samplePath); err != nil {
		t.Fatalf("sample parse failed (T1000/T255 should be allowed): %v", err)
	}
}

// errReader returns err after n successful reads.
type errReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestParseIOError(t *testing.T) {
	partial := []byte("; HEADER_BLOCK_START\n; generated by OrcaSlicer 2.3.2 on 2026-04-20 at 22:17:12\n")
	r := &errReader{data: partial, err: io.ErrUnexpectedEOF}
	_, err := Parse(r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("error should wrap io.ErrUnexpectedEOF, got: %v", err)
	}
}

func TestConvertNoDensity(t *testing.T) {
	// Diameter present, density missing → VolumeCM3 > 0, WeightG == 0.
	src := `; HEADER_BLOCK_START
; generated by OrcaSlicer 2.3.2 on 2026-04-20 at 22:17:12
; total layer number: 1
; filament_diameter: 1.75
; HEADER_BLOCK_END
; CONFIG_BLOCK_START
; filament_type = PLA
; CONFIG_BLOCK_END
; EXECUTABLE_BLOCK_START
M83
; layer num/total_layer_count: 1/1
G1 X10 E10
; EXECUTABLE_BLOCK_END
`
	pf, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Layers) != 1 {
		t.Fatalf("layers: got %d, want 1", len(pf.Layers))
	}
	u := pf.Layers[0].Usage
	if u.LengthMM != 10 {
		t.Errorf("LengthMM: got %v, want 10", u.LengthMM)
	}
	if u.VolumeCM3 <= 0 {
		t.Errorf("VolumeCM3 should be > 0 when diameter is known, got %v", u.VolumeCM3)
	}
	if u.WeightG != 0 {
		t.Errorf("WeightG should be 0 without density, got %v", u.WeightG)
	}
}

func TestParseTruncatedMidLayer(t *testing.T) {
	lines := readFirstNLines(t, samplePath, 5000)
	pf, err := Parse(strings.NewReader(lines))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Status != ParsePartial {
		t.Errorf("Status: got %d, want ParsePartial", pf.Status)
	}
	if len(pf.Layers) == 0 {
		t.Fatal("expected some layers to be flushed")
	}
	if len(pf.Layers) >= pf.Metadata.TotalLayers {
		t.Errorf("truncated file should have fewer than %d layers, got %d",
			pf.Metadata.TotalLayers, len(pf.Layers))
	}
	if pf.TotalUsage().LengthMM <= pf.StartupUsage.LengthMM {
		t.Errorf("TotalUsage (%.2f) should exceed StartupUsage (%.2f) — final partial layer not flushed",
			pf.TotalUsage().LengthMM, pf.StartupUsage.LengthMM)
	}
}

// ---- helpers ----

func assertApprox(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %v, want %v (tolerance %v)", name, got, want, tol)
	}
}

func readFirstNLines(t *testing.T, path string, n int) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
