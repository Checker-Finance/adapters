package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
)

// ─── mock store ───────────────────────────────────────────────────────────────

type mockStore struct {
	jsonStore   map[string][]byte
	quoteRecord *model.QuoteRecord
	quoteErr    error
}

func newMockStore() *mockStore {
	return &mockStore{jsonStore: make(map[string][]byte)}
}

func (m *mockStore) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.jsonStore[key] = b
	return nil
}

func (m *mockStore) GetJSON(_ context.Context, key string, dest any) error {
	data, ok := m.jsonStore[key]
	if !ok {
		return errors.New("key not found")
	}
	return json.Unmarshal(data, dest)
}

func (m *mockStore) GetQuoteByQuoteID(_ context.Context, _ string) (*model.QuoteRecord, error) {
	return m.quoteRecord, m.quoteErr
}

// Satisfy the rest of store.Store interface (unused in these tests).
func (m *mockStore) RecordBalanceEvent(_ context.Context, _ model.Balance) error { return nil }
func (m *mockStore) UpdateBalanceSnapshot(_ context.Context, _ model.Balance) error { return nil }
func (m *mockStore) GetBalance(_ context.Context, _, _, _, _ string) (*model.Balance, error) {
	return nil, nil
}
func (m *mockStore) GetClientBalances(_ context.Context, _ string) ([]model.Balance, error) {
	return nil, nil
}
func (m *mockStore) StoreProduct(_ context.Context, _ model.Product) error { return nil }
func (m *mockStore) ListProducts(_ context.Context, _ string) ([]model.Product, error) {
	return nil, nil
}
func (m *mockStore) GetOrderIDByRFQ(_ context.Context, _ string) (string, error) { return "", nil }
func (m *mockStore) HealthCheck(_ context.Context) error                          { return nil }
func (m *mockStore) Close() error                                                  { return nil }

// ─── mock mapper ──────────────────────────────────────────────────────────────

type mockWebhookMapper struct {
	tx    *zodia.ZodiaTransaction
	trade *model.TradeConfirmation
}

func (m *mockWebhookMapper) WebhookToTransaction(event *zodia.ZodiaWebhookEvent) *zodia.ZodiaTransaction {
	if event == nil {
		return nil
	}
	if m.tx != nil {
		return m.tx
	}
	return &zodia.ZodiaTransaction{
		TradeID: event.TradeID,
		State:   event.State,
	}
}

func (m *mockWebhookMapper) MapTransactionToTrade(tx *zodia.ZodiaTransaction, clientID string) *model.TradeConfirmation {
	if tx == nil {
		return nil
	}
	if m.trade != nil {
		return m.trade
	}
	return &model.TradeConfirmation{
		TradeID:  tx.TradeID,
		ClientID: clientID,
		Status:   "filled",
	}
}

// ─── mock trade sync ──────────────────────────────────────────────────────────

type mockTradeSync struct {
	called int
	err    error
}

func (m *mockTradeSync) SyncTradeUpsert(_ context.Context, _ *model.TradeConfirmation) error {
	m.called++
	return m.err
}

// ─── test helpers ─────────────────────────────────────────────────────────────

func newWebhookTestApp(st *mockStore, mapper WebhookMapper, ts *mockTradeSync) *fiber.App {
	h := NewWebhookHandler(st, mapper, ts, nil) // nil publisher
	app := fiber.New()
	app.Post("/webhooks/zodia/transactions", h.Handle)
	return app
}

func postWebhook(t *testing.T, app *fiber.App, event zodia.ZodiaWebhookEvent) *http.Response {
	t.Helper()
	b, _ := json.Marshal(event)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/zodia/transactions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	return resp
}

// ─── WebhookHandler.Handle ────────────────────────────────────────────────────

