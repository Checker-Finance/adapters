package xfx

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
)

// Poller continuously checks XFX transaction status for active trades.
// XFX does not support webhooks so polling is the only mechanism for status updates.
type Poller struct {
	cfg          config.Config
	service      *Service
	publisher    *publisher.Publisher
	store        store.Store
	pollInterval time.Duration
	stopCh       chan struct{}

	activeTrades sync.Map // txID → cancel func
	tradeSync    *legacy.TradeSyncWriter
}

// NewPoller constructs a new XFX poller.
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

// Stop signals the poller to stop all active polling goroutines.
func (p *Poller) Stop() {
	close(p.stopCh)
}

// PollTradeStatus continuously polls XFX for a transaction's status until it
// reaches a terminal state or the poller is stopped.
func (p *Poller) PollTradeStatus(
	parentCtx context.Context,
	clientID,
	quoteID,
	txID string,
) {
	// Prevent duplicate polling for the same transaction
	if _, exists := p.activeTrades.Load(txID); exists {
		slog.Debug("xfx.trade_poll_already_active",
			"tx_id", txID,
			"client", clientID,
		)
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	p.activeTrades.Store(txID, cancel)

	go func() {
		defer func() {
			p.activeTrades.Delete(txID)
			cancel()
		}()

		ticker := time.NewTicker(p.pollInterval)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				slog.Info("xfx.trade_poll_stopped",
					"tx_id", txID,
					"client", clientID,
					"last_status", lastStatus)
				return

			case <-p.stopCh:
				slog.Info("xfx.trade_poll_stopped",
					"tx_id", txID,
					"reason", "poller_shutdown")
				return

			case <-ticker.C:
				tx, err := p.service.FetchTransactionStatus(ctx, clientID, txID)
				if err != nil {
					slog.Warn("xfx.trade_poll_error",
						"tx_id", txID,
						"client", clientID,
						"error", err)
					continue
				}

				rawStatus := tx.Status
				status := NormalizeXFXStatus(rawStatus)

				// Emit status change only when it actually changes
				if status != lastStatus {
					lastStatus = status

					if p.publisher != nil {
						event := map[string]any{
							"client_id":  clientID,
							"trade_id":   txID,
							"quote_id":   quoteID,
							"status":     status,
							"raw":        rawStatus,
							"updated_at": time.Now().UTC(),
						}
						subject := "evt.trade.status_changed.v1.XFX"
						if err := p.publisher.Publish(ctx, subject, event); err != nil {
							metrics.IncNATSPublishError(subject)
							slog.Debug("nats.publish_failed",
								"subject", subject,
								"error", err)
						}
					}

					slog.Info("xfx.trade_status_changed",
						"tx_id", txID,
						"client", clientID,
						"raw_status", rawStatus,
						"normalized_status", status)
				}

				if IsTerminalStatus(rawStatus) {
					p.handleTerminalStatus(ctx, clientID, txID, quoteID, tx, status)
					return
				}
			}
		}
	}()
}

// handleTerminalStatus processes a trade that has reached a terminal state.
func (p *Poller) handleTerminalStatus(
	ctx context.Context,
	clientID,
	txID,
	quoteID string,
	tx *XFXTransaction,
	status string,
) {
	// 1. Sync to legacy database
	if p.tradeSync != nil {
		trade := p.service.BuildTradeConfirmationFromTx(clientID, tx)
		if trade != nil {
			if err := p.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
				slog.Warn("legacy.trade_sync_failed",
					"tx_id", txID,
					"client", clientID,
					"error", err)
			} else {
				slog.Info("legacy.trade_sync_upsert",
					"tx_id", txID,
					"client", clientID,
					"status", trade.Status,
					"venue", trade.Venue)
			}
		}
	}

	// 2. Emit final event
	if p.publisher != nil {
		finalSubject := "evt.trade." + strings.ToLower(status) + ".v1.XFX"
		if err := p.publisher.Publish(ctx, finalSubject, map[string]any{
			"client_id": clientID,
			"trade_id":  txID,
			"quote_id":  quoteID,
			"status":    status,
			"final":     true,
			"timestamp": time.Now().UTC(),
		}); err != nil {
			metrics.IncNATSPublishError(finalSubject)
			slog.Debug("nats.publish_failed",
				"subject", finalSubject,
				"error", err)
		}
	}

	slog.Info("xfx.trade_poll_complete",
		"tx_id", txID,
		"client", clientID,
		"final_status", status)
}
