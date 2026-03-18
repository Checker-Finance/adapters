package capa

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
	"github.com/Checker-Finance/adapters/capa-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/capa-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// Service orchestrates Capa API operations: quote creation, trade execution,
// status tracking, and normalized event publishing to NATS.
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

// NewService constructs a fully wired Capa adapter service.
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

// SetPoller sets the poller reference for async trade status tracking.
func (s *Service) SetPoller(p *Poller) {
	s.poller = p
}

// resolveConfig resolves the per-client Capa configuration.
func (s *Service) resolveConfig(ctx context.Context, clientID string) (*CapaClientConfig, error) {
	cfg, err := s.configResolver.Resolve(ctx, clientID)
	if err != nil {
		s.logger.Error("capa.resolve_config_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new executable quote on Capa, routing to the correct endpoint
// based on the transaction type detected from the currency pair.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	s.logger.Info("capa.create_rfq.start",
		zap.String("client", req.ClientID),
		zap.String("pair", req.CurrencyPair),
		zap.String("side", req.Side),
		zap.Float64("amount", req.Amount),
	)

	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	txType := DetectTransactionType(req.CurrencyPair)
	s.logger.Debug("capa.rfq.tx_type",
		zap.String("pair", req.CurrencyPair),
		zap.String("tx_type", string(txType)))

	var quoteResp *CapaQuoteResponse
	switch txType {
	case CrossRamp:
		capaReq := s.mapper.ToCrossRampQuoteRequest(req, clientCfg.UserID)
		s.logger.Debug("capa.cross_ramp_quote_request", zap.String("json", pretty(capaReq)))
		quoteResp, err = s.client.GetCrossRampQuote(ctx, clientCfg, capaReq)
	default: // OnRamp or OffRamp
		capaReq := s.mapper.ToOnOffRampQuoteRequest(req, clientCfg.UserID, txType)
		s.logger.Debug("capa.on_off_ramp_quote_request", zap.String("json", pretty(capaReq)))
		quoteResp, err = s.client.GetQuote(ctx, clientCfg, capaReq)
	}

	if err != nil {
		s.logger.Error("capa.create_rfq.failed",
			zap.String("client", req.ClientID),
			zap.Error(err))
		return nil, fmt.Errorf("capa quote creation failed: %w", err)
	}

	quote := s.mapper.FromCapaQuote(quoteResp, req.ClientID)

	s.logger.Info("capa.rfq_created",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quote.ID),
		zap.Float64("price", quote.Price),
		zap.String("instrument", quote.Instrument),
	)

	return quote, nil
}

// ExecuteRFQ executes an existing quote on Capa, creating a transaction.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	s.logger.Info("capa.execute_rfq.start",
		zap.String("client", clientID),
		zap.String("quote_id", quoteID),
	)

	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// Determine transaction type from per-client config routing fields.
	// Clients are configured for a specific flow:
	//   WalletAddress set → on-ramp (fiat→crypto)
	//   ReceiverID set    → off-ramp (crypto→fiat)
	//   otherwise         → cross-ramp (fiat→fiat)
	txType := clientTxType(clientCfg)

	var execResp *CapaExecuteResponse
	switch txType {
	case OnRamp:
		execReq := s.mapper.ToOnRampExecuteRequest(clientCfg.UserID, quoteID, clientCfg)
		execResp, err = s.client.CreateOnRamp(ctx, clientCfg, execReq)
	case OffRamp:
		execReq := s.mapper.ToOffRampExecuteRequest(clientCfg.UserID, quoteID, clientCfg)
		execResp, err = s.client.CreateOffRamp(ctx, clientCfg, execReq)
	default: // CrossRamp
		execReq := s.mapper.ToCrossRampExecuteRequest(clientCfg.UserID, quoteID)
		execResp, err = s.client.CreateCrossRamp(ctx, clientCfg, execReq)
	}

	if err != nil {
		s.logger.Error("capa.execute_rfq.failed",
			zap.String("client", clientID),
			zap.String("quote_id", quoteID),
			zap.Error(err))
		return nil, fmt.Errorf("capa quote execution failed: %w", err)
	}

	trade := s.mapper.FromCapaExecuteResponse(execResp, clientID, quoteID)

	s.logger.Info("capa.trade_created",
		zap.String("client", clientID),
		zap.String("transaction_id", trade.TradeID),
		zap.String("quote_id", quoteID),
		zap.String("status", trade.Status),
	)

	// Store tx→clientID mapping in Redis so webhooks can resolve the client.
	if trade.TradeID != "" && s.store != nil {
		if err := s.store.SetJSON(ctx, "capa:tx:"+trade.TradeID+":client", clientID, 48*time.Hour); err != nil {
			s.logger.Warn("capa.tx_client_store_failed",
				zap.String("tx_id", trade.TradeID),
				zap.Error(err))
		}
	}

	// Start async polling if not in terminal state.
	if !IsTerminalStatus(execResp.Transaction.Status) && s.poller != nil {
		s.logger.Info("capa.starting_status_poll",
			zap.String("transaction_id", trade.TradeID),
			zap.String("client", clientID))
		go s.poller.PollTradeStatus(s.ctx, clientID, quoteID, trade.TradeID)
	} else if IsTerminalStatus(execResp.Transaction.Status) {
		s.syncTerminalTrade(ctx, trade)
	}

	return trade, nil
}

