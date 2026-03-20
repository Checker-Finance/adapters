package xfx

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/nats-io/nats.go"
)

// CommandConsumer subscribes to NATS command subjects and routes them to the service.
type CommandConsumer struct {
	nc   *nats.Conn
	svc  *Service
	subs []*nats.Subscription
}

// NewCommandConsumer creates a new CommandConsumer.
func NewCommandConsumer(nc *nats.Conn, svc *Service) *CommandConsumer {
	return &CommandConsumer{nc: nc, svc: svc}
}

// Subscribe registers NATS subscriptions for quote request and trade execute commands.
// Messages are dispatched asynchronously; the handler goroutines inherit the provided context.
func (c *CommandConsumer) Subscribe(ctx context.Context, quoteSubject, tradeSubject string) error {
	subQ, err := c.nc.Subscribe(quoteSubject, func(msg *nats.Msg) {
		var env model.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			slog.Error("xfx.cmd.quote_request.unmarshal_failed", "error", err)
			return
		}
		var req model.QuoteRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			slog.Error("xfx.cmd.quote_request.payload_failed",
				"client", env.ClientID,
				"error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := c.svc.HandleQuoteRequest(msgCtx, env, req); err != nil {
			slog.Error("xfx.cmd.quote_request.handle_failed",
				"client", env.ClientID,
				"error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, subQ)

	subT, err := c.nc.Subscribe(tradeSubject, func(msg *nats.Msg) {
		var env model.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			slog.Error("xfx.cmd.trade_execute.unmarshal_failed", "error", err)
			return
		}
		var cmd model.TradeCommand
		if err := json.Unmarshal(env.Payload, &cmd); err != nil {
			slog.Error("xfx.cmd.trade_execute.payload_failed",
				"client", env.ClientID,
				"error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := c.svc.HandleTradeExecute(msgCtx, env, cmd); err != nil {
			slog.Error("xfx.cmd.trade_execute.handle_failed",
				"client", env.ClientID,
				"error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, subT)

	slog.Info("xfx.command_consumer.subscribed",
		"quote_subject", quoteSubject,
		"trade_subject", tradeSubject)
	return nil
}

// Drain drains all active subscriptions gracefully.
func (c *CommandConsumer) Drain() {
	for _, sub := range c.subs {
		_ = sub.Drain()
	}
}
