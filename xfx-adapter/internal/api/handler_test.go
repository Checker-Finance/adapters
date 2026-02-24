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

	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── Mock service ─────────────────────────────────────────────────────────────

type mockRFQService struct {
	createRFQFn  func(ctx context.Context, req model.RFQRequest) (*model.Quote, error)
	executeRFQFn func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error)
}

func (m *mockRFQService) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	if m.createRFQFn != nil {
		return m.createRFQFn(ctx, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRFQService) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	if m.executeRFQFn != nil {
		return m.executeRFQFn(ctx, clientID, quoteID)
	}
	return nil, fmt.Errorf("not implemented")
}

// ─── Mock validator ───────────────────────────────────────────────────────────

type mockClientValidator struct {
	known map[string]bool
}

func (m *mockClientValidator) IsKnownClient(_ context.Context, clientID string) bool {
	return m.known[clientID]
}

// ─── Test app helpers ─────────────────────────────────────────────────────────

func newTestApp(svc RFQService) *fiber.App {
	app := fiber.New()
	handler := NewXFXHandler(zap.NewNop(), svc, nil)
	v1 := app.Group("/api/v1")
	v1.Post("/quotes", handler.CreateRFQHandler)
	v1.Post("/orders", handler.ExecuteRFQHandler)
	return app
}

func newTestAppWithValidator(svc RFQService, validator ClientValidator) *fiber.App {
	app := fiber.New()
	handler := NewXFXHandler(zap.NewNop(), svc, validator)
	v1 := app.Group("/api/v1")
	v1.Post("/quotes", handler.CreateRFQHandler)
	v1.Post("/orders", handler.ExecuteRFQHandler)
	return app
}

// ─── CreateRFQHandler ─────────────────────────────────────────────────────────

func TestCreateRFQHandler_Success(t *testing.T) {
	svc := &mockRFQService{
		createRFQFn: func(_ context.Context, req model.RFQRequest) (*model.Quote, error) {
			assert.Equal(t, "client-001", req.ClientID)
			assert.Equal(t, "USD/MXN", req.CurrencyPair)
			assert.Equal(t, "buy", req.Side)
			assert.Equal(t, 100000.0, req.Amount)
			return &model.Quote{
				ID:        "xfx-qt-001",
				Price:     17.45,
				ExpiresAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}

	app := newTestApp(svc)
	body := `{
		"quoteId":   "q-001",
		"clientId":  "client-001",
		"pair":      "USD/MXN",
		"orderSide": "buy",
		"quantity":  100000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var result RFQResponse
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))

	assert.Equal(t, "q-001", result.QuoteID)
	assert.Equal(t, "xfx-qt-001", result.ProviderQuoteId)
	assert.Equal(t, 17.45, result.Price)
	assert.Empty(t, result.ErrorMsg)
}

func TestCreateRFQHandler_InvalidJSON(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateRFQHandler_MissingClientID(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	body := `{"pair": "USD/MXN", "orderSide": "buy", "quantity": 100000}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "clientId is required")
}

func TestCreateRFQHandler_MissingPair(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	body := `{"clientId": "client-001", "orderSide": "buy", "quantity": 100000}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "pair is required")
}

func TestCreateRFQHandler_ZeroQuantity(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	body := `{"clientId": "client-001", "pair": "USD/MXN", "orderSide": "buy", "quantity": 0}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "quantity must be positive")
}

