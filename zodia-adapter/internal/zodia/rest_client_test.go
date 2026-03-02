package zodia

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// helper: build a RESTClient pointing at the given test server URL via cfg.BaseURL.
func newTestRESTClient(t *testing.T) *RESTClient {
	t.Helper()
	return NewRESTClient(zap.NewNop(), nil, NewHMACSigner())
}

func testCfgFor(serverURL string) *ZodiaClientConfig {
	return &ZodiaClientConfig{
		APIKey:    "test-api-key",
		APISecret: "test-api-secret",
		BaseURL:   serverURL,
	}
}

// verifyHMACHeaders asserts that the canonical HMAC auth headers are present.
func verifyHMACHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	assert.NotEmpty(t, r.Header.Get("Rest-Key"), "Rest-Key header must be set")
	assert.NotEmpty(t, r.Header.Get("Rest-Sign"), "Rest-Sign header must be set")
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
}

// ─── GetAccounts ─────────────────────────────────────────────────────────────

func TestRESTClient_GetAccounts_Success(t *testing.T) {
	resp := ZodiaAccountResponse{
		Result: map[string]ZodiaAccountBalance{
			"USD": {Available: "100000.00", Orders: "0"},
		},
	}
	body, _ := json.Marshal(resp)

	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/3/account", r.URL.Path)
		verifyHMACHeaders(t, r)
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	cfg := testCfgFor(srv.URL)
	result, err := client.GetAccounts(t.Context(), cfg)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Result, "USD")
	assert.Equal(t, "100000.00", result.Result["USD"].Available)

	// Verify the request body contained a tonce
	var req ZodiaAccountRequest
	require.NoError(t, json.Unmarshal(capturedBody, &req))
	assert.Greater(t, req.Tonce, int64(0), "tonce must be positive")
}

func TestRESTClient_GetAccounts_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	_, err := client.GetAccounts(t.Context(), testCfgFor(srv.URL))
	assert.Error(t, err)
}

// ─── GetInstruments ──────────────────────────────────────────────────────────

func TestRESTClient_GetInstruments_Success(t *testing.T) {
	resp := ZodiaInstrumentsResponse{
		Instruments: []ZodiaInstrument{
			{Symbol: "USD.MXN", Base: "USD", Quote: "MXN", Status: "active", MinSize: 100_000},
			{Symbol: "BTC.USDC", Base: "BTC", Quote: "USDC", Status: "active"},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/zm/rest/available-instruments", r.URL.Path)
		// GET should NOT have HMAC headers (unauthenticated)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	result, err := client.GetInstruments(t.Context(), testCfgFor(srv.URL))

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Instruments, 2)
	assert.Equal(t, "USD.MXN", result.Instruments[0].Symbol)
}

// ─── ListTransactions ─────────────────────────────────────────────────────────

func TestRESTClient_ListTransactions_Success(t *testing.T) {
	resp := ZodiaTransactionListResponse{
		Result: []ZodiaTransaction{
			{
				TradeID:    "trade-123",
				State:      "PROCESSED",
				Instrument: "USD.MXN",
				Side:       "BUY",
				Quantity:   100_000,
				Price:      17.25,
			},
		},
	}
	body, _ := json.Marshal(resp)

	var capturedFilter ZodiaTransactionFilter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/3/transaction/list", r.URL.Path)
		verifyHMACHeaders(t, r)

		rawBody, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(rawBody, &capturedFilter)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	filter := ZodiaTransactionFilter{TradeID: "trade-123", Type: "OTCTRADE"}
	result, err := client.ListTransactions(t.Context(), testCfgFor(srv.URL), filter)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Result, 1)
	assert.Equal(t, "trade-123", result.Result[0].TradeID)

	// Verify tonce was injected
	assert.Greater(t, capturedFilter.Tonce, int64(0), "tonce must be injected into filter")
	assert.Equal(t, "trade-123", capturedFilter.TradeID)
}

// ─── GetWSAuthToken ───────────────────────────────────────────────────────────

func TestRESTClient_GetWSAuthToken_Success(t *testing.T) {
	resp := ZodiaWSAuthResponse{Token: "ws-token-abc"}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/ws/auth", r.URL.Path)
		verifyHMACHeaders(t, r)

		rawBody, _ := io.ReadAll(r.Body)
		var req ZodiaWSAuthRequest
		_ = json.Unmarshal(rawBody, &req)
		assert.Greater(t, req.Tonce, int64(0))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	token, err := client.GetWSAuthToken(t.Context(), testCfgFor(srv.URL))

	require.NoError(t, err)
	assert.Equal(t, "ws-token-abc", token)
}

func TestRESTClient_GetWSAuthToken_EmptyToken(t *testing.T) {
	resp := ZodiaWSAuthResponse{Token: ""}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	_, err := client.GetWSAuthToken(t.Context(), testCfgFor(srv.URL))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty token")
}

func TestRESTClient_GetWSAuthToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	client := newTestRESTClient(t)
	_, err := client.GetWSAuthToken(t.Context(), testCfgFor(srv.URL))
	assert.Error(t, err)
}

// ─── HMAC header values ───────────────────────────────────────────────────────

func TestRESTClient_HMACHeaders_CorrectValues(t *testing.T) {
	signer := NewHMACSigner()
	var capturedKey, capturedSign string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("Rest-Key")
		capturedSign = r.Header.Get("Rest-Sign")
		body, _ := io.ReadAll(r.Body)

		// Recompute expected signature
		expectedSig := signer.Sign(body, "test-api-secret")
		assert.Equal(t, expectedSig, capturedSign, "signature must match HMAC(body, secret)")
		assert.Equal(t, "test-api-key", capturedKey)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{}}`))
	}))
	defer srv.Close()

	client := NewRESTClient(zap.NewNop(), nil, signer)
	_, _ = client.GetAccounts(t.Context(), testCfgFor(srv.URL))
	assert.Equal(t, "test-api-key", capturedKey)
	assert.NotEmpty(t, capturedSign)
}
