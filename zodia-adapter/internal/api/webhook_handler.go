package api

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// WebhookTradeSync syncs terminal trades from webhook events.
type WebhookTradeSync interface {
	SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error
}

// WebhookMapper converts Zodia webhook events to canonical models.
type WebhookMapper interface {
	WebhookToTransaction(event *zodia.ZodiaWebhookEvent) *zodia.ZodiaTransaction
	MapTransactionToTrade(tx *zodia.ZodiaTransaction, clientID string) *model.TradeConfirmation
}

// WebhookHandler handles incoming Zodia webhook notifications.
// POST /webhooks/zodia/transactions
//
// Zodia sends a webhook for each transaction state change.
// This handler processes PROCESSED transactions to finalize trades.
//
// Idempotency: event UUID is stored in Redis with a 48h TTL to prevent duplicate processing.
// ⚠️ Zodia may not send a signature header. If they do, add HMAC validation here.
type WebhookHandler struct {
	logger    *zap.Logger
	store     store.Store
	mapper    WebhookMapper
	tradeSync WebhookTradeSync
	publisher *publisher.Publisher
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(
	logger *zap.Logger,
	st store.Store,
	mapper WebhookMapper,
	tradeSync WebhookTradeSync,
	pub *publisher.Publisher,
) *WebhookHandler {
	return &WebhookHandler{
		logger:    logger,
		store:     st,
		mapper:    mapper,
		tradeSync: tradeSync,
		publisher: pub,
	}
}

// Handle processes incoming Zodia webhook events.
func (h *WebhookHandler) Handle(c *fiber.Ctx) error {
	var event zodia.ZodiaWebhookEvent
	if err := c.BodyParser(&event); err != nil {
		h.logger.Warn("zodia.webhook.parse_failed", zap.Error(err))
		// Return 200 to prevent Zodia from retrying on our parse error
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "parse_error"})
	}

	ctx := context.Background()

	// Filter: only process trade types
	if event.Type != "OTCTRADE" && event.Type != "RFSTRADE" {
		h.logger.Debug("zodia.webhook.ignored_type",
			zap.String("type", event.Type),
			zap.String("uuid", event.UUID))
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "type_not_applicable"})
	}

	// Filter: only process PROCESSED (terminal) state
	if event.State != "PROCESSED" {
		h.logger.Debug("zodia.webhook.non_terminal",
			zap.String("state", event.State),
			zap.String("trade_id", event.TradeID))
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "acknowledged", "reason": "non_terminal"})
	}

	// Idempotency: check if we've already processed this event UUID.
	// Try to get the key; if it exists, it's a duplicate.
	dedupKey := "zodia:webhook:dedup:" + event.UUID
	if event.UUID != "" {
		var existing bool
		getErr := h.store.GetJSON(ctx, dedupKey, &existing)
		if getErr == nil && existing {
			// Key exists → duplicate
			h.logger.Debug("zodia.webhook.duplicate",
				zap.String("uuid", event.UUID),
				zap.String("trade_id", event.TradeID))
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "duplicate"})
		}
		// Mark as processed (48h TTL)
		if err := h.store.SetJSON(ctx, dedupKey, true, 48*time.Hour); err != nil {
			h.logger.Warn("zodia.webhook.dedup_set_failed",
				zap.String("uuid", event.UUID),
				zap.Error(err))
			// Proceed anyway
		}
	}

	// Look up internal order by quoteID to get clientID.
	// NOTE: GetClientIDByTradeID is not available on the store interface.
	// The webhook must include enough context, or this requires an additional store method.
	// For now, we use the tradeID as a best-effort lookup via quote records.
	// ⚠️ This will be a no-op until the store has a TradeID→ClientID index.
	// Callers are expected to extend the store with a GetClientIDByTradeID method.
	clientID := ""
	if qrec, err := h.store.GetQuoteByQuoteID(ctx, event.TradeID); err == nil && qrec != nil {
		clientID = qrec.ClientID
	}
	if clientID == "" {
		h.logger.Warn("zodia.webhook.client_not_found",
			zap.String("trade_id", event.TradeID))
		// Return 200 to prevent retries; log for investigation
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "client_not_found"})
	}

	// Convert webhook to transaction and build trade confirmation
	tx := h.mapper.WebhookToTransaction(&event)
	trade := h.mapper.MapTransactionToTrade(tx, clientID)
	if trade == nil {
		h.logger.Warn("zodia.webhook.map_failed", zap.String("trade_id", event.TradeID))
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "error", "reason": "mapping_failed"})
	}

	// Sync to legacy database
	if err := h.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
		h.logger.Error("zodia.webhook.sync_failed",
			zap.String("trade_id", event.TradeID),
			zap.Error(err))
		// Return 200 to prevent infinite retries; logged for alerting
	} else {
		h.logger.Info("zodia.webhook.trade_synced",
			zap.String("trade_id", event.TradeID),
			zap.String("client", clientID),
			zap.String("status", trade.Status))
	}

	// Publish final NATS event
	if h.publisher != nil {
		subject := "evt.trade." + trade.Status + ".v1.ZODIA"
		if err := h.publisher.Publish(ctx, subject, trade); err != nil {
			metrics.IncNATSPublishError(subject)
			h.logger.Warn("zodia.webhook.publish_failed",
				zap.String("subject", subject),
				zap.Error(err))
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "processed"})
}
