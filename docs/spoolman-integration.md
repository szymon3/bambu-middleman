# Spoolman Integration

When a print finishes (or fails), bambu-middleman can automatically record the filament consumed on the matching spool in [Spoolman](https://github.com/Donkie/Spoolman).

## Enabling

Set `SPOOLMAN_URL` to the base URL of your Spoolman instance:

```
SPOOLMAN_URL=http://192.168.1.10:7912
```

## How it works

1. A print finishes or fails.
2. bambu-middleman downloads and parses the gcode file from the printer.
3. Filament usage is computed -- for failed prints, only usage up to the last printed layer is counted (not the full file).
4. A spool ID is resolved (see below).
5. The computed weight is sent to Spoolman via `PUT /api/v1/spool/{id}/use`.
6. The result (success or failure) is recorded in the audit log.

If no spool ID can be resolved, the print is still logged but Spoolman is not updated.

## Spool ID resolution

The `SPOOLMAN_SOURCE` env var controls how bambu-middleman finds the spool ID when a print finishes. The value is a comma-separated, ordered list -- sources are tried left to right and the first match wins.

| Source | Description |
|--------|-------------|
| `api` | Active spool set via the web UI (NFC/QR). Requires `WEBUI_ADDR` and `AUDIT_DB_PATH`. |
| `notes` | Spoolman ID tag embedded in the OrcaSlicer filament profile notes (from 3MF metadata). |

Default: `api,notes`

### Resolution matrix

| `SPOOLMAN_SOURCE` | active spool set, notes has ID | active spool set, no notes ID | notes has ID, no active spool | neither |
|---|---|---|---|---|
| `api,notes` *(default)* | active spool | active spool | notes | skipped |
| `notes,api` | notes | active spool | notes | skipped |
| `api` | active spool | active spool | skipped | skipped |
| `notes` | notes | skipped | notes | skipped |

### Filament notes tagging

If you always print with the same filament profile for a given spool, you can embed the Spoolman ID directly in the profile so no NFC/QR tapping is required.

In OrcaSlicer, open the filament profile and add to the **Notes** field:

```
spoolman#42
```

The tag is case-insensitive and can appear anywhere in the notes alongside other text:

```
Bought 2025-01. SPOOLMAN#42 leftover spool.
```

This requires the file to be sliced as a `.3mf` archive (OrcaSlicer's default) -- plain `.gcode` files do not carry filament notes metadata.
