package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/szymon3/bambu-middleman/auditlog"
)

func testAuditLogger(t *testing.T) *auditlog.Logger {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	l, err := auditlog.Open(dbPath, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("auditlog.Open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestResolveSpoolID(t *testing.T) {
	ctx := context.Background()
	log := slog.Default()

	const activeID = 10
	const notesID = 20
	const notesStr = "spoolman#20"

	tests := []struct {
		name      string
		sources   []string
		setActive bool   // whether to call SetActiveSpool(activeID)
		notes     string // filament notes string
		auditNil  bool   // whether audit logger is nil
		want      int    // 0 = not found
	}{
		// api,notes (default) — 4 availability combinations
		{"api,notes / active+notes", []string{"api", "notes"}, true, notesStr, false, activeID},
		{"api,notes / active+noNotes", []string{"api", "notes"}, true, "", false, activeID},
		{"api,notes / noActive+notes", []string{"api", "notes"}, false, notesStr, false, notesID},
		{"api,notes / neither", []string{"api", "notes"}, false, "", false, 0},

		// notes,api — 4 availability combinations
		{"notes,api / active+notes", []string{"notes", "api"}, true, notesStr, false, notesID},
		{"notes,api / active+noNotes", []string{"notes", "api"}, true, "", false, activeID},
		{"notes,api / noActive+notes", []string{"notes", "api"}, false, notesStr, false, notesID},
		{"notes,api / neither", []string{"notes", "api"}, false, "", false, 0},

		// api only
		{"api / active", []string{"api"}, true, notesStr, false, activeID},
		{"api / noActive", []string{"api"}, false, notesStr, false, 0},

		// notes only
		{"notes / notes", []string{"notes"}, true, notesStr, false, notesID},
		{"notes / noNotes", []string{"notes"}, true, "", false, 0},

		// nil audit — api source must be silently skipped
		{"nil audit + api,notes + notes", []string{"api", "notes"}, false, notesStr, true, notesID},
		{"nil audit + api only", []string{"api"}, false, notesStr, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var audit *auditlog.Logger
			if !tt.auditNil {
				audit = testAuditLogger(t)
				if tt.setActive {
					if err := audit.SetActiveSpool(ctx, activeID); err != nil {
						t.Fatalf("SetActiveSpool: %v", err)
					}
				}
			}

			got := resolveSpoolID(ctx, tt.sources, audit, tt.notes, log)
			if got != tt.want {
				t.Errorf("resolveSpoolID = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseSpoolmanSources(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{"api", "notes"}},
		{"api,notes", []string{"api", "notes"}},
		{"notes,api", []string{"notes", "api"}},
		{"api", []string{"api"}},
		{"notes", []string{"notes"}},
		{"INVALID", []string{"api", "notes"}},             // all invalid → default
		{"api,invalid,notes", []string{"api", "notes"}},   // filter invalid
		{" api , notes ", []string{"api", "notes"}},       // trim spaces
		{"API,NOTES", []string{"api", "notes"}},           // case-insensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSpoolmanSources(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d: %v vs %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
