# HTTP API Reference

The built-in HTTP server is started when `WEBUI_ADDR` is set. It serves the active spool tracking web UI and a JSON API.

## Endpoints

| Method | Path | Response | Description |
|--------|------|----------|-------------|
| `GET` | `/spool/active` | `application/json` | Current active spool status |
| `GET` | `/spool/{id}/activate` | `text/html` | Activation confirmation page (safe for NFC/QR) |
| `POST` | `/spool/{id}/activate` | `text/html` | Sets spool as active |
| `GET` | `/spool/clear` | `text/html` | Clear confirmation page showing current active spool |
| `POST` | `/spool/clear` | `text/html` | Clears the active spool |
| `GET` | `/spool/{id}/qr` | `image/png` | QR code PNG encoding the activate URL (256x256, cached 24h) |
| `GET` | `/spool/{id}/label` | `text/html` | Print-ready label with QR code and filament details |

## Spool ID validation

Valid spool IDs are integers in the range **1 -- 999999**. Requests outside that range return `400 Bad Request`.

## GET /spool/active

Returns the currently active spool as JSON.

When a spool is active:

```json
{
  "spool_id": 42,
  "activated_at": "2026-04-22T20:31:00.000Z",
  "manufacturer": "Jayo",
  "filament_name": "Generic PLA",
  "material": "PLA",
  "color_hex": "FF0000",
  "remaining_weight_g": 250.5
}
```

When no spool is active:

```json
{
  "spool_id": null
}
```

The `manufacturer`, `filament_name`, `material`, `color_hex`, and `remaining_weight_g` fields are only populated when Spoolman is configured (`SPOOLMAN_URL`). They are omitted otherwise.

## GET/POST /spool/{id}/activate

The activate flow uses a **GET -> POST two-step** deliberately. NFC tags and QR codes can only trigger GET requests (that is all a phone browser does when it reads them), so the GET endpoint serves a confirmation page rather than performing the action directly. The actual state change happens on the subsequent POST submitted by the HTML form.

This also prevents accidental activations from an unintentional NFC tap.

When Spoolman is configured, the confirmation page shows a spool card with filament details (manufacturer, name, material, remaining weight, color).

## GET/POST /spool/clear

Same two-step flow as activation. The GET page shows the currently active spool; the POST clears it.

## GET /spool/{id}/qr

Returns a 256x256 PNG QR code encoding `{WEBUI_BASE_URL}/spool/{id}/activate`. Requires `WEBUI_BASE_URL` to be set (returns `503 Service Unavailable` otherwise).

The response is served with `Cache-Control: public, max-age=86400` (24 hours).

## GET /spool/{id}/label

Returns a self-contained, print-ready HTML page with:

- A QR code linking to the activate URL
- Filament details from Spoolman (manufacturer, name, material)
- A color-coded spool icon

Requires `WEBUI_BASE_URL` to be set.

Query parameters:

| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `orientation` | `vertical`, `horizontal` | `vertical` | `vertical` stacks QR above details; `horizontal` places them side by side |
