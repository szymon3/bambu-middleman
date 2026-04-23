// Package webui provides a lightweight HTTP server for active spool tracking
// via NFC stickers and QR codes.
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/szymon3/bambu-middleman/auditlog"
	"github.com/szymon3/bambu-middleman/spoolman"
)

const htmlBoilerplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>bambu-middleman</title>
  <style>
    body   { font-family: sans-serif; max-width: 420px; margin: 3rem auto; padding: 0 1.5rem; text-align: center; }
    h1     { font-size: 1.3rem; margin-bottom: 1.5rem; }
    p      { color: #555; margin-bottom: 1.5rem; }
    button { font-size: 1.1rem; padding: 0.8rem 0; width: 100%%; cursor: pointer; }
    .spool-card { display:flex; align-items:center; gap:1.2rem; margin-bottom:1.5rem; background:#f8f8f8; border-radius:10px; padding:0.9rem 1.1rem; text-align:left; }
    .spool-card svg { flex-shrink:0; }
    .spool-meta { flex:1; }
    .spool-meta strong { display:block; margin-bottom:0.35rem; font-size:1rem; }
    .spool-meta table { border-collapse:collapse; width:100%%; }
    .spool-meta td { padding:0.2rem 0.4rem; font-size:0.85rem; color:#444; }
    .spool-meta td:first-child { color:#888; white-space:nowrap; padding-right:0.8rem; }
  </style>
</head>
<body>
%s
</body>
</html>`

// htmlPage writes a complete HTML response using the shared boilerplate skeleton.
func htmlPage(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, htmlBoilerplate, content)
}

// New returns an http.Handler implementing all active-spool endpoints.
// audit must be non-nil. spoolClient may be nil (Spoolman not configured).
// baseURL is the externally reachable base URL (no trailing slash); it is only
// required at request time for the /qr endpoint.
func New(audit *auditlog.Logger, spoolClient *spoolman.Client, baseURL string) http.Handler {
	h := &handler{audit: audit, spoolClient: spoolClient, baseURL: baseURL}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /spool/active", h.getActive)
	mux.HandleFunc("GET /spool/clear", h.getClear)
	mux.HandleFunc("POST /spool/clear", h.postClear)
	mux.HandleFunc("GET /spool/{id}/activate", h.getActivate)
	mux.HandleFunc("POST /spool/{id}/activate", h.postActivate)
	mux.HandleFunc("GET /spool/{id}/qr", h.getQR)

	return limitBody(mux)
}

// limitBody wraps every request so that request bodies are capped at 1 KiB.
// POST bodies for our endpoints are not read, but the limit is a safety measure.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		next.ServeHTTP(w, r)
	})
}

type handler struct {
	audit       *auditlog.Logger
	spoolClient *spoolman.Client
	baseURL     string
}

// parseID extracts and validates the {id} path value.
// Returns (id, true) for a valid integer in [1, 999999], or (0, false) otherwise.
func parseID(r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || n < 1 || n > 999999 {
		return 0, false
	}
	return n, true
}

// fetchSpool retrieves spool details from Spoolman, returning nil if the client
// is not configured or the request fails. Errors are silently discarded so
// callers always get a usable (possibly nil) result.
func (h *handler) fetchSpool(ctx context.Context, id int) *spoolman.Spool {
	if h.spoolClient == nil {
		return nil
	}
	spool, _ := h.spoolClient.GetSpool(ctx, id)
	return spool
}

// spoolSVG returns an inline SVG resembling a spool of filament.
// colorHex is a 6-char hex color without '#' (e.g. "FF0000"). Empty → gray.
func spoolSVG(colorHex string) string {
	if colorHex == "" {
		colorHex = "CCCCCC"
	}
	return fmt.Sprintf(
		`<svg width="48" height="48" viewBox="0 0 48 48" xmlns="http://www.w3.org/2000/svg">`+
			`<circle cx="24" cy="24" r="23" fill="#e0e0e0" stroke="#ccc" stroke-width="1"/>`+
			`<circle cx="24" cy="24" r="17" fill="#%s"/>`+
			`<circle cx="24" cy="24" r="9" fill="#bbb"/>`+
			`<circle cx="24" cy="24" r="4" fill="#888"/>`+
			`</svg>`, colorHex)
}

// spoolCardHTML builds the .spool-card div for a given spool.
// spool may be nil — the card still renders with the ID and a gray icon.
func spoolCardHTML(spool *spoolman.Spool, id int) string {
	colorHex := ""
	if spool != nil {
		colorHex = spool.Filament.ColorHex
	}

	var rows strings.Builder
	if spool != nil {
		if spool.Filament.Vendor != nil && spool.Filament.Vendor.Name != "" {
			fmt.Fprintf(&rows, `<tr><td>Manufacturer</td><td>%s</td></tr>`,
				html.EscapeString(spool.Filament.Vendor.Name))
		}
		if spool.Filament.Name != "" {
			fmt.Fprintf(&rows, `<tr><td>Name</td><td>%s</td></tr>`,
				html.EscapeString(spool.Filament.Name))
		}
		if spool.Filament.Material != "" {
			fmt.Fprintf(&rows, `<tr><td>Material</td><td>%s</td></tr>`,
				html.EscapeString(spool.Filament.Material))
		}
		if spool.RemainingWeight != nil {
			fmt.Fprintf(&rows, `<tr><td>Remaining</td><td>%.0f g</td></tr>`,
				math.Round(*spool.RemainingWeight))
		}
	}

	meta := fmt.Sprintf(`<strong>#%d</strong>`, id)
	if rows.Len() > 0 {
		meta += fmt.Sprintf(`<table>%s</table>`, rows.String())
	}

	return fmt.Sprintf(`<div class="spool-card">%s<div class="spool-meta">%s</div></div>`,
		spoolSVG(colorHex), meta)
}

// GET /spool/{id}/activate — confirmation page
func (h *handler) getActivate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "invalid spool id", http.StatusBadRequest)
		return
	}
	spool := h.fetchSpool(r.Context(), id)
	htmlPage(w, fmt.Sprintf(`%s
<h1>Activate spool #%d?</h1>
<form method="POST" action="/spool/%d/activate">
  <button type="submit">Activate</button>
</form>`, spoolCardHTML(spool, id), id, id))
}

