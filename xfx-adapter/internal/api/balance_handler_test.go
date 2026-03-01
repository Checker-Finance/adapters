package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── mock store ───────────────────────────────────────────────────────────────

type mockBalanceStore struct {
	store.Store
	balances []model.Balance
	err      error
}

func (m *mockBalanceStore) GetClientBalances(_ context.Context, _ string) ([]model.Balance, error) {
	return m.balances, m.err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newBalanceApp(st store.Store) *fiber.App {
	app := fiber.New()
	h := NewBalanceHandler(zap.NewNop(), st)
	app.Get("/api/v1/balances/:client_id", h.GetBalances)
	return app
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestGetBalances_Success(t *testing.T) {
	balances := []model.Balance{
		{ClientID: "c1", Venue: "XFX", Instrument: "USD", Available: 1000},
		{ClientID: "c1", Venue: "XFX", Instrument: "MXN", Available: 50000},
	}
	app := newBalanceApp(&mockBalanceStore{balances: balances})

	req := httptest.NewRequest("GET", "/api/v1/balances/c1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var got []model.Balance
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Len(t, got, 2)
	assert.Equal(t, "USD", got[0].Instrument)
	assert.Equal(t, "MXN", got[1].Instrument)
}

func TestGetBalances_EmptyList(t *testing.T) {
	app := newBalanceApp(&mockBalanceStore{balances: []model.Balance{}})

	req := httptest.NewRequest("GET", "/api/v1/balances/c1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var got []model.Balance
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Empty(t, got)
}

func TestGetBalances_StoreError(t *testing.T) {
	app := newBalanceApp(&mockBalanceStore{err: fmt.Errorf("db error")})

	req := httptest.NewRequest("GET", "/api/v1/balances/c1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "db error")
}

func TestGetBalances_NilList(t *testing.T) {
	app := newBalanceApp(&mockBalanceStore{balances: nil})

	req := httptest.NewRequest("GET", "/api/v1/balances/c1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
