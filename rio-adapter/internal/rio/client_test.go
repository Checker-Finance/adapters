package rio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/httpclient"
)

// testClientConfig returns a RioClientConfig pointing at the given server URL.
func testClientConfig(serverURL string) *RioClientConfig {
	return &RioClientConfig{
		BaseURL: serverURL,
		APIKey:  "test-api-key",
		Country: "US",
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server, *RioClientConfig) {
	server := httptest.NewServer(handler)
	logger := zap.NewNop()
	client := NewClient(logger, nil)
	return client, server, testClientConfig(server.URL)
}

func TestClient_CreateQuote(t *testing.T) {
	expectedResp := &RioQuoteResponse{
		ID:           "quote-123",
		Crypto:       "USDC",
		Fiat:         "USD",
		Side:         "buy",
		AmountFiat:   1000.0,
		AmountCrypto: 999.5,
		NetPrice:     1.0005,
		MarketPrice:  1.0003,
		ExpiresAt:    "2024-01-15T10:30:00Z",
	}

	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/quotes", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify body
		var req RioQuoteRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "USDC", req.Crypto)
		assert.Equal(t, "USD", req.Fiat)
		assert.Equal(t, "buy", req.Side)

		// Send response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedResp)
	})
	defer server.Close()

	req := &RioQuoteRequest{
		Crypto:     "USDC",
		Fiat:       "USD",
		Side:       "buy",
		Country:    "US",
		AmountFiat: 1000.0,
	}

	resp, err := client.CreateQuote(context.Background(), cfg, req)

	require.NoError(t, err)
	assert.Equal(t, expectedResp.ID, resp.ID)
	assert.Equal(t, expectedResp.NetPrice, resp.NetPrice)
	assert.Equal(t, expectedResp.AmountCrypto, resp.AmountCrypto)
}

func TestClient_CreateOrder(t *testing.T) {
	expectedResp := &RioOrderResponse{
		ID:      "order-456",
		QuoteID: "quote-123",
		Status:  "processing",
		Side:    "buy",
		Crypto:  "USDC",
		Fiat:    "USD",
	}

	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/orders", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		var req RioOrderRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "quote-123", req.QuoteID)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expectedResp)
	})
	defer server.Close()

	req := &RioOrderRequest{
		QuoteID:           "quote-123",
		ClientReferenceID: "client-ref",
	}

	resp, err := client.CreateOrder(context.Background(), cfg, req)

	require.NoError(t, err)
	assert.Equal(t, expectedResp.ID, resp.ID)
	assert.Equal(t, expectedResp.Status, resp.Status)
}

func TestClient_GetOrder(t *testing.T) {
	expectedResp := &RioOrderResponse{
		ID:      "order-456",
		QuoteID: "quote-123",
		Status:  "completed",
		Side:    "buy",
		Crypto:  "USDC",
		Fiat:    "USD",
	}

	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/orders/order-456", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expectedResp)
	})
	defer server.Close()

	resp, err := client.GetOrder(context.Background(), cfg, "order-456")

	require.NoError(t, err)
	assert.Equal(t, "completed", resp.Status)
}

func TestClient_RegisterWebhook(t *testing.T) {
	expectedResp := &RioWebhookRegistrationResponse{
		ID:   "webhook-123",
		URL:  "https://example.com/webhook",
		Type: "orders",
	}

	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/webhooks/orders", r.URL.Path)

		var req RioWebhookRegistration
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/webhook", req.URL)
		assert.True(t, req.RetryOnFailure)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expectedResp)
	})
	defer server.Close()

	resp, err := client.RegisterWebhook(context.Background(), cfg, "https://example.com/webhook", true)

	require.NoError(t, err)
	assert.Equal(t, "webhook-123", resp.ID)
	assert.Equal(t, "orders", resp.Type)
}

func TestClient_ErrorResponse(t *testing.T) {
	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(RioErrorResponse{
			Error:   "validation_error",
			Message: "Invalid crypto currency",
		})
	})
	defer server.Close()

	req := &RioQuoteRequest{
		Crypto: "INVALID",
		Fiat:   "USD",
		Side:   "buy",
	}

	resp, err := client.CreateQuote(context.Background(), cfg, req)

	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "Invalid crypto currency")
}

func TestClient_ServerError_Retry(t *testing.T) {
	attempts := 0

	client, server, cfg := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Success on third attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&RioQuoteResponse{
			ID: "quote-123",
		})
	})
	defer server.Close()

	req := &RioQuoteRequest{
		Crypto:     "USDC",
		Fiat:       "USD",
		Side:       "buy",
		AmountFiat: 100,
	}

	resp, err := client.CreateQuote(context.Background(), cfg, req)

	require.NoError(t, err)
	assert.Equal(t, "quote-123", resp.ID)
	assert.Equal(t, 3, attempts)
}

func TestBackoff(t *testing.T) {
	assert.Equal(t, 100*1e6, float64(httpclient.Backoff(0)))  // 100ms
	assert.Equal(t, 250*1e6, float64(httpclient.Backoff(1)))  // 250ms
	assert.Equal(t, 500*1e6, float64(httpclient.Backoff(2)))  // 500ms
	assert.Equal(t, 500*1e6, float64(httpclient.Backoff(10))) // 500ms (max)
}
