package rio

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/internal/webhooks"
)

// WebhookHandler handles incoming webhook events from Rio.
type WebhookHandler struct {
	publisher *publisher.Publisher
	store     store.Store
	poller    *Poller
	tradeSync *legacy.TradeSyncWriter
	service   *Service
	resolver  ConfigResolver
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(
	pub *publisher.Publisher,
	st store.Store,
	poller *Poller,
	tradeSync *legacy.TradeSyncWriter,
	service *Service,
	resolver ConfigResolver,
) *WebhookHandler {
	return &WebhookHandler{
		publisher: pub,
		store:     st,
		poller:    poller,
		tradeSync: tradeSync,
		service:   service,
		resolver:  resolver,
	}
}

// HandleOrderWebhook processes order status change webhooks from Rio.
// POST /webhooks/rio/orders
func (h *WebhookHandler) HandleOrderWebhook(c *fiber.Ctx) error {
	// Parse body first so we can identify the client for per-client signature validation.
	var event RioOrderWebhookEvent
	if err := c.BodyParser(&event); err != nil {
		slog.Warn("rio.webhook.parse_error",
			"error", err,
			"body", string(c.Body()))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid payload",
		})
	}

	// Validate HMAC signature using the per-client webhook secret (if configured).
	if h.resolver != nil {
		clientID := event.Data.ClientReferenceID
		if clientID == "" {
			clientID = event.Data.UserID
		}
		if clientID != "" {
			if clientCfg, err := h.resolver.Resolve(c.UserContext(), clientID); err == nil && clientCfg.WebhookSecret != "" {
				sigHeader := clientCfg.WebhookSigHeader
				signature := c.Get(sigHeader)
				if signature == "" || !webhooks.ValidateHMACSHA256(clientCfg.WebhookSecret, signature, c.Body()) {
					slog.Warn("rio.webhook.invalid_signature",
						"client", clientID,
						"header", sigHeader)
					return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
						"error": "invalid signature",
					})
				}
			}
		}
	}

	order := event.Data
	slog.Info("rio.webhook.received",
		"event", event.Event,
		"order_id", order.ID,
		"status", order.Status,
		"client_ref", order.ClientReferenceID)

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
			slog.Warn("rio.webhook.publish_failed",
				"subject", subject,
				"error", err)
		}
	}

	// Handle terminal statuses
	if IsTerminalStatus(order.Status) {
		h.handleTerminalWebhook(ctx, &order, normalizedStatus)
	}

	return c.SendStatus(fiber.StatusOK)
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
				slog.Warn("rio.webhook.trade_sync_failed",
					"order_id", order.ID,
					"client", clientID,
					"error", err)
			} else {
				slog.Info("rio.webhook.trade_synced",
					"order_id", order.ID,
					"client", clientID,
					"status", status)
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
			slog.Warn("rio.webhook.publish_final_failed",
				"subject", finalSubject,
				"error", err)
		}
	}

	slog.Info("rio.webhook.terminal_processed",
		"order_id", order.ID,
		"client", clientID,
		"status", status)
}
