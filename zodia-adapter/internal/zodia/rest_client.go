package zodia

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
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
)

// RESTClient wraps low-level HMAC-signed HTTP communication with the Zodia REST API.
// Configuration (base URL, credentials) is supplied per-request so a single
// RESTClient instance can serve multiple tenants.
type RESTClient struct {
	logger *zap.Logger
	exec   *httpclient.Executor
	signer *HMACSigner
}

// NewRESTClient constructs a new Zodia REST client.
func NewRESTClient(logger *zap.Logger, rateMgr *rate.Manager, signer *HMACSigner) *RESTClient {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	exec := httpclient.New(logger, rateMgr, httpClient, 2, "zodia", func(status int, body []byte) error {
		var errResp ZodiaErrorResponse
		_ = json.Unmarshal(body, &errResp)

		msg := errResp.Error
		if msg == "" {
			msg = errResp.Message
		}
		if msg == "" {
			msg = string(body)
		}

		logger.Warn("zodia.rest_client_error",
			zap.Int("status", status),
			zap.String("message", msg),
			zap.String("body", string(body)))

		return fmt.Errorf("zodia returned %d: %s", status, msg)
	})
	return &RESTClient{
		logger: logger,
		exec:   exec,
		signer: signer,
	}
}

// GetAccounts fetches account balances for a client.
// POST /api/3/account (HMAC-signed)
func (c *RESTClient) GetAccounts(ctx context.Context, cfg *ZodiaClientConfig) (*ZodiaAccountResponse, error) {
	const endpoint, method = "/api/3/account", http.MethodPost
	start := time.Now()
	var resp ZodiaAccountResponse
	err := c.postSigned(ctx, cfg, endpoint, ZodiaAccountRequest{Tonce: Tonce()}, &resp)
	metrics.IncZodiaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.ZodiaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetInstruments fetches available trading instruments.
// GET /zm/rest/available-instruments
// ⚠️ Auth requirements unknown — attempts unauthenticated first; add signing if needed.
func (c *RESTClient) GetInstruments(ctx context.Context, cfg *ZodiaClientConfig) (*ZodiaInstrumentsResponse, error) {
	const endpoint, method = "/zm/rest/available-instruments", http.MethodGet
	start := time.Now()
	var resp ZodiaInstrumentsResponse
	err := c.getJSON(ctx, cfg, endpoint, &resp)
	metrics.IncZodiaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.ZodiaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListTransactions fetches transactions matching the given filter.
// POST /api/3/transaction/list (HMAC-signed)
func (c *RESTClient) ListTransactions(ctx context.Context, cfg *ZodiaClientConfig, filter ZodiaTransactionFilter) (*ZodiaTransactionListResponse, error) {
	const endpoint, method = "/api/3/transaction/list", http.MethodPost
	start := time.Now()
	filter.Tonce = Tonce()
	var resp ZodiaTransactionListResponse
	err := c.postSigned(ctx, cfg, endpoint, filter, &resp)
	metrics.IncZodiaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.ZodiaRequestDuration, start, endpoint, method)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetWSAuthToken obtains a WebSocket auth token via the REST API.
// POST /ws/auth (HMAC-signed)
func (c *RESTClient) GetWSAuthToken(ctx context.Context, cfg *ZodiaClientConfig) (string, error) {
	const endpoint, method = "/ws/auth", http.MethodPost
	start := time.Now()
	var resp ZodiaWSAuthResponse
	err := c.postSigned(ctx, cfg, endpoint, ZodiaWSAuthRequest{Tonce: Tonce()}, &resp)
	metrics.IncZodiaRequest(endpoint, method, statusLabel(err))
	metrics.ObserveDuration(metrics.ZodiaRequestDuration, start, endpoint, method)
	if err != nil {
		return "", err
	}
	if resp.Token == "" {
		return "", fmt.Errorf("zodia: /ws/auth returned empty token")
	}
	return resp.Token, nil
}

// postSigned marshals body to JSON, signs it with HMAC-SHA512, and POSTs to path.
// Sets Rest-Key and Rest-Sign headers per Zodia auth requirements.
func (c *RESTClient) postSigned(ctx context.Context, cfg *ZodiaClientConfig, path string, body any, out any) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("zodia: marshal request body: %w", err)
	}

	signature := c.signer.Sign(bodyBytes, cfg.APISecret)

	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	setSignedHeaders(req, cfg.APIKey, signature)

	return c.exec.DoJSON(ctx, req, cfg.APIKey, out)
}

// getJSON performs an unauthenticated GET request and decodes the JSON response.
func (c *RESTClient) getJSON(ctx context.Context, cfg *ZodiaClientConfig, path string, out any) error {
	url := cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.exec.DoJSON(ctx, req, cfg.APIKey, out)
}

// setSignedHeaders sets the HMAC authentication headers for Zodia REST requests.
func setSignedHeaders(req *http.Request, apiKey, signature string) {
	req.Header.Set("Rest-Key", apiKey)
	req.Header.Set("Rest-Sign", signature)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
