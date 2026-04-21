package spoolman

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client sends filament usage updates to a Spoolman instance.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a Client targeting the given Spoolman base URL.
// Trailing slashes are trimmed.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// UseSpool records filament consumption on spool spoolID.
// It calls PATCH {baseURL}/api/v1/spool/{id}/use with body {"use_weight": <weightG>}.
// Returns an error for non-2xx responses.
func (c *Client) UseSpool(ctx context.Context, spoolID int, weightG float64) error {
	url := fmt.Sprintf("%s/api/v1/spool/%d/use", c.baseURL, spoolID)
	body := fmt.Sprintf(`{"use_weight":%g}`, weightG)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("spoolman returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}