func TestWebhookHandler_IgnoredType(t *testing.T) {
	st := newMockStore()
	app := newWebhookTestApp(st, &mockWebhookMapper{}, &mockTradeSync{})

	event := zodia.ZodiaWebhookEvent{
		UUID:  "uuid-1",
		Type:  "DEPOSIT",
		State: "PROCESSED",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ignored", body["status"])
	assert.Equal(t, "type_not_applicable", body["reason"])
}

func TestWebhookHandler_NonTerminalState(t *testing.T) {
	app := newWebhookTestApp(newMockStore(), &mockWebhookMapper{}, &mockTradeSync{})
	event := zodia.ZodiaWebhookEvent{
		UUID:  "uuid-2",
		Type:  "OTCTRADE",
		State: "PENDING",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "acknowledged", body["status"])
	assert.Equal(t, "non_terminal", body["reason"])
}

func TestWebhookHandler_DuplicateUUID(t *testing.T) {
	st := newMockStore()
	// Pre-mark this UUID as processed
	_ = st.SetJSON(context.Background(), "zodia:webhook:dedup:uuid-dup", true, time.Hour)

	app := newWebhookTestApp(st, &mockWebhookMapper{}, &mockTradeSync{})
	event := zodia.ZodiaWebhookEvent{
		UUID:    "uuid-dup",
		Type:    "OTCTRADE",
		State:   "PROCESSED",
		TradeID: "trade-dup",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "duplicate", body["status"])
}

func TestWebhookHandler_ClientNotFound(t *testing.T) {
	st := newMockStore()
	st.quoteErr = errors.New("not found") // GetQuoteByQuoteID returns error

	app := newWebhookTestApp(st, &mockWebhookMapper{}, &mockTradeSync{})
	event := zodia.ZodiaWebhookEvent{
		UUID:    "uuid-3",
		Type:    "RFSTRADE",
		State:   "PROCESSED",
		TradeID: "trade-unknown",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ignored", body["status"])
	assert.Equal(t, "client_not_found", body["reason"])
}

func TestWebhookHandler_ProcessedOTCTrade(t *testing.T) {
	st := newMockStore()
	st.quoteRecord = &model.QuoteRecord{ClientID: "client-wh"}

	ts := &mockTradeSync{}
	mapper := &mockWebhookMapper{
		trade: &model.TradeConfirmation{
			TradeID:  "trade-ok",
			ClientID: "client-wh",
			Status:   "filled",
		},
	}
	app := newWebhookTestApp(st, mapper, ts)

	event := zodia.ZodiaWebhookEvent{
		UUID:    "uuid-ok",
		Type:    "OTCTRADE",
		State:   "PROCESSED",
		TradeID: "trade-ok",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "processed", body["status"])
	assert.Equal(t, 1, ts.called, "SyncTradeUpsert should be called once")
}

func TestWebhookHandler_ProcessedRFSTrade(t *testing.T) {
	st := newMockStore()
	st.quoteRecord = &model.QuoteRecord{ClientID: "client-rfs"}

	ts := &mockTradeSync{}
	app := newWebhookTestApp(st, &mockWebhookMapper{}, ts)

	event := zodia.ZodiaWebhookEvent{
		UUID:    "uuid-rfs",
		Type:    "RFSTRADE",
		State:   "PROCESSED",
		TradeID: "trade-rfs-1",
	}
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "processed", body["status"])
}

func TestWebhookHandler_SyncError_StillReturns200(t *testing.T) {
	st := newMockStore()
	st.quoteRecord = &model.QuoteRecord{ClientID: "c"}

	ts := &mockTradeSync{err: errors.New("db error")}
	app := newWebhookTestApp(st, &mockWebhookMapper{}, ts)

	event := zodia.ZodiaWebhookEvent{
		UUID: "uuid-sync-err", Type: "OTCTRADE", State: "PROCESSED", TradeID: "t1",
	}
	// Even if sync fails, we return 200 to prevent retries
	resp := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWebhookHandler_InvalidBody(t *testing.T) {
	app := newWebhookTestApp(newMockStore(), &mockWebhookMapper{}, &mockTradeSync{})
	req := httptest.NewRequest(http.MethodPost, "/webhooks/zodia/transactions",
		bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	// Returns 200 to prevent retries on parse failures
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ignored", body["status"])
}

func TestWebhookHandler_Idempotency_SecondCallSkipped(t *testing.T) {
	st := newMockStore()
	st.quoteRecord = &model.QuoteRecord{ClientID: "client-idem"}

	ts := &mockTradeSync{}
	app := newWebhookTestApp(st, &mockWebhookMapper{}, ts)

	event := zodia.ZodiaWebhookEvent{
		UUID:    "uuid-idem",
		Type:    "OTCTRADE",
		State:   "PROCESSED",
		TradeID: "trade-idem",
	}

	// First call processes it
	resp1 := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Equal(t, 1, ts.called)

	// Second call should be a duplicate
	resp2 := postWebhook(t, app, event)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, 1, ts.called, "SyncTradeUpsert should not be called again")

	var body2 map[string]string
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&body2))
	assert.Equal(t, "duplicate", body2["status"])
}
