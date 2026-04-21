package spoolman

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUseSpool_CorrectRequest(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]float64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	if err := c.UseSpool(context.Background(), 42, 12.5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/spool/42/use" {
		t.Errorf("path = %q, want /api/v1/spool/42/use", gotPath)
	}
	if gotBody["use_weight"] != 12.5 {
		t.Errorf("use_weight = %v, want 12.5", gotBody["use_weight"])
	}
}

func TestUseSpool_NonTwoXX_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.UseSpool(context.Background(), 1, 5.0)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestUseSpool_TrailingSlashTrimmed(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL + "/") // trailing slash
	if err := c.UseSpool(context.Background(), 7, 3.0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/spool/7/use" {
		t.Errorf("path = %q, want /api/v1/spool/7/use", gotPath)
	}
}
