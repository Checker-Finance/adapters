package nats

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
)

// B2C2Service defines the service interface consumed by the NATS command consumer.
type B2C2Service interface {
	HandleRFQCommand(ctx context.Context, cmd *b2c2.SubmitRequestForQuoteCommand) error
	HandleOrderCommand(ctx context.Context, cmd *b2c2.SubmitOrderCommand) error
	HandleCancelCommand(ctx context.Context, cmd *b2c2.CancelOrderCommand) error
}

// CommandConsumer subscribes to NATS subjects and dispatches commands to the B2C2 service.
type CommandConsumer struct {
	nc      *nats.Conn
	service B2C2Service
	subs    []*nats.Subscription
}

// NewCommandConsumer creates a CommandConsumer. Call Subscribe to begin receiving messages.
func NewCommandConsumer(nc *nats.Conn, service B2C2Service) *CommandConsumer {
	return &CommandConsumer{nc: nc, service: service}
}

// Subscribe registers NATS subscriptions for the three inbound B2C2 command subjects.
// Handlers inherit ctx so they respect shutdown cancellation.
func (c *CommandConsumer) Subscribe(ctx context.Context, rfqSubject, orderSubject, cancelSubject string) error {
	rfqSub, err := c.nc.Subscribe(rfqSubject, func(msg *nats.Msg) {
		var cmd b2c2.SubmitRequestForQuoteCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			slog.Error("b2c2.consumer.rfq_unmarshal_failed", "error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := c.service.HandleRFQCommand(msgCtx, &cmd); err != nil {
			slog.Error("b2c2.consumer.rfq_handle_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, rfqSub)

	orderSub, err := c.nc.Subscribe(orderSubject, func(msg *nats.Msg) {
		var cmd b2c2.SubmitOrderCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			slog.Error("b2c2.consumer.order_unmarshal_failed", "error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := c.service.HandleOrderCommand(msgCtx, &cmd); err != nil {
			slog.Error("b2c2.consumer.order_handle_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, orderSub)

	cancelSub, err := c.nc.Subscribe(cancelSubject, func(msg *nats.Msg) {
		var cmd b2c2.CancelOrderCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			slog.Error("b2c2.consumer.cancel_unmarshal_failed", "error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := c.service.HandleCancelCommand(msgCtx, &cmd); err != nil {
			slog.Error("b2c2.consumer.cancel_handle_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, cancelSub)

	slog.Info("b2c2.consumer.started",
		"rfq_subject", rfqSubject,
		"order_subject", orderSubject,
		"cancel_subject", cancelSubject,
	)
	return nil
}

// Drain unsubscribes all subscriptions and waits for pending messages to be processed.
func (c *CommandConsumer) Drain() {
	for _, sub := range c.subs {
		if err := sub.Drain(); err != nil {
			slog.Error("b2c2.consumer.drain_failed", "error", err)
		}
	}
}
