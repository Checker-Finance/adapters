package rio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// --- Mock ConfigResolver ---

type mockConfigResolver struct {
	cfg     *RioClientConfig
	err     error
	clients []string
}

func (m *mockConfigResolver) Resolve(_ context.Context, _ string) (*RioClientConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

func (m *mockConfigResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return m.clients, m.err
}

// --- Test Helpers ---

// mockRioServer creates a test HTTP server that responds with canned Rio API responses.
func mockRioServer(t *testing.T, quoteResp *RioQuoteResponse, orderResp *RioOrderResponse, orderGetResp *RioOrderResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/quotes":
			if quoteResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, RioErrorResponse{Error: "bad_request", Message: "invalid quote"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, quoteResp)

		case r.Method == http.MethodPost && r.URL.Path == "/api/orders":
			if orderResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, RioErrorResponse{Error: "bad_request", Message: "invalid order"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, orderResp)

		case r.Method == http.MethodGet && len(r.URL.Path) > len("/api/orders/"):
			if orderGetResp == nil {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, RioErrorResponse{Error: "not_found", Message: "order not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, orderGetResp)

		case r.Method == http.MethodPost && r.URL.Path == "/api/webhooks/orders":
			w.WriteHeader(http.StatusOK)
			writeJSON(w, RioWebhookRegistrationResponse{
				ID:   "wh-123",
				URL:  "https://example.com/webhook",
				Type: "orders",
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newTestService(t *testing.T, serverURL string) *Service {
	t.Helper()
	logger := zap.NewNop()
	client := NewClient(logger, nil)

	resolver := &mockConfigResolver{
		cfg: &RioClientConfig{
			BaseURL: serverURL,
			APIKey:  "test-api-key",
			Country: "US",
		},
		clients: []string{"client-001"},
	}

	return &Service{
		ctx:            context.Background(),
		cfg:            config.Config{},
		logger:         logger,
		client:         client,
		configResolver: resolver,
		publisher:      nil, // syncTerminalTrade safely handles nil publisher
		mapper:         NewMapper(),
	}
}

// --- Tests ---

func TestService_CreateRFQ_Success(t *testing.T) {
	quoteResp := &RioQuoteResponse{
		ID:           "qt-abc-123",
		Crypto:       "USDC",
		Fiat:         "BRL",
		Side:         "buy",
		Country:      "US",
		AmountFiat:   5000,
		AmountCrypto: 1000,
		NetPrice:     5.0,
		MarketPrice:  4.98,
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	server := mockRioServer(t, quoteResp, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "client-001",
		Side:         "buy",
		CurrencyPair: "USDC/BRL",
		Amount:       5000,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "qt-abc-123", quote.ID)
	assert.Equal(t, 5.0, quote.Price, "Price should be AmountFiat/AmountCrypto = 5000/1000")
	assert.Equal(t, "USDC/BRL", quote.Instrument)
	assert.Equal(t, "BUY", quote.Side)
	assert.Equal(t, "client-001", quote.TakerID)
	assert.Equal(t, "RIO", quote.Venue)
}

func TestService_CreateRFQ_SellSide(t *testing.T) {
	quoteResp := &RioQuoteResponse{
		ID:           "qt-sell-001",
		Crypto:       "BTC",
		Fiat:         "USD",
		Side:         "sell",
		Country:      "US",
		AmountCrypto: 0.5,
		NetPrice:     42000.0,
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	server := mockRioServer(t, quoteResp, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "client-001",
		Side:         "sell",
		CurrencyPair: "BTC/USD",
		Amount:       0.5,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "qt-sell-001", quote.ID)
	assert.Equal(t, "SELL", quote.Side)
	assert.Equal(t, "BTC/USD", quote.Instrument)
}

func TestService_CreateRFQ_ClientError(t *testing.T) {
	// nil quoteResp causes mock server to return 400
	server := mockRioServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "client-001",
		Side:         "buy",
		CurrencyPair: "USDC/BRL",
		Amount:       5000,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	assert.Nil(t, quote)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rio quote creation failed")
}

func TestService_ExecuteRFQ_Success_TerminalStatus(t *testing.T) {
	orderResp := &RioOrderResponse{
		ID:           "ord-xyz-789",
		QuoteID:      "qt-abc-123",
		Status:       "completed",
		Side:         "buy",
		Crypto:       "USDC",
		Fiat:         "BRL",
		AmountFiat:   5000,
		AmountCrypto: 1000,
		NetPrice:     5.0,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	server := mockRioServer(t, nil, orderResp, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	trade, err := svc.ExecuteRFQ(context.Background(), "client-001", "qt-abc-123")
	require.NoError(t, err)
	assert.Equal(t, "ord-xyz-789", trade.TradeID)
	assert.Equal(t, "filled", trade.Status) // "completed" normalizes to "filled"
	assert.Equal(t, "client-001", trade.ClientID)
	assert.Equal(t, "USDC/BRL", trade.Instrument)
	assert.Equal(t, "BUY", trade.Side)
}

func TestService_ExecuteRFQ_Success_NonTerminal_NoPoller(t *testing.T) {
	orderResp := &RioOrderResponse{
		ID:        "ord-xyz-789",
		QuoteID:   "qt-abc-123",
		Status:    "processing",
		Side:      "buy",
		Crypto:    "USDC",
		Fiat:      "BRL",
		NetPrice:  5.0,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	server := mockRioServer(t, nil, orderResp, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)
	// poller is nil — verify no panic

	trade, err := svc.ExecuteRFQ(context.Background(), "client-001", "qt-abc-123")
	require.NoError(t, err)
	assert.Equal(t, "submitted", trade.Status) // "processing" normalizes to "submitted"
}

func TestService_ExecuteRFQ_ClientError(t *testing.T) {
	// nil orderResp causes mock server to return 400
	server := mockRioServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	trade, err := svc.ExecuteRFQ(context.Background(), "client-001", "qt-abc-123")
	assert.Nil(t, trade)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rio order creation failed")
}

func TestService_FetchTradeStatus_Success(t *testing.T) {
	getResp := &RioOrderResponse{
		ID:     "ord-xyz-789",
		Status: "paid",
		Side:   "buy",
		Crypto: "USDC",
		Fiat:   "BRL",
	}

	server := mockRioServer(t, nil, nil, getResp)
	defer server.Close()

	svc := newTestService(t, server.URL)

	order, err := svc.FetchTradeStatus(context.Background(), "client-001", "ord-xyz-789")
	require.NoError(t, err)
	assert.Equal(t, "paid", order.Status)
	assert.Equal(t, "ord-xyz-789", order.ID)
}

func TestService_FetchTradeStatus_NotFound(t *testing.T) {
	server := mockRioServer(t, nil, nil, nil) // nil orderGetResp returns 404
	defer server.Close()

	svc := newTestService(t, server.URL)

	order, err := svc.FetchTradeStatus(context.Background(), "client-001", "nonexistent")
	assert.Nil(t, order)
	assert.Error(t, err)
}

func TestService_BuildTradeConfirmationFromOrder_Success(t *testing.T) {
	svc := &Service{
		logger: zap.NewNop(),
		mapper: NewMapper(),
	}

	order := &RioOrderResponse{
		ID:           "ord-123",
		QuoteID:      "qt-456",
		Status:       "completed",
		Side:         "sell",
		Crypto:       "BTC",
		Fiat:         "USD",
		AmountFiat:   21000.0,
		AmountCrypto: 0.5,
		NetPrice:     42000.0,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	trade := svc.BuildTradeConfirmationFromOrder("client-001", "ord-123", order)
	require.NotNil(t, trade)
	assert.Equal(t, "ord-123", trade.TradeID)
	assert.Equal(t, "client-001", trade.ClientID)
	assert.Equal(t, "filled", trade.Status)
	assert.Equal(t, "BTC/USD", trade.Instrument)
	assert.Equal(t, "SELL", trade.Side)
	assert.Equal(t, 42000.0, trade.Price, "Price should be AmountFiat/AmountCrypto = 21000/0.5")
	assert.Equal(t, "RIO", trade.Venue)
}

func TestService_BuildTradeConfirmationFromOrder_NilOrder(t *testing.T) {
	svc := &Service{
		logger: zap.NewNop(),
		mapper: NewMapper(),
	}

	trade := svc.BuildTradeConfirmationFromOrder("client-001", "ord-123", nil)
	assert.Nil(t, trade)
}

func TestService_RegisterOrderWebhook_Success(t *testing.T) {
	server := mockRioServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	err := svc.RegisterOrderWebhook(context.Background(), "https://example.com/webhook")
	require.NoError(t, err)
}

func TestService_ExecuteRFQ_UsesServiceContext(t *testing.T) {
	// Verify that the poller receives the service-level context (s.ctx),
	// not the HTTP request context, so polling survives after response.
	orderResp := &RioOrderResponse{
		ID:        "ord-poll-001",
		QuoteID:   "qt-poll-001",
		Status:    "processing", // non-terminal triggers poller
		Side:      "buy",
		Crypto:    "USDC",
		Fiat:      "BRL",
		NetPrice:  5.0,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// For the poller's FetchTradeStatus call, return terminal status
	terminalResp := &RioOrderResponse{
		ID:        "ord-poll-001",
		QuoteID:   "qt-poll-001",
		Status:    "completed",
		Side:      "buy",
		Crypto:    "USDC",
		Fiat:      "BRL",
		NetPrice:  5.0,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	server := mockRioServer(t, nil, orderResp, terminalResp)
	defer server.Close()

	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	defer serviceCancel()

	svc := newTestService(t, server.URL)
	svc.ctx = serviceCtx

	poller := NewPoller(zap.NewNop(), config.Config{}, svc, nil, nil, 10*time.Millisecond, nil)
	svc.SetPoller(poller)

	// Use a request context that we cancel immediately after
	reqCtx, reqCancel := context.WithCancel(context.Background())

	trade, err := svc.ExecuteRFQ(reqCtx, "client-001", "qt-poll-001")
	require.NoError(t, err)
	assert.Equal(t, "submitted", trade.Status)

	// Cancel the request context — poller should still be running
	// because it uses serviceCtx, not reqCtx
	reqCancel()
	time.Sleep(5 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-poll-001"),
		"poller should still be active after request context cancellation")

	// Wait for terminal status to be reached
	time.Sleep(50 * time.Millisecond)
	assert.False(t, poller.IsPolling("ord-poll-001"),
		"poller should stop after reaching terminal status")

	poller.Stop()
}

func TestService_SyncTerminalTrade_NilPublisher(t *testing.T) {
	svc := &Service{
		logger:    zap.NewNop(),
		publisher: nil, // nil publisher should not panic
	}

	trade := &model.TradeConfirmation{
		TradeID:  "ord-001",
		ClientID: "client-001",
		Status:   "filled",
	}

	// Should not panic
	svc.syncTerminalTrade(context.Background(), trade)
}

func TestService_SyncTerminalTrade_NilTradeSyncWriter(t *testing.T) {
	svc := &Service{
		logger:          zap.NewNop(),
		publisher:       nil,
		tradeSyncWriter: nil, // nil writer should not panic
	}

	trade := &model.TradeConfirmation{
		TradeID:  "ord-001",
		ClientID: "client-001",
		Status:   "filled",
	}

	// Should not panic
	svc.syncTerminalTrade(context.Background(), trade)
}
