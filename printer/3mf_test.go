package printer

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

// makeTestZip builds a minimal ZIP archive with the given filename→content map.
func makeTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestExtractFromThreeMF(t *testing.T) {
	const gcodeContent = "; HEADER_BLOCK_START\nM73 P0 R60\n"

	t.Run("uses plate from slice_info.config", func(t *testing.T) {
		sliceInfoXML := `<config><plate><metadata key="index" value="2"/></plate></config>`
		data := makeTestZip(t, map[string]string{
			"Metadata/slice_info.config": sliceInfoXML,
			"Metadata/plate_2.gcode":     gcodeContent,
		})

		rc, _, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer rc.Close()

		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read extracted gcode: %v", err)
		}
		if string(got) != gcodeContent {
			t.Errorf("got %q, want %q", got, gcodeContent)
		}
	})

	t.Run("falls back to plate_1 when slice_info missing", func(t *testing.T) {
		data := makeTestZip(t, map[string]string{
			"Metadata/plate_1.gcode": gcodeContent,
		})

		rc, _, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer rc.Close()

		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read extracted gcode: %v", err)
		}
		if string(got) != gcodeContent {
			t.Errorf("got %q, want %q", got, gcodeContent)
		}
	})

	t.Run("returns error when no gcode in archive", func(t *testing.T) {
		data := makeTestZip(t, map[string]string{
			"Metadata/slice_info.config": `<config><plate><metadata key="index" value="1"/></plate></config>`,
		})

		_, _, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns filament notes from project_settings.config", func(t *testing.T) {
		data := makeTestZip(t, map[string]string{
			"Metadata/plate_1.gcode":           gcodeContent,
			"Metadata/project_settings.config": `{"filament_notes":["spoolman#42"]}`,
		})

		_, info, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(info.FilamentNotes) == 0 || info.FilamentNotes[0] != "spoolman#42" {
			t.Errorf("got FilamentNotes %v, want [spoolman#42]", info.FilamentNotes)
		}
	})

	t.Run("returns zero ThreeMFInfo when filament_notes is empty string", func(t *testing.T) {
		data := makeTestZip(t, map[string]string{
			"Metadata/plate_1.gcode":           gcodeContent,
			"Metadata/project_settings.config": `{"filament_notes":[""]}`,
		})

		_, info, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Empty string is present but ParseSpoolmanID won't find a tag — info is populated
		// but the notes value itself is empty, so callers should check before using.
		if len(info.FilamentNotes) > 0 && info.FilamentNotes[0] != "" {
			t.Errorf("expected empty notes, got %q", info.FilamentNotes[0])
		}
	})

	t.Run("returns zero ThreeMFInfo when project_settings.config absent", func(t *testing.T) {
		data := makeTestZip(t, map[string]string{
			"Metadata/plate_1.gcode": gcodeContent,
		})

		_, info, err := ExtractFromThreeMF(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(info.FilamentNotes) != 0 {
			t.Errorf("expected no FilamentNotes, got %v", info.FilamentNotes)
		}
	})
}

func TestParseSpoolmanID(t *testing.T) {
	tests := []struct {
		notes  string
		wantID int
		wantOK bool
	}{
		{"spoolman#42", 42, true},
		{"Bought 2025-01. SPOOLMAN#7 leftover spool", 7, true},
		{"just notes, no tag", 0, false},
		{"", 0, false},
	}

	for _, tc := range tests {
		id, ok := ParseSpoolmanID(tc.notes)
		if ok != tc.wantOK || id != tc.wantID {
			t.Errorf("ParseSpoolmanID(%q) = (%d, %v), want (%d, %v)",
				tc.notes, id, ok, tc.wantID, tc.wantOK)
		}
	}
}
