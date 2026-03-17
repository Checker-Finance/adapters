package nats

import (
	"context"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
	"github.com/Checker-Finance/adapters/internal/publisher"
)

const (
	subjectQuoteReady = "evt.trade.quote_ready.v1.B2C2"
	subjectFilled     = "evt.trade.filled.v1.B2C2"
	subjectCancelled  = "evt.trade.cancelled.v1.B2C2"
)

// Publisher implements b2c2.Publisher using NATS JetStream.
type Publisher struct {
	pub    *publisher.Publisher
	logger *zap.Logger
}

// NewPublisher creates a Publisher that sends events to NATS JetStream.
func NewPublisher(pub *publisher.Publisher, logger *zap.Logger) *Publisher {
	return &Publisher{pub: pub, logger: logger}
}

// PublishQuoteEvent publishes a QuoteArrivedEvent to NATS.
func (p *Publisher) PublishQuoteEvent(ctx context.Context, event *b2c2.QuoteArrivedEvent) error {
	p.logger.Info("b2c2.publisher.quote",
		zap.String("requestForQuoteId", event.RequestForQuoteID),
		zap.String("externalQuoteId", event.ExternalQuoteID),
	)
	return p.pub.Publish(ctx, subjectQuoteReady, event)
}

// PublishFillEvent publishes a FillArrivedEvent to NATS.
func (p *Publisher) PublishFillEvent(ctx context.Context, event *b2c2.FillArrivedEvent) error {
	p.logger.Info("b2c2.publisher.fill",
		zap.String("orderId", event.OrderID),
		zap.String("externalOrderId", event.ExternalOrderID),
	)
	return p.pub.Publish(ctx, subjectFilled, event)
}

// PublishCancelEvent publishes an OrderCanceledEvent to NATS.
func (p *Publisher) PublishCancelEvent(ctx context.Context, event *b2c2.OrderCanceledEvent) error {
	p.logger.Info("b2c2.publisher.cancel",
		zap.String("orderId", event.OrderID),
		zap.String("reason", event.Reason),
	)
	return p.pub.Publish(ctx, subjectCancelled, event)
}
