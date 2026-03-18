package api

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/capa-adapter/internal/capa"
	"github.com/Checker-Finance/adapters/internal/store"
)

// WebhookProcessor handles Capa webhook event processing.
type WebhookProcessor interface {
	ProcessWebhookEvent(ctx context.Context, clientID string, event *capa.CapaWebhookEvent, signature string, body []byte) error
}

// WebhookAPIHandler handles the POST /webhooks/capa/transactions route.
type WebhookAPIHandler struct {
	logger    *zap.Logger
	processor WebhookProcessor
	store     store.Store
	resolver  capa.ConfigResolver
}

// NewWebhookAPIHandler creates a new WebhookAPIHandler.
func NewWebhookAPIHandler(
	logger *zap.Logger,
	processor WebhookProcessor,
	st store.Store,
	resolver capa.ConfigResolver,
) *WebhookAPIHandler {
	return &WebhookAPIHandler{
		logger:    logger,
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
		h.logger.Warn("capa.webhook.parse_error",
			zap.Error(err),
			zap.String("body", string(body)))
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

	h.logger.Info("capa.webhook.incoming",
		zap.String("event", event.Event),
		zap.String("tx_id", event.TransactionID),
		zap.String("status", event.Status),
		zap.String("client_id", clientID))

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
		h.logger.Error("capa.webhook.process_error",
			zap.String("tx_id", event.TransactionID),
			zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to process webhook",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}
