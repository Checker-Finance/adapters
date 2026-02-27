package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── Mocks ────────────────────────────────────────────────────────────────────

type mockOrderResolverService struct {
	resolveTransactionFn func(ctx context.Context, clientID, txID string) (*model.TradeConfirmation, error)
}

func (m *mockOrderResolverService) ResolveTransaction(ctx context.Context, clientID, txID string) (*model.TradeConfirmation, error) {
	if m.resolveTransactionFn != nil {
		return m.resolveTransactionFn(ctx, clientID, txID)
	}
	return nil, fmt.Errorf("not implemented")
}

type mockTradeSync struct {
	syncFn func(ctx context.Context, trade *model.TradeConfirmation) error
}

func (m *mockTradeSync) SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error {
	if m.syncFn != nil {
		return m.syncFn(ctx, trade)
	}
	return nil
}

// mockStore embeds store.Store to satisfy the interface; only methods used by
// OrderResolveHandler are overridden.
type mockStore struct {
	store.Store
	getQuoteByQuoteIDFn func(ctx context.Context, quoteID string) (*model.QuoteRecord, error)
	getOrderIDByRFQFn   func(ctx context.Context, rfqID string) (string, error)
}

func (m *mockStore) GetQuoteByQuoteID(ctx context.Context, quoteID string) (*model.QuoteRecord, error) {
	return m.getQuoteByQuoteIDFn(ctx, quoteID)
}

func (m *mockStore) GetOrderIDByRFQ(ctx context.Context, rfqID string) (string, error) {
	return m.getOrderIDByRFQFn(ctx, rfqID)
}

// ─── Test helper ──────────────────────────────────────────────────────────────

func newResolveApp(svc OrderResolverService, st store.Store, ts TradeSync) *fiber.App {
	app := fiber.New()
	handler := NewOrderResolveHandler(zap.NewNop(), svc, st, ts)
	v1 := app.Group("/api/v1")
	v1.Post("/resolve-order/:quoteId", handler.ResolveOrder)
	return app
}

func postResolve(app *fiber.App, quoteID string) (*http.Response, error) {
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/resolve-order/"+quoteID, strings.NewReader(""))
	return app.Test(req, -1)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestResolveOrder_Success(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, quoteID string) (*model.QuoteRecord, error) {
			assert.Equal(t, "q-001", quoteID)
			return &model.QuoteRecord{
				QuoteID:  "q-001",
				RFQID:    "rfq-001",
				ClientID: "client-001",
			}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, rfqID string) (string, error) {
			assert.Equal(t, "rfq-001", rfqID)
			return "tx-abc", nil
		},
	}
	svc := &mockOrderResolverService{
		resolveTransactionFn: func(_ context.Context, clientID, txID string) (*model.TradeConfirmation, error) {
			assert.Equal(t, "client-001", clientID)
			assert.Equal(t, "tx-abc", txID)
			return &model.TradeConfirmation{
				TradeID:    "tx-abc",
				ClientID:   "client-001",
				Status:     "filled",
				Price:      17.45,
				ExecutedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	ts := &mockTradeSync{}

	app := newResolveApp(svc, st, ts)
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]any
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))

	assert.Equal(t, "q-001", result["quoteId"])
	assert.Equal(t, "rfq-001", result["rfqId"])
	assert.Equal(t, "tx-abc", result["txId"])
	assert.Equal(t, "filled", result["status"])
	assert.Equal(t, true, result["synced"])
}

func TestResolveOrder_QuoteNotFound(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return nil, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			t.Fatal("should not reach GetOrderIDByRFQ")
			return "", nil
		},
	}

	app := newResolveApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := postResolve(app, "unknown-quote")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "quote not found")
}

func TestResolveOrder_TxIDNotYetKnown(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{QuoteID: "q-001", RFQID: "rfq-001", ClientID: "client-001"}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "", nil // not yet known
		},
	}

	app := newResolveApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "transaction ID not yet known")
}

func TestResolveOrder_StoreGetQuoteError(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return nil, fmt.Errorf("redis timeout")
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			t.Fatal("should not reach GetOrderIDByRFQ")
			return "", nil
		},
	}

	app := newResolveApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "redis timeout")
}

func TestResolveOrder_StoreGetOrderIDError(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{QuoteID: "q-001", RFQID: "rfq-001", ClientID: "client-001"}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("postgres connection refused")
		},
	}

	app := newResolveApp(&mockOrderResolverService{}, st, &mockTradeSync{})
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "postgres connection refused")
}

func TestResolveOrder_ServiceFetchError(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{QuoteID: "q-001", RFQID: "rfq-001", ClientID: "client-001"}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "tx-abc", nil
		},
	}
	svc := &mockOrderResolverService{
		resolveTransactionFn: func(_ context.Context, _, _ string) (*model.TradeConfirmation, error) {
			return nil, fmt.Errorf("XFX API 503")
		},
	}

	app := newResolveApp(svc, st, &mockTradeSync{})
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "failed to fetch status from XFX")
}

func TestResolveOrder_SyncError_ReturnsSyncFalse(t *testing.T) {
	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{QuoteID: "q-001", RFQID: "rfq-001", ClientID: "client-001"}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "tx-abc", nil
		},
	}
	svc := &mockOrderResolverService{
		resolveTransactionFn: func(_ context.Context, _, _ string) (*model.TradeConfirmation, error) {
			return &model.TradeConfirmation{TradeID: "tx-abc", Status: "filled"}, nil
		},
	}
	ts := &mockTradeSync{
		syncFn: func(_ context.Context, _ *model.TradeConfirmation) error {
			return fmt.Errorf("legacy DB unreachable")
		},
	}

	app := newResolveApp(svc, st, ts)
	resp, err := postResolve(app, "q-001")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	var result map[string]any
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))

	assert.Equal(t, "q-001", result["quoteId"])
	assert.Equal(t, "tx-abc", result["txId"])
	assert.Equal(t, "filled", result["status"])
	assert.Equal(t, false, result["synced"])
	assert.Contains(t, result["sync_error"], "legacy DB unreachable")
}

func TestResolveOrder_PassesCorrectClientAndTxIDToService(t *testing.T) {
	var gotClientID, gotTxID string

	st := &mockStore{
		getQuoteByQuoteIDFn: func(_ context.Context, _ string) (*model.QuoteRecord, error) {
			return &model.QuoteRecord{QuoteID: "q-555", RFQID: "rfq-555", ClientID: "tenant-xyz"}, nil
		},
		getOrderIDByRFQFn: func(_ context.Context, _ string) (string, error) {
			return "tx-999", nil
		},
	}
	svc := &mockOrderResolverService{
		resolveTransactionFn: func(_ context.Context, clientID, txID string) (*model.TradeConfirmation, error) {
			gotClientID = clientID
			gotTxID = txID
			return &model.TradeConfirmation{TradeID: "tx-999", Status: "pending"}, nil
		},
	}

	app := newResolveApp(svc, st, &mockTradeSync{})
	resp, err := postResolve(app, "q-555")
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "tenant-xyz", gotClientID)
	assert.Equal(t, "tx-999", gotTxID)
}
