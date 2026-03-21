// Package nats provides shared NATS command consumer infrastructure for adapters
// that use the standard Envelope+QuoteRequest/TradeCommand dispatch pattern.
package nats

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	natsio "github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// CommandService is implemented by adapters that handle quote and trade commands.
type CommandService interface {
	HandleQuoteRequest(ctx context.Context, env model.Envelope, req model.QuoteRequest) error
	HandleTradeExecute(ctx context.Context, env model.Envelope, cmd model.TradeCommand) error
}

// CommandConsumer subscribes to NATS command subjects and dispatches them to a CommandService.
// It handles the standard Envelope unwrapping, payload unmarshaling, timeout management,
// and error logging common to XFX, Zodia, and Capa adapters.
type CommandConsumer struct {
	nc    *natsio.Conn
	svc   CommandService
	venue string // log key prefix, e.g. "xfx", "zodia", "capa"
	subs  []*natsio.Subscription
}

// NewCommandConsumer creates a CommandConsumer for the given venue. Call Subscribe to start.
func NewCommandConsumer(nc *natsio.Conn, svc CommandService, venue string) *CommandConsumer {
	return &CommandConsumer{nc: nc, svc: svc, venue: venue}
}

// Subscribe registers NATS subscriptions for quote request and trade execute commands.
// ctx is checked before processing each message to gate new work during shutdown;
// per-message timeouts use context.Background() so in-flight handlers complete during Drain.
func (c *CommandConsumer) Subscribe(ctx context.Context, quoteSubject, tradeSubject string) error {
	subQ, err := c.nc.Subscribe(quoteSubject, func(msg *natsio.Msg) {
		if ctx.Err() != nil {
			return
		}
		var env model.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			slog.Error(c.venue+".cmd.quote_request.unmarshal_failed", "error", err)
			return
		}
		var req model.QuoteRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			slog.Error(c.venue+".cmd.quote_request.payload_failed",
				"client", env.ClientID,
				"error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.svc.HandleQuoteRequest(msgCtx, env, req); err != nil {
			slog.Error(c.venue+".cmd.quote_request.handle_failed",
				"client", env.ClientID,
				"error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, subQ)

	subT, err := c.nc.Subscribe(tradeSubject, func(msg *natsio.Msg) {
		if ctx.Err() != nil {
			return
		}
		var env model.Envelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			slog.Error(c.venue+".cmd.trade_execute.unmarshal_failed", "error", err)
			return
		}
		var cmd model.TradeCommand
		if err := json.Unmarshal(env.Payload, &cmd); err != nil {
			slog.Error(c.venue+".cmd.trade_execute.payload_failed",
				"client", env.ClientID,
				"error", err)
			return
		}
		msgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.svc.HandleTradeExecute(msgCtx, env, cmd); err != nil {
			slog.Error(c.venue+".cmd.trade_execute.handle_failed",
				"client", env.ClientID,
				"error", err)
		}
	})
	if err != nil {
		return err
	}
	c.subs = append(c.subs, subT)

	slog.Info(c.venue+".command_consumer.subscribed",
		"quote_subject", quoteSubject,
		"trade_subject", tradeSubject)
	return nil
}

// Drain drains all active subscriptions gracefully.
func (c *CommandConsumer) Drain() {
	for _, sub := range c.subs {
		if err := sub.Drain(); err != nil {
			slog.Warn(c.venue+".cmd.drain_failed", "error", err)
		}
	}
}
