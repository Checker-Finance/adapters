package rio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
)

// Service orchestrates Rio API operations: quote creation, order execution,
// and normalized event publishing to NATS.
type Service struct {
	ctx             context.Context
	cfg             config.Config
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
		slog.Error("rio.resolve_config_failed",
			"client", clientID,
			"error", err)
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new quote (RFQ) on Rio.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	slog.Info("rio.create_rfq.start",
		"client", req.ClientID,
		"pair", req.CurrencyPair,
		"side", req.Side,
		"amount", req.Amount,
	)

	// Resolve per-client configuration
	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	// Convert to Rio request format
	rioReq := s.mapper.ToRioQuoteRequest(req, clientCfg.Country)

	slog.Debug("rio.rfq_request",
		"json", pretty(rioReq))

	// Call Rio API with per-client config
	rioResp, err := s.client.CreateQuote(ctx, clientCfg, rioReq)
	if err != nil {
		slog.Error("rio.create_rfq.failed",
			"client", req.ClientID,
			"error", err)
		return nil, fmt.Errorf("rio quote creation failed: %w", err)
	}

	// Convert to canonical quote
	quote := s.mapper.FromRioQuote(rioResp, req.ClientID)

	slog.Info("rio.rfq_created",
		"client", req.ClientID,
		"quote_id", quote.ID,
		"price", quote.Price,
		"instrument", quote.Instrument,
	)

	return quote, nil
}

// ExecuteRFQ creates an order from an existing quote on Rio.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	slog.Info("rio.execute_rfq.start",
		"client", clientID,
		"quote_id", quoteID,
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
		slog.Error("rio.execute_rfq.failed",
			"client", clientID,
			"quote_id", quoteID,
			"error", err)
		return nil, fmt.Errorf("rio order creation failed: %w", err)
	}

	// Convert to canonical trade confirmation
	trade := s.mapper.FromRioOrder(orderResp, clientID)

	slog.Info("rio.order_created",
		"client", clientID,
		"order_id", trade.TradeID,
		"quote_id", quoteID,
		"status", trade.Status,
	)

	// Start async polling if not in terminal state.
	// Use the service-level context (s.ctx) — not the HTTP request context —
	// so polling survives after the HTTP response is sent.
	if !IsTerminalStatus(orderResp.Status) && s.poller != nil {
		slog.Info("rio.starting_status_poll",
			"order_id", trade.TradeID,
			"client", clientID)
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
		slog.Warn("rio.fetch_trade_status.failed",
			"client", clientID,
			"order_id", orderID,
			"error", err)
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
// Each client's callback URL is taken from its per-client config (WebhookURL field).
// Clients without a WebhookURL configured are skipped.
func (s *Service) RegisterOrderWebhook(ctx context.Context) error {
	clients, err := s.configResolver.DiscoverClients(ctx)
	if err != nil || len(clients) == 0 {
		slog.Warn("rio.register_webhook.no_clients",
			"error", err)
		return fmt.Errorf("no client configs available for webhook registration")
	}

	var lastErr error
	registered := 0
	for _, clientID := range clients {
		clientCfg, err := s.configResolver.Resolve(ctx, clientID)
		if err != nil {
			slog.Error("rio.register_webhook.resolve_failed",
				"client", clientID,
				"error", err)
			lastErr = err
			continue
		}

		if clientCfg.WebhookURL == "" {
			slog.Debug("rio.register_webhook.skipped_no_url",
				"client", clientID)
			continue
		}

		resp, err := s.client.RegisterWebhook(ctx, clientCfg, clientCfg.WebhookURL, true)
		if err != nil {
			slog.Error("rio.register_webhook.failed",
				"client", clientID,
				"callback_url", clientCfg.WebhookURL,
				"error", err)
			lastErr = err
			continue
		}

		registered++
		slog.Info("rio.webhook_registered",
			"client", clientID,
			"webhook_id", resp.ID,
			"url", resp.URL,
			"type", resp.Type)
	}

	if registered == 0 {
		return fmt.Errorf("webhook registration failed for all %d clients: %w", len(clients), lastErr)
	}

	slog.Info("rio.webhooks_registered",
		"registered", registered,
		"total", len(clients))

	return nil
}

// syncTerminalTrade syncs a terminal trade to the legacy database and publishes events.
func (s *Service) syncTerminalTrade(ctx context.Context, trade *model.TradeConfirmation) {
	// Sync to legacy database
	if s.tradeSyncWriter != nil {
		if err := s.tradeSyncWriter.SyncTradeUpsert(ctx, trade); err != nil {
			slog.Warn("rio.trade_sync_failed",
				"order_id", trade.TradeID,
				"client", trade.ClientID,
				"error", err)
		} else {
			slog.Info("rio.trade_sync_complete",
				"order_id", trade.TradeID,
				"client", trade.ClientID,
				"status", trade.Status)
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
		slog.Warn("rio.publish_failed",
			"subject", subject,
			"error", err)
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
