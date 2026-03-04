package b2c2

import (
	"context"
	"fmt"

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

	cfg, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return fmt.Errorf("b2c2.rfq: resolve client config for %q: %w", clientID, err)
	}

	req := ToRFQRequest(cmd)
	resp, err := s.client.RequestQuote(ctx, cfg, req)
	if err != nil {
		return fmt.Errorf("b2c2.rfq: request quote: %w", err)
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

	cfg, err := s.resolver.Resolve(ctx, cmd.ClientID)
	if err != nil {
		return fmt.Errorf("b2c2.order: resolve client config for %q: %w", cmd.ClientID, err)
	}

	req := ToOrderRequest(cmd)
	resp, err := s.client.ExecuteOrder(ctx, cfg, req)
	if err != nil {
		return fmt.Errorf("b2c2.order: execute order: %w", err)
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
