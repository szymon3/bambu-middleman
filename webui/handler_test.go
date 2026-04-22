package webui_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/szymon3/bambu-middleman/auditlog"
	"github.com/szymon3/bambu-middleman/webui"
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

func doRequest(h http.Handler, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// --- /spool/{id}/activate ---

func TestGetActivate_ValidID(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/42/activate")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "42") {
		t.Errorf("body does not contain spool id: %s", body)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type = %q, want text/html", w.Header().Get("Content-Type"))
	}
}

func TestGetActivate_InvalidID(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	tests := []string{"/spool/0/activate", "/spool/abc/activate", "/spool/1000000/activate", "/spool/-1/activate"}
	for _, path := range tests {
		w := doRequest(h, http.MethodGet, path)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want 400", path, w.Code)
		}
	}
}

func TestPostActivate_ValidID(t *testing.T) {
	audit := testAuditLogger(t)
	h := webui.New(audit, "http://localhost:8080")
	w := doRequest(h, http.MethodPost, "/spool/7/activate")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "7") {
		t.Errorf("body does not contain spool id: %s", w.Body.String())
	}
	// Verify the spool was actually set.
	id, _, ok, err := audit.GetActiveSpool(t.Context())
	if err != nil {
		t.Fatalf("GetActiveSpool: %v", err)
	}
	if !ok || id != 7 {
		t.Errorf("active spool = %d, ok=%v; want id=7, ok=true", id, ok)
	}
}

func TestPostActivate_InvalidID(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodPost, "/spool/0/activate")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- /spool/{id}/qr ---

func TestGetQR_Valid(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/42/qr")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if w.Header().Get("Content-Length") == "" {
		t.Error("Content-Length not set")
	}
	if w.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q, want public, max-age=86400", w.Header().Get("Cache-Control"))
	}
	if w.Body.Len() == 0 {
		t.Error("empty response body for QR PNG")
	}
}

func TestGetQR_InvalidID(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/abc/qr")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetQR_NoBaseURL(t *testing.T) {
	h := webui.New(testAuditLogger(t), "") // empty baseURL
	w := doRequest(h, http.MethodGet, "/spool/42/qr")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "WEBUI_BASE_URL") {
		t.Errorf("body does not mention WEBUI_BASE_URL: %s", w.Body.String())
	}
}

// --- /spool/active ---

func TestGetActive_Inactive(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/active")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type = %q, want application/json", w.Header().Get("Content-Type"))
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(w.Body.String())), &m); err != nil {
		t.Fatalf("unmarshal: %v — body: %s", err, w.Body.String())
	}
	if v, ok := m["spool_id"]; !ok || v != nil {
		t.Errorf("spool_id = %v, want null", v)
	}
}

func TestGetActive_Active(t *testing.T) {
	audit := testAuditLogger(t)
	if err := audit.SetActiveSpool(t.Context(), 42); err != nil {
		t.Fatalf("SetActiveSpool: %v", err)
	}
	h := webui.New(audit, "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/active")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(w.Body.String())), &m); err != nil {
		t.Fatalf("unmarshal: %v — body: %s", err, w.Body.String())
	}
	if id, ok := m["spool_id"].(float64); !ok || int(id) != 42 {
		t.Errorf("spool_id = %v, want 42", m["spool_id"])
	}
	if _, ok := m["activated_at"].(string); !ok {
		t.Errorf("activated_at missing or not a string: %v", m["activated_at"])
	}
}

// --- /spool/clear ---

func TestGetClear_Inactive(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/clear")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No spool is currently active") {
		t.Errorf("body missing expected text: %s", w.Body.String())
	}
}

func TestGetClear_Active(t *testing.T) {
	audit := testAuditLogger(t)
	if err := audit.SetActiveSpool(t.Context(), 5); err != nil {
		t.Fatalf("SetActiveSpool: %v", err)
	}
	h := webui.New(audit, "http://localhost:8080")
	w := doRequest(h, http.MethodGet, "/spool/clear")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "5") {
		t.Errorf("body does not contain spool id 5: %s", body)
	}
	if !strings.Contains(body, `action="/spool/clear"`) {
		t.Errorf("body missing clear form: %s", body)
	}
}

func TestPostClear(t *testing.T) {
	audit := testAuditLogger(t)
	if err := audit.SetActiveSpool(t.Context(), 3); err != nil {
		t.Fatalf("SetActiveSpool: %v", err)
	}
	h := webui.New(audit, "http://localhost:8080")
	w := doRequest(h, http.MethodPost, "/spool/clear")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "cleared") {
		t.Errorf("body missing 'cleared': %s", w.Body.String())
	}
	// Verify actually cleared.
	_, _, ok, err := audit.GetActiveSpool(t.Context())
	if err != nil {
		t.Fatalf("GetActiveSpool: %v", err)
	}
	if ok {
		t.Error("expected no active spool after POST /spool/clear")
	}
}

// --- body size limit ---

func TestBodySizeLimit(t *testing.T) {
	h := webui.New(testAuditLogger(t), "http://localhost:8080")
	// Send a 2 KiB body to POST /spool/clear — should not panic or stall.
	req := httptest.NewRequest(http.MethodPost, "/spool/clear", strings.NewReader(strings.Repeat("x", 2048)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// We don't care about the status code here — just that it doesn't hang.
	io.Discard.Write([]byte{}) // keep compiler happy
}
