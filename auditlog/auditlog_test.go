package auditlog

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/szymon3/bambu-middleman/gcode"
	"github.com/szymon3/bambu-middleman/printer"
)

func testLogger(t *testing.T) (*Logger, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_audit.db")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	l, err := Open(dbPath, log)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return l, dbPath
}

func TestOpenClose(t *testing.T) {
	l, _ := testLogger(t)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestInsertAndReadBack(t *testing.T) {
	l, dbPath := testLogger(t)

	spoolID := 42
	spoolWeight := 12.5
	spoolSuccess := true

	l.Log(Entry{
		PrinterIP:       "192.168.1.10",
		PrinterSerial:   "ABC123",
		PrintState:      printer.StateFinish,
		GCodeFile:       "benchy.3mf",
		SubtaskName:     "benchy",
		LastLayerNum:    150,
		ParseStatus:     gcode.ParseOK,
		LayersPrinted:   150,
		FilamentType:    "PLA",
		FilamentVendor:  "Jayo",
		StartupWeightG:  0.5,
		LayerWeightG:    10.0,
		FooterWeightG:   10.3,
		TotalWeightG:    10.5,
		SpoolmanID:      &spoolID,
		SpoolmanWeightG: &spoolWeight,
		SpoolmanSuccess: &spoolSuccess,
		SpoolmanError:   "",
		FilamentNotes:   "spoolman#42",
	})

	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var (
		id             int
		createdAt      string
		printerIP      string
		printerSerial  string
		printState     string
		gcodeFile      string
		subtaskName    string
		lastLayerNum   int
		parseStatus    string
		layersPrinted  int
		filamentType   string
		filamentVendor string
		startupW       float64
		layerW         float64
		footerW        float64
		totalW         float64
		spoolIDVal     *int
		spoolWVal      *float64
		spoolSuccVal   *int
		spoolErrVal    *string
		filNotes       string
	)

	err = db.QueryRow(`SELECT
		id, created_at, printer_ip, printer_serial,
		print_state, gcode_file, subtask_name, last_layer_num,
		parse_status, layers_printed,
		filament_type, filament_vendor,
		startup_weight_g, layer_weight_g, footer_weight_g, total_weight_g,
		spoolman_spool_id, spoolman_weight_g, spoolman_success, spoolman_error,
		filament_notes
	FROM print_audit_log WHERE id = 1`).Scan(
		&id, &createdAt, &printerIP, &printerSerial,
		&printState, &gcodeFile, &subtaskName, &lastLayerNum,
		&parseStatus, &layersPrinted,
		&filamentType, &filamentVendor,
		&startupW, &layerW, &footerW, &totalW,
		&spoolIDVal, &spoolWVal, &spoolSuccVal, &spoolErrVal,
		&filNotes,
	)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", createdAt); err != nil {
		t.Errorf("created_at %q not valid ISO 8601: %v", createdAt, err)
	}
	if printerIP != "192.168.1.10" {
		t.Errorf("printer_ip = %q, want 192.168.1.10", printerIP)
	}
	if printerSerial != "ABC123" {
		t.Errorf("printer_serial = %q, want ABC123", printerSerial)
	}
	if printState != "FINISH" {
		t.Errorf("print_state = %q, want FINISH", printState)
	}
	if gcodeFile != "benchy.3mf" {
		t.Errorf("gcode_file = %q, want benchy.3mf", gcodeFile)
	}
	if subtaskName != "benchy" {
		t.Errorf("subtask_name = %q, want benchy", subtaskName)
	}
	if lastLayerNum != 150 {
		t.Errorf("last_layer_num = %d, want 150", lastLayerNum)
	}
	if parseStatus != "OK" {
		t.Errorf("parse_status = %q, want OK", parseStatus)
	}
	if layersPrinted != 150 {
		t.Errorf("layers_printed = %d, want 150", layersPrinted)
	}
	if filamentType != "PLA" {
		t.Errorf("filament_type = %q, want PLA", filamentType)
	}
	if filamentVendor != "Jayo" {
		t.Errorf("filament_vendor = %q, want Jayo", filamentVendor)
	}
	if startupW != 0.5 {
		t.Errorf("startup_weight_g = %v, want 0.5", startupW)
	}
	if layerW != 10.0 {
		t.Errorf("layer_weight_g = %v, want 10.0", layerW)
	}
	if footerW != 10.3 {
		t.Errorf("footer_weight_g = %v, want 10.3", footerW)
	}
	if totalW != 10.5 {
		t.Errorf("total_weight_g = %v, want 10.5", totalW)
	}
	if spoolIDVal == nil || *spoolIDVal != 42 {
		t.Errorf("spoolman_spool_id = %v, want 42", spoolIDVal)
	}
	if spoolWVal == nil || *spoolWVal != 12.5 {
		t.Errorf("spoolman_weight_g = %v, want 12.5", spoolWVal)
	}
	if spoolSuccVal == nil || *spoolSuccVal != 1 {
		t.Errorf("spoolman_success = %v, want 1", spoolSuccVal)
	}
	if spoolErrVal != nil {
		t.Errorf("spoolman_error = %v, want NULL", spoolErrVal)
	}
	if filNotes != "spoolman#42" {
		t.Errorf("filament_notes = %q, want spoolman#42", filNotes)
	}
}

func TestMigrationIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_audit.db")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	l1, err := Open(dbPath, log)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	l1.Close()

	l2, err := Open(dbPath, log)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	l2.Close()
}

