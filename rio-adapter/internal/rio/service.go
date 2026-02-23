package rio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// Service orchestrates Rio API operations: quote creation, order execution,
// and normalized event publishing to NATS.
type Service struct {
	ctx             context.Context
	cfg             config.Config
	logger          *zap.Logger
	nc              *nats.Conn
	client          *Client
	configResolver  ConfigResolver
	publisher       *publisher.Publisher
	store           store.Store
	mapper          *Mapper
	tradeSyncWriter *legacy.TradeSyncWriter
	poller          *Poller
}

// NewService constructs a fully wired Rio adapter service.
func NewService(
	ctx context.Context,
	cfg config.Config,
	logger *zap.Logger,
	nc *nats.Conn,
	client *Client,
	resolver ConfigResolver,
	pub *publisher.Publisher,
	st store.Store,
	tradeSyncWriter *legacy.TradeSyncWriter,
) *Service {
	return &Service{
		ctx:             ctx,
		cfg:             cfg,
		logger:          logger,
		nc:              nc,
		client:          client,
		configResolver:  resolver,
		publisher:       pub,
		store:           st,
		mapper:          NewMapper(),
		tradeSyncWriter: tradeSyncWriter,
	}
}

// SetPoller sets the poller reference for async trade tracking.
func (s *Service) SetPoller(p *Poller) {
	s.poller = p
}

// resolveConfig resolves the per-client Rio configuration, returning an error if not found.
func (s *Service) resolveConfig(ctx context.Context, clientID string) (*RioClientConfig, error) {
	cfg, err := s.configResolver.Resolve(ctx, clientID)
	if err != nil {
		s.logger.Error("rio.resolve_config_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new quote (RFQ) on Rio.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	s.logger.Info("rio.create_rfq.start",
		zap.String("client", req.ClientID),
		zap.String("pair", req.CurrencyPair),
		zap.String("side", req.Side),
		zap.Float64("amount", req.Amount),
	)

	// Resolve per-client configuration
	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	// Convert to Rio request format
	rioReq := s.mapper.ToRioQuoteRequest(req, clientCfg.Country)

	s.logger.Debug("rio.rfq_request",
		zap.String("json", pretty(rioReq)))

	// Call Rio API with per-client config
	rioResp, err := s.client.CreateQuote(ctx, clientCfg, rioReq)
	if err != nil {
		s.logger.Error("rio.create_rfq.failed",
			zap.String("client", req.ClientID),
			zap.Error(err))
		return nil, fmt.Errorf("rio quote creation failed: %w", err)
	}

	// Convert to canonical quote
	quote := s.mapper.FromRioQuote(rioResp, req.ClientID)

	s.logger.Info("rio.rfq_created",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quote.ID),
		zap.Float64("price", quote.Price),
		zap.String("instrument", quote.Instrument),
	)

	return quote, nil
}

// ExecuteRFQ creates an order from an existing quote on Rio.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	s.logger.Info("rio.execute_rfq.start",
		zap.String("client", clientID),
		zap.String("quote_id", quoteID),
	)

	// Resolve per-client configuration
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// Create order request
	orderReq := &RioOrderRequest{
		QuoteID:           quoteID,
		ClientReferenceID: clientID,
	}

	// Call Rio API with per-client config
	orderResp, err := s.client.CreateOrder(ctx, clientCfg, orderReq)
	if err != nil {
		s.logger.Error("rio.execute_rfq.failed",
			zap.String("client", clientID),
			zap.String("quote_id", quoteID),
			zap.Error(err))
		return nil, fmt.Errorf("rio order creation failed: %w", err)
	}

	// Convert to canonical trade confirmation
	trade := s.mapper.FromRioOrder(orderResp, clientID)

	s.logger.Info("rio.order_created",
		zap.String("client", clientID),
		zap.String("order_id", trade.TradeID),
		zap.String("quote_id", quoteID),
		zap.String("status", trade.Status),
	)

	// Start async polling if not in terminal state.
	// Use the service-level context (s.ctx) — not the HTTP request context —
	// so polling survives after the HTTP response is sent.
	if !IsTerminalStatus(orderResp.Status) && s.poller != nil {
		s.logger.Info("rio.starting_status_poll",
			zap.String("order_id", trade.TradeID),
			zap.String("client", clientID))
		go s.poller.PollTradeStatus(s.ctx, clientID, quoteID, trade.TradeID)
	} else if IsTerminalStatus(orderResp.Status) {
		// Immediately sync terminal trades
		s.syncTerminalTrade(ctx, trade)
	}

	return trade, nil
}

