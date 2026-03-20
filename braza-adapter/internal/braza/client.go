package braza

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Checker-Finance/adapters/internal/httpclient"
	"github.com/Checker-Finance/adapters/internal/rate"
)

// Client wraps low-level HTTP communication with Braza's API.
// It handles JWT authorization, rate limiting, and basic retry logic.
type Client struct {
	baseURL string
	token   string
	exec    *httpclient.Executor
}

// NewClient constructs a new Braza HTTP client instance.
func NewClient(baseURL, token string, rateMgr *rate.Manager) *Client {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	exec := httpclient.New(rateMgr, httpClient, 2, "braza", func(status int, body []byte) error {
		slog.Warn("braza.non_200",
			"status", status,
			"body", string(body))
		return fmt.Errorf("braza returned %d", status)
	})
	return &Client{
		baseURL: baseURL,
		token:   token,
		exec:    exec,
	}
}

// WithToken updates the bearer token (used when the auth manager refreshes JWTs).
func (c *Client) WithToken(token string) {
	c.token = token
}

// GetJSON performs an authenticated GET request and decodes the JSON response into `out`.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	return c.exec.DoJSON(ctx, req, "braza_api", out)
}

// PostJSON performs an authenticated POST request with a JSON body.
func (c *Client) PostJSON(ctx context.Context, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s%s", c.baseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	return c.exec.DoJSON(ctx, req, "braza_api", out)
}
