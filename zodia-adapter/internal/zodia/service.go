package zodia

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// Service orchestrates Zodia API operations: quote creation via WebSocket RFS,
// trade execution, status tracking, and normalized event publishing to NATS.
type Service struct {
	ctx             context.Context
	cfg             config.Config
	logger          *zap.Logger
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
	logger *zap.Logger,
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
		logger:          logger,
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
		s.logger.Error("zodia.resolve_config_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new executable quote via the Zodia WebSocket RFS flow.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	s.logger.Info("zodia.create_rfq.start",
		zap.String("client", req.ClientID),
		zap.String("pair", req.CurrencyPair),
		zap.String("side", req.Side),
		zap.Float64("amount", req.Amount),
	)

	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	sess, err := s.sessionMgr.GetOrCreate(ctx, req.ClientID, clientCfg)
	if err != nil {
		s.logger.Error("zodia.create_rfq.session_failed",
			zap.String("client", req.ClientID),
			zap.Error(err))
		return nil, fmt.Errorf("zodia: get ws session: %w", err)
	}

	instrument := ToZodiaPair(req.CurrencyPair)

	pricePayload, err := sess.RequestPrice(ctx, instrument, req.Side, req.Amount)
	if err != nil {
		s.logger.Error("zodia.create_rfq.price_failed",
			zap.String("client", req.ClientID),
			zap.String("instrument", instrument),
			zap.Error(err))
		return nil, fmt.Errorf("zodia ws price request failed: %w", err)
	}

	quote := s.mapper.MapWSPriceToQuote(*pricePayload, req)

	s.logger.Info("zodia.rfq_created",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quote.ID),
		zap.Float64("price", quote.Price),
		zap.String("instrument", quote.Instrument),
	)

	return quote, nil
}

// ExecuteRFQ executes an existing quote on Zodia via the WebSocket RFS flow.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	s.logger.Info("zodia.execute_rfq.start",
		zap.String("client", clientID),
		zap.String("quote_id", quoteID),
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
		s.logger.Error("zodia.execute_rfq.failed",
			zap.String("client", clientID),
			zap.String("quote_id", quoteID),
			zap.Error(err))
		return nil, fmt.Errorf("zodia ws execute order failed: %w", err)
	}

	trade := s.mapper.MapWSOrderToTrade(*confirm, clientID, quoteID)

	s.logger.Info("zodia.trade_created",
		zap.String("client", clientID),
		zap.String("trade_id", trade.TradeID),
		zap.String("quote_id", quoteID),
		zap.String("status", trade.Status),
	)

	// Start async polling if not in terminal state.
	if !IsTerminalState(confirm.Status) && s.poller != nil {
		s.logger.Info("zodia.starting_status_poll",
			zap.String("trade_id", trade.TradeID),
			zap.String("client", clientID))
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
		s.logger.Warn("zodia.fetch_tx_status.failed",
			zap.String("client", clientID),
			zap.String("trade_id", tradeID),
			zap.Error(err))
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
			s.logger.Warn("zodia.trade_sync_failed",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.Error(err))
		} else {
			s.logger.Info("zodia.trade_sync_complete",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.String("status", trade.Status))
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
		s.logger.Warn("zodia.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
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
		s.logger.Warn("zodia.fetch_balances.failed",
			zap.String("client", clientID),
			zap.Error(err))
		return fmt.Errorf("zodia get accounts: %w", err)
	}

	balances := s.mapper.MapAccountToBalances(resp, clientID)
	for _, bal := range balances {
		if err := s.store.RecordBalanceEvent(ctx, bal); err != nil {
			s.logger.Warn("zodia.balance_event_failed",
				zap.String("instrument", bal.Instrument),
				zap.Error(err))
		}
		if err := s.store.UpdateBalanceSnapshot(ctx, bal); err != nil {
			s.logger.Warn("zodia.balance_snapshot_failed",
				zap.String("instrument", bal.Instrument),
				zap.Error(err))
		}
		if s.publisher != nil {
			if err := s.publisher.Publish(ctx, "evt.balance.update.v1", bal); err != nil {
				metrics.IncNATSPublishError("evt.balance.update.v1")
				s.logger.Warn("zodia.balance_publish_failed",
					zap.String("instrument", bal.Instrument),
					zap.Error(err))
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
		s.logger.Warn("zodia.list_products.resolve_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return zodiaSupportedProducts
	}

	resp, err := s.restClient.GetInstruments(ctx, clientCfg)
	if err != nil {
		s.logger.Warn("zodia.list_products.fetch_failed",
			zap.String("client", clientID),
			zap.Error(err))
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
	s.logger.Info("zodia.handle_quote_request",
		zap.String("tenant_id", env.TenantID),
		zap.String("client_id", env.ClientID),
		zap.String("instrument", req.Instrument),
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
		s.logger.Error("zodia.handle_quote_request.failed",
			zap.String("client", env.ClientID),
			zap.Error(err))
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
		s.logger.Warn("zodia.handle_quote_request.publish_failed",
			zap.String("subject", s.cfg.OutboundSubject),
			zap.Error(err))
	}

	return nil
}

// HandleTradeExecute processes a NATS trade execute command by executing the RFQ
// and publishing the trade confirmation event.
func (s *Service) HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error {
	s.logger.Info("zodia.handle_trade_execute",
		zap.String("tenant_id", env.TenantID),
		zap.String("client_id", env.ClientID),
		zap.String("quote_id", cmd.QuoteID),
	)

	trade, err := s.ExecuteRFQ(ctx, cmd.ClientID, cmd.QuoteID)
	if err != nil {
		s.logger.Error("zodia.handle_trade_execute.failed",
			zap.String("client", cmd.ClientID),
			zap.String("quote_id", cmd.QuoteID),
			zap.Error(err))
		return err
	}

	subject := "evt.trade." + trade.Status + ".v1.ZODIA"
	if err := s.publisher.Publish(ctx, subject, trade); err != nil {
		metrics.IncNATSPublishError(subject)
		s.logger.Warn("zodia.handle_trade_execute.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
	}

	return nil
}

// Config returns the service configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}
