package rio

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWebhookHandler_HandleOrderWebhook(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		payload        RioOrderWebhookEvent
		expectedStatus int
	}{
		{
			name: "processing order",
			payload: RioOrderWebhookEvent{
				Event: "order.status_changed",
				Data: RioOrderResponse{
					ID:                "order-123",
					QuoteID:           "quote-456",
					Status:            "processing",
					Side:              "buy",
					Crypto:            "USDC",
					Fiat:              "USD",
					ClientReferenceID: "client-ref-789",
				},
			},
			expectedStatus: fiber.StatusOK,
		},
		{
			name: "completed order",
			payload: RioOrderWebhookEvent{
				Event: "order.status_changed",
				Data: RioOrderResponse{
					ID:                "order-123",
					QuoteID:           "quote-456",
					Status:            "completed",
					Side:              "buy",
					Crypto:            "USDC",
					Fiat:              "USD",
					ClientReferenceID: "client-ref-789",
				},
			},
			expectedStatus: fiber.StatusOK,
		},
		{
			name: "failed order",
			payload: RioOrderWebhookEvent{
				Event: "order.status_changed",
				Data: RioOrderResponse{
					ID:                "order-123",
					QuoteID:           "quote-456",
					Status:            "failed",
					Side:              "sell",
					Crypto:            "USDT",
					Fiat:              "MXN",
					ClientReferenceID: "client-ref-789",
				},
			},
			expectedStatus: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWebhookHandler(logger, nil, nil, nil, nil, nil, "", "")

			app := fiber.New()
			app.Post("/webhooks/rio/orders", handler.HandleOrderWebhook)

			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/webhooks/rio/orders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestWebhookHandler_InvalidPayload(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler(logger, nil, nil, nil, nil, nil, "", "")

	app := fiber.New()
	app.Post("/webhooks/rio/orders", handler.HandleOrderWebhook)

	// Send invalid JSON
	req := httptest.NewRequest("POST", "/webhooks/rio/orders", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid payload")
}

func TestWebhookHandler_EmptyBody(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler(logger, nil, nil, nil, nil, nil, "", "")

	app := fiber.New()
	app.Post("/webhooks/rio/orders", handler.HandleOrderWebhook)

	req := httptest.NewRequest("POST", "/webhooks/rio/orders", nil)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	// Empty body should return bad request
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler(logger, nil, nil, nil, nil, nil, "secret", "X-Rio-Signature")

	app := fiber.New()
	app.Post("/webhooks/rio/orders", handler.HandleOrderWebhook)

	payload := RioOrderWebhookEvent{
		Event: "order.status_changed",
		Data: RioOrderResponse{
			ID:                "order-123",
			QuoteID:           "quote-456",
			Status:            "processing",
			Side:              "buy",
			Crypto:            "USDC",
			Fiat:              "USD",
			ClientReferenceID: "client-ref-789",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/webhooks/rio/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Rio-Signature", "invalid")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
}

func TestWebhookHandler_ValidSignature(t *testing.T) {
	logger := zap.NewNop()
	handler := NewWebhookHandler(logger, nil, nil, nil, nil, nil, "secret", "X-Rio-Signature")

	app := fiber.New()
	app.Post("/webhooks/rio/orders", handler.HandleOrderWebhook)

	payload := RioOrderWebhookEvent{
		Event: "order.status_changed",
		Data: RioOrderResponse{
			ID:                "order-123",
			QuoteID:           "quote-456",
			Status:            "processing",
			Side:              "buy",
			Crypto:            "USDC",
			Fiat:              "USD",
			ClientReferenceID: "client-ref-789",
		},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhooks/rio/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Rio-Signature", signature)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
