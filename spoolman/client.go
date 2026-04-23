package spoolman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SpoolVendor is the vendor sub-object in a Spoolman spool response.
type SpoolVendor struct {
	Name string `json:"name"`
}

// SpoolFilament is the filament sub-object in a Spoolman spool response.
type SpoolFilament struct {
	Name     string       `json:"name"`
	Material string       `json:"material"`
	ColorHex string       `json:"color_hex"` // 6-char hex without '#', e.g. "FF0000"; empty if unset
	Vendor   *SpoolVendor `json:"vendor"`
}

// Spool holds the display fields from a Spoolman spool record.
type Spool struct {
	ID              int           `json:"id"`
	RemainingWeight *float64      `json:"remaining_weight"`
	Filament        SpoolFilament `json:"filament"`
}

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

// GetSpool fetches spool details from GET {baseURL}/api/v1/spool/{id}.
// Returns an error for non-2xx responses or JSON decode failures.
func (c *Client) GetSpool(ctx context.Context, spoolID int) (*Spool, error) {
	url := fmt.Sprintf("%s/api/v1/spool/%d", c.baseURL, spoolID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("spoolman returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var spool Spool
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&spool); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &spool, nil
}

// UseSpool records filament consumption on spool spoolID.
// It calls PUT {baseURL}/api/v1/spool/{id}/use with body {"use_weight": <weightG>}.
// Returns an error for non-2xx responses.
func (c *Client) UseSpool(ctx context.Context, spoolID int, weightG float64) error {
	url := fmt.Sprintf("%s/api/v1/spool/%d/use", c.baseURL, spoolID)
	body := fmt.Sprintf(`{"use_weight":%g}`, weightG)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBufferString(body))
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
