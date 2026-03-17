package b2c2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Service contains business logic for the B2C2 adapter.
// It handles RFQ and order commands, mapping them to B2C2 API calls,
// and publishing canonical events back to RabbitMQ.
type Service struct {
	logger    *zap.Logger
	client    *Client
	resolver  ConfigResolver
	publisher Publisher
}

// NewService constructs a new B2C2 service.
func NewService(logger *zap.Logger, client *Client, resolver ConfigResolver, publisher Publisher) *Service {
	return &Service{
		logger:    logger,
		client:    client,
		resolver:  resolver,
		publisher: publisher,
	}
}

// CreateRFQ requests a quote from B2C2. pair is in canonical format (e.g. "usd:btc").
func (s *Service) CreateRFQ(ctx context.Context, clientID, pair, side, quantity, clientRFQID string) (*RFQResponse, error) {
	cfg, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("b2c2.create_rfq: resolve config for %q: %w", clientID, err)
	}
	req := &RFQRequest{
		Instrument:  ToB2C2Instrument(pair),
		Side:        strings.ToLower(side),
		Quantity:    quantity,
		ClientRFQID: clientRFQID,
	}
	resp, err := s.client.RequestQuote(ctx, cfg, req)
	if err != nil {
		return nil, fmt.Errorf("b2c2.create_rfq: %w", err)
	}
	return resp, nil
}

// ExecuteRFQ submits a FOK order to B2C2. pair is in canonical format (e.g. "usd:btc").
func (s *Service) ExecuteRFQ(ctx context.Context, clientID, pair, side, quantity, price, rfqID, clientOrderID string) (*OrderResponse, error) {
	cfg, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("b2c2.execute_rfq: resolve config for %q: %w", clientID, err)
	}
	req := &OrderRequest{
		Instrument:    ToB2C2Instrument(pair),
		Side:          strings.ToLower(side),
		Quantity:      quantity,
		Price:         price,
		OrderType:     "FOK",
		RFQID:         rfqID,
		ClientOrderID: clientOrderID,
		ValidUntil:    time.Now().UTC().Add(10 * time.Second).Format(time.RFC3339),
	}
	resp, err := s.client.ExecuteOrder(ctx, cfg, req)
	if err != nil {
		return nil, fmt.Errorf("b2c2.execute_rfq: %w", err)
	}
	return resp, nil
}

// GetBalance fetches the account balance for a client.
func (s *Service) GetBalance(ctx context.Context, clientID string) (BalanceResponse, error) {
	cfg, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("b2c2.get_balance: resolve config for %q: %w", clientID, err)
	}
	return s.client.GetBalance(ctx, cfg)
}

// GetProducts fetches the list of available trading instruments from B2C2.
func (s *Service) GetProducts(ctx context.Context, clientID string) ([]Instrument, error) {
	cfg, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("b2c2.get_products: resolve config for %q: %w", clientID, err)
	}
	return s.client.GetInstruments(ctx, cfg)
}

// HandleRFQCommand processes a SubmitRequestForQuoteCommand:
// resolves client config → calls B2C2 RFQ API → publishes QuoteArrivedEvent.
func (s *Service) HandleRFQCommand(ctx context.Context, cmd *SubmitRequestForQuoteCommand) error {
	clientID := cmd.EffectiveClientID()
	s.logger.Info("b2c2.rfq.received",
		zap.String("clientId", clientID),
		zap.String("instrumentPair", cmd.InstrumentPair),
		zap.String("side", cmd.Side),
		zap.String("quantity", cmd.Quantity),
	)

	resp, err := s.CreateRFQ(ctx, clientID, cmd.InstrumentPair, cmd.Side, cmd.Quantity, cmd.ID)
	if err != nil {
		return fmt.Errorf("b2c2.rfq: %w", err)
	}

	s.logger.Info("b2c2.rfq.received_price",
		zap.String("rfqId", resp.RFQID),
		zap.String("price", resp.Price),
		zap.String("validUntil", resp.ValidUntil),
	)

	event := FromRFQResponse(resp, cmd)
	if err := s.publisher.PublishQuoteEvent(ctx, event); err != nil {
		return fmt.Errorf("b2c2.rfq: publish quote event: %w", err)
	}

	return nil
}

// HandleOrderCommand processes a SubmitOrderCommand:
// resolves client config → calls B2C2 order API → publishes FillArrivedEvent or OrderCanceledEvent.
func (s *Service) HandleOrderCommand(ctx context.Context, cmd *SubmitOrderCommand) error {
	s.logger.Info("b2c2.order.received",
		zap.String("clientId", cmd.ClientID),
		zap.String("orderId", cmd.OrderID),
		zap.String("instrumentPair", cmd.InstrumentPair),
		zap.String("side", cmd.Side),
	)

	resp, err := s.ExecuteRFQ(ctx, cmd.ClientID, cmd.InstrumentPair, cmd.Side, cmd.Quantity, cmd.Price, cmd.RequestForQuoteID, cmd.ClientOrderID)
	if err != nil {
		return fmt.Errorf("b2c2.order: %w", err)
	}

	if resp.ExecutedPrice != nil {
		s.logger.Info("b2c2.order.filled",
			zap.String("orderId", resp.OrderID),
			zap.String("executedPrice", *resp.ExecutedPrice),
		)
		event := FromOrderResponseFilled(resp, cmd)
		if err := s.publisher.PublishFillEvent(ctx, event); err != nil {
			return fmt.Errorf("b2c2.order: publish fill event: %w", err)
		}
	} else {
		s.logger.Info("b2c2.order.no_liquidity",
			zap.String("orderId", resp.OrderID),
			zap.String("status", resp.Status),
		)
		event := FromOrderResponseCanceled(resp, cmd)
		if err := s.publisher.PublishCancelEvent(ctx, event); err != nil {
			return fmt.Errorf("b2c2.order: publish cancel event: %w", err)
		}
	}

	return nil
}

// HandleCancelCommand processes a CancelOrderCommand.
// B2C2 FOK orders are synchronous and cannot be cancelled post-submission;
// this is a no-op that logs the attempt.
func (s *Service) HandleCancelCommand(ctx context.Context, cmd *CancelOrderCommand) error {
	s.logger.Info("b2c2.cancel.noop",
		zap.String("orderId", cmd.OrderID),
		zap.String("clientId", cmd.ClientID),
		zap.String("reason", "FOK orders cannot be cancelled post-submission"),
	)
	return nil
}
