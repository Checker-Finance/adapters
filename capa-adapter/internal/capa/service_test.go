package capa

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Checker-Finance/adapters/capa-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("test writeJSON: " + err.Error())
	}
}

// mockCapaServer sets up a minimal HTTP server for Capa API.
// Pass nil for responses to trigger 4xx errors on that endpoint.
func mockCapaServer(
	t *testing.T,
	quoteResp *CapaQuoteResponse,
	execResp *CapaExecuteResponse,
	txResp *CapaTransactionResponse,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.Contains(path, "quotes"):
			if quoteResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, CapaErrorResponse{Message: "invalid request"})
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, quoteResp)

		case r.Method == http.MethodPost && (strings.Contains(path, "cross-ramp") ||
			strings.Contains(path, "on-ramp") || strings.Contains(path, "off-ramp")):
			if execResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, CapaErrorResponse{Message: "quote not found"})
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, execResp)

		case r.Method == http.MethodGet && strings.Contains(path, "transactions"):
			if txResp == nil {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, CapaErrorResponse{Message: "transaction not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, txResp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

type mockConfigResolver struct {
	cfg     *CapaClientConfig
	err     error
	clients []string
}

func (m *mockConfigResolver) Resolve(_ context.Context, _ string) (*CapaClientConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

func (m *mockConfigResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return m.clients, m.err
}

func newTestService(t *testing.T, serverURL string) *Service {
	t.Helper()
	client := NewClient(nil)
	resolver := &mockConfigResolver{
		cfg: &CapaClientConfig{
			APIKey:  "test-api-key",
			BaseURL: serverURL,
			UserID:  "user-001",
		},
		clients: []string{"client-001"},
	}
	return &Service{
		ctx:            context.Background(),
		cfg:            config.Config{},
		client:         client,
		configResolver: resolver,
		publisher:      nil,
		store:          nil,
		mapper:         NewMapper(),
	}
}

// ─── CreateRFQ ─────────────────────────────────────────────────────────────────

func TestService_CreateRFQ_CrossRamp_Success(t *testing.T) {
	expiresAt := time.Now().Add(30 * time.Second).Format(time.RFC3339)
	quoteResp := &CapaQuoteResponse{
		ID:                  "capa-qt-001",
		SourceCurrency:      "USD",
		DestinationCurrency: "MXN",
		SourceAmount:        100000,
		DestinationAmount:   1700000,
		ExchangeRate:        17.0,
		ExpiresAt:           expiresAt,
		Status:              "ACTIVE",
	}

	server := mockCapaServer(t, quoteResp, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100000,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "capa-qt-001", quote.ID)
	assert.InDelta(t, 17.0, quote.Price, 0.001)
	assert.Equal(t, "CAPA", quote.Venue)
}

func TestService_CreateRFQ_ConfigError(t *testing.T) {
	svc := &Service{
		ctx:    context.Background(),
		mapper: NewMapper(),
		configResolver: &mockConfigResolver{
			err: errors.New("secret not found"),
		},
	}

	req := model.RFQRequest{
		ClientID:     "unknown-client",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100000,
	}

	_, err := svc.CreateRFQ(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve client config")
}

func TestService_CreateRFQ_VenueError(t *testing.T) {
	// nil quoteResp → server returns 400
	server := mockCapaServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100000,
	}

	_, err := svc.CreateRFQ(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capa quote creation failed")
}

// ─── ExecuteRFQ ────────────────────────────────────────────────────────────────

func TestService_ExecuteRFQ_CrossRamp_Terminal(t *testing.T) {
	execResp := &CapaExecuteResponse{
		ID:      "exec-001",
		QuoteID: "capa-qt-001",
		Transaction: CapaTransaction{
			ID:      "tx-001",
			QuoteID: "capa-qt-001",
			Status:  "COMPLETED",
		},
	}

	server := mockCapaServer(t, nil, execResp, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	trade, err := svc.ExecuteRFQ(context.Background(), "client-001", "capa-qt-001")
	require.NoError(t, err)
	require.NotNil(t, trade)
	assert.Equal(t, "tx-001", trade.TradeID)
	assert.Equal(t, model.StatusFilled, trade.Status, "COMPLETED should normalize to filled")
	assert.Equal(t, "CAPA", trade.Venue)
}

func TestService_ExecuteRFQ_NonTerminal_StartsPoller(t *testing.T) {
	execResp := &CapaExecuteResponse{
		ID:      "exec-002",
		QuoteID: "capa-qt-002",
		Transaction: CapaTransaction{
			ID:      "tx-002",
			QuoteID: "capa-qt-002",
			Status:  "IN_PROGRESS",
		},
	}

	server := mockCapaServer(t, nil, execResp, nil)
	defer server.Close()

	serviceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := newTestService(t, server.URL)
	svc.ctx = serviceCtx

	// Wire in a real poller so we can verify it starts
	poller := NewPoller(config.Config{}, svc, nil, nil, 10*time.Millisecond, nil)
	svc.SetPoller(poller)

	trade, err := svc.ExecuteRFQ(context.Background(), "client-001", "capa-qt-002")
	require.NoError(t, err)
	require.NotNil(t, trade)
	assert.Equal(t, model.StatusPending, trade.Status, "IN_PROGRESS should normalize to pending")

	// Give poller goroutine time to start
	time.Sleep(5 * time.Millisecond)
	_, active := poller.activeTrades.Load("tx-002")
	assert.True(t, active, "poller should be tracking tx-002")

	poller.Stop()
}

func TestService_ExecuteRFQ_ConfigError(t *testing.T) {
	svc := &Service{
		ctx:    context.Background(),
		mapper: NewMapper(),
		configResolver: &mockConfigResolver{
			err: errors.New("config unavailable"),
		},
	}

	_, err := svc.ExecuteRFQ(context.Background(), "unknown-client", "qt-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve client config")
}

func TestService_ExecuteRFQ_VenueError(t *testing.T) {
	// nil execResp → server returns 400
	server := mockCapaServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	_, err := svc.ExecuteRFQ(context.Background(), "client-001", "expired-qt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capa quote execution failed")
}

// ─── FetchTransactionStatus ───────────────────────────────────────────────────

func TestService_FetchTransactionStatus_Success(t *testing.T) {
	txResp := &CapaTransactionResponse{
		Transaction: CapaTransaction{
			ID:      "tx-fetch-001",
			QuoteID: "qt-001",
			Status:  "COMPLETED",
		},
	}

	server := mockCapaServer(t, nil, nil, txResp)
	defer server.Close()

	svc := newTestService(t, server.URL)

	tx, err := svc.FetchTransactionStatus(context.Background(), "client-001", "tx-fetch-001")
	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, "tx-fetch-001", tx.ID)
	assert.Equal(t, "COMPLETED", tx.Status)
}

func TestService_FetchTransactionStatus_NotFound(t *testing.T) {
	// nil txResp → 404
	server := mockCapaServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	tx, err := svc.FetchTransactionStatus(context.Background(), "client-001", "nonexistent")
	assert.Nil(t, tx)
	require.Error(t, err)
}

// ─── syncTerminalTrade ────────────────────────────────────────────────────────

func TestService_SyncTerminalTrade_NilPublisher(t *testing.T) {
	svc := &Service{
		publisher:       nil,
		tradeSyncWriter: nil,
	}

	trade := &model.TradeConfirmation{
		TradeID:  "tx-nil-pub",
		ClientID: "client-001",
		Status:   model.StatusFilled,
	}

	// Should not panic
	svc.syncTerminalTrade(context.Background(), trade)
}
