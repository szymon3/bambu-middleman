package auditlog

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/szymon3/bambu-middleman/gcode"
	"github.com/szymon3/bambu-middleman/printer"

	_ "modernc.org/sqlite"
)

// Entry holds all data for a single audit log row.
type Entry struct {
	PrinterIP     string
	PrinterSerial string

	PrintState   printer.PrintState
	GCodeFile    string
	SubtaskName  string
	LastLayerNum int

	ParseStatus    gcode.ParseStatus
	LayersPrinted  int
	FilamentType   string
	FilamentVendor string

	StartupWeightG float64
	LayerWeightG   float64
	FooterWeightG  float64
	TotalWeightG   float64

	SpoolmanID      *int
	SpoolmanWeightG *float64
	SpoolmanSuccess *bool
	SpoolmanError   string

	FilamentNotes string
}

// Logger writes audit entries to a SQLite database asynchronously.
type Logger struct {
	db   *sql.DB
	log  *slog.Logger
	stmt *sql.Stmt
	ch   chan Entry
	done chan struct{}
}

// Open creates a Logger backed by the SQLite database at dbPath.
// It creates the file and tables if they do not exist.
func Open(dbPath string, log *slog.Logger) (*Logger, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("auditlog: open db: %w", err)
	}

	ctx := context.Background()

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			db.Close()
			return nil, fmt.Errorf("auditlog: %s: %w", p, err)
		}
	}

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("auditlog: migrate: %w", err)
	}

	db.SetMaxOpenConns(1)

	stmt, err := db.PrepareContext(ctx, insertSQL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("auditlog: prepare insert: %w", err)
	}

	l := &Logger{
		db:   db,
		log:  log,
		stmt: stmt,
		ch:   make(chan Entry, 64),
		done: make(chan struct{}),
	}
	go l.writeLoop()
	return l, nil
}

// Log enqueues an entry for asynchronous writing.
// Never blocks; drops the entry if the buffer is full.
func (l *Logger) Log(entry Entry) {
	select {
	case l.ch <- entry:
	default:
		l.log.Warn("audit log buffer full, dropping entry",
			"file", entry.GCodeFile, "state", entry.PrintState)
	}
}

// Close drains the write queue and closes the database.
func (l *Logger) Close() error {
	close(l.ch)
	<-l.done
	l.stmt.Close()
	return l.db.Close()
}

func (l *Logger) writeLoop() {
	defer close(l.done)
	for entry := range l.ch {
		if err := l.insertEntry(entry); err != nil {
			l.log.Error("audit log write failed", "err", err,
				"file", entry.GCodeFile, "state", entry.PrintState)
		}
	}
}

func (l *Logger) insertEntry(e Entry) error {
	var spoolSuccess *int
	if e.SpoolmanSuccess != nil {
		v := 0
		if *e.SpoolmanSuccess {
			v = 1
		}
		spoolSuccess = &v
	}

	var spoolError *string
	if e.SpoolmanError != "" {
		spoolError = &e.SpoolmanError
	}

	_, err := l.stmt.Exec(
		e.PrinterIP,
		e.PrinterSerial,
		string(e.PrintState),
		e.GCodeFile,
		e.SubtaskName,
		e.LastLayerNum,
		statusString(e.ParseStatus),
		e.LayersPrinted,
		e.FilamentType,
		e.FilamentVendor,
		e.StartupWeightG,
		e.LayerWeightG,
		e.FooterWeightG,
		e.TotalWeightG,
		e.SpoolmanID,
		e.SpoolmanWeightG,
		spoolSuccess,
		spoolError,
		e.FilamentNotes,
	)
	return err
}

func statusString(s gcode.ParseStatus) string {
	switch s {
	case gcode.ParseOK:
		return "OK"
	case gcode.ParsePartial:
		return "PARTIAL"
	default:
		return "FAILED"
	}
}

// SetActiveSpool upserts the singleton active-spool row. Uses l.db directly
// (synchronous) rather than the async write channel.
func (l *Logger) SetActiveSpool(ctx context.Context, spoolID int) error {
	_, err := l.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO active_spool (id, spool_id) VALUES (1, ?)`,
		spoolID)
	return err
}

// GetActiveSpool returns the currently active spool ID and its activation
// timestamp. ok is false when no spool is active.
func (l *Logger) GetActiveSpool(ctx context.Context) (spoolID int, activatedAt time.Time, ok bool, err error) {
	var ts string
	row := l.db.QueryRowContext(ctx, `SELECT spool_id, activated_at FROM active_spool WHERE id = 1`)
	if scanErr := row.Scan(&spoolID, &ts); scanErr == sql.ErrNoRows {
		return 0, time.Time{}, false, nil
	} else if scanErr != nil {
		return 0, time.Time{}, false, scanErr
	}
	activatedAt, err = time.Parse("2006-01-02T15:04:05.000Z", ts)
	if err != nil {
		return 0, time.Time{}, false, fmt.Errorf("parse activated_at %q: %w", ts, err)
	}
	return spoolID, activatedAt, true, nil
}

// ClearActiveSpool removes the active spool row, making no spool active.
func (l *Logger) ClearActiveSpool(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, `DELETE FROM active_spool WHERE id = 1`)
	return err
}

const insertSQL = `INSERT INTO print_audit_log (
	printer_ip, printer_serial,
	print_state, gcode_file, subtask_name, last_layer_num,
	parse_status, layers_printed,
	filament_type, filament_vendor,
	startup_weight_g, layer_weight_g, footer_weight_g, total_weight_g,
	spoolman_spool_id, spoolman_weight_g, spoolman_success, spoolman_error,
	filament_notes
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
