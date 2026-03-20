package zodia

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
)

// Service orchestrates Zodia API operations: quote creation via WebSocket RFS,
// trade execution, status tracking, and normalized event publishing to NATS.
type Service struct {
	ctx             context.Context
	cfg             config.Config
	nc              *nats.Conn
	restClient      *RESTClient
	sessionMgr      *SessionManager
	configResolver  ConfigResolver
	publisher       *publisher.Publisher
	store           store.Store
	mapper          *Mapper
	tradeSyncWriter *legacy.TradeSyncWriter
	poller          *Poller
}

// NewService constructs a fully wired Zodia adapter service.
func NewService(
	ctx context.Context,
	cfg config.Config,
	nc *nats.Conn,
	restClient *RESTClient,
	sessionMgr *SessionManager,
	resolver ConfigResolver,
	pub *publisher.Publisher,
	st store.Store,
	tradeSyncWriter *legacy.TradeSyncWriter,
) *Service {
	return &Service{
		ctx:             ctx,
		cfg:             cfg,
		nc:              nc,
		restClient:      restClient,
		sessionMgr:      sessionMgr,
		configResolver:  resolver,
		publisher:       pub,
		store:           st,
		mapper:          NewMapper(),
		tradeSyncWriter: tradeSyncWriter,
	}
}

// SetPoller sets the poller reference for async trade status tracking.
func (s *Service) SetPoller(p *Poller) {
	s.poller = p
}

// resolveConfig resolves the per-client Zodia configuration.
func (s *Service) resolveConfig(ctx context.Context, clientID string) (*ZodiaClientConfig, error) {
	cfg, err := s.configResolver.Resolve(ctx, clientID)
	if err != nil {
		slog.Error("zodia.resolve_config_failed",
			"client", clientID,
			"error", err)
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new executable quote via the Zodia WebSocket RFS flow.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	slog.Info("zodia.create_rfq.start",
		"client", req.ClientID,
		"pair", req.CurrencyPair,
		"side", req.Side,
		"amount", req.Amount,
	)

	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	sess, err := s.sessionMgr.GetOrCreate(ctx, req.ClientID, clientCfg)
	if err != nil {
		slog.Error("zodia.create_rfq.session_failed",
			"client", req.ClientID,
			"error", err)
		return nil, fmt.Errorf("zodia: get ws session: %w", err)
	}

	instrument := ToZodiaPair(req.CurrencyPair)

	pricePayload, err := sess.RequestPrice(ctx, instrument, req.Side, req.Amount)
	if err != nil {
		slog.Error("zodia.create_rfq.price_failed",
			"client", req.ClientID,
			"instrument", instrument,
			"error", err)
		return nil, fmt.Errorf("zodia ws price request failed: %w", err)
	}

	quote := s.mapper.MapWSPriceToQuote(*pricePayload, req)

	slog.Info("zodia.rfq_created",
		"client", req.ClientID,
		"quote_id", quote.ID,
		"price", quote.Price,
		"instrument", quote.Instrument,
	)

	return quote, nil
}

// ExecuteRFQ executes an existing quote on Zodia via the WebSocket RFS flow.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	slog.Info("zodia.execute_rfq.start",
		"client", clientID,
		"quote_id", quoteID,
	)

	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	sess, err := s.sessionMgr.GetOrCreate(ctx, clientID, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("zodia: get ws session: %w", err)
	}

	confirm, err := sess.ExecuteOrder(ctx, quoteID)
	if err != nil {
		slog.Error("zodia.execute_rfq.failed",
			"client", clientID,
			"quote_id", quoteID,
			"error", err)
		return nil, fmt.Errorf("zodia ws execute order failed: %w", err)
	}

	trade := s.mapper.MapWSOrderToTrade(*confirm, clientID, quoteID)

	slog.Info("zodia.trade_created",
		"client", clientID,
		"trade_id", trade.TradeID,
		"quote_id", quoteID,
		"status", trade.Status,
	)

	// Start async polling if not in terminal state.
	if !IsTerminalState(confirm.Status) && s.poller != nil {
		slog.Info("zodia.starting_status_poll",
			"trade_id", trade.TradeID,
			"client", clientID)
		go s.poller.PollTradeStatus(s.ctx, clientID, quoteID, trade.TradeID)
	} else if IsTerminalState(confirm.Status) {
		s.syncTerminalTrade(ctx, trade)
	}

	return trade, nil
}

