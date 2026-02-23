package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
)

// Consumer consumes messages from RabbitMQ
type Consumer struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	orderService OrderService
	provider     string
	logger       *zap.Logger
	done         chan struct{}
}

// OrderService defines the order service interface
type OrderService interface {
	ExecuteOrder(ctx context.Context, cmd *order.SubmitOrderCommand) error
	CancelOrder(ctx context.Context, orderID string) error
}

// NewConsumer creates a new RabbitMQ consumer
func NewConsumer(url, provider string, orderService OrderService, logger *zap.Logger) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	return &Consumer{
		conn:         conn,
		channel:      channel,
		orderService: orderService,
		provider:     provider,
		logger:       logger,
		done:         make(chan struct{}),
	}, nil
}

// Start starts consuming messages
func (c *Consumer) Start(ctx context.Context) error {
	// Declare queues
	createdQueue := fmt.Sprintf("outbound.orders.created.%s", c.provider)
	canceledQueue := fmt.Sprintf("outbound.orders.canceled.%s", c.provider)

	if _, err := c.channel.QueueDeclare(createdQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", createdQueue, err)
	}

	if _, err := c.channel.QueueDeclare(canceledQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", canceledQueue, err)
	}

	// Consume from created orders queue
	createdMsgs, err := c.channel.Consume(createdQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to consume from %s: %w", createdQueue, err)
	}

	// Consume from canceled orders queue
	canceledMsgs, err := c.channel.Consume(canceledQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("failed to consume from %s: %w", canceledQueue, err)
	}

	c.logger.Info("Started consuming from RabbitMQ",
		zap.String("createdQueue", createdQueue),
		zap.String("canceledQueue", canceledQueue),
	)

	// Start consumer goroutines
	go c.consumeCreatedOrders(ctx, createdMsgs)
	go c.consumeCanceledOrders(ctx, canceledMsgs)

	return nil
}

func (c *Consumer) consumeCreatedOrders(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warn("Created orders channel closed")
				return
			}

			c.logger.Debug("Received created order message", zap.String("body", string(msg.Body)))

			var cmd order.SubmitOrderCommand
			if err := json.Unmarshal(msg.Body, &cmd); err != nil {
				c.logger.Error("Failed to unmarshal SubmitOrderCommand", zap.Error(err))
				msg.Nack(false, false)
				continue
			}

			if err := c.orderService.ExecuteOrder(ctx, &cmd); err != nil {
				c.logger.Error("Failed to execute order", zap.Error(err))
				msg.Nack(false, true) // Requeue on failure
				continue
			}

			msg.Ack(false)
		}
	}
}

func (c *Consumer) consumeCanceledOrders(ctx context.Context, msgs <-chan amqp.Delivery) {
	for {
		select {
		case <-c.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				c.logger.Warn("Canceled orders channel closed")
				return
			}

			c.logger.Debug("Received cancel order message", zap.String("body", string(msg.Body)))

			var cmd order.CancelOrderCommand
			if err := json.Unmarshal(msg.Body, &cmd); err != nil {
				c.logger.Error("Failed to unmarshal CancelOrderCommand", zap.Error(err))
				msg.Nack(false, false)
				continue
			}

			if err := c.orderService.CancelOrder(ctx, cmd.OrderID); err != nil {
				c.logger.Error("Failed to cancel order", zap.Error(err))
				msg.Nack(false, true) // Requeue on failure
				continue
			}

			msg.Ack(false)
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	close(c.done)

	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
