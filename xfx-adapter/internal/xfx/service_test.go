package xfx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
)

// ─── CreateRFQ ────────────────────────────────────────────────────────────────

func TestService_CreateRFQ_Success(t *testing.T) {
	quoteResp := &XFXQuoteResponse{
		Success: true,
		Quote: XFXQuote{
			ID:         "xfx-qt-abc",
			Symbol:     "USD/MXN",
			Side:       "BUY",
			Quantity:   100000.0,
			Price:      17.45,
			ValidUntil: time.Now().Add(15 * time.Second).UTC().Format(time.RFC3339),
			Status:     "ACTIVE",
		},
	}

	server := newMockXFXServer(t, quoteResp, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "test-client-id",
		Side:         "buy",
		CurrencyPair: "USD/MXN",
		Amount:       100000,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "xfx-qt-abc", quote.ID)
	assert.Equal(t, 17.45, quote.Price)
	assert.Equal(t, "USD/MXN", quote.Instrument)
	assert.Equal(t, "BUY", quote.Side)
	assert.Equal(t, "test-client-id", quote.TakerID)
	assert.Equal(t, "XFX", quote.Venue)
	assert.Equal(t, "MXN", quote.Currency)
}

func TestService_CreateRFQ_SellSide(t *testing.T) {
	quoteResp := &XFXQuoteResponse{
		Success: true,
		Quote: XFXQuote{
			ID:         "xfx-qt-sell",
			Symbol:     "USDT/COP",
			Side:       "SELL",
			Quantity:   50000.0,
			Price:      4250.0,
			ValidUntil: time.Now().Add(15 * time.Second).UTC().Format(time.RFC3339),
			Status:     "ACTIVE",
		},
	}

	server := newMockXFXServer(t, quoteResp, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{
		ClientID:     "test-client-id",
		Side:         "sell",
		CurrencyPair: "USDT/COP",
		Amount:       50000,
	}

	quote, err := svc.CreateRFQ(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "SELL", quote.Side)
	assert.Equal(t, "USDT/COP", quote.Instrument)
}

func TestService_CreateRFQ_ResolveConfigError(t *testing.T) {
	svc := &Service{
		ctx:    context.Background(),
		cfg:    config.Config{},
		logger: zap.NewNop(),
		configResolver: &mockConfigResolver{
			err: assert.AnError,
		},
		mapper: NewMapper(),
	}

	req := model.RFQRequest{ClientID: "unknown-client", Side: "buy", CurrencyPair: "USD/MXN", Amount: 100000}
	quote, err := svc.CreateRFQ(context.Background(), req)
	assert.Nil(t, quote)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve client config")
}

func TestService_CreateRFQ_ClientError(t *testing.T) {
	// nil quoteResp makes the mock server return 400
	server := newMockXFXServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	req := model.RFQRequest{ClientID: "test-client-id", Side: "buy", CurrencyPair: "USD/MXN", Amount: 100000}

	quote, err := svc.CreateRFQ(context.Background(), req)
	assert.Nil(t, quote)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xfx quote creation failed")
}

// ─── ExecuteRFQ ───────────────────────────────────────────────────────────────

func TestService_ExecuteRFQ_Success_TerminalStatus(t *testing.T) {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	settledAt := time.Now().Add(1 * time.Second).UTC().Format(time.RFC3339)

	execResp := &XFXExecuteResponse{
		Success: true,
		Transaction: XFXTransaction{
			ID:        "tx-settled-001",
			QuoteID:   "xfx-qt-abc",
			Symbol:    "USD/MXN",
			Side:      "buy",
			Quantity:  100000.0,
			Price:     17.45,
			Status:    "SETTLED", // terminal → syncTerminalTrade called, not poller
			CreatedAt: createdAt,
			SettledAt: settledAt,
		},
	}

	server := newMockXFXServer(t, nil, execResp, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	trade, err := svc.ExecuteRFQ(context.Background(), "test-client-id", "xfx-qt-abc")
	require.NoError(t, err)
	assert.Equal(t, "tx-settled-001", trade.TradeID)
	assert.Equal(t, "filled", trade.Status)
	assert.Equal(t, "test-client-id", trade.ClientID)
	assert.Equal(t, "USD/MXN", trade.Instrument)
	assert.Equal(t, "XFX", trade.Venue)
}

func TestService_ExecuteRFQ_Success_NonTerminal_NoPoller(t *testing.T) {
	createdAt := time.Now().UTC().Format(time.RFC3339)

	execResp := &XFXExecuteResponse{
		Success: true,
		Transaction: XFXTransaction{
			ID:        "tx-pending-001",
			QuoteID:   "xfx-qt-abc",
			Symbol:    "USD/MXN",
			Side:      "buy",
			Status:    "PENDING", // non-terminal → would normally start polling
			CreatedAt: createdAt,
		},
	}

	server := newMockXFXServer(t, nil, execResp, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)
	// poller is nil — should not panic

	trade, err := svc.ExecuteRFQ(context.Background(), "test-client-id", "xfx-qt-abc")
	require.NoError(t, err)
	assert.Equal(t, "tx-pending-001", trade.TradeID)
	assert.Equal(t, "pending", trade.Status)
}

func TestService_ExecuteRFQ_NonTerminal_StartsPoller(t *testing.T) {
	createdAt := time.Now().UTC().Format(time.RFC3339)

	execResp := &XFXExecuteResponse{
		Success: true,
		Transaction: XFXTransaction{
			ID:        "tx-poll-001",
			QuoteID:   "xfx-qt-poll",
			Symbol:    "USD/MXN",
			Side:      "buy",
			Status:    "PENDING",
			CreatedAt: createdAt,
		},
	}

	// The poller will call FetchTransactionStatus which calls GET /v1/customer/transactions/{id}
	txResp := &XFXTransactionResponse{
		Success: true,
		Transaction: XFXTransaction{
			ID:        "tx-poll-001",
			Status:    "SETTLED",
			CreatedAt: createdAt,
			SettledAt: time.Now().Add(time.Second).UTC().Format(time.RFC3339),
		},
	}

	server := newMockXFXServer(t, nil, execResp, txResp)
	defer server.Close()

	serviceCtx, serviceCancel := context.WithCancel(context.Background())
	defer serviceCancel()

	svc := newTestService(t, server.URL)
	svc.ctx = serviceCtx

	poller := newTestPoller(t, svc, 10*time.Millisecond)
	svc.SetPoller(poller)

	reqCtx, reqCancel := context.WithCancel(context.Background())

	trade, err := svc.ExecuteRFQ(reqCtx, "test-client-id", "xfx-qt-poll")
	require.NoError(t, err)
	assert.Equal(t, "pending", trade.Status)

	// Cancel request context — poller should still run because it uses service context
	reqCancel()
	time.Sleep(5 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-poll-001"),
		"poller should still be active after request context cancellation")

	// Wait for poller to reach terminal status and stop
	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-poll-001")
	}, 300*time.Millisecond, 10*time.Millisecond, "poller should stop after terminal status")

	poller.Stop()
}

