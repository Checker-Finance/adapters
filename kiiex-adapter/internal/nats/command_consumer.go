package nats

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
)

// OrderService defines the order service interface consumed by the NATS command consumer.
type OrderService interface {
	ExecuteOrder(ctx context.Context, cmd *order.SubmitOrderCommand) error
	CancelOrder(ctx context.Context, clientID, orderID string) error
}

// CommandConsumer subscribes to NATS subjects and dispatches commands to the order service.
type CommandConsumer struct {
	nc           *nats.Conn
	orderService OrderService
	subs         []*nats.Subscription
}

// NewCommandConsumer creates a CommandConsumer. Call Subscribe to begin receiving messages.
func NewCommandConsumer(nc *nats.Conn, orderService OrderService) *CommandConsumer {
	return &CommandConsumer{nc: nc, orderService: orderService}
}

// Subscribe registers NATS subscriptions for the execute and cancel subjects.
// Handlers inherit ctx so they respect shutdown cancellation.
func (c *CommandConsumer) Subscribe(ctx context.Context, inboundSubject, cancelSubject string) error {
	executeSub, err := c.nc.Subscribe(inboundSubject, func(msg *nats.Msg) {
		var cmd order.SubmitOrderCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			slog.Error("kiiex.consumer.execute_unmarshal_failed", "error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := c.orderService.ExecuteOrder(msgCtx, &cmd); err != nil {
			slog.Error("kiiex.consumer.execute_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, executeSub)

	cancelSub, err := c.nc.Subscribe(cancelSubject, func(msg *nats.Msg) {
		var cmd order.CancelOrderCommand
		if err := json.Unmarshal(msg.Data, &cmd); err != nil {
			slog.Error("kiiex.consumer.cancel_unmarshal_failed", "error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := c.orderService.CancelOrder(msgCtx, cmd.ClientID, cmd.OrderID); err != nil {
			slog.Error("kiiex.consumer.cancel_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, cancelSub)

	slog.Info("kiiex.consumer.started",
		"inbound_subject", inboundSubject,
		"cancel_subject", cancelSubject,
	)
	return nil
}

// Drain unsubscribes all subscriptions and waits for pending messages to be processed.
func (c *CommandConsumer) Drain() {
	for _, sub := range c.subs {
		if err := sub.Drain(); err != nil {
			slog.Error("kiiex.consumer.drain_failed", "error", err)
		}
	}
}
