package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
)

const (
	exchangeRFQ    = "exchange.outbound.rfq"
	exchangeOrders = "exchange.outbound.orders"

	routingKeyQuotes  = "inbound.quotes.creates"
	routingKeyFills   = "inbound.fills.creates"
	routingKeyCancels = "returned.orders.canceled"
)

// Publisher publishes canonical events to RabbitMQ exchanges.
// It implements the b2c2.Publisher interface.
type Publisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	logger  *zap.Logger
}

// NewPublisher creates a Publisher connected to RabbitMQ and declares required exchanges.
func NewPublisher(url string, logger *zap.Logger) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq publisher: connect: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher: open channel: %w", err)
	}

	// Declare exchanges (topic, durable)
	for _, exchange := range []string{exchangeRFQ, exchangeOrders} {
		if err := channel.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("rabbitmq publisher: declare exchange %s: %w", exchange, err)
		}
	}

	return &Publisher{
		conn:    conn,
		channel: channel,
		logger:  logger,
	}, nil
}

// PublishQuoteEvent publishes a QuoteArrivedEvent to exchange.outbound.rfq / inbound.quotes.creates.
func (p *Publisher) PublishQuoteEvent(_ context.Context, event *b2c2.QuoteArrivedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("rabbitmq publisher: marshal QuoteArrivedEvent: %w", err)
	}

	p.logger.Info("b2c2.publisher.quote",
		zap.String("requestForQuoteId", event.RequestForQuoteID),
		zap.String("externalQuoteId", event.ExternalQuoteID),
	)

	return p.publish(exchangeRFQ, routingKeyQuotes, body)
}

// PublishFillEvent publishes a FillArrivedEvent to exchange.outbound.orders / inbound.fills.creates.
func (p *Publisher) PublishFillEvent(_ context.Context, event *b2c2.FillArrivedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("rabbitmq publisher: marshal FillArrivedEvent: %w", err)
	}

	p.logger.Info("b2c2.publisher.fill",
		zap.String("orderId", event.OrderID),
		zap.String("externalOrderId", event.ExternalOrderID),
	)

	return p.publish(exchangeOrders, routingKeyFills, body)
}

// PublishCancelEvent publishes an OrderCanceledEvent to exchange.outbound.orders / returned.orders.canceled.
func (p *Publisher) PublishCancelEvent(_ context.Context, event *b2c2.OrderCanceledEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("rabbitmq publisher: marshal OrderCanceledEvent: %w", err)
	}

	p.logger.Info("b2c2.publisher.cancel",
		zap.String("orderId", event.OrderID),
		zap.String("reason", event.Reason),
	)

	return p.publish(exchangeOrders, routingKeyCancels, body)
}

func (p *Publisher) publish(exchange, routingKey string, body []byte) error {
	err := p.channel.PublishWithContext(
		context.Background(),
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		p.logger.Error("b2c2.publisher.publish_failed",
			zap.String("exchange", exchange),
			zap.String("routingKey", routingKey),
			zap.Error(err),
		)
		return fmt.Errorf("rabbitmq publisher: publish to %s/%s: %w", exchange, routingKey, err)
	}
	return nil
}

// Close closes the AMQP channel and connection.
func (p *Publisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
