package zodia

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
)

// Poller continuously checks Zodia transaction status for active trades,
// and periodically polls account balances.
type Poller struct {
	cfg          config.Config
	service      *Service
	publisher    *publisher.Publisher
	store        store.Store
	pollInterval time.Duration
	stopCh       chan struct{}
	activeTrades sync.Map // tradeID → cancel func
	tradeSync    *legacy.TradeSyncWriter
}

// NewPoller constructs a new Zodia poller.
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

// PollTradeStatus continuously polls Zodia for a transaction's status until it
// reaches a terminal state or the poller is stopped.
func (p *Poller) PollTradeStatus(
	parentCtx context.Context,
	clientID,
	quoteID,
	tradeID string,
) {
	// Prevent duplicate polling for the same trade
	if _, exists := p.activeTrades.Load(tradeID); exists {
		slog.Debug("zodia.trade_poll_already_active",
			"trade_id", tradeID,
			"client", clientID,
		)
		return
	}

	ctx, cancel := context.WithCancel(parentCtx)
	p.activeTrades.Store(tradeID, cancel)

	go func() {
		defer func() {
			p.activeTrades.Delete(tradeID)
			cancel()
		}()

		ticker := time.NewTicker(p.pollInterval)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				slog.Info("zodia.trade_poll_stopped",
					"trade_id", tradeID,
					"client", clientID,
					"last_status", lastStatus)
				return

			case <-p.stopCh:
				slog.Info("zodia.trade_poll_stopped",
					"trade_id", tradeID,
					"reason", "poller_shutdown")
				return

			case <-ticker.C:
				tx, err := p.service.FetchTransactionStatus(ctx, clientID, tradeID)
				if err != nil {
					slog.Warn("zodia.trade_poll_error",
						"trade_id", tradeID,
						"client", clientID,
						"error", err)
					continue
				}

				rawState := tx.State
				status := NormalizeTransactionState(rawState)

				// Emit status change only when it actually changes
				if status != lastStatus {
					lastStatus = status

					if p.publisher != nil {
						event := map[string]any{
							"client_id":  clientID,
							"trade_id":   tradeID,
							"quote_id":   quoteID,
							"status":     status,
							"raw":        rawState,
							"updated_at": time.Now().UTC(),
						}
						subject := "evt.trade.status_changed.v1.ZODIA"
						if err := p.publisher.Publish(ctx, subject, event); err != nil {
							metrics.IncNATSPublishError(subject)
							slog.Debug("nats.publish_failed",
								"subject", subject,
								"error", err)
						}
					}

					slog.Info("zodia.trade_status_changed",
						"trade_id", tradeID,
						"client", clientID,
						"raw_state", rawState,
						"normalized_status", status)
				}

				if IsTerminalState(rawState) {
					p.handleTerminalState(ctx, clientID, tradeID, quoteID, tx, status)
					return
				}
			}
		}
	}()
}

// StartBalancePolling starts periodic account-balance polling for the given client IDs.
func (p *Poller) StartBalancePolling(ctx context.Context, clients []string) {
	if len(clients) == 0 {
		return
	}
	slog.Info("zodia.balance_polling_started", "clients", len(clients))

	for _, client := range clients {
		if client == "" {
			continue
		}
		go p.pollBalancesOnce(ctx, client)
	}

	ticker := time.NewTicker(p.cfg.BalancePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, client := range clients {
				if client == "" {
					continue
				}
				go p.pollBalancesOnce(ctx, client)
			}
		case <-ctx.Done():
			slog.Info("zodia.balance_polling_stopped", "reason", "context_done")
			return
		case <-p.stopCh:
			slog.Info("zodia.balance_polling_stopped", "reason", "poller_shutdown")
			return
		}
	}
}

// pollBalancesOnce executes one balance poll cycle for a single client.
func (p *Poller) pollBalancesOnce(ctx context.Context, clientID string) {
	if err := p.service.FetchAndPublishBalances(ctx, clientID); err != nil {
		slog.Warn("zodia.balance_poll_failed",
			"client", clientID,
			"error", err)
	}
}

// handleTerminalState processes a trade that has reached a terminal state.
func (p *Poller) handleTerminalState(
	ctx context.Context,
	clientID,
	tradeID,
	quoteID string,
	tx *ZodiaTransaction,
	status string,
) {
	// 1. Sync to legacy database
	if p.tradeSync != nil {
		trade := p.service.BuildTradeConfirmationFromTransaction(clientID, tx)
		if trade != nil {
			if err := p.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
				slog.Warn("legacy.trade_sync_failed",
					"trade_id", tradeID,
					"client", clientID,
					"error", err)
			} else {
				slog.Info("legacy.trade_sync_upsert",
					"trade_id", tradeID,
					"client", clientID,
					"status", trade.Status,
					"venue", trade.Venue)
			}
		}
	}

	// 2. Emit final event
	if p.publisher != nil {
		finalSubject := "evt.trade." + strings.ToLower(status) + ".v1.ZODIA"
		if err := p.publisher.Publish(ctx, finalSubject, map[string]any{
			"client_id": clientID,
			"trade_id":  tradeID,
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

	slog.Info("zodia.trade_poll_complete",
		"trade_id", tradeID,
		"client", clientID,
		"final_status", status)
}
