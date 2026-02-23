package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// --- Mocks ---

// mockOrderResolverService implements OrderResolverService for testing.
type mockOrderResolverService struct {
	fetchTradeStatusFn             func(ctx context.Context, clientID, orderID string) (*rio.RioOrderResponse, error)
	buildTradeConfirmationFromOrder func(clientID, orderID string, order *rio.RioOrderResponse) *model.TradeConfirmation
}

func (m *mockOrderResolverService) FetchTradeStatus(ctx context.Context, clientID, orderID string) (*rio.RioOrderResponse, error) {
	if m.fetchTradeStatusFn != nil {
		return m.fetchTradeStatusFn(ctx, clientID, orderID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockOrderResolverService) BuildTradeConfirmationFromOrder(clientID, orderID string, order *rio.RioOrderResponse) *model.TradeConfirmation {
	if m.buildTradeConfirmationFromOrder != nil {
		return m.buildTradeConfirmationFromOrder(clientID, orderID, order)
	}
	return nil
}

// mockTradeSync implements TradeSync for testing.
type mockTradeSync struct {
	syncTradeUpsertFn func(ctx context.Context, trade *model.TradeConfirmation) error
}

func (m *mockTradeSync) SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error {
	if m.syncTradeUpsertFn != nil {
		return m.syncTradeUpsertFn(ctx, trade)
	}
	return nil
}

// mockResolveStore implements store.Store with only the methods needed for OrderResolveHandler.
type mockResolveStore struct {
	getQuoteByQuoteIDFn func(ctx context.Context, quoteID string) (*model.QuoteRecord, error)
	getOrderIDByRFQFn   func(ctx context.Context, rfqID string) (string, error)
}

func (m *mockResolveStore) GetQuoteByQuoteID(ctx context.Context, quoteID string) (*model.QuoteRecord, error) {
	if m.getQuoteByQuoteIDFn != nil {
		return m.getQuoteByQuoteIDFn(ctx, quoteID)
	}
	return nil, nil
}

func (m *mockResolveStore) GetOrderIDByRFQ(ctx context.Context, rfqID string) (string, error) {
	if m.getOrderIDByRFQFn != nil {
		return m.getOrderIDByRFQFn(ctx, rfqID)
	}
	return "", nil
}

// Unused Store interface methods — stubs to satisfy the interface.
func (m *mockResolveStore) RecordBalanceEvent(context.Context, model.Balance) error         { return nil }
func (m *mockResolveStore) UpdateBalanceSnapshot(context.Context, model.Balance) error      { return nil }
func (m *mockResolveStore) GetBalance(context.Context, string, string, string, string) (*model.Balance, error) {
	return nil, nil
}
func (m *mockResolveStore) GetClientBalances(context.Context, string) ([]model.Balance, error) {
	return nil, nil
}
func (m *mockResolveStore) SetJSON(context.Context, string, any, time.Duration) error { return nil }
func (m *mockResolveStore) GetJSON(context.Context, string, any) error                { return nil }
func (m *mockResolveStore) StoreProduct(context.Context, model.Product) error          { return nil }
func (m *mockResolveStore) ListProducts(context.Context, string) ([]model.Product, error) {
	return nil, nil
}
func (m *mockResolveStore) HealthCheck(context.Context) error { return nil }
func (m *mockResolveStore) Close() error                      { return nil }

// Compile-time check that mockResolveStore satisfies the Store interface.
var _ store.Store = (*mockResolveStore)(nil)

// --- Test Helpers ---

func newResolveTestApp(
	svc OrderResolverService,
	st store.Store,
	ts TradeSync,
) *fiber.App {
	app := fiber.New()
	h := &OrderResolveHandler{
		Logger:    zap.NewNop(),
		Service:   svc,
		Store:     st,
		TradeSync: ts,
	}
	app.Post("/api/v1/resolve-order/:quoteId", h.ResolveOrder)
	return app
}

func doResolve(app *fiber.App, quoteID string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/resolve-order/"+quoteID, nil)
	return app.Test(req, -1)
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// --- Tests ---

func TestResolveOrder_Success(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, quoteID string) (*model.QuoteRecord, error) {
			assert.Equal(t, "qt-001", quoteID)
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-ord-001",
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, rfqID string) (string, error) {
			assert.Equal(t, "rfq-001", rfqID)
			return "ord-001", nil
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, clientID, orderID string) (*rio.RioOrderResponse, error) {
			assert.Equal(t, "client-001", clientID)
			assert.Equal(t, "rio-ord-001", orderID)
			return &rio.RioOrderResponse{
				ID:        "rio-ord-001",
				QuoteID:   "rio-qt-001",
				Status:    "completed",
				Side:      "buy",
				Crypto:    "USDC",
				Fiat:      "BRL",
				NetPrice:  5.0,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		buildTradeConfirmationFromOrder: func(clientID, orderID string, order *rio.RioOrderResponse) *model.TradeConfirmation {
			return &model.TradeConfirmation{
				TradeID:    orderID,
				ClientID:   clientID,
				Status:     "filled",
				Instrument: "USDC/BRL",
				Side:       "BUY",
				Price:      5.0,
				Venue:      "RIO",
			}
		},
	}

	var syncedTrade *model.TradeConfirmation
	ts := &mockTradeSync{
		syncTradeUpsertFn: func(_ context.Context, trade *model.TradeConfirmation) error {
			syncedTrade = trade
			return nil
		},
	}

	app := newResolveTestApp(svc, st, ts)
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "qt-001", result["quoteId"])
	assert.Equal(t, "rfq-001", result["rfqId"])
	assert.Equal(t, "ord-001", result["orderId"])
	assert.Equal(t, "filled", result["status"])
	assert.Equal(t, true, result["synced"])

	// Verify trade was synced
	require.NotNil(t, syncedTrade)
	assert.Equal(t, "ord-001", syncedTrade.TradeID)
	assert.Equal(t, "client-001", syncedTrade.ClientID)
}

func TestResolveOrder_QuoteNotFound(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return nil, nil // not found
		},
	}

	app := newResolveTestApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := doResolve(app, "nonexistent-qt")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "quote not found", result["error"])
}

