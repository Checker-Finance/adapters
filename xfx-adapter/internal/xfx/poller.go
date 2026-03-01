package xfx

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
)

// Poller continuously checks XFX transaction status for active trades.
// XFX does not support webhooks so polling is the only mechanism for status updates.
type Poller struct {
	logger       *zap.Logger
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
	logger *zap.Logger,
	cfg config.Config,
	service *Service,
	pub *publisher.Publisher,
	st store.Store,
	interval time.Duration,
	tradeSync *legacy.TradeSyncWriter,
) *Poller {
	return &Poller{
		logger:       logger,
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
		p.logger.Debug("xfx.trade_poll_already_active",
			zap.String("tx_id", txID),
			zap.String("client", clientID),
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
				p.logger.Info("xfx.trade_poll_stopped",
					zap.String("tx_id", txID),
					zap.String("client", clientID),
					zap.String("last_status", lastStatus))
				return

			case <-p.stopCh:
				p.logger.Info("xfx.trade_poll_stopped",
					zap.String("tx_id", txID),
					zap.String("reason", "poller_shutdown"))
				return

			case <-ticker.C:
				tx, err := p.service.FetchTransactionStatus(ctx, clientID, txID)
				if err != nil {
					p.logger.Warn("xfx.trade_poll_error",
						zap.String("tx_id", txID),
						zap.String("client", clientID),
						zap.Error(err))
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
							p.logger.Debug("nats.publish_failed",
								zap.String("subject", subject),
								zap.Error(err))
						}
					}

					p.logger.Info("xfx.trade_status_changed",
						zap.String("tx_id", txID),
						zap.String("client", clientID),
						zap.String("raw_status", rawStatus),
						zap.String("normalized_status", status))
				}

				if IsTerminalStatus(rawStatus) {
					p.handleTerminalStatus(ctx, clientID, txID, quoteID, tx, status)
					return
				}
			}
		}
	}()
}

// StartBalancePolling starts periodic account-balance polling for the given client IDs.
// It runs an immediate poll on startup and then on each tick of BalancePollInterval.
func (p *Poller) StartBalancePolling(ctx context.Context, clients []string) {
	if len(clients) == 0 {
		return
	}
	p.logger.Info("xfx.balance_polling_started", zap.Int("clients", len(clients)))

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
			p.logger.Info("xfx.balance_polling_stopped", zap.String("reason", "context_done"))
			return
		case <-p.stopCh:
			p.logger.Info("xfx.balance_polling_stopped", zap.String("reason", "poller_shutdown"))
			return
		}
	}
}

// pollBalancesOnce executes one balance poll cycle for a single client.
func (p *Poller) pollBalancesOnce(ctx context.Context, clientID string) {
	if err := p.service.FetchAndPublishBalances(ctx, clientID); err != nil {
		p.logger.Warn("xfx.balance_poll_failed",
			zap.String("client", clientID),
			zap.Error(err))
	}
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
				p.logger.Warn("legacy.trade_sync_failed",
					zap.String("tx_id", txID),
					zap.String("client", clientID),
					zap.Error(err))
			} else {
				p.logger.Info("legacy.trade_sync_upsert",
					zap.String("tx_id", txID),
					zap.String("client", clientID),
					zap.String("status", trade.Status),
					zap.String("venue", trade.Venue))
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
			p.logger.Debug("nats.publish_failed",
				zap.String("subject", finalSubject),
				zap.Error(err))
		}
	}

	p.logger.Info("xfx.trade_poll_complete",
		zap.String("tx_id", txID),
		zap.String("client", clientID),
		zap.String("final_status", status))
}
