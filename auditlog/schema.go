package auditlog

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 2

// Each entry is executed as a single transaction when upgrading from the
// previous version. Append new entries for future schema changes.
var migrations = []string{
	// Version 1: initial audit log table and indexes.
	`CREATE TABLE IF NOT EXISTS print_audit_log (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),

	printer_ip      TEXT    NOT NULL,
	printer_serial  TEXT    NOT NULL,

	print_state     TEXT    NOT NULL,
	gcode_file      TEXT    NOT NULL,
	subtask_name    TEXT    NOT NULL DEFAULT '',
	last_layer_num  INTEGER NOT NULL DEFAULT 0,

	parse_status    TEXT    NOT NULL,
	layers_printed  INTEGER NOT NULL DEFAULT 0,

	filament_type   TEXT    NOT NULL DEFAULT '',
	filament_vendor TEXT    NOT NULL DEFAULT '',

	startup_weight_g REAL   NOT NULL DEFAULT 0,
	layer_weight_g   REAL   NOT NULL DEFAULT 0,
	footer_weight_g  REAL   NOT NULL DEFAULT 0,
	total_weight_g   REAL   NOT NULL DEFAULT 0,

	spoolman_spool_id INTEGER,
	spoolman_weight_g REAL,
	spoolman_success  INTEGER,
	spoolman_error    TEXT,

	filament_notes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pal_created_at ON print_audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_pal_spoolman   ON print_audit_log(spoolman_spool_id);
CREATE INDEX IF NOT EXISTS idx_pal_serial     ON print_audit_log(printer_serial);`,

	// Version 2: active spool singleton table.
	// CHECK (id = 1) + INSERT OR REPLACE enforces a single row.
	`CREATE TABLE IF NOT EXISTS active_spool (
	id           INTEGER PRIMARY KEY CHECK (id = 1),
	spool_id     INTEGER NOT NULL,
	activated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`,
}

// migrate applies pending schema migrations inside transactions.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRowContext(ctx, `SELECT version FROM schema_version LIMIT 1`)
	if err := row.Scan(&current); err == sql.ErrNoRows {
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return fmt.Errorf("init schema_version: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_version SET version = ?`, i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("update schema_version to %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}
	return nil
}
