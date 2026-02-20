package braza

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/rate"
)

// Client wraps low-level HTTP communication with Braza’s API.
// It handles JWT authorization, rate limiting, and basic retry logic.
type Client struct {
	logger   *zap.Logger
	baseURL  string
	token    string
	rateMgr  *rate.Manager
	http     *http.Client
	retryMax int
}

// NewClient constructs a new Braza HTTP client instance.
func NewClient(logger *zap.Logger, baseURL, token string, rateMgr *rate.Manager) *Client {
	return &Client{
		logger:   logger,
		baseURL:  baseURL,
		token:    token,
		rateMgr:  rateMgr,
		retryMax: 2,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
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

	return c.doJSON(ctx, req, out)
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

	return c.doJSON(ctx, req, out)
}

// doJSON executes the HTTP request with rate limiting and retries, and unmarshals JSON.
func (c *Client) doJSON(ctx context.Context, req *http.Request, out any) error {
	// Respect rate limiter
	if c.rateMgr != nil {
		if err := c.rateMgr.Wait(ctx, "braza_api"); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		start := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("braza.http_failed",
				zap.String("url", req.URL.String()),
				zap.Error(err),
				zap.Int("attempt", attempt))
			time.Sleep(backoff(attempt))
			continue
		}
		defer resp.Body.Close() //nolint:errcheck

		body, _ := io.ReadAll(resp.Body)
		elapsed := time.Since(start)

		if resp.StatusCode >= 500 {
			c.logger.Warn("braza.server_error",
				zap.Int("status", resp.StatusCode),
				zap.String("url", req.URL.String()),
				zap.Duration("latency", elapsed))
			lastErr = fmt.Errorf("braza server error: %d", resp.StatusCode)
			time.Sleep(backoff(attempt))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			c.logger.Warn("braza.non_200",
				zap.Int("status", resp.StatusCode),
				zap.String("body", string(body)))
			return fmt.Errorf("braza returned %d", resp.StatusCode)
		}

		if err := json.Unmarshal(body, out); err != nil {
			c.logger.Warn("braza.decode_failed",
				zap.Error(err),
				zap.String("url", req.URL.String()))
			return fmt.Errorf("decode failed: %w", err)
		}

		c.logger.Debug("braza.http_success",
			zap.String("url", req.URL.String()),
			zap.Duration("elapsed", elapsed))
		return nil
	}

	return fmt.Errorf("braza request failed after %d attempts: %w", c.retryMax, lastErr)
}

// backoff implements simple exponential retry delays.
func backoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 100 * time.Millisecond
	case 1:
		return 250 * time.Millisecond
	default:
		return 500 * time.Millisecond
	}
}
