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

// --- Mock Service ---

type mockService struct {
	createRFQFn  func(ctx context.Context, req model.RFQRequest) (*model.Quote, error)
	executeRFQFn func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error)
}

func (m *mockService) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	if m.createRFQFn != nil {
		return m.createRFQFn(ctx, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockService) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	if m.executeRFQFn != nil {
		return m.executeRFQFn(ctx, clientID, quoteID)
	}
	return nil, fmt.Errorf("not implemented")
}

// --- Test Helpers ---

func newTestApp(svc RFQService) *fiber.App {
	app := fiber.New()
	handler := NewRioHandler(zap.NewNop(), svc, nil)
	v1 := app.Group("/api/v1")
	v1.Post("/quotes", handler.CreateRFQHandler)
	v1.Post("/quotes/:quotation_id/execute", handler.ExecuteRFQHandler)
	return app
}

func newTestAppWithValidator(svc RFQService, validator ClientValidator) *fiber.App {
	app := fiber.New()
	handler := NewRioHandler(zap.NewNop(), svc, validator)
	v1 := app.Group("/api/v1")
	v1.Post("/quotes", handler.CreateRFQHandler)
	v1.Post("/quotes/:quotation_id/execute", handler.ExecuteRFQHandler)
	return app
}

// --- CreateRFQHandler Tests ---