func TestCreateRFQHandler_ServiceError(t *testing.T) {
	svc := &mockRFQService{
		createRFQFn: func(_ context.Context, _ model.RFQRequest) (*model.Quote, error) {
			return nil, fmt.Errorf("XFX API timeout")
		},
	}
	app := newTestApp(svc)

	body := `{"quoteId": "q-err", "clientId": "client-001", "pair": "USD/MXN", "orderSide": "buy", "quantity": 100000}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result RFQResponse
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Equal(t, "q-err", result.QuoteID)
	assert.Contains(t, result.ErrorMsg, "XFX API timeout")
}

func TestCreateRFQHandler_UnknownClient_Forbidden(t *testing.T) {
	svc := &mockRFQService{
		createRFQFn: func(_ context.Context, _ model.RFQRequest) (*model.Quote, error) {
			t.Fatal("service should not be called for unknown client")
			return nil, nil
		},
	}
	validator := &mockClientValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{"clientId": "unknown-client", "pair": "USD/MXN", "orderSide": "buy", "quantity": 100000}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "unknown or unauthorized clientId")
}

func TestCreateRFQHandler_KnownClient_Allowed(t *testing.T) {
	svc := &mockRFQService{
		createRFQFn: func(_ context.Context, _ model.RFQRequest) (*model.Quote, error) {
			return &model.Quote{ID: "qt-ok", Price: 17.5, ExpiresAt: time.Now().Add(15 * time.Second)}, nil
		},
	}
	validator := &mockClientValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{"clientId": "client-001", "pair": "USD/MXN", "orderSide": "buy", "quantity": 100000}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
}

// ─── ExecuteRFQHandler ────────────────────────────────────────────────────────

func TestExecuteRFQHandler_Success_BodyQuoteID(t *testing.T) {
	var receivedQuoteID string
	svc := &mockRFQService{
		executeRFQFn: func(_ context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
			receivedQuoteID = quoteID
			assert.Equal(t, "client-001", clientID)
			return &model.TradeConfirmation{
				TradeID:    "tx-001",
				Status:     "pending",
				Price:      17.45,
				ExecutedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	app := newTestApp(svc)

	body := `{"clientId": "client-001", "orderId": "my-order-001", "quoteId": "qt-from-body"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "qt-from-body", receivedQuoteID)

	var result RFQExecutionResponse
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Equal(t, "my-order-001", result.OrderID)
	assert.Equal(t, "tx-001", result.ProviderOrderID)
	assert.Equal(t, "pending", result.Status)
	assert.Equal(t, 17.45, result.Price)
	assert.Empty(t, result.ErrorMsg)
}

func TestExecuteRFQHandler_Success_ProviderQuoteID_Fallback(t *testing.T) {
	var receivedQuoteID string
	svc := &mockRFQService{
		executeRFQFn: func(_ context.Context, _, quoteID string) (*model.TradeConfirmation, error) {
			receivedQuoteID = quoteID
			return &model.TradeConfirmation{TradeID: "tx-002", Status: "pending", ExecutedAt: time.Now()}, nil
		},
	}
	app := newTestApp(svc)

	// quoteId is empty, providerQuoteId is set → should fall back to providerQuoteId
	body := `{"clientId": "client-001", "providerQuoteId": "xfx-qt-prov-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "xfx-qt-prov-001", receivedQuoteID)
}

func TestExecuteRFQHandler_MissingQuoteID_AllSources(t *testing.T) {
	svc := &mockRFQService{}
	app := newTestApp(svc)

	// No quoteId, no providerQuoteId — handler should return 400
	body := `{"clientId": "client-001", "orderId": "my-order"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "quoteId is required")
}

func TestExecuteRFQHandler_MissingClientID(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	body := `{"quoteId": "qt-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Contains(t, result["error"], "clientId is required")
}

func TestExecuteRFQHandler_UnknownClient_Forbidden(t *testing.T) {
	svc := &mockRFQService{
		executeRFQFn: func(_ context.Context, _, _ string) (*model.TradeConfirmation, error) {
			t.Fatal("service should not be called for unknown client")
			return nil, nil
		},
	}
	validator := &mockClientValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{"clientId": "intruder", "quoteId": "qt-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestExecuteRFQHandler_ServiceError(t *testing.T) {
	svc := &mockRFQService{
		executeRFQFn: func(_ context.Context, _, _ string) (*model.TradeConfirmation, error) {
			return nil, fmt.Errorf("quote already executed")
		},
	}
	app := newTestApp(svc)

	body := `{"clientId": "client-001", "orderId": "ord-err", "quoteId": "qt-expired"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result RFQExecutionResponse
	raw, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(raw, &result))
	assert.Equal(t, "ord-err", result.OrderID)
	assert.Contains(t, result.ErrorMsg, "quote already executed")
}

func TestExecuteRFQHandler_InvalidJSON(t *testing.T) {
	app := newTestApp(&mockRFQService{})

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// ─── toRFQRequest ─────────────────────────────────────────────────────────────

func TestToRFQRequest(t *testing.T) {
	req := RFQCreateRequest{
		ID:                 "q-001",
		ClientID:           "client-001",
		CurrencyPair:       "USD/MXN",
		AmountDenomination: "USD",
		Side:               "buy",
		Amount:             100000,
	}

	result := toRFQRequest(req)

	assert.Equal(t, "client-001", result.ClientID)
	assert.Equal(t, "buy", result.Side)
	assert.Equal(t, "USD/MXN", result.CurrencyPair)
	assert.Equal(t, 100000.0, result.Amount)
	assert.Equal(t, "USD", result.CurrencyAmount)
}
