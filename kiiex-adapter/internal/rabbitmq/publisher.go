package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

const (
	// TopicFillsCreated is the topic for fill events
	TopicFillsCreated = "inbound.fills.creates"
	// TopicReturnedOrderCanceled is the topic for canceled order events
	TopicReturnedOrderCanceled = "returned.orders.canceled"
)

// Publisher publishes events to RabbitMQ
type Publisher struct {
	conn     *amqp.Connection
	channel  *amqp.Channel
	eventBus *eventbus.EventBus
	logger   *zap.Logger
}

// NewPublisher creates a new RabbitMQ publisher
func NewPublisher(url string, eventBus *eventbus.EventBus, logger *zap.Logger) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	p := &Publisher{
		conn:     conn,
		channel:  channel,
		eventBus: eventBus,
		logger:   logger,
	}

	// Subscribe to events
	p.subscribeToEvents()

	return p, nil
}

func (p *Publisher) subscribeToEvents() {
	// Subscribe to FillArrivedEvent
	p.eventBus.Subscribe(order.FillArrivedEvent{}, func(event interface{}) {
		if fillEvent, ok := event.(*order.FillArrivedEvent); ok {
			p.publishFillArrived(fillEvent)
		} else if fillEvent, ok := event.(order.FillArrivedEvent); ok {
			p.publishFillArrived(&fillEvent)
		}
	})

	// Subscribe to OrderCanceledEvent
	p.eventBus.Subscribe(order.OrderCanceledEvent{}, func(event interface{}) {
		if cancelEvent, ok := event.(*order.OrderCanceledEvent); ok {
			p.publishOrderCanceled(cancelEvent)
		} else if cancelEvent, ok := event.(order.OrderCanceledEvent); ok {
			p.publishOrderCanceled(&cancelEvent)
		}
	})
}

func (p *Publisher) publishFillArrived(event *order.FillArrivedEvent) {
	if event == nil || event.OrderID == "" {
		p.logger.Error("Received event with null Fill object", zap.Any("event", event))
		return
	}

	p.logger.Info("Publishing fill", zap.Any("event", event))

	body, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("Failed to marshal FillArrivedEvent", zap.Error(err))
		return
	}

	err = p.channel.PublishWithContext(
		context.Background(),
		"",                // exchange
		TopicFillsCreated, // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		p.logger.Error("Failed to publish FillArrivedEvent", zap.Error(err))
	}
}

func (p *Publisher) publishOrderCanceled(event *order.OrderCanceledEvent) {
	if event == nil || event.OrderID == "" {
		p.logger.Error("Received null event or unknown order id", zap.Any("event", event))
		return
	}

	p.logger.Info("Publishing order canceled", zap.Any("event", event))

	body, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("Failed to marshal OrderCanceledEvent", zap.Error(err))
		return
	}

	err = p.channel.PublishWithContext(
		context.Background(),
		"",                         // exchange
		TopicReturnedOrderCanceled, // routing key
		false,                      // mandatory
		false,                      // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Priority:    10, // Set priority for canceled orders
		},
	)
	if err != nil {
		p.logger.Error("Failed to publish OrderCanceledEvent", zap.Error(err))
	}
}

// Close closes the publisher
func (p *Publisher) Close() error {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
