package xfx

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

// Client wraps low-level HTTP communication with the XFX API.
// Configuration (base URL, credentials) is supplied per-request so that a
// single Client instance can serve multiple tenants.
type Client struct {
	logger  *zap.Logger
	exec    *httpclient.Executor
	tokens  *TokenManager
}

// NewClient constructs a new XFX HTTP client.
func NewClient(logger *zap.Logger, rateMgr *rate.Manager, tokens *TokenManager) *Client {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	exec := httpclient.New(logger, rateMgr, httpClient, 2, "xfx", func(status int, body []byte) error {
		var errResp XFXErrorResponse
		_ = json.Unmarshal(body, &errResp)

		logger.Warn("xfx.client_error",
			zap.Int("status", status),
			zap.String("code", errResp.Error.Code),
			zap.String("message", errResp.Error.Message),
			zap.String("body", string(body)))

		msg := errResp.Error.Message
		if msg == "" {
			msg = errResp.Message
		}
		if msg == "" {
			msg = string(body)
		}
		return fmt.Errorf("xfx returned %d: %s", status, msg)
	})
	return &Client{
		logger: logger,
		exec:   exec,
		tokens: tokens,
	}
}

// RequestQuote creates a new executable quote.
// POST /v1/customer/quotes
func (c *Client) RequestQuote(ctx context.Context, cfg *XFXClientConfig, req *XFXQuoteRequest) (*XFXQuoteResponse, error) {
	var resp XFXQuoteResponse
	if err := c.postJSON(ctx, cfg, "/v1/customer/quotes", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetQuote retrieves an existing quote by ID.
// GET /v1/customer/quotes/{quoteId}
func (c *Client) GetQuote(ctx context.Context, cfg *XFXClientConfig, quoteID string) (*XFXQuoteResponse, error) {
	var resp XFXQuoteResponse
	if err := c.getJSON(ctx, cfg, "/v1/customer/quotes/"+quoteID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteQuote executes an active quote, creating a transaction.
// POST /v1/customer/quotes/{quoteId}/execute
func (c *Client) ExecuteQuote(ctx context.Context, cfg *XFXClientConfig, quoteID string) (*XFXExecuteResponse, error) {
	var resp XFXExecuteResponse
	if err := c.postJSON(ctx, cfg, "/v1/customer/quotes/"+quoteID+"/execute", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetTransaction retrieves a transaction by ID.
// GET /v1/customer/transactions/{transactionId}
func (c *Client) GetTransaction(ctx context.Context, cfg *XFXClientConfig, txID string) (*XFXTransactionResponse, error) {
	var resp XFXTransactionResponse
	if err := c.getJSON(ctx, cfg, "/v1/customer/transactions/"+txID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// getJSON performs an authenticated GET request and decodes the JSON response.
func (c *Client) getJSON(ctx context.Context, cfg *XFXClientConfig, path string, out any) error {
	token, err := c.tokens.GetToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("xfx: get auth token: %w", err)
	}

	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	setHeaders(req, token)

	return c.exec.DoJSON(ctx, req, cfg.ClientID, out)
}

// postJSON performs an authenticated POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, cfg *XFXClientConfig, path string, body any, out any) error {
	token, err := c.tokens.GetToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("xfx: get auth token: %w", err)
	}

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
	setHeaders(req, token)

	return c.exec.DoJSON(ctx, req, cfg.ClientID, out)
}

// setHeaders sets required headers for XFX API requests.
func setHeaders(req *http.Request, bearerToken string) {
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
