package nats

import (
	"context"
	"log/slog"

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
	pub *publisher.Publisher
}

// NewPublisher creates a Publisher that sends events to NATS JetStream.
func NewPublisher(pub *publisher.Publisher) *Publisher {
	return &Publisher{pub: pub}
}

// PublishQuoteEvent publishes a QuoteArrivedEvent to NATS.
func (p *Publisher) PublishQuoteEvent(ctx context.Context, event *b2c2.QuoteArrivedEvent) error {
	slog.Info("b2c2.publisher.quote",
		"requestForQuoteId", event.RequestForQuoteID,
		"externalQuoteId", event.ExternalQuoteID,
	)
	return p.pub.Publish(ctx, subjectQuoteReady, event)
}

// PublishFillEvent publishes a FillArrivedEvent to NATS.
func (p *Publisher) PublishFillEvent(ctx context.Context, event *b2c2.FillArrivedEvent) error {
	slog.Info("b2c2.publisher.fill",
		"orderId", event.OrderID,
		"externalOrderId", event.ExternalOrderID,
	)
	return p.pub.Publish(ctx, subjectFilled, event)
}

// PublishCancelEvent publishes an OrderCanceledEvent to NATS.
func (p *Publisher) PublishCancelEvent(ctx context.Context, event *b2c2.OrderCanceledEvent) error {
	slog.Info("b2c2.publisher.cancel",
		"orderId", event.OrderID,
		"reason", event.Reason,
	)
	return p.pub.Publish(ctx, subjectCancelled, event)
}
