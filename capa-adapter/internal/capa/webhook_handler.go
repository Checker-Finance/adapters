package capa

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
)

// WebhookHandler handles incoming webhook events from Capa.
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

// ProcessWebhookEvent validates the HMAC-SHA256 signature and processes a Capa webhook event.
// It cancels active polling on terminal events and publishes NATS events.
func (h *WebhookHandler) ProcessWebhookEvent(ctx context.Context, clientID string, event *CapaWebhookEvent, signature string, body []byte) error {
	// Validate HMAC-SHA256 signature using the per-client webhook secret.
	if h.resolver != nil && clientID != "" {
		if clientCfg, err := h.resolver.Resolve(ctx, clientID); err == nil && clientCfg.WebhookSecret != "" {
			if signature == "" || !validateWebhookSignature(clientCfg.WebhookSecret, signature, body) {
				slog.Warn("capa.webhook.invalid_signature",
					"client", clientID)
				return ErrInvalidSignature
			}
		}
	}

	txID := event.ResolvedTxID()
	quoteID := event.ResolvedQuoteID()
	rawStatus := event.ResolvedStatus()

	slog.Info("capa.webhook.received",
		"event", event.Event,
		"tx_id", txID,
		"status", rawStatus,
		"client", clientID)

	// Cancel any active polling for this transaction (webhook takes over)
	if h.poller != nil && txID != "" {
		h.poller.CancelPolling(txID)
	}

	normalizedStatus := NormalizeCapaStatus(rawStatus)

	// Publish status change event
	if h.publisher != nil {
		statusEvent := map[string]any{
			"client_id":  clientID,
			"trade_id":   txID,
			"quote_id":   quoteID,
			"status":     normalizedStatus,
			"raw_status": rawStatus,
			"source":     "webhook",
			"updated_at": time.Now().UTC(),
		}
		subject := "evt.trade.status_changed.v1.CAPA"
		if err := h.publisher.Publish(ctx, subject, statusEvent); err != nil {
			slog.Warn("capa.webhook.publish_failed",
				"subject", subject,
				"error", err)
		}
	}

	// Handle terminal statuses
	if IsTerminalStatus(rawStatus) {
		h.handleTerminalWebhook(ctx, clientID, txID, quoteID, event, normalizedStatus)
	}

	return nil
}

// ErrInvalidSignature is returned when webhook signature validation fails.
var ErrInvalidSignature = errInvalidSignature("invalid webhook signature")

type errInvalidSignature string

func (e errInvalidSignature) Error() string { return string(e) }

// validateWebhookSignature validates the HMAC-SHA256 signature of a webhook payload.
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

// handleTerminalWebhook processes a terminal transaction status from webhook.
func (h *WebhookHandler) handleTerminalWebhook(
	ctx context.Context,
	clientID, txID, quoteID string,
	event *CapaWebhookEvent,
	status string,
) {
	// Sync to legacy database
	if h.tradeSync != nil && h.service != nil {
		tx := event.Transaction
		if tx.ID == "" {
			tx.ID = txID
		}
		if tx.QuoteID == "" {
			tx.QuoteID = quoteID
		}
		if tx.Status == "" {
			tx.Status = event.Status
		}
		trade := h.service.BuildTradeConfirmationFromTx(clientID, &tx)
		if trade != nil {
			if err := h.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
				slog.Warn("capa.webhook.trade_sync_failed",
					"tx_id", txID,
					"client", clientID,
					"error", err)
			} else {
				slog.Info("capa.webhook.trade_synced",
					"tx_id", txID,
					"client", clientID,
					"status", status)
			}
		}
	}

	// Publish final event
	if h.publisher != nil {
		finalSubject := tradeEventSubject(status)
		if err := h.publisher.Publish(ctx, finalSubject, map[string]any{
			"client_id": clientID,
			"trade_id":  txID,
			"quote_id":  quoteID,
			"status":    status,
			"final":     true,
			"source":    "webhook",
			"timestamp": time.Now().UTC(),
		}); err != nil {
			slog.Warn("capa.webhook.publish_final_failed",
				"subject", finalSubject,
				"error", err)
		}
	}

	slog.Info("capa.webhook.terminal_processed",
		"tx_id", txID,
		"client", clientID,
		"status", status)
}
