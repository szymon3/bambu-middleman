# Audit Log

bambu-middleman can record every print completion to a local SQLite database. This provides a persistent history of all prints, filament usage, and Spoolman update results.

## Enabling

Set `AUDIT_DB_PATH` to the desired file path:

```
AUDIT_DB_PATH=/var/lib/bambu-observer/audit.db
```

The database and directory are created automatically on first run. Schema migrations are applied on startup.

> The audit database is also required for [active spool tracking](active-spool-tracking.md). The observer will refuse to start if `WEBUI_ADDR` is set without `AUDIT_DB_PATH`.

## What's recorded

Every print completion (both successful and failed) creates a row in the `print_audit_log` table:

| Field | Description |
|-------|-------------|
| `created_at` | Timestamp (ISO 8601) |
| `printer_ip`, `printer_serial` | Printer identification |
| `print_state` | `FINISH` or `FAILED` |
| `gcode_file`, `subtask_name` | Printed file name and subtask |
| `last_layer_num` | Last layer reached (for failed prints) |
| `parse_status` | `OK` or `PARTIAL` |
| `layers_printed` | Number of layers with filament tracked |
| `filament_type`, `filament_vendor` | Filament metadata from gcode |
| `startup_weight_g` | Priming/purge filament weight |
| `layer_weight_g` | Model layer filament weight |
| `footer_weight_g` | Slicer-reported footer weight |
| `total_weight_g` | Total filament consumed (startup + layers) |
| `spoolman_spool_id` | Spool ID if Spoolman update was attempted |
| `spoolman_weight_g` | Weight reported to Spoolman |
| `spoolman_success` | 1 = success, 0 = failed, NULL = skipped |
| `spoolman_error` | Error message if Spoolman update failed |
| `filament_notes` | OrcaSlicer filament profile notes |

## Active spool state

The `active_spool` table stores the currently active spool (singleton row). This is managed by the [active spool tracking](active-spool-tracking.md) web UI and cleared automatically on filament load events.

## SQLite configuration

The database uses WAL (Write-Ahead Logging) mode for concurrent read/write access. Audit log entries are written asynchronously via a buffered channel (64 entries deep) to avoid blocking the main event loop. Active spool operations (set/get/clear) are synchronous.
