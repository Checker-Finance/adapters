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
	queueRFQsCreated    = "outbound.rfqs.created.b2c2"
	queueOrdersCreated  = "outbound.orders.created.b2c2"
	queueOrdersCanceled = "outbound.orders.canceled.b2c2"
)

// B2C2Service defines the service interface consumed by the RabbitMQ consumer.
type B2C2Service interface {
	HandleRFQCommand(ctx context.Context, cmd *b2c2.SubmitRequestForQuoteCommand) error
	HandleOrderCommand(ctx context.Context, cmd *b2c2.SubmitOrderCommand) error
	HandleCancelCommand(ctx context.Context, cmd *b2c2.CancelOrderCommand) error
}

// Consumer consumes RabbitMQ messages and dispatches them to the B2C2 service.
type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	service B2C2Service
	logger  *zap.Logger
	done    chan struct{}
}

// NewConsumer creates a new Consumer connected to RabbitMQ.
func NewConsumer(url string, service B2C2Service, logger *zap.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq consumer: connect: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq consumer: open channel: %w", err)
	}

	return &Consumer{
		conn:    conn,
		channel: channel,
		service: service,
		logger:  logger,
		done:    make(chan struct{}),
	}, nil
}

// Start declares queues and begins consuming messages in background goroutines.
func (c *Consumer) Start(ctx context.Context) error {
	for _, q := range []string{queueRFQsCreated, queueOrdersCreated, queueOrdersCanceled} {
		if _, err := c.channel.QueueDeclare(q, true, false, false, false, nil); err != nil {
			return fmt.Errorf("rabbitmq consumer: declare queue %s: %w", q, err)
		}
	}

	rfqMsgs, err := c.channel.Consume(queueRFQsCreated, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: consume %s: %w", queueRFQsCreated, err)
	}

	orderMsgs, err := c.channel.Consume(queueOrdersCreated, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: consume %s: %w", queueOrdersCreated, err)
	}

	cancelMsgs, err := c.channel.Consume(queueOrdersCanceled, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consumer: consume %s: %w", queueOrdersCanceled, err)
	}

	c.logger.Info("b2c2.consumer.started",
		zap.String("rfqQueue", queueRFQsCreated),
		zap.String("orderQueue", queueOrdersCreated),
		zap.String("cancelQueue", queueOrdersCanceled),
	)

	go c.consumeRFQs(ctx, rfqMsgs)
	go c.consumeOrders(ctx, orderMsgs)
	go c.consumeCancels(ctx, cancelMsgs)

	return nil
}

func (c *Consumer) consumeRFQs(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warn("b2c2.consumer.rfq_channel_closed")
				return
			}
			var cmd b2c2.SubmitRequestForQuoteCommand
			if err := json.Unmarshal(msg.Body, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.rfq_unmarshal_failed", zap.Error(err))
				_ = msg.Nack(false, false)
				continue
			}
			if err := c.service.HandleRFQCommand(ctx, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.rfq_handle_failed", zap.Error(err))
				_ = msg.Nack(false, true) // requeue
				continue
			}
			_ = msg.Ack(false)
		}
	}
}

func (c *Consumer) consumeOrders(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warn("b2c2.consumer.order_channel_closed")
				return
			}
			var cmd b2c2.SubmitOrderCommand
			if err := json.Unmarshal(msg.Body, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.order_unmarshal_failed", zap.Error(err))
				_ = msg.Nack(false, false)
				continue
			}
			if err := c.service.HandleOrderCommand(ctx, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.order_handle_failed", zap.Error(err))
				_ = msg.Nack(false, true) // requeue
				continue
			}
			_ = msg.Ack(false)
		}
	}
}

func (c *Consumer) consumeCancels(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warn("b2c2.consumer.cancel_channel_closed")
				return
			}
			var cmd b2c2.CancelOrderCommand
			if err := json.Unmarshal(msg.Body, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.cancel_unmarshal_failed", zap.Error(err))
				_ = msg.Nack(false, false)
				continue
			}
			if err := c.service.HandleCancelCommand(ctx, &cmd); err != nil {
				c.logger.Error("b2c2.consumer.cancel_handle_failed", zap.Error(err))
				_ = msg.Nack(false, true)
				continue
			}
			_ = msg.Ack(false)
		}
	}
}

// Close stops consuming and closes the AMQP connection.
func (c *Consumer) Close() error {
	close(c.done)
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
