package api

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/capa-adapter/internal/capa"
	"github.com/Checker-Finance/adapters/internal/store"
)

// WebhookProcessor handles Capa webhook event processing.
type WebhookProcessor interface {
	ProcessWebhookEvent(ctx context.Context, clientID string, event *capa.CapaWebhookEvent, signature string, body []byte) error
}

// WebhookAPIHandler handles the POST /webhooks/capa/transactions route.
type WebhookAPIHandler struct {
	processor WebhookProcessor
	store     store.Store
	resolver  capa.ConfigResolver
}

// NewWebhookAPIHandler creates a new WebhookAPIHandler.
func NewWebhookAPIHandler(
	processor WebhookProcessor,
	st store.Store,
	resolver capa.ConfigResolver,
) *WebhookAPIHandler {
	return &WebhookAPIHandler{
		processor: processor,
		store:     st,
		resolver:  resolver,
	}
}

// HandleWebhook processes incoming Capa webhook events.
// POST /webhooks/capa/transactions
func (h *WebhookAPIHandler) HandleWebhook(c *fiber.Ctx) error {
	body := c.Body()

	var event capa.CapaWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		slog.Warn("capa.webhook.parse_error",
			"error", err,
			"body", string(body))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid payload",
		})
	}

	// Identify the client associated with this transaction.
	// Look up the client ID stored in Redis when the trade was executed.
	clientID := ""
	txID := event.TransactionID
	if txID == "" {
		txID = event.Transaction.ID
	}
	if txID != "" {
		var storedClientID string
		if err := h.store.GetJSON(c.Context(), "capa:tx:"+txID+":client", &storedClientID); err == nil && storedClientID != "" {
			clientID = storedClientID
		}
	}

	slog.Info("capa.webhook.incoming",
		"event", event.Event,
		"tx_id", event.TransactionID,
		"status", event.Status,
		"client_id", clientID)

	signature := c.Get("X-Capa-Signature")
	if signature == "" {
		signature = c.Get("X-Webhook-Signature")
	}

	ctx := c.UserContext()
	if err := h.processor.ProcessWebhookEvent(ctx, clientID, &event, signature, body); err != nil {
		if err == capa.ErrInvalidSignature {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid signature",
			})
		}
		slog.Error("capa.webhook.process_error",
			"tx_id", event.TransactionID,
			"error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to process webhook",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}