func TestCreateRFQHandler_Success(t *testing.T) {
	svc := &mockService{
		createRFQFn: func(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
			return &model.Quote{
				ID:         "rio-qt-001",
				Price:      5.05,
				Instrument: "USDC/BRL",
				Side:       "BUY",
				ExpiresAt:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				Venue:      "RIO",
			}, nil
		},
	}

	app := newTestApp(svc)

	body := `{
		"quoteId": "q-001",
		"clientId": "client-001",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var result RFQResponse
	respBody, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	assert.Equal(t, "q-001", result.QuoteID)
	assert.Equal(t, "rio-qt-001", result.ProviderQuoteId)
	assert.Equal(t, 5.05, result.Price)
	assert.Empty(t, result.ErrorMsg)
}

func TestCreateRFQHandler_InvalidJSON(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestCreateRFQHandler_ValidationError_MissingClientID(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	body := `{
		"quoteId": "q-001",
		"clientId": "",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Contains(t, result["error"], "clientId is required")
}

func TestCreateRFQHandler_ValidationError_InvalidSide(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	body := `{
		"quoteId": "q-001",
		"clientId": "client-001",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "hold",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Contains(t, result["error"], "orderSide must be")
}

func TestCreateRFQHandler_ValidationError_ZeroQuantity(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	body := `{
		"quoteId": "q-001",
		"clientId": "client-001",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 0
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Contains(t, result["error"], "quantity must be greater than 0")
}

func TestCreateRFQHandler_ServiceError(t *testing.T) {
	svc := &mockService{
		createRFQFn: func(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
			return nil, fmt.Errorf("rio API timeout")
		},
	}

	app := newTestApp(svc)

	body := `{
		"quoteId": "q-001",
		"clientId": "client-001",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result RFQResponse
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "q-001", result.QuoteID)
	assert.Contains(t, result.ErrorMsg, "rio API timeout")
}

// --- ExecuteRFQHandler Tests ---

func TestExecuteRFQHandler_Success(t *testing.T) {
	svc := &mockService{
		executeRFQFn: func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
			assert.Equal(t, "client-001", clientID)
			assert.Equal(t, "qt-from-url", quoteID)
			return &model.TradeConfirmation{
				TradeID:    "ord-001",
				Status:     "filled",
				Price:      5.05,
				ExecutedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}

	app := newTestApp(svc)

	body := `{"clientId": "client-001", "orderId": "my-order-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-from-url/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result RFQExecutionResponse
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))

	assert.Equal(t, "my-order-001", result.OrderID)
	assert.Equal(t, "ord-001", result.ProviderOrderID)
	assert.Equal(t, "filled", result.Status)
	assert.Equal(t, 5.05, result.Price)
	assert.Empty(t, result.ErrorMsg)
}

func TestExecuteRFQHandler_URLParamTakesPrecedence(t *testing.T) {
	var receivedQuoteID string
	svc := &mockService{
		executeRFQFn: func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
			receivedQuoteID = quoteID
			return &model.TradeConfirmation{
				TradeID:    "ord-002",
				Status:     "filled",
				ExecutedAt: time.Now(),
			}, nil
		},
	}

	app := newTestApp(svc)

	// URL has qt-from-url, body has qt-from-body — URL should win
	body := `{"clientId": "client-001", "quoteId": "qt-from-body"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-from-url/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "qt-from-url", receivedQuoteID)
}

func TestExecuteRFQHandler_ValidationError(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	// Missing clientId
	body := `{"quoteId": "qt-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-001/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result map[string]string
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Contains(t, result["error"], "clientId is required")
}

func TestExecuteRFQHandler_ServiceError(t *testing.T) {
	svc := &mockService{
		executeRFQFn: func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
			return nil, fmt.Errorf("quote expired")
		},
	}

	app := newTestApp(svc)

	body := `{"clientId": "client-001", "orderId": "ord-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-expired/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var result RFQExecutionResponse
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "ord-001", result.OrderID)
	assert.Contains(t, result.ErrorMsg, "quote expired")
}

func TestExecuteRFQHandler_InvalidJSON(t *testing.T) {
	svc := &mockService{}
	app := newTestApp(svc)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-001/execute", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// --- Client Validation Tests ---

type mockValidator struct {
	known map[string]bool
}

func (m *mockValidator) IsKnownClient(_ context.Context, clientID string) bool {
	return m.known[clientID]
}

func TestCreateRFQHandler_UnknownClient_Forbidden(t *testing.T) {
	svc := &mockService{
		createRFQFn: func(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
			t.Fatal("service should not be called for unknown client")
			return nil, nil
		},
	}
	validator := &mockValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{
		"quoteId": "q-001",
		"clientId": "unknown-client",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)

	var result map[string]string
	respBody, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Contains(t, result["error"], "unknown or unauthorized clientId")
}

func TestCreateRFQHandler_KnownClient_Allowed(t *testing.T) {
	svc := &mockService{
		createRFQFn: func(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
			return &model.Quote{
				ID:        "qt-001",
				Price:     5.0,
				ExpiresAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	}
	validator := &mockValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{
		"quoteId": "q-001",
		"clientId": "client-001",
		"pair": "USDC:BRL",
		"amountDenomination": "BRL",
		"orderSide": "buy",
		"quantity": 5000
	}`

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
}

func TestExecuteRFQHandler_UnknownClient_Forbidden(t *testing.T) {
	svc := &mockService{
		executeRFQFn: func(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
			t.Fatal("service should not be called for unknown client")
			return nil, nil
		},
	}
	validator := &mockValidator{known: map[string]bool{"client-001": true}}
	app := newTestAppWithValidator(svc, validator)

	body := `{"clientId": "unknown-client", "orderId": "ord-001"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/quotes/qt-001/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

// --- toRFQRequest Tests ---

func TestToRFQRequest(t *testing.T) {
	req := RFQCreateRequest{
		ID:                 "q-001",
		ClientID:           "client-001",
		CurrencyPair:       "USDC:BRL",
		AmountDenomination: "BRL",
		Side:               "buy",
		Amount:             5000,
	}

	result := toRFQRequest(req)

	assert.Equal(t, "client-001", result.ClientID)
	assert.Equal(t, "buy", result.Side)
	assert.Equal(t, "USDC:BRL", result.CurrencyPair)
	assert.Equal(t, 5000.0, result.Amount)
	assert.Equal(t, "BRL", result.CurrencyAmount)
}
