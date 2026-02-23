package rio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/httpclient"
	"github.com/Checker-Finance/adapters/internal/rate"
)

// Client wraps low-level HTTP communication with Rio's API.
// Configuration (base URL, API key) is supplied per-request via RioClientConfig
// so that a single Client instance can serve multiple tenants.
type Client struct {
	logger *zap.Logger
	exec   *httpclient.Executor
}

// NewClient constructs a new Rio HTTP client instance.
func NewClient(logger *zap.Logger, rateMgr *rate.Manager) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	exec := httpclient.New(logger, rateMgr, httpClient, 2, "rio", func(status int, body []byte) error {
		var errResp RioErrorResponse
		_ = json.Unmarshal(body, &errResp)

		logger.Warn("rio.client_error",
			zap.Int("status", status),
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
		return fmt.Errorf("rio returned %d: %s", status, errMsg)
	})
	return &Client{
		logger: logger,
		exec:   exec,
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

	return c.exec.DoJSON(ctx, req, cfg.rateLimitKey(), out)
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

	return c.exec.DoJSON(ctx, req, cfg.rateLimitKey(), out)
}

// setHeaders sets the required headers for Rio API requests.
func setHeaders(req *http.Request, apiKey string) {
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
