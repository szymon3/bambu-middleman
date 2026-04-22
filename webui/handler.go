// Package webui provides a lightweight HTTP server for active spool tracking
// via NFC stickers and QR codes.
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/szymon3/bambu-middleman/auditlog"
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
// audit must be non-nil. baseURL is the externally reachable base URL
// (no trailing slash); it is only required at request time for the /qr endpoint.
func New(audit *auditlog.Logger, baseURL string) http.Handler {
	h := &handler{audit: audit, baseURL: baseURL}

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
	audit   *auditlog.Logger
	baseURL string
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

// GET /spool/{id}/activate — confirmation page
func (h *handler) getActivate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		http.Error(w, "invalid spool id", http.StatusBadRequest)
		return
	}
	htmlPage(w, fmt.Sprintf(`<h1>Activate spool #%d?</h1>
<form method="POST" action="/spool/%d/activate">
  <button type="submit">Activate</button>
</form>`, id, id))
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
	htmlPage(w, fmt.Sprintf(`<h1>Spool #%d is now active.</h1>`, id))
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
		SpoolID     int    `json:"spool_id"`
		ActivatedAt string `json:"activated_at"`
	}
	ts := activatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	json.NewEncoder(w).Encode(activeResp{SpoolID: spoolID, ActivatedAt: ts}) //nolint:errcheck
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
	ts := activatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	htmlPage(w, fmt.Sprintf(`<h1>Clear active spool?</h1>
<p>Currently active: spool #%d (activated %s)</p>
<form method="POST" action="/spool/clear">
  <button type="submit">Clear</button>
</form>`, spoolID, ts))
}

// POST /spool/clear — clear active spool
func (h *handler) postClear(w http.ResponseWriter, r *http.Request) {
	if err := h.audit.ClearActiveSpool(r.Context()); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	htmlPage(w, `<h1>Active spool cleared.</h1>`)
}
