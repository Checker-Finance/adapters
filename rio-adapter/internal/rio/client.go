package rio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/rate"
)

// Client wraps low-level HTTP communication with Rio's API.
// Configuration (base URL, API key) is supplied per-request via RioClientConfig
// so that a single Client instance can serve multiple tenants.
type Client struct {
	logger   *zap.Logger
	rateMgr  *rate.Manager
	http     *http.Client
	retryMax int
}

// NewClient constructs a new Rio HTTP client instance.
func NewClient(logger *zap.Logger, rateMgr *rate.Manager) *Client {
	return &Client{
		logger:   logger,
		rateMgr:  rateMgr,
		retryMax: 2,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateQuote creates a new quote on Rio.
// POST /api/quotes
func (c *Client) CreateQuote(ctx context.Context, cfg *RioClientConfig, req *RioQuoteRequest) (*RioQuoteResponse, error) {
	var resp RioQuoteResponse
	if err := c.postJSON(ctx, cfg, "/api/quotes", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateOrder creates a new order from a quote on Rio.
// POST /api/orders
func (c *Client) CreateOrder(ctx context.Context, cfg *RioClientConfig, req *RioOrderRequest) (*RioOrderResponse, error) {
	var resp RioOrderResponse
	if err := c.postJSON(ctx, cfg, "/api/orders", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetOrder retrieves an order by ID.
// GET /api/orders/{id}
func (c *Client) GetOrder(ctx context.Context, cfg *RioClientConfig, orderID string) (*RioOrderResponse, error) {
	var resp RioOrderResponse
	if err := c.getJSON(ctx, cfg, "/api/orders/"+orderID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RegisterWebhook registers a webhook for order status changes.
// POST /api/webhooks/orders
// Webhook registration uses an explicit config since it is a global operation.
func (c *Client) RegisterWebhook(ctx context.Context, cfg *RioClientConfig, callbackURL string, retryOnFailure bool) (*RioWebhookRegistrationResponse, error) {
	req := &RioWebhookRegistration{
		URL:            callbackURL,
		RetryOnFailure: retryOnFailure,
	}
	var resp RioWebhookRegistrationResponse
	if err := c.postJSON(ctx, cfg, "/api/webhooks/orders", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// getJSON performs an authenticated GET request and decodes the JSON response.
func (c *Client) getJSON(ctx context.Context, cfg *RioClientConfig, path string, out any) error {
	url := fmt.Sprintf("%s%s", cfg.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIKey)

	return c.doJSON(ctx, req, cfg.rateLimitKey(), out)
}

// postJSON performs an authenticated POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, cfg *RioClientConfig, path string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s", cfg.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIKey)

	return c.doJSON(ctx, req, cfg.rateLimitKey(), out)
}

// setHeaders sets the required headers for Rio API requests.
func setHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// doJSON executes the HTTP request with rate limiting and retries, and unmarshals JSON.
// The rateLimitKey isolates rate limits per client.
func (c *Client) doJSON(ctx context.Context, req *http.Request, rateLimitKey string, out any) error {
	// Respect rate limiter (per-client key)
	if c.rateMgr != nil {
		if err := c.rateMgr.Wait(ctx, rateLimitKey); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		start := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("rio.http_failed",
				zap.String("url", req.URL.String()),
				zap.Error(err),
				zap.Int("attempt", attempt))
			time.Sleep(backoff(attempt))
			continue
		}
		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)
		elapsed := time.Since(start)

		// Retry on server errors
		if resp.StatusCode >= 500 {
			c.logger.Warn("rio.server_error",
				zap.Int("status", resp.StatusCode),
				zap.String("url", req.URL.String()),
				zap.Duration("latency", elapsed))
			lastErr = fmt.Errorf("rio server error: %d", resp.StatusCode)
			time.Sleep(backoff(attempt))
			continue
		}

		// Check for client errors
		if resp.StatusCode >= 400 {
			var errResp RioErrorResponse
			_ = json.Unmarshal(body, &errResp)

			c.logger.Warn("rio.client_error",
				zap.Int("status", resp.StatusCode),
				zap.String("error", errResp.Error),
				zap.String("message", errResp.Message),
				zap.String("body", string(body)))

			errMsg := errResp.Message
			if errMsg == "" {
				errMsg = errResp.Error
			}
			if errMsg == "" {
				errMsg = string(body)
			}

			return fmt.Errorf("rio returned %d: %s", resp.StatusCode, errMsg)
		}

		// Success - decode response
		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				c.logger.Warn("rio.decode_failed",
					zap.Error(err),
					zap.String("url", req.URL.String()),
					zap.String("body", string(body)))
				return fmt.Errorf("decode failed: %w", err)
			}
		}

		c.logger.Debug("rio.http_success",
			zap.String("url", req.URL.String()),
			zap.Int("status", resp.StatusCode),
			zap.Duration("elapsed", elapsed))

		return nil
	}

	return fmt.Errorf("rio request failed after %d attempts: %w", c.retryMax, lastErr)
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
