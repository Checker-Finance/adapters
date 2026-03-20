package rio

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
)

// Poller handles scheduled polling of Rio order/trade status.
// Used as a fallback when webhooks are not available or miss events.
type Poller struct {
	cfg          config.Config
	service      *Service
	publisher    *publisher.Publisher
	store        store.Store
	pollInterval time.Duration
	stopCh       chan struct{}

	activeTrades sync.Map // order_id -> cancel function
	tradeSync    *legacy.TradeSyncWriter
}

// NewPoller constructs a new Rio poller for trade status tracking.
func NewPoller(
	cfg config.Config,
	service *Service,
	pub *publisher.Publisher,
	st store.Store,
	interval time.Duration,
	tradeSync *legacy.TradeSyncWriter,
) *Poller {
	return &Poller{
		cfg:          cfg,
		service:      service,
		publisher:    pub,
		store:        st,
		pollInterval: interval,
		stopCh:       make(chan struct{}),
		tradeSync:    tradeSync,
	}
}

// Stop signals the poller to stop gracefully.
func (p *Poller) Stop() {
	close(p.stopCh)
}

// PollTradeStatus continuously checks a Rio order until it reaches a terminal state.
// This is used as a fallback when webhooks might miss events.
func (p *Poller) PollTradeStatus(
	parentCtx context.Context,
	clientID,
	quoteID,
	orderID string,
) {
	// Prevent duplicate polling for the same order
	if _, exists := p.activeTrades.Load(orderID); exists {
		slog.Debug("rio.trade_poll_already_active",
			"order_id", orderID,
			"client", clientID,
		)
		return
	}

	// Create dedicated child context for this poller
	ctx, cancel := context.WithCancel(parentCtx)
	p.activeTrades.Store(orderID, cancel)

	go func() {
		defer func() {
			p.activeTrades.Delete(orderID)
			cancel()
		}()

		// Use longer interval since webhooks are primary
		ticker := time.NewTicker(p.pollInterval)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				slog.Info("rio.trade_poll_stopped",
					"order_id", orderID,
					"client", clientID,
					"last_status", lastStatus)
				return

			case <-p.stopCh:
				slog.Info("rio.trade_poll_stopped",
					"order_id", orderID,
					"reason", "poller_shutdown")
				return

			case <-ticker.C:
				order, err := p.service.FetchTradeStatus(ctx, clientID, orderID)
				if err != nil {
					slog.Warn("rio.trade_poll_error",
						"order_id", orderID,
						"client", clientID,
						"error", err)
					continue
				}

				rawStatus := order.Status
				status := NormalizeRioStatus(rawStatus)

				// Emit status change only when it actually changes
				if status != lastStatus {
					lastStatus = status

					if p.publisher != nil {
						event := map[string]any{
							"client_id":  clientID,
							"order_id":   orderID,
							"quote_id":   quoteID,
							"status":     status,
							"raw":        rawStatus,
							"updated_at": time.Now().UTC(),
						}

						subject := "evt.trade.status_changed.v1.RIO"
						if err := p.publisher.Publish(ctx, subject, event); err != nil {
							slog.Debug("nats.publish_failed",
								"subject", subject,
								"error", err)
						}
					}

					slog.Info("rio.trade_status_changed",
						"order_id", orderID,
						"client", clientID,
						"raw_status", rawStatus,
						"normalized_status", status)
				}

				// Handle terminal status
				if IsTerminalStatus(status) {
					p.handleTerminalStatus(ctx, clientID, orderID, quoteID, order, status)
					return
				}
			}
		}
	}()
}

// CancelPolling cancels active polling for an order.
// Called by webhook handler when it receives a status update.
func (p *Poller) CancelPolling(orderID string) {
	if cancel, ok := p.activeTrades.Load(orderID); ok {
		slog.Info("rio.polling_cancelled_by_webhook",
			"order_id", orderID)
		cancel.(context.CancelFunc)()
		p.activeTrades.Delete(orderID)
	}
}

// IsPolling returns true if the order is currently being polled.
func (p *Poller) IsPolling(orderID string) bool {
	_, exists := p.activeTrades.Load(orderID)
	return exists
}

// handleTerminalStatus processes a trade that has reached a terminal state.
func (p *Poller) handleTerminalStatus(
	ctx context.Context,
	clientID,
	orderID,
	quoteID string,
	order *RioOrderResponse,
	status string,
) {
	// 1. Sync into legacy database
	if p.tradeSync != nil {
		trade := p.service.BuildTradeConfirmationFromOrder(clientID, orderID, order)
		if trade != nil {
			if err := p.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
				slog.Warn("legacy.trade_sync_failed",
					"order_id", orderID,
					"client", clientID,
					"error", err)
			} else {
				slog.Info("legacy.trade_sync_upsert",
					"order_id", orderID,
					"client", clientID,
					"status", trade.Status,
					"venue", trade.Venue)
			}
		}
	}

	// 2. Emit final event
	if p.publisher != nil {
		finalSubject := "evt.trade." + strings.ToLower(status) + ".v1.RIO"
		if err := p.publisher.Publish(ctx, finalSubject, map[string]any{
			"client_id": clientID,
			"order_id":  orderID,
			"quote_id":  quoteID,
			"status":    status,
			"final":     true,
			"timestamp": time.Now().UTC(),
		}); err != nil {
			slog.Debug("nats.publish_failed",
				"subject", finalSubject,
				"error", err)
		}
	}

	slog.Info("rio.trade_poll_complete",
		"order_id", orderID,
		"client", clientID,
		"final_status", status)
}
