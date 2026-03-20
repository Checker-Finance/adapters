package xfx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
)

// Service orchestrates XFX API operations: quote creation, trade execution,
// status tracking, and normalized event publishing to NATS.
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

// NewService constructs a fully wired XFX adapter service.
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

// SetPoller sets the poller reference for async trade status tracking.
func (s *Service) SetPoller(p *Poller) {
	s.poller = p
}

// resolveConfig resolves the per-client XFX configuration.
func (s *Service) resolveConfig(ctx context.Context, clientID string) (*XFXClientConfig, error) {
	cfg, err := s.configResolver.Resolve(ctx, clientID)
	if err != nil {
		slog.Error("xfx.resolve_config_failed",
			"client", clientID,
			"error", err)
		return nil, fmt.Errorf("resolve client config for %q: %w", clientID, err)
	}
	return cfg, nil
}

// CreateRFQ creates a new executable quote on XFX.
func (s *Service) CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error) {
	slog.Info("xfx.create_rfq.start",
		"client", req.ClientID,
		"pair", req.CurrencyPair,
		"side", req.Side,
		"amount", req.Amount,
	)

	clientCfg, err := s.resolveConfig(ctx, req.ClientID)
	if err != nil {
		return nil, err
	}

	xfxReq := s.mapper.ToXFXQuoteRequest(req)

	slog.Debug("xfx.rfq_request", "json", pretty(xfxReq))

	xfxResp, err := s.client.RequestQuote(ctx, clientCfg, xfxReq)
	if err != nil {
		slog.Error("xfx.create_rfq.failed",
			"client", req.ClientID,
			"error", err)
		return nil, fmt.Errorf("xfx quote creation failed: %w", err)
	}

	quote := s.mapper.FromXFXQuote(xfxResp, req.ClientID)

	slog.Info("xfx.rfq_created",
		"client", req.ClientID,
		"quote_id", quote.ID,
		"price", quote.Price,
		"instrument", quote.Instrument,
	)

	return quote, nil
}

// ExecuteRFQ executes an existing quote on XFX, creating a transaction.
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error) {
	slog.Info("xfx.execute_rfq.start",
		"client", clientID,
		"quote_id", quoteID,
	)

	clientCfg, err := s.resolveConfig(ctx, clientID)
	if err != nil {
		return nil, err
	}

	execResp, err := s.client.ExecuteQuote(ctx, clientCfg, quoteID)
	if err != nil {
		slog.Error("xfx.execute_rfq.failed",
			"client", clientID,
			"quote_id", quoteID,
			"error", err)
		return nil, fmt.Errorf("xfx quote execution failed: %w", err)
	}

	trade := s.mapper.FromXFXExecute(execResp, clientID, quoteID)

	slog.Info("xfx.trade_created",
		"client", clientID,
		"transaction_id", trade.TradeID,
		"quote_id", quoteID,
		"status", trade.Status,
	)

	// Start async polling if not in terminal state.
	// Use service-level context so polling survives after the HTTP response.
	if !IsTerminalStatus(execResp.Transaction.Status) && s.poller != nil {
		slog.Info("xfx.starting_status_poll",
			"transaction_id", trade.TradeID,
			"client", clientID)
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
		slog.Warn("xfx.fetch_tx_status.failed",
			"client", clientID,
			"tx_id", txID,
			"error", err)
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
			slog.Warn("xfx.trade_sync_failed",
				"trade_id", trade.TradeID,
				"client", trade.ClientID,
				"error", err)
		} else {
			slog.Info("xfx.trade_sync_complete",
				"trade_id", trade.TradeID,
				"client", trade.ClientID,
				"status", trade.Status)
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
		metrics.IncNATSPublishError(subject)
		slog.Warn("xfx.publish_failed",
			"subject", subject,
			"error", err)
	}
}

// ResolveTransaction fetches the live transaction status from XFX and returns
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

// ListProducts returns the static list of XFX supported products.
func (s *Service) ListProducts() []model.Product {
	return xfxSupportedProducts
}

// HandleQuoteRequest processes a NATS quote request command by creating an RFQ
// and publishing the quote response to the outbound subject.
func (s *Service) HandleQuoteRequest(ctx context.Context, env model.Envelope, req model.QuoteRequest) error {
	slog.Info("xfx.handle_quote_request",
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
		slog.Error("xfx.handle_quote_request.failed",
			"client", env.ClientID,
			"error", err)
		return err
	}

	resp := model.QuoteResponse{
		ID:             quote.ID,
		QuoteRequestID: req.RequestID.String(),
		Instrument:     quote.Instrument,
		Venue:          "XFX",
		Side:           req.Side,
		Quantity:       req.Quantity,
		ExpiresAt:      quote.ExpiresAt,
		ReceivedAt:     time.Now().UTC(),
	}

	if err := s.publisher.Publish(ctx, s.cfg.OutboundSubject, resp); err != nil {
		metrics.IncNATSPublishError(s.cfg.OutboundSubject)
		slog.Warn("xfx.handle_quote_request.publish_failed",
			"subject", s.cfg.OutboundSubject,
			"error", err)
	}

	return nil
}

// HandleTradeExecute processes a NATS trade execute command by executing the RFQ
// and publishing the trade confirmation event.
func (s *Service) HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error {
	slog.Info("xfx.handle_trade_execute",
		"tenant_id", env.TenantID,
		"client_id", env.ClientID,
		"quote_id", cmd.QuoteID,
	)

	trade, err := s.ExecuteRFQ(ctx, cmd.ClientID, cmd.QuoteID)
	if err != nil {
		slog.Error("xfx.handle_trade_execute.failed",
			"client", cmd.ClientID,
			"quote_id", cmd.QuoteID,
			"error", err)
		return err
	}

	subject := "evt.trade." + trade.Status + ".v1.XFX"
	if err := s.publisher.Publish(ctx, subject, trade); err != nil {
		metrics.IncNATSPublishError(subject)
		slog.Warn("xfx.handle_trade_execute.publish_failed",
			"subject", subject,
			"error", err)
	}

	return nil
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
