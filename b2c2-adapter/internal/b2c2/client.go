package b2c2

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

// Client wraps HTTP communication with the B2C2 API.
// A single Client instance serves all tenants; credentials are supplied per-request.
type Client struct {
	logger *zap.Logger
	exec   *httpclient.Executor
}

// NewClient constructs a B2C2 HTTP client.
func NewClient(logger *zap.Logger, rateMgr *rate.Manager) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	exec := httpclient.New(logger, rateMgr, httpClient, 2, "b2c2", func(status int, body []byte) error {
		var errResp ErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return fmt.Errorf("b2c2 returned %d: %s", status, string(body))
	})
	return &Client{
		logger: logger,
		exec:   exec,
	}
}

// RequestQuote sends an RFQ to B2C2.
// POST /request_for_quote/
func (c *Client) RequestQuote(ctx context.Context, cfg *B2C2ClientConfig, req *RFQRequest) (*RFQResponse, error) {
	var resp RFQResponse
	if err := c.postJSON(ctx, cfg, "/request_for_quote/", req, &resp); err != nil {
		return nil, fmt.Errorf("b2c2: request_for_quote: %w", err)
	}
	return &resp, nil
}

// ExecuteOrder submits a FOK order to B2C2.
// POST /v2/order/
func (c *Client) ExecuteOrder(ctx context.Context, cfg *B2C2ClientConfig, req *OrderRequest) (*OrderResponse, error) {
	var resp OrderResponse
	if err := c.postJSON(ctx, cfg, "/v2/order/", req, &resp); err != nil {
		return nil, fmt.Errorf("b2c2: execute_order: %w", err)
	}
	return &resp, nil
}

// GetBalance retrieves account balances.
// GET /balance
func (c *Client) GetBalance(ctx context.Context, cfg *B2C2ClientConfig) (BalanceResponse, error) {
	var resp BalanceResponse
	if err := c.getJSON(ctx, cfg, "/balance", &resp); err != nil {
		return nil, fmt.Errorf("b2c2: get_balance: %w", err)
	}
	return resp, nil
}

// GetInstruments retrieves available trading instruments.
// GET /instruments
func (c *Client) GetInstruments(ctx context.Context, cfg *B2C2ClientConfig) ([]Instrument, error) {
	var resp []Instrument
	if err := c.getJSON(ctx, cfg, "/instruments", &resp); err != nil {
		return nil, fmt.Errorf("b2c2: get_instruments: %w", err)
	}
	return resp, nil
}

// getJSON performs an authenticated GET request and decodes the JSON response.
func (c *Client) getJSON(ctx context.Context, cfg *B2C2ClientConfig, path string, out any) error {
	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIToken)
	return c.exec.DoJSON(ctx, req, "", out)
}

// postJSON performs an authenticated POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, cfg *B2C2ClientConfig, path string, body any, out any) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	setHeaders(req, cfg.APIToken)
	return c.exec.DoJSON(ctx, req, "", out)
}

// setHeaders adds the B2C2 authorization and content-type headers.
func setHeaders(req *http.Request, apiToken string) {
	req.Header.Set("Authorization", "Token "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