func TestService_ExecuteRFQ_ClientError(t *testing.T) {
	// nil execResp makes mock server return 400
	server := newMockXFXServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	trade, err := svc.ExecuteRFQ(context.Background(), "test-client-id", "xfx-qt-abc")
	assert.Nil(t, trade)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xfx quote execution failed")
}

// ─── FetchTransactionStatus ───────────────────────────────────────────────────

func TestService_FetchTransactionStatus_Success(t *testing.T) {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	txResp := &XFXTransactionResponse{
		Success: true,
		Transaction: txAt("tx-fetch-001", "qt-001", "SETTLED", createdAt),
	}

	server := newMockXFXServer(t, nil, nil, txResp)
	defer server.Close()

	svc := newTestService(t, server.URL)

	tx, err := svc.FetchTransactionStatus(context.Background(), "test-client-id", "tx-fetch-001")
	require.NoError(t, err)
	require.NotNil(t, tx)
	assert.Equal(t, "tx-fetch-001", tx.ID)
	assert.Equal(t, "SETTLED", tx.Status)
}

func TestService_FetchTransactionStatus_NotFound(t *testing.T) {
	// nil txResp returns 404 from mock server
	server := newMockXFXServer(t, nil, nil, nil)
	defer server.Close()

	svc := newTestService(t, server.URL)

	tx, err := svc.FetchTransactionStatus(context.Background(), "test-client-id", "nonexistent")
	assert.Nil(t, tx)
	require.Error(t, err)
}

// ─── BuildTradeConfirmationFromTx ─────────────────────────────────────────────

func TestService_BuildTradeConfirmationFromTx_Success(t *testing.T) {
	svc := &Service{
		logger: zap.NewNop(),
		mapper: NewMapper(),
	}

	createdAt := "2025-06-01T10:00:00Z"
	tx := &XFXTransaction{
		ID:        "tx-build-001",
		QuoteID:   "qt-build-001",
		Symbol:    "USDT/MXN",
		Side:      "sell",
		Quantity:  200000.0,
		Price:     17.3,
		Status:    "SETTLED",
		CreatedAt: createdAt,
		SettledAt: "2025-06-01T10:01:00Z",
	}

	trade := svc.BuildTradeConfirmationFromTx("client-build", tx)
	require.NotNil(t, trade)
	assert.Equal(t, "tx-build-001", trade.TradeID)
	assert.Equal(t, "client-build", trade.ClientID)
	assert.Equal(t, "filled", trade.Status)
	assert.Equal(t, "USDT/MXN", trade.Instrument)
	assert.Equal(t, "SELL", trade.Side)
	assert.Equal(t, "XFX", trade.Venue)
}

func TestService_BuildTradeConfirmationFromTx_NilTx(t *testing.T) {
	svc := &Service{logger: zap.NewNop(), mapper: NewMapper()}
	trade := svc.BuildTradeConfirmationFromTx("client-123", nil)
	assert.Nil(t, trade)
}

// ─── syncTerminalTrade: nil-safety ────────────────────────────────────────────

func TestService_SyncTerminalTrade_NilPublisher(t *testing.T) {
	svc := &Service{
		logger:    zap.NewNop(),
		publisher: nil, // nil publisher should not panic
	}

	trade := &model.TradeConfirmation{
		TradeID:  "tx-nil-pub",
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
		TradeID:  "tx-nil-writer",
		ClientID: "client-001",
		Status:   "cancelled",
	}

	// Should not panic
	svc.syncTerminalTrade(context.Background(), trade)
}