// FetchTransactionStatus retrieves the latest transaction status from Zodia REST API.
func (s *Service) FetchTransactionStatus(ctx context.Context, clientID, tradeID string) (*ZodiaTransaction, error) {
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	resp, err := s.restClient.ListTransactions(ctx, clientCfg, ZodiaTransactionFilter{
		TradeID: tradeID,
		Type:    "RFSTRADE",
	})
	if err != nil {
		slog.Warn("zodia.fetch_tx_status.failed",
			"client", clientID,
			"trade_id", tradeID,
			"error", err)
		return nil, err
	}

	for i := range resp.Result {
		if resp.Result[i].TradeID == tradeID {
			return &resp.Result[i], nil
		}
	}

	return nil, fmt.Errorf("zodia: transaction %q not found", tradeID)
}

// BuildTradeConfirmationFromTransaction converts a Zodia transaction to a canonical TradeConfirmation.
func (s *Service) BuildTradeConfirmationFromTransaction(clientID string, tx *ZodiaTransaction) *model.TradeConfirmation {
	return s.mapper.MapTransactionToTrade(tx, clientID)
}

// syncTerminalTrade syncs a terminal trade to the legacy database and publishes final event.
func (s *Service) syncTerminalTrade(ctx context.Context, trade *model.TradeConfirmation) {
	if s.tradeSyncWriter != nil {
		if err := s.tradeSyncWriter.SyncTradeUpsert(ctx, trade); err != nil {
			slog.Warn("zodia.trade_sync_failed",
				"trade_id", trade.TradeID,
				"client", trade.ClientID,
				"error", err)
		} else {
			slog.Info("zodia.trade_sync_complete",
				"trade_id", trade.TradeID,
				"client", trade.ClientID,
				"status", trade.Status)
		}
	}

	if s.publisher == nil {
		return
	}
	subject := "evt.trade." + trade.Status + ".v1.ZODIA"
	if err := s.publisher.Publish(ctx, subject, map[string]any{
		"client_id": trade.ClientID,
		"trade_id":  trade.TradeID,
		"status":    trade.Status,
		"final":     true,
		"timestamp": time.Now().UTC(),
	}); err != nil {
		metrics.IncNATSPublishError(subject)
		slog.Warn("zodia.publish_failed",
			"subject", subject,
			"error", err)
	}
}

// ResolveTransaction fetches live transaction status and returns a canonical TradeConfirmation.
// Used by the resolve-order endpoint.
func (s *Service) ResolveTransaction(ctx context.Context, clientID, tradeID string) (*model.TradeConfirmation, error) {
	tx, err := s.FetchTransactionStatus(ctx, clientID, tradeID)
	if err != nil {
		return nil, err
	}
	trade := s.BuildTradeConfirmationFromTransaction(clientID, tx)
	if trade == nil {
		return nil, fmt.Errorf("could not build trade confirmation for transaction %s", tradeID)
	}
	return trade, nil
}

// FetchAndPublishBalances fetches Zodia account balances, persists them, and publishes NATS events.
func (s *Service) FetchAndPublishBalances(ctx context.Context, clientID string) error {
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return err
	}

	resp, err := s.restClient.GetAccounts(ctx, clientCfg)
	if err != nil {
		slog.Warn("zodia.fetch_balances.failed",
			"client", clientID,
			"error", err)
		return fmt.Errorf("zodia get accounts: %w", err)
	}

	balances := s.mapper.MapAccountToBalances(resp, clientID)
	for _, bal := range balances {
		if err := s.store.RecordBalanceEvent(ctx, bal); err != nil {
			slog.Warn("zodia.balance_event_failed",
				"instrument", bal.Instrument,
				"error", err)
		}
		if err := s.store.UpdateBalanceSnapshot(ctx, bal); err != nil {
			slog.Warn("zodia.balance_snapshot_failed",
				"instrument", bal.Instrument,
				"error", err)
		}
		if s.publisher != nil {
			if err := s.publisher.Publish(ctx, "evt.balance.update.v1", bal); err != nil {
				metrics.IncNATSPublishError("evt.balance.update.v1")
				slog.Warn("zodia.balance_publish_failed",
					"instrument", bal.Instrument,
					"error", err)
			}
		}
	}
	return nil
}

