package rio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/legacy"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/publisher"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/store"
)

// WebhookHandler handles incoming webhook events from Rio.
type WebhookHandler struct {
	logger    *zap.Logger
	publisher *publisher.Publisher
	store     store.Store
	poller    *Poller
	tradeSync *legacy.TradeSyncWriter
	service   *Service
	secret    string
	sigHeader string
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(
	logger *zap.Logger,
	pub *publisher.Publisher,
	st store.Store,
	poller *Poller,
	tradeSync *legacy.TradeSyncWriter,
	service *Service,
	secret string,
	sigHeader string,
) *WebhookHandler {
	if strings.TrimSpace(sigHeader) == "" {
		sigHeader = "X-Rio-Signature"
	}
	return &WebhookHandler{
		logger:    logger,
		publisher: pub,
		store:     st,
		poller:    poller,
		tradeSync: tradeSync,
		service:   service,
		secret:    secret,
		sigHeader: sigHeader,
	}
}

// HandleOrderWebhook processes order status change webhooks from Rio.
// POST /webhooks/rio/orders
func (h *WebhookHandler) HandleOrderWebhook(c *fiber.Ctx) error {
	if h.secret != "" {
		signature := c.Get(h.sigHeader)
		if signature == "" || !validateWebhookSignature(h.secret, signature, c.Body()) {
			h.logger.Warn("rio.webhook.invalid_signature",
				zap.String("header", h.sigHeader))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid signature",
			})
		}
	}

	var event RioOrderWebhookEvent
	if err := c.BodyParser(&event); err != nil {
		h.logger.Warn("rio.webhook.parse_error",
			zap.Error(err),
			zap.String("body", string(c.Body())))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid payload",
		})
	}

	order := event.Data
	h.logger.Info("rio.webhook.received",
		zap.String("event", event.Event),
		zap.String("order_id", order.ID),
		zap.String("status", order.Status),
		zap.String("client_ref", order.ClientReferenceID))

	ctx := c.UserContext()

	// Cancel any active polling for this order (webhook takes over)
	if h.poller != nil {
		h.poller.CancelPolling(order.ID)
	}

	// Normalize the status
	normalizedStatus := NormalizeRioStatus(order.Status)

	// Publish status change event
	if h.publisher != nil {
		statusEvent := map[string]any{
			"client_id":  order.ClientReferenceID,
			"order_id":   order.ID,
			"quote_id":   order.QuoteID,
			"status":     normalizedStatus,
			"raw_status": order.Status,
			"source":     "webhook",
			"updated_at": time.Now().UTC(),
		}

		subject := "evt.trade.status_changed.v1.RIO"
		if err := h.publisher.Publish(ctx, subject, statusEvent); err != nil {
			h.logger.Warn("rio.webhook.publish_failed",
				zap.String("subject", subject),
				zap.Error(err))
		}
	}

	// Handle terminal statuses
	if IsTerminalStatus(order.Status) {
		h.handleTerminalWebhook(ctx, &order, normalizedStatus)
	}

	return c.SendStatus(fiber.StatusOK)
}

func validateWebhookSignature(secret, signature string, body []byte) bool {
	normalized := strings.TrimSpace(signature)
	if strings.HasPrefix(strings.ToLower(normalized), "sha256=") {
		normalized = normalized[7:]
	}
	expected, err := hex.DecodeString(normalized)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	actual := mac.Sum(nil)
	return hmac.Equal(actual, expected)
}

// handleTerminalWebhook processes a terminal order status from webhook.
func (h *WebhookHandler) handleTerminalWebhook(ctx context.Context, order *RioOrderResponse, status string) {
	clientID := order.ClientReferenceID
	if clientID == "" {
		clientID = order.UserID
	}

	// Sync to legacy database
	if h.tradeSync != nil && h.service != nil {
		trade := h.service.BuildTradeConfirmationFromOrder(clientID, order.ID, order)
		if trade != nil {
			if err := h.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
				h.logger.Warn("rio.webhook.trade_sync_failed",
					zap.String("order_id", order.ID),
					zap.String("client", clientID),
					zap.Error(err))
			} else {
				h.logger.Info("rio.webhook.trade_synced",
					zap.String("order_id", order.ID),
					zap.String("client", clientID),
					zap.String("status", status))
			}
		}
	}

	// Publish final event
	if h.publisher != nil {
		finalSubject := "evt.trade." + strings.ToLower(status) + ".v1.RIO"
		if err := h.publisher.Publish(ctx, finalSubject, map[string]any{
			"client_id": clientID,
			"order_id":  order.ID,
			"quote_id":  order.QuoteID,
			"status":    status,
			"final":     true,
			"source":    "webhook",
			"timestamp": time.Now().UTC(),
		}); err != nil {
			h.logger.Warn("rio.webhook.publish_final_failed",
				zap.String("subject", finalSubject),
				zap.Error(err))
		}
	}

	h.logger.Info("rio.webhook.terminal_processed",
		zap.String("order_id", order.ID),
		zap.String("client", clientID),
		zap.String("status", status))
}