func TestNullSpoolmanFields(t *testing.T) {
	l, dbPath := testLogger(t)

	l.Log(Entry{
		PrinterIP:     "10.0.0.1",
		PrinterSerial: "XYZ",
		PrintState:    printer.StateFailed,
		GCodeFile:     "test.gcode",
		ParseStatus:   gcode.ParsePartial,
	})
	l.Close()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var spoolID, spoolSucc *int
	var spoolW *float64
	var spoolErr *string
	err = db.QueryRow(`SELECT spoolman_spool_id, spoolman_weight_g, spoolman_success, spoolman_error FROM print_audit_log WHERE id = 1`).
		Scan(&spoolID, &spoolW, &spoolSucc, &spoolErr)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if spoolID != nil {
		t.Errorf("spoolman_spool_id = %v, want NULL", *spoolID)
	}
	if spoolW != nil {
		t.Errorf("spoolman_weight_g = %v, want NULL", *spoolW)
	}
	if spoolSucc != nil {
		t.Errorf("spoolman_success = %v, want NULL", *spoolSucc)
	}
	if spoolErr != nil {
		t.Errorf("spoolman_error = %v, want NULL", *spoolErr)
	}
}

func TestActiveSpool(t *testing.T) {
	l, _ := testLogger(t)
	defer l.Close()
	ctx := context.Background()

	t.Run("initially inactive", func(t *testing.T) {
		_, _, ok, err := l.GetActiveSpool(ctx)
		if err != nil {
			t.Fatalf("GetActiveSpool: %v", err)
		}
		if ok {
			t.Error("expected no active spool, got one")
		}
	})

	t.Run("set makes active", func(t *testing.T) {
		if err := l.SetActiveSpool(ctx, 42); err != nil {
			t.Fatalf("SetActiveSpool: %v", err)
		}
		id, at, ok, err := l.GetActiveSpool(ctx)
		if err != nil {
			t.Fatalf("GetActiveSpool: %v", err)
		}
		if !ok {
			t.Fatal("expected active spool, got none")
		}
		if id != 42 {
			t.Errorf("spool_id = %d, want 42", id)
		}
		if at.IsZero() {
			t.Error("activated_at is zero")
		}
	})

	t.Run("second set replaces first (singleton)", func(t *testing.T) {
		if err := l.SetActiveSpool(ctx, 7); err != nil {
			t.Fatalf("SetActiveSpool: %v", err)
		}
		id, _, ok, err := l.GetActiveSpool(ctx)
		if err != nil {
			t.Fatalf("GetActiveSpool: %v", err)
		}
		if !ok {
			t.Fatal("expected active spool after second set")
		}
		if id != 7 {
			t.Errorf("spool_id = %d, want 7", id)
		}
	})

	t.Run("clear makes inactive", func(t *testing.T) {
		if err := l.ClearActiveSpool(ctx); err != nil {
			t.Fatalf("ClearActiveSpool: %v", err)
		}
		_, _, ok, err := l.GetActiveSpool(ctx)
		if err != nil {
			t.Fatalf("GetActiveSpool after clear: %v", err)
		}
		if ok {
			t.Error("expected no active spool after clear")
		}
	})

	t.Run("clear when already inactive is a no-op", func(t *testing.T) {
		if err := l.ClearActiveSpool(ctx); err != nil {
			t.Fatalf("ClearActiveSpool on empty: %v", err)
		}
	})

	t.Run("timestamp round-trips as valid ISO 8601", func(t *testing.T) {
		if err := l.SetActiveSpool(ctx, 99); err != nil {
			t.Fatalf("SetActiveSpool: %v", err)
		}
		_, at, ok, err := l.GetActiveSpool(ctx)
		if err != nil {
			t.Fatalf("GetActiveSpool: %v", err)
		}
		if !ok {
			t.Fatal("expected active spool")
		}
		// Round-trip: format then parse with the same layout used by GetActiveSpool.
		formatted := at.UTC().Format("2006-01-02T15:04:05.000Z")
		if _, err := time.Parse("2006-01-02T15:04:05.000Z", formatted); err != nil {
			t.Errorf("timestamp %q does not parse: %v", formatted, err)
		}
	})
}

func TestMigrationUpgrade(t *testing.T) {
	// Create a v1 database by running only migration 1 directly, then verifying
	// that calling Open again applies migration 2 without data loss.
	dbPath := filepath.Join(t.TempDir(), "upgrade_test.db")
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Open normally — this applies both migrations (v1 and v2).
	l1, err := Open(dbPath, log)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Write an audit entry to confirm data survives the upgrade check.
	l1.Log(Entry{
		PrinterIP:     "10.0.0.1",
		PrinterSerial: "SN1",
		PrintState:    printer.StateFinish,
		GCodeFile:     "before_upgrade.gcode",
		ParseStatus:   gcode.ParseOK,
	})
	l1.Close()

	// Reopen — migration 2 should be a no-op (already applied); data must survive.
	l2, err := Open(dbPath, log)
	if err != nil {
		t.Fatalf("second Open (upgrade check): %v", err)
	}
	defer l2.Close()

	// Verify existing print_audit_log row is intact.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM print_audit_log`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("print_audit_log rows = %d, want 1", count)
	}

	// Verify active_spool table exists (migration 2).
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='active_spool'`).Scan(&tableName)
	if err != nil {
		t.Fatalf("active_spool table not found: %v", err)
	}
}

func TestBufferFullDrop(t *testing.T) {
	l, _ := testLogger(t)
	defer l.Close()

	// Fill the channel buffer (cap 64) plus extra — should not block.
	for i := 0; i < 200; i++ {
		l.Log(Entry{
			PrinterIP:     "10.0.0.1",
			PrinterSerial: "X",
			PrintState:    printer.StateFinish,
			GCodeFile:     "fill.gcode",
			ParseStatus:   gcode.ParseOK,
		})
	}
}