// FetchTradeStatus retrieves the latest order status from Rio.
func (s *Service) FetchTradeStatus(ctx context.Context, clientID, orderID string) (*RioOrderResponse, error) {
	// Resolve per-client configuration
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	order, err := s.client.GetOrder(ctx, clientCfg, orderID)
	if err != nil {
		s.logger.Warn("rio.fetch_trade_status.failed",
			zap.String("client", clientID),
			zap.String("order_id", orderID),
			zap.Error(err))
		return nil, err
	}

	return order, nil
}

// BuildTradeConfirmationFromOrder converts a Rio order response to a canonical TradeConfirmation.
func (s *Service) BuildTradeConfirmationFromOrder(clientID, orderID string, order *RioOrderResponse) *model.TradeConfirmation {
	if order == nil {
		return nil
	}

	return s.mapper.FromRioOrder(order, clientID)
}

// RegisterOrderWebhook registers a webhook for order status changes for all discovered clients.
func (s *Service) RegisterOrderWebhook(ctx context.Context, callbackURL string) error {
	clients, err := s.configResolver.DiscoverClients(ctx)
	if err != nil || len(clients) == 0 {
		s.logger.Warn("rio.register_webhook.no_clients",
			zap.Error(err))
		return fmt.Errorf("no client configs available for webhook registration")
	}

	var lastErr error
	registered := 0
	for _, clientID := range clients {
		clientCfg, err := s.configResolver.Resolve(ctx, clientID)
		if err != nil {
			s.logger.Error("rio.register_webhook.resolve_failed",
				zap.String("client", clientID),
				zap.Error(err))
			lastErr = err
			continue
		}

		resp, err := s.client.RegisterWebhook(ctx, clientCfg, callbackURL, true)
		if err != nil {
			s.logger.Error("rio.register_webhook.failed",
				zap.String("client", clientID),
				zap.String("callback_url", callbackURL),
				zap.Error(err))
			lastErr = err
			continue
		}

		registered++
		s.logger.Info("rio.webhook_registered",
			zap.String("client", clientID),
			zap.String("webhook_id", resp.ID),
			zap.String("url", resp.URL),
			zap.String("type", resp.Type))
	}

	if registered == 0 {
		return fmt.Errorf("webhook registration failed for all %d clients: %w", len(clients), lastErr)
	}

	s.logger.Info("rio.webhooks_registered",
		zap.Int("registered", registered),
		zap.Int("total", len(clients)))

	return nil
}

// syncTerminalTrade syncs a terminal trade to the legacy database and publishes events.
func (s *Service) syncTerminalTrade(ctx context.Context, trade *model.TradeConfirmation) {
	// Sync to legacy database
	if s.tradeSyncWriter != nil {
		if err := s.tradeSyncWriter.SyncTradeUpsert(ctx, trade); err != nil {
			s.logger.Warn("rio.trade_sync_failed",
				zap.String("order_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.Error(err))
		} else {
			s.logger.Info("rio.trade_sync_complete",
				zap.String("order_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.String("status", trade.Status))
		}
	}

	// Publish final event
	if s.publisher == nil {
		return
	}
	subject := "evt.trade." + trade.Status + ".v1.RIO"
	if err := s.publisher.Publish(ctx, subject, map[string]any{
		"client_id": trade.ClientID,
		"order_id":  trade.TradeID,
		"status":    trade.Status,
		"final":     true,
		"timestamp": time.Now().UTC(),
	}); err != nil {
		s.logger.Warn("rio.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
	}
}

// Config returns the service configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}

// pretty formats a value as indented JSON for logging.
func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
