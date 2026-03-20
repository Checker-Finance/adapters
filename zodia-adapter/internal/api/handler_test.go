package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── mock RFQService ──────────────────────────────────────────────────────────

type mockRFQService struct {
	quote      *model.Quote
	trade      *model.TradeConfirmation
	createErr  error
	executeErr error
}

func (m *mockRFQService) CreateRFQ(_ context.Context, _ model.RFQRequest) (*model.Quote, error) {
	return m.quote, m.createErr
}

func (m *mockRFQService) ExecuteRFQ(_ context.Context, _, _ string) (*model.TradeConfirmation, error) {
	return m.trade, m.executeErr
}

// ─── mock ClientValidator ─────────────────────────────────────────────────────

type mockValidator struct{ known bool }

func (m *mockValidator) IsKnownClient(_ context.Context, _ string) bool { return m.known }

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestHandler(svc RFQService, known bool) *ZodiaHandler {
	return NewZodiaHandler(svc, &mockValidator{known: known})
}

func newTestApp(svc RFQService, known bool) *fiber.App {
	app := fiber.New()
	h := newTestHandler(svc, known)
	app.Post("/api/v1/quotes", h.CreateRFQHandler)
	app.Post("/api/v1/orders", h.ExecuteRFQHandler)
	return app
}

func doRequest(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 5000)
	require.NoError(t, err)
	return resp
}

// ─── CreateRFQHandler ─────────────────────────────────────────────────────────

func TestCreateRFQHandler_Success(t *testing.T) {
	expiresAt := time.Now().Add(15 * time.Second)
	svc := &mockRFQService{
		quote: &model.Quote{
			ID:        "zodia-q-1",
			Price:     17.25,
			ExpiresAt: expiresAt,
		},
	}
	app := newTestApp(svc, true)

	body := map[string]any{
		"clientId":  "client-A",
		"pair":      "USD:MXN",
		"orderSide": "BUY",
		"quantity":  100_000.0,
	}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var result RFQResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "zodia-q-1", result.ProviderQuoteId)
	assert.Equal(t, 17.25, result.Price)
	assert.Equal(t, expiresAt.Unix(), result.ExpireAt)
	assert.Empty(t, result.ErrorMsg)
}

func TestCreateRFQHandler_MissingClientID(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	body := map[string]any{
		"pair":      "USD:MXN",
		"orderSide": "BUY",
		"quantity":  100_000.0,
	}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateRFQHandler_MissingPair(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	body := map[string]any{"clientId": "c", "orderSide": "BUY", "quantity": 100_000.0}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateRFQHandler_ZeroQuantity(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	body := map[string]any{"clientId": "c", "pair": "USD:MXN", "orderSide": "BUY", "quantity": 0.0}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateRFQHandler_UnknownClient(t *testing.T) {
	app := newTestApp(&mockRFQService{}, false) // validator returns false
	body := map[string]any{
		"clientId": "unknown-client", "pair": "USD:MXN",
		"orderSide": "BUY", "quantity": 100_000.0,
	}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestCreateRFQHandler_ServiceError(t *testing.T) {
	svc := &mockRFQService{createErr: errors.New("session not connected")}
	app := newTestApp(svc, true)
	body := map[string]any{
		"clientId": "c", "pair": "USD:MXN", "orderSide": "BUY", "quantity": 100_000.0,
	}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/quotes", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result RFQResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Contains(t, result.ErrorMsg, "session not connected")
}

func TestCreateRFQHandler_InvalidJSON(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quotes", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ─── ExecuteRFQHandler ────────────────────────────────────────────────────────

func TestExecuteRFQHandler_Success(t *testing.T) {
	svc := &mockRFQService{
		trade: &model.TradeConfirmation{
			TradeID:    "trade-1",
			Status:     "filled",
			Price:      17.30,
			ExecutedAt: time.Now(),
		},
	}
	app := newTestApp(svc, true)
	body := map[string]any{
		"clientId": "client-A",
		"quoteId":  "zodia-q-1",
	}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/orders", body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result RFQExecutionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Equal(t, "trade-1", result.ProviderOrderID)
	assert.Equal(t, "filled", result.Status)
	assert.Equal(t, 17.30, result.Price)
}

func TestExecuteRFQHandler_MissingClientID(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	resp := doRequest(t, app, http.MethodPost, "/api/v1/orders", map[string]any{"quoteId": "q1"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestExecuteRFQHandler_MissingQuoteID(t *testing.T) {
	app := newTestApp(&mockRFQService{}, true)
	resp := doRequest(t, app, http.MethodPost, "/api/v1/orders", map[string]any{"clientId": "c"})
	// quoteId missing → falls through to 400
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestExecuteRFQHandler_UnknownClient(t *testing.T) {
	app := newTestApp(&mockRFQService{}, false)
	body := map[string]any{"clientId": "c", "quoteId": "q1"}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/orders", body)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestExecuteRFQHandler_ServiceError(t *testing.T) {
	svc := &mockRFQService{executeErr: errors.New("quote expired")}
	app := newTestApp(svc, true)
	body := map[string]any{"clientId": "c", "quoteId": "q1"}
	resp := doRequest(t, app, http.MethodPost, "/api/v1/orders", body)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var result RFQExecutionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Contains(t, result.ErrorMsg, "quote expired")
}

// ─── Validation ───────────────────────────────────────────────────────────────

func TestRFQCreateRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     RFQCreateRequest
		wantErr bool
	}{
		{"valid", RFQCreateRequest{ClientID: "c", CurrencyPair: "USD:MXN", Side: "BUY", Amount: 100}, false},
		{"missing client", RFQCreateRequest{CurrencyPair: "USD:MXN", Side: "BUY", Amount: 100}, true},
		{"missing pair", RFQCreateRequest{ClientID: "c", Side: "BUY", Amount: 100}, true},
		{"missing side", RFQCreateRequest{ClientID: "c", CurrencyPair: "USD:MXN", Amount: 100}, true},
		{"zero amount", RFQCreateRequest{ClientID: "c", CurrencyPair: "USD:MXN", Side: "BUY", Amount: 0}, true},
		{"negative amount", RFQCreateRequest{ClientID: "c", CurrencyPair: "USD:MXN", Side: "BUY", Amount: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRFQExecuteRequest_Validate(t *testing.T) {
	assert.Error(t, (&RFQExecuteRequest{}).Validate(), "missing clientId should fail")
	assert.NoError(t, (&RFQExecuteRequest{ClientID: "c"}).Validate())
}