// POST /spool/{id}/activate — set active spool
func (h *handler) postActivate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "invalid spool id", http.StatusBadRequest)
		return
	}
	if err := h.audit.SetActiveSpool(r.Context(), id); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	spool := h.fetchSpool(r.Context(), id)
	htmlPage(w, fmt.Sprintf(`%s<h1>Spool #%d is now active.</h1>`,
		spoolCardHTML(spool, id), id))
}

// GET /spool/{id}/qr — QR code PNG for the activate URL
func (h *handler) getQR(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "invalid spool id", http.StatusBadRequest)
		return
	}
	if h.baseURL == "" {
		http.Error(w, "WEBUI_BASE_URL not configured", http.StatusServiceUnavailable)
		return
	}
	target := fmt.Sprintf("%s/spool/%d/activate", h.baseURL, id)
	png, err := qrcode.Encode(target, qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(png) //nolint:errcheck // write to ResponseWriter; error not actionable
}

// GET /spool/active — JSON status of the active spool
func (h *handler) getActive(w http.ResponseWriter, r *http.Request) {
	spoolID, activatedAt, ok, err := h.audit.GetActiveSpool(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.Write([]byte("{\"spool_id\":null}\n")) //nolint:errcheck
		return
	}

	type activeResp struct {
		SpoolID          int      `json:"spool_id"`
		ActivatedAt      string   `json:"activated_at"`
		Manufacturer     string   `json:"manufacturer,omitempty"`
		FilamentName     string   `json:"filament_name,omitempty"`
		Material         string   `json:"material,omitempty"`
		ColorHex         string   `json:"color_hex,omitempty"`
		RemainingWeightG *float64 `json:"remaining_weight_g,omitempty"`
	}

	ts := activatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	resp := activeResp{SpoolID: spoolID, ActivatedAt: ts}

	if spool := h.fetchSpool(r.Context(), spoolID); spool != nil {
		if spool.Filament.Vendor != nil {
			resp.Manufacturer = spool.Filament.Vendor.Name
		}
		resp.FilamentName = spool.Filament.Name
		resp.Material = spool.Filament.Material
		resp.ColorHex = spool.Filament.ColorHex
		resp.RemainingWeightG = spool.RemainingWeight
	}

	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// GET /spool/clear — clear confirmation page
func (h *handler) getClear(w http.ResponseWriter, r *http.Request) {
	spoolID, activatedAt, ok, err := h.audit.GetActiveSpool(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		htmlPage(w, `<h1>No spool is currently active.</h1>`)
		return
	}
	spool := h.fetchSpool(r.Context(), spoolID)
	ts := activatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	htmlPage(w, fmt.Sprintf(`<h1>Clear active spool?</h1>
%s
<p>Activated %s</p>
<form method="POST" action="/spool/clear">
  <button type="submit">Clear</button>
</form>`, spoolCardHTML(spool, spoolID), ts))
}

// POST /spool/clear — clear active spool
func (h *handler) postClear(w http.ResponseWriter, r *http.Request) {
	if err := h.audit.ClearActiveSpool(r.Context()); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	htmlPage(w, `<h1>Active spool cleared.</h1>`)
}
