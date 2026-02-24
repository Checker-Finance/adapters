package xfx

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
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// Service orchestrates XFX API operations: quote creation, trade execution,
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

// NewService constructs a fully wired XFX adapter service.
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

// resolveConfig resolves the per-client XFX configuration.
func (s *Service) resolveConfig(ctx context.Context, clientID string) (*XFXClientConfig, error) {
	cfg, err := s.configResolver.Resolve(ctx, clientID)
	if err != nil {
		s.logger.Error("xfx.resolve_config_failed",
			zap.String("client", clientID),
			zap.Error(err))
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new executable quote on XFX.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	s.logger.Info("xfx.create_rfq.start",
		zap.String("client", req.ClientID),
		zap.String("pair", req.CurrencyPair),
		zap.String("side", req.Side),
		zap.Float64("amount", req.Amount),
	)

	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	xfxReq := s.mapper.ToXFXQuoteRequest(req)

	s.logger.Debug("xfx.rfq_request", zap.String("json", pretty(xfxReq)))

	xfxResp, err := s.client.RequestQuote(ctx, clientCfg, xfxReq)
	if err != nil {
		s.logger.Error("xfx.create_rfq.failed",
			zap.String("client", req.ClientID),
			zap.Error(err))
		return nil, fmt.Errorf("xfx quote creation failed: %w", err)
	}

	quote := s.mapper.FromXFXQuote(xfxResp, req.ClientID)

	s.logger.Info("xfx.rfq_created",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quote.ID),
		zap.Float64("price", quote.Price),
		zap.String("instrument", quote.Instrument),
	)

	return quote, nil
}

// ExecuteRFQ executes an existing quote on XFX, creating a transaction.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	s.logger.Info("xfx.execute_rfq.start",
		zap.String("client", clientID),
		zap.String("quote_id", quoteID),
	)

	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	execResp, err := s.client.ExecuteQuote(ctx, clientCfg, quoteID)
	if err != nil {
		s.logger.Error("xfx.execute_rfq.failed",
			zap.String("client", clientID),
			zap.String("quote_id", quoteID),
			zap.Error(err))
		return nil, fmt.Errorf("xfx quote execution failed: %w", err)
	}

	trade := s.mapper.FromXFXExecute(execResp, clientID, quoteID)

	s.logger.Info("xfx.trade_created",
		zap.String("client", clientID),
		zap.String("transaction_id", trade.TradeID),
		zap.String("quote_id", quoteID),
		zap.String("status", trade.Status),
	)

	// Start async polling if not in terminal state.
	// Use service-level context so polling survives after the HTTP response.
	if !IsTerminalStatus(execResp.Transaction.Status) && s.poller != nil {
		s.logger.Info("xfx.starting_status_poll",
			zap.String("transaction_id", trade.TradeID),
			zap.String("client", clientID))
		go s.poller.PollTradeStatus(s.ctx, clientID, quoteID, trade.TradeID)
	} else if IsTerminalStatus(execResp.Transaction.Status) {
		s.syncTerminalTrade(ctx, trade)
	}

	return trade, nil
}

// FetchTransactionStatus retrieves the latest transaction status from XFX.
func (s *Service) FetchTransactionStatus(ctx context.Context, clientID, txID string) (*XFXTransaction, error) {
	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.GetTransaction(ctx, clientCfg, txID)
	if err != nil {
		s.logger.Warn("xfx.fetch_tx_status.failed",
			zap.String("client", clientID),
			zap.String("tx_id", txID),
			zap.Error(err))
		return nil, err
	}

	return &resp.Transaction, nil
}

// BuildTradeConfirmationFromTx converts an XFX transaction to a canonical TradeConfirmation.
func (s *Service) BuildTradeConfirmationFromTx(clientID string, tx *XFXTransaction) *model.TradeConfirmation {
	if tx == nil {
		return nil
	}
	resp := &XFXTransactionResponse{Transaction: *tx}
	return s.mapper.FromXFXTransaction(resp, clientID)
}

// syncTerminalTrade syncs a terminal trade to the legacy database and publishes final event.
func (s *Service) syncTerminalTrade(ctx context.Context, trade *model.TradeConfirmation) {
	if s.tradeSyncWriter != nil {
		if err := s.tradeSyncWriter.SyncTradeUpsert(ctx, trade); err != nil {
			s.logger.Warn("xfx.trade_sync_failed",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.Error(err))
		} else {
			s.logger.Info("xfx.trade_sync_complete",
				zap.String("trade_id", trade.TradeID),
				zap.String("client", trade.ClientID),
				zap.String("status", trade.Status))
		}
	}

	if s.publisher == nil {
		return
	}
	subject := "evt.trade." + trade.Status + ".v1.XFX"
	if err := s.publisher.Publish(ctx, subject, map[string]any{
		"client_id": trade.ClientID,
		"trade_id":  trade.TradeID,
		"status":    trade.Status,
		"final":     true,
		"timestamp": time.Now().UTC(),
	}); err != nil {
		s.logger.Warn("xfx.publish_failed",
			zap.String("subject", subject),
			zap.Error(err))
	}
}

// Config returns the service configuration.
func (s *Service) Config() config.Config {
	return s.cfg
}

// pretty formats a value as indented JSON for debug logging.
func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
