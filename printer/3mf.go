package printer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// ThreeMFInfo holds slicer metadata extracted alongside the GCode from a .3mf archive.
type ThreeMFInfo struct {
	// FilamentNotes contains the per-slot notes from the OrcaSlicer filament profile.
	// Index 0 is the first (or only) filament. Empty if not set or not present.
	FilamentNotes []string
}

var spoolmanTagRe = regexp.MustCompile(`(?i)spoolman#(\d+)`)

// ParseSpoolmanID scans notes for a spoolman#<id> tag (case-insensitive) and
// returns the parsed integer ID. Returns 0, false if no tag is found.
func ParseSpoolmanID(notes string) (int, bool) {
	m := spoolmanTagRe.FindStringSubmatch(notes)
	if m == nil {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return id, true
}

// ExtractFromThreeMF reads a .3mf ZIP archive from r and returns a ReadCloser
// over the embedded GCode file plus any slicer metadata parsed from the archive.
//
// The plate number is resolved from Metadata/slice_info.config; if that file is
// absent or unreadable, plate 1 is used as a fallback.
//
// Metadata (filament notes) is read from Metadata/project_settings.config. If
// that file is absent or malformed, ThreeMFInfo is returned as a zero value —
// this is never treated as an error.
//
// The entire archive is buffered into memory because archive/zip requires
// random access (io.ReaderAt). For typical Bambu 3MF files this is acceptable.
func ExtractFromThreeMF(r io.Reader) (io.ReadCloser, ThreeMFInfo, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, ThreeMFInfo{}, fmt.Errorf("read 3mf: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ThreeMFInfo{}, fmt.Errorf("open 3mf as zip: %w", err)
	}

	plateNum := "1"
	if entry := zipEntry(zr, "Metadata/slice_info.config"); entry != nil {
		if n, err := plateNumberFromSliceInfo(entry); err == nil {
			plateNum = n
		}
	}

	path := fmt.Sprintf("Metadata/plate_%s.gcode", plateNum)
	entry := zipEntry(zr, path)
	if entry == nil && plateNum != "1" {
		path = "Metadata/plate_1.gcode"
		entry = zipEntry(zr, path)
	}
	if entry == nil {
		return nil, ThreeMFInfo{}, fmt.Errorf("no gcode file found in 3mf archive (tried %s)", path)
	}

	rc, err := entry.Open()
	if err != nil {
		return nil, ThreeMFInfo{}, fmt.Errorf("open zip entry %s: %w", entry.Name, err)
	}

	info := parseThreeMFInfo(zr)
	return rc, info, nil
}

// parseThreeMFInfo reads Metadata/project_settings.config and extracts filament
// notes. Returns a zero ThreeMFInfo on any failure — callers treat absence as
// non-fatal.
func parseThreeMFInfo(zr *zip.Reader) ThreeMFInfo {
	entry := zipEntry(zr, "Metadata/project_settings.config")
	if entry == nil {
		return ThreeMFInfo{}
	}

	rc, err := entry.Open()
	if err != nil {
		return ThreeMFInfo{}
	}
	defer rc.Close()

	var settings struct {
		FilamentNotes []string `json:"filament_notes"`
	}
	if err := json.NewDecoder(rc).Decode(&settings); err != nil {
		return ThreeMFInfo{}
	}

	return ThreeMFInfo{FilamentNotes: settings.FilamentNotes}
}

// zipEntry returns the first zip.File whose name matches name, or nil.
func zipEntry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

type sliceInfo struct {
	Plate struct {
		Metadata []struct {
			Key   string `xml:"key,attr"`
			Value string `xml:"value,attr"`
		} `xml:"metadata"`
	} `xml:"plate"`
}

// plateNumberFromSliceInfo opens the given zip entry, parses it as a
// slice_info.config XML, and returns the plate index value.
func plateNumberFromSliceInfo(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var cfg sliceInfo
	if err := xml.NewDecoder(rc).Decode(&cfg); err != nil {
		return "", err
	}

	for _, m := range cfg.Plate.Metadata {
		if m.Key == "index" && m.Value != "" {
			return m.Value, nil
		}
	}
	return "", fmt.Errorf("plate index not found in slice_info.config")
}
