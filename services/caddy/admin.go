package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultAdminURL      = "http://127.0.0.1:2019"
	maxCaddyfileSize     = 512 * 1024
	maxAdminResponseSize = 2 * 1024 * 1024
)

// AdminClient applies a complete Caddyfile through Caddy's local admin API.
// The endpoint is intentionally configurable for tests and for installations
// where the admin listener is bound to a non-default address.
type AdminClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewAdminClient(baseURL string, client *http.Client) (*AdminClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAdminURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("invalid Caddy admin URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &AdminClient{BaseURL: baseURL, HTTPClient: client}, nil
}

// Apply adapts and loads the supplied Caddyfile. The previous JSON config is
// captured before loading so an admin API rejection can be rolled back without
// interrupting the last known-good gateway configuration.
func (c *AdminClient) Apply(ctx context.Context, caddyfile []byte) error {
	if c == nil || c.HTTPClient == nil || strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("Caddy admin client is not configured")
	}
	if len(caddyfile) > maxCaddyfileSize {
		return fmt.Errorf("Caddyfile exceeds %d bytes", maxCaddyfileSize)
	}
	previous, previousErr := c.snapshot(ctx)
	if err := c.adapt(ctx, caddyfile); err != nil {
		return err
	}
	// /load needs the adapter query as well; Content-Type alone does not make
	// Caddy parse a Caddyfile and would otherwise result in a JSON parse error.
	if err := c.load(ctx, caddyfile, "text/caddyfile", "caddyfile"); err != nil {
		if previousErr == nil && len(previous) > 0 {
			if rollbackErr := c.load(ctx, previous, "application/json", ""); rollbackErr != nil {
				return fmt.Errorf("load Caddyfile: %w; rollback failed: %v", err, rollbackErr)
			}
		}
		return err
	}
	return nil
}

func (c *AdminClient) snapshot(ctx context.Context) ([]byte, error) {
	return c.request(ctx, http.MethodGet, "/config/", nil, "")
}

func (c *AdminClient) adapt(ctx context.Context, caddyfile []byte) error {
	// /adapt validates and converts the Caddyfile without changing the active
	// configuration. Caddy returns a JSON config on success.
	if _, err := c.request(ctx, http.MethodPost, "/adapt?adapter=caddyfile", caddyfile, "text/caddyfile"); err != nil {
		return fmt.Errorf("adapt Caddyfile: %w", err)
	}
	return nil
}

func (c *AdminClient) load(ctx context.Context, body []byte, contentType, adapter string) error {
	path := "/load"
	if adapter != "" {
		path += "?adapter=" + url.QueryEscape(adapter)
	}
	if _, err := c.request(ctx, http.MethodPost, path, body, contentType); err != nil {
		return fmt.Errorf("load Caddy configuration: %w", err)
	}
	return nil
}

func (c *AdminClient) request(ctx context.Context, method, path string, body []byte, contentType string) ([]byte, error) {
	endpoint := c.BaseURL + "/" + strings.TrimLeft(path, "/")
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxAdminResponseSize))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("Caddy admin API %s: %s", response.Status, message)
	}
	return data, nil
}

// ValidateJSON is a small helper for tests and diagnostics. Caddy's snapshot
// is JSON, and validating it before rollback avoids posting a truncated body.
func ValidateJSON(raw []byte) bool {
	var value any
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil
}