// ListProducts returns the Zodia supported products.
// Attempts to fetch from the REST API; falls back to static list on error.
func (s *Service) ListProducts(ctx context.Context, clientID string) []model.Product {
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		slog.Warn("zodia.list_products.resolve_failed",
			"client", clientID,
			"error", err)
		return zodiaSupportedProducts
	}

	resp, err := s.restClient.GetInstruments(ctx, clientCfg)
	if err != nil {
		slog.Warn("zodia.list_products.fetch_failed",
			"client", clientID,
			"error", err)
		return zodiaSupportedProducts
	}

	if len(resp.Instruments) == 0 {
		return zodiaSupportedProducts
	}

	products := make([]model.Product, 0, len(resp.Instruments))
	for _, instr := range resp.Instruments {
		if instr.Status == "" || instr.Status == "active" {
			products = append(products, s.mapper.MapInstrumentToProduct(instr))
		}
	}
	return products
}

// HandleQuoteRequest processes a NATS quote request command by creating an RFQ
// and publishing the quote response to the outbound subject.
func (s *Service) HandleQuoteRequest(ctx context.Context, env model.Envelope, req model.QuoteRequest) error {
	slog.Info("zodia.handle_quote_request",
		"tenant_id", env.TenantID,
		"client_id", env.ClientID,
		"instrument", req.Instrument,
	)

	rfqReq := model.RFQRequest{
		TenantID:      env.TenantID,
		ClientID:      env.ClientID,
		CurrencyPair:  req.Instrument,
		Side:          req.Side,
		Amount:        req.Quantity,
		CorrelationID: env.CorrelationID.String(),
		RequestTime:   req.Timestamp,
	}

	quote, err := s.CreateRFQ(ctx, rfqReq)
	if err != nil {
		slog.Error("zodia.handle_quote_request.failed",
			"client", env.ClientID,
			"error", err)
		return err
	}

	resp := model.QuoteResponse{
		ID:             quote.ID,
		QuoteRequestID: req.RequestID.String(),
		Instrument:     quote.Instrument,
		Venue:          "ZODIA",
		Side:           req.Side,
		Quantity:       req.Quantity,
		ExpiresAt:      quote.ExpiresAt,
		ReceivedAt:     time.Now().UTC(),
	}

	if err := s.publisher.Publish(ctx, s.cfg.OutboundSubject, resp); err != nil {
		metrics.IncNATSPublishError(s.cfg.OutboundSubject)
		slog.Warn("zodia.handle_quote_request.publish_failed",
			"subject", s.cfg.OutboundSubject,
			"error", err)
	}

	return nil
}

// HandleTradeExecute processes a NATS trade execute command by executing the RFQ
// and publishing the trade confirmation event.
func (s *Service) HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error {
	slog.Info("zodia.handle_trade_execute",
		"tenant_id", env.TenantID,
		"client_id", env.ClientID,
		"quote_id", cmd.QuoteID,
	)

	trade, err := s.ExecuteRFQ(ctx, cmd.ClientID, cmd.QuoteID)
	if err != nil {
		slog.Error("zodia.handle_trade_execute.failed",
			"client", cmd.ClientID,
			"quote_id", cmd.QuoteID,
			"error", err)
		return err
	}

	subject := "evt.trade." + trade.Status + ".v1.ZODIA"
	if err := s.publisher.Publish(ctx, subject, trade); err != nil {
		metrics.IncNATSPublishError(subject)
		slog.Warn("zodia.handle_trade_execute.publish_failed",
			"subject", subject,
			"error", err)
	}

	return nil
}

// Config returns the service configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}