// FetchTransactionStatus retrieves the latest transaction status from Capa.
func (s *Service) FetchTransactionStatus(ctx context.Context, clientID, txID string) (*CapaTransaction, error) {
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.GetTransaction(ctx, clientCfg, txID)
	if err != nil {
		s.logger.Warn("capa.fetch_tx_status.failed",
			zap.String("client", clientID),
			zap.String("tx_id", txID),
			zap.Error(err))
		return nil, err
	}

	return &resp.Transaction, nil
}

// BuildTradeConfirmationFromTx converts a Capa transaction to a canonical TradeConfirmation.
func (s *Service) BuildTradeConfirmationFromTx(clientID string, tx *CapaTransaction) *model.TradeConfirmation {
	if tx == nil {
		return nil
	}
	return s.mapper.FromCapaTransaction(tx, clientID)
}

// ResolveTransaction fetches the live transaction status from Capa and returns
// a canonical TradeConfirmation. Used by the resolve-order endpoint.
func (s *Service) ResolveTransaction(ctx context.Context, clientID, txID string) (*model.TradeConfirmation, error) {
	tx, err := s.FetchTransactionStatus(ctx, clientID, txID)
	if err != nil {
		return nil, err
	}
	trade := s.BuildTradeConfirmationFromTx(clientID, tx)
	if trade == nil {
		return nil, fmt.Errorf("could not build trade confirmation for transaction %s", txID)
	}
	return trade, nil
}

// ListProducts returns the static list of Capa supported products.
func (s *Service) ListProducts() []model.Product {
	return capaSupportedProducts
}

// syncTerminalTrade syncs a terminal trade to the legacy database and publishes final event.
func (s *Service) syncTerminalTrade(ctx context.Context, trade *model.TradeConfirmation) {
	if s.tradeSyncWriter != nil {
		if err := s.tradeSyncWriter.SyncTradeUpsert(ctx, trade); err != nil {
			s.logger.Warn("capa.trade_sync_failed",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.Error(err))
		} else {
			s.logger.Info("capa.trade_sync_complete",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.String("status", trade.Status))
		}
	}

	if s.publisher == nil {
		return
	}
	subject := tradeEventSubject(trade.Status)
	if err := s.publisher.Publish(ctx, subject, map[string]any{
		"client_id": trade.ClientID,
		"trade_id":  trade.TradeID,
		"status":    trade.Status,
		"final":     true,
		"timestamp": time.Now().UTC(),
	}); err != nil {
		metrics.IncNATSPublishError(subject)
		s.logger.Warn("capa.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
	}
}

// HandleQuoteRequest processes a NATS quote request command by creating an RFQ
// and publishing the quote response to the outbound subject.
func (s *Service) HandleQuoteRequest(ctx context.Context, env model.Envelope, req model.QuoteRequest) error {
	s.logger.Info("capa.handle_quote_request",
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
		s.logger.Error("capa.handle_quote_request.failed",
			zap.String("client", env.ClientID),
			zap.Error(err))
		return err
	}

	resp := model.QuoteResponse{
		ID:             quote.ID,
		QuoteRequestID: req.RequestID.String(),
		Instrument:     quote.Instrument,
		Venue:          "CAPA",
		Side:           req.Side,
		Quantity:       req.Quantity,
		ExpiresAt:      quote.ExpiresAt,
		ReceivedAt:     time.Now().UTC(),
	}

	if err := s.publisher.Publish(ctx, s.cfg.OutboundSubject, resp); err != nil {
		metrics.IncNATSPublishError(s.cfg.OutboundSubject)
		s.logger.Warn("capa.handle_quote_request.publish_failed",
			zap.String("subject", s.cfg.OutboundSubject),
			zap.Error(err))
	}

	return nil
}

// HandleTradeExecute processes a NATS trade execute command by executing the RFQ
// and publishing the trade confirmation event.
func (s *Service) HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error {
	s.logger.Info("capa.handle_trade_execute",
		zap.String("tenant_id", env.TenantID),
		zap.String("client_id", env.ClientID),
		zap.String("quote_id", cmd.QuoteID),
	)

	trade, err := s.ExecuteRFQ(ctx, cmd.ClientID, cmd.QuoteID)
	if err != nil {
		s.logger.Error("capa.handle_trade_execute.failed",
			zap.String("client", cmd.ClientID),
			zap.String("quote_id", cmd.QuoteID),
			zap.Error(err))
		return err
	}

	subject := tradeEventSubject(trade.Status)
	if err := s.publisher.Publish(ctx, subject, trade); err != nil {
		metrics.IncNATSPublishError(subject)
		s.logger.Warn("capa.handle_trade_execute.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
	}

	return nil
}

// Config returns the service configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}

// clientTxType infers the transaction type from the per-client config routing fields.
func clientTxType(cfg *CapaClientConfig) TransactionType {
	if cfg.WalletAddress != "" {
		return OnRamp
	}
	if cfg.ReceiverID != "" {
		return OffRamp
	}
	return CrossRamp
}

// tradeEventSubject builds a NATS event subject for a trade status.
// status must already be normalized (lowercase, e.g. "filled", "cancelled", "rejected").
func tradeEventSubject(status string) string {
	return "evt.trade." + status + ".v1.CAPA"
}

// pretty formats a value as indented JSON for debug logging.
func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
