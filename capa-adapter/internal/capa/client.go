package capa

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
	"github.com/Checker-Finance/adapters/capa-adapter/internal/metrics"
)

// Client wraps low-level HTTP communication with the Capa API.
// A single Client instance can serve multiple tenants; credentials are
// supplied per-request via CapaClientConfig.
type Client struct {
	logger *zap.Logger
	exec   *httpclient.Executor
}

// NewClient constructs a new Capa HTTP client with rate limiting and retries.
func NewClient(logger *zap.Logger, rateMgr *rate.Manager) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	exec := httpclient.New(logger, rateMgr, httpClient, 2, "capa", func(status int, body []byte) error {
		var errResp CapaErrorResponse
		_ = json.Unmarshal(body, &errResp)

		logger.Warn("capa.client_error",
			zap.Int("status", status),
			zap.String("code", errResp.Code),
			zap.String("message", errResp.Message),
			zap.String("body", string(body)))

		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		if msg == "" {
			msg = string(body)
		}
		return fmt.Errorf("capa returned %d: %s", status, msg)
	})
	return &Client{
		logger: logger,
		exec:   exec,
	}
}

// GetCrossRampQuote creates a new cross-ramp (fiat→fiat) quote.
// POST /api/partner/v2/cross-ramp/quotes
func (c *Client) GetCrossRampQuote(ctx context.Context, cfg *CapaClientConfig, req *CapaCrossRampQuoteRequest) (*CapaQuoteResponse, error) {
	const endpoint, method = "/api/partner/v2/cross-ramp/quotes", http.MethodPost
	start := time.Now()
	var resp CapaQuoteResponse
	err := c.postJSON(ctx, cfg, endpoint, req, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetQuote creates a new on-ramp or off-ramp quote.
// POST /api/partner/v2/quotes
func (c *Client) GetQuote(ctx context.Context, cfg *CapaClientConfig, req *CapaQuoteRequest) (*CapaQuoteResponse, error) {
	const endpoint, method = "/api/partner/v2/quotes", http.MethodPost
	start := time.Now()
	var resp CapaQuoteResponse
	err := c.postJSON(ctx, cfg, endpoint, req, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateCrossRamp executes a cross-ramp (fiat→fiat) transaction.
// POST /api/partner/v2/cross-ramp
func (c *Client) CreateCrossRamp(ctx context.Context, cfg *CapaClientConfig, req *CapaCrossRampExecuteRequest) (*CapaExecuteResponse, error) {
	const endpoint, method = "/api/partner/v2/cross-ramp", http.MethodPost
	start := time.Now()
	var resp CapaExecuteResponse
	err := c.postJSON(ctx, cfg, endpoint, req, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateOnRamp executes an on-ramp (fiat→crypto) transaction.
// POST /api/partner/v2/on-ramp
func (c *Client) CreateOnRamp(ctx context.Context, cfg *CapaClientConfig, req *CapaOnRampExecuteRequest) (*CapaExecuteResponse, error) {
	const endpoint, method = "/api/partner/v2/on-ramp", http.MethodPost
	start := time.Now()
	var resp CapaExecuteResponse
	err := c.postJSON(ctx, cfg, endpoint, req, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateOffRamp executes an off-ramp (crypto→fiat) transaction.
// POST /api/partner/v2/off-ramp
func (c *Client) CreateOffRamp(ctx context.Context, cfg *CapaClientConfig, req *CapaOffRampExecuteRequest) (*CapaExecuteResponse, error) {
	const endpoint, method = "/api/partner/v2/off-ramp", http.MethodPost
	start := time.Now()
	var resp CapaExecuteResponse
	err := c.postJSON(ctx, cfg, endpoint, req, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransaction retrieves a transaction by ID.
// GET /api/partner/v2/transactions/{transactionId}
func (c *Client) GetTransaction(ctx context.Context, cfg *CapaClientConfig, txID string) (*CapaTransactionResponse, error) {
	const endpoint, method = "/api/partner/v2/transactions/{id}", http.MethodGet
	start := time.Now()
	var resp CapaTransactionResponse
	err := c.getJSON(ctx, cfg, "/api/partner/v2/transactions/"+txID, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransactions retrieves a list of transactions for the partner.
// GET /api/partner/v2/transactions
func (c *Client) GetTransactions(ctx context.Context, cfg *CapaClientConfig) ([]CapaTransaction, error) {
	const endpoint, method = "/api/partner/v2/transactions", http.MethodGet
	start := time.Now()
	var resp struct {
		Transactions []CapaTransaction `json:"transactions"`
	}
	err := c.getJSON(ctx, cfg, endpoint, &resp)
	metrics.IncCapaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.CapaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return resp.Transactions, nil
}

// statusLabel returns "ok" or "error" for use as a Prometheus label.
func statusLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// getJSON performs an authenticated GET request and decodes the JSON response.
func (c *Client) getJSON(ctx context.Context, cfg *CapaClientConfig, path string, out any) error {
	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIKey)
	return c.exec.DoJSON(ctx, req, cfg.UserID, out)
}

// postJSON performs an authenticated POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, cfg *CapaClientConfig, path string, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		var marshalErr error
		bodyBytes, marshalErr = json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
	}

	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIKey)
	return c.exec.DoJSON(ctx, req, cfg.UserID, out)
}

// setHeaders sets required headers for Capa API requests.
func setHeaders(req *http.Request, apiKey string) {
	req.Header.Set("partner-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
