package braza

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/legacy"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/publisher"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/store"
)

// Poller handles scheduled polling of Braza balances and order/trade status.
type Poller struct {
	logger         *zap.Logger
	cfg            config.Config
	service        *Service
	publisher      *publisher.Publisher
	store          store.Store
	secretProvider secrets.Provider
	authMgr        *auth.Manager
	cache          *auth.CacheAdapter
	pollInterval   time.Duration
	stopCh         chan struct{}

	activeTrades sync.Map // order_id -> cancel function
	tradeSync    *legacy.TradeSyncWriter
}

// NewPoller constructs a new Braza poller (balances + trade tracking).
func NewPoller(
	logger *zap.Logger,
	cfg config.Config,
	service *Service,
	pub *publisher.Publisher,
	st store.Store,
	secretProvider secrets.Provider,
	authMgr *auth.Manager,
	cache *auth.CacheAdapter,
	interval time.Duration,
	tradeSync *legacy.TradeSyncWriter,
) *Poller {
	return &Poller{
		logger:         logger,
		cfg:            cfg,
		service:        service,
		publisher:      pub,
		store:          st,
		secretProvider: secretProvider,
		authMgr:        authMgr,
		cache:          cache,
		pollInterval:   interval,
		stopCh:         make(chan struct{}),
		tradeSync:      tradeSync,
	}
}

// Start begins periodic polling for all known tenant/client pairs.
func (p *Poller) Start(ctx context.Context, clients []string) {
	p.logger.Info("braza.poller_started",
		zap.Int("clients", len(clients)))

	// Kick off an immediate balance poll for each client at startup
	for _, client := range clients {
		if client == "" {
			continue
		}
		go p.pollBalancesOnce(ctx, client)
	}

	ticker := time.NewTicker(p.pollInterval)
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
			p.logger.Info("braza.poller_stopped (context cancelled)")
			return
		case <-p.stopCh:
			p.logger.Info("braza.poller_stopped (manual stop)")
			return
		}
	}
}

// Stop signals the poller to stop gracefully.
func (p *Poller) Stop() {
	close(p.stopCh)
}

// pollBalancesOnce executes one balance poll for a single client.
func (p *Poller) pollBalancesOnce(ctx context.Context, clientID string) {
	// Resolve Braza credentials
	rcreds, err := p.secretProvider.Resolve(ctx, p.cfg, clientID, "braza")
	if err != nil {
		p.logger.Warn("braza.resolve_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return
	}

	authCreds := auth.Credentials{
		Username: rcreds.Username,
		Password: rcreds.Password,
	}

	// Fetch and publish balances
	if err := p.service.FetchAndPublishBalances(ctx, clientID, p.publisher, p.store, authCreds); err != nil {
		p.logger.Warn("braza.poll_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return
	}
}

// PollTradeStatus continuously checks a Braza order until it reaches a terminal state.
func (p *Poller) PollTradeStatus(
	parentCtx context.Context,
	clientID,
	externalOrderID,
	orderID string,
	creds auth.Credentials,
) {

	// Prevent duplicate polling for the same order
	if _, exists := p.activeTrades.Load(externalOrderID); exists {
		p.logger.Debug("braza.trade_poll_already_active",
			zap.String("external_order_id", externalOrderID),
			zap.String("client", clientID),
		)
		return
	}

	// Create *dedicated child context* for this poller
	ctx, cancel := context.WithCancel(parentCtx)
	p.activeTrades.Store(externalOrderID, cancel)

	go func() {
		defer func() {
			p.activeTrades.Delete(externalOrderID)
			cancel() // ensure cleanup
		}()

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				p.logger.Info("braza.trade_poll_stopped",
					zap.String("external_order_id", externalOrderID),
					zap.String("client", clientID),
					zap.String("last_status", lastStatus))
				return

			case <-ticker.C:
				// IMPORTANT: use child ctx
				order, err := p.service.FetchTradeStatus(ctx, clientID, externalOrderID, creds)
				if err != nil {
					p.logger.Warn("braza.trade_poll_error",
						zap.String("external_order_id", externalOrderID),
						zap.String("client", clientID),
						zap.Error(err))
					continue
				}

				rawStatus := order.Status
				status := NormalizeOrderStatus(rawStatus)

				// Emit status change only when it actually changes
				if status != lastStatus {
					lastStatus = status

					event := map[string]any{
						"client_id":         clientID,
						"order_id":          orderID,
						"external_order_id": externalOrderID,
						"status":            status,
						"updated_at":        time.Now().UTC(),
					}

					subject := "evt.trade.status_changed.v1.BRAZA"
					if err := p.publisher.Publish(ctx, subject, event); err != nil {
						p.logger.Debug("nats.publish_failed",
							zap.String("subject", subject),
							zap.Error(err))
					}

					p.logger.Info("braza.trade_status_changed",
						zap.String("order_id", orderID),
						zap.String("client", clientID),
						zap.String("raw_status", rawStatus),
						zap.String("normalized_status", status))
				}

				// --- Terminal Status Handling ---
				if isTerminalStatus(status) {

					// --- 1. Sync into legacy.activity.t_order ---
					if p.tradeSync != nil {
						trade := p.service.BuildTradeConfirmationFromOrder(clientID, orderID, order)
						if trade != nil {
							if err := p.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
								p.logger.Warn("legacy.trade_sync_failed",
									zap.String("order_id", trade.TradeID),
									zap.String("external_order_id", externalOrderID),
									zap.String("client_id", trade.ClientID),
									zap.Error(err),
								)
							} else {
								p.logger.Info("legacy.trade_sync_upsert",
									zap.String("order_id", trade.TradeID),
									zap.String("client_id", trade.ClientID),
									zap.String("status", trade.Status),
									zap.String("venue", trade.Venue),
								)
							}
						} else {
							p.logger.Warn("legacy.trade_sync_skipped",
								zap.String("order_id", orderID),
								zap.String("external_order_id", externalOrderID),
								zap.String("client", clientID),
								zap.String("reason", "nil_trade_from_braza_status"),
							)
						}
					}

					// --- 2. Emit final event ---
					finalSubject := "evt.trade." + strings.ToLower(status) + ".v1.BRAZA"
					if err := p.publisher.Publish(ctx, finalSubject, map[string]any{
						"client_id":         clientID,
						"order_id":          orderID,
						"external_order_id": externalOrderID,
						"status":            status,
						"final":             true,
						"timestamp":         time.Now().UTC(),
					}); err != nil {
						p.logger.Debug("nats.publish_failed",
							zap.String("subject", finalSubject),
							zap.Error(err))
					}

					p.logger.Info("braza.trade_poll_complete",
						zap.String("order_id", orderID),
						zap.String("external_order_id", externalOrderID),
						zap.String("client", clientID),
						zap.String("final_status", status))

					return
				}
			}
		}
	}()
}

// isTerminalStatus encapsulates what we consider a final / non-pollable state.
// These should be the *normalized* statuses returned by NormalizeOrderStatus.
func isTerminalStatus(status string) bool {
	switch status {
	case "COMPLETED", "FAILED", "CANCELED", "CANCELLED", "FILLED", "REJECTED":
		return true
	default:
		return false
	}
}
