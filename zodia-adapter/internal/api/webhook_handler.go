package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
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
	store     store.Store
	mapper    WebhookMapper
	tradeSync WebhookTradeSync
	publisher *publisher.Publisher
}

// NewWebhookHandler constructs a WebhookHandler.
func NewWebhookHandler(
	st store.Store,
	mapper WebhookMapper,
	tradeSync WebhookTradeSync,
	pub *publisher.Publisher,
) *WebhookHandler {
	return &WebhookHandler{
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
		slog.Warn("zodia.webhook.parse_failed", "error", err)
		// Return 200 to prevent Zodia from retrying on our parse error
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "parse_error"})
	}

	ctx := context.Background()

	// Filter: only process trade types
	if event.Type != "OTCTRADE" && event.Type != "RFSTRADE" {
		slog.Debug("zodia.webhook.ignored_type",
			"type", event.Type,
			"uuid", event.UUID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "type_not_applicable"})
	}

	// Filter: only process PROCESSED (terminal) state
	if event.State != "PROCESSED" {
		slog.Debug("zodia.webhook.non_terminal",
			"state", event.State,
			"trade_id", event.TradeID)
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
			slog.Debug("zodia.webhook.duplicate",
				"uuid", event.UUID,
				"trade_id", event.TradeID)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "duplicate"})
		}
		// Mark as processed (48h TTL)
		if err := h.store.SetJSON(ctx, dedupKey, true, 48*time.Hour); err != nil {
			slog.Warn("zodia.webhook.dedup_set_failed",
				"uuid", event.UUID,
				"error", err)
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
		slog.Warn("zodia.webhook.client_not_found",
			"trade_id", event.TradeID)
		// Return 200 to prevent retries; log for investigation
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ignored", "reason": "client_not_found"})
	}

	// Convert webhook to transaction and build trade confirmation
	tx := h.mapper.WebhookToTransaction(&event)
	trade := h.mapper.MapTransactionToTrade(tx, clientID)
	if trade == nil {
		slog.Warn("zodia.webhook.map_failed", "trade_id", event.TradeID)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "error", "reason": "mapping_failed"})
	}

	// Sync to legacy database
	if err := h.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
		slog.Error("zodia.webhook.sync_failed",
			"trade_id", event.TradeID,
			"error", err)
		// Return 200 to prevent infinite retries; logged for alerting
	} else {
		slog.Info("zodia.webhook.trade_synced",
			"trade_id", event.TradeID,
			"client", clientID,
			"status", trade.Status)
	}

	// Publish final NATS event
	if h.publisher != nil {
		subject := "evt.trade." + trade.Status + ".v1.ZODIA"
		if err := h.publisher.Publish(ctx, subject, trade); err != nil {
			metrics.IncNATSPublishError(subject)
			slog.Warn("zodia.webhook.publish_failed",
				"subject", subject,
				"error", err)
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "processed"})
}