func TestResolveOrder_QuoteLookupError(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}

	app := newResolveTestApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Contains(t, result["error"], "db connection lost")
}

func TestResolveOrder_OrderIDNotYetKnown(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID: "qt-001",
				RFQID:   "rfq-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "", nil // not yet known
		},
	}

	app := newResolveTestApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "orderId not yet known for RFQ", result["error"])
}

func TestResolveOrder_OrderIDLookupError(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID: "qt-001",
				RFQID:   "rfq-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("rfq table unreachable")
		},
	}

	app := newResolveTestApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Contains(t, result["error"], "rfq table unreachable")
}

func TestResolveOrder_FetchTradeStatusError(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-ord-001",
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "ord-001", nil
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, _, _ string) (*rio.RioOrderResponse, error) {
			return nil, fmt.Errorf("rio API timeout")
		},
	}

	app := newResolveTestApp(svc, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "failed to fetch status from Rio", result["error"])
}

func TestResolveOrder_BuildTradeConfirmationReturnsNil(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-ord-001",
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "ord-001", nil
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, _, _ string) (*rio.RioOrderResponse, error) {
			return &rio.RioOrderResponse{
				ID:     "rio-ord-001",
				Status: "completed",
			}, nil
		},
		buildTradeConfirmationFromOrder: func(_, _ string, _ *rio.RioOrderResponse) *model.TradeConfirmation {
			return nil // e.g. mapping failure
		},
	}

	app := newResolveTestApp(svc, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "could not build trade confirmation", result["error"])
}

func TestResolveOrder_SyncError_ReturnsSyncedFalse(t *testing.T) {
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-ord-001",
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "ord-001", nil
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, _, _ string) (*rio.RioOrderResponse, error) {
			return &rio.RioOrderResponse{
				ID:        "rio-ord-001",
				Status:    "paid",
				Side:      "buy",
				Crypto:    "USDC",
				Fiat:      "BRL",
				NetPrice:  5.0,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		buildTradeConfirmationFromOrder: func(clientID, orderID string, _ *rio.RioOrderResponse) *model.TradeConfirmation {
			return &model.TradeConfirmation{
				TradeID:  orderID,
				ClientID: clientID,
				Status:   "filled",
			}
		},
	}

	ts := &mockTradeSync{
		syncTradeUpsertFn: func(_ context.Context, _ *model.TradeConfirmation) error {
			return fmt.Errorf("legacy db write failed")
		},
	}

	app := newResolveTestApp(svc, st, ts)
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "qt-001", result["quoteId"])
	assert.Equal(t, "rfq-001", result["rfqId"])
	assert.Equal(t, "ord-001", result["orderId"])
	assert.Equal(t, "filled", result["status"])
	assert.Equal(t, false, result["synced"])
	assert.Contains(t, result["sync_error"], "legacy db write failed")
}

func TestResolveOrder_StatusNormalization(t *testing.T) {
	// Verify that the raw Rio status is properly normalized in the response.
	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-ord-001",
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "ord-001", nil
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, _, _ string) (*rio.RioOrderResponse, error) {
			return &rio.RioOrderResponse{
				ID:        "rio-ord-001",
				Status:    "processing", // raw Rio status
				Side:      "buy",
				Crypto:    "USDC",
				Fiat:      "BRL",
				NetPrice:  5.0,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		buildTradeConfirmationFromOrder: func(clientID, orderID string, _ *rio.RioOrderResponse) *model.TradeConfirmation {
			return &model.TradeConfirmation{
				TradeID:  orderID,
				ClientID: clientID,
				Status:   "submitted",
			}
		},
	}

	app := newResolveTestApp(svc, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	result := readJSON(t, resp)
	assert.Equal(t, "submitted", result["status"]) // "processing" → "submitted"
	assert.Equal(t, true, result["synced"])
}

func TestResolveOrder_PassesCorrectProviderOrderID(t *testing.T) {
	// Verify that FetchTradeStatus receives the ProviderOrderID from the quote record,
	// not the internal orderID from the RFQ lookup.
	var receivedOrderID string

	st := &mockResolveStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{
				QuoteID:         "qt-001",
				RFQID:           "rfq-001",
				ProviderOrderID: "rio-provider-order-999", // <-- this should be used
				ClientID:        "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "internal-order-123", nil // <-- this should NOT be passed to FetchTradeStatus
		},
	}

	svc := &mockOrderResolverService{
		fetchTradeStatusFn: func(_ context.Context, _, orderID string) (*rio.RioOrderResponse, error) {
			receivedOrderID = orderID
			return &rio.RioOrderResponse{
				ID:        "rio-provider-order-999",
				Status:    "completed",
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		buildTradeConfirmationFromOrder: func(clientID, orderID string, _ *rio.RioOrderResponse) *model.TradeConfirmation {
			return &model.TradeConfirmation{
				TradeID:  orderID,
				ClientID: clientID,
				Status:   "filled",
			}
		},
	}

	app := newResolveTestApp(svc, st, &mockTradeSync{})
	resp, err := doResolve(app, "qt-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	assert.Equal(t, "rio-provider-order-999", receivedOrderID,
		"FetchTradeStatus should receive ProviderOrderID, not internal orderID")
}
