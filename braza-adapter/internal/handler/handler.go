package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	_ "github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	_ "github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// Handler consumes NATS commands for the Braza Adapter
// and delegates processing to the Braza service layer.
type Handler struct {
	ctx      context.Context
	nc       *nats.Conn
	service  *braza.Service
	subjects []string // NATS subjects to subscribe to
}

// NewHandler constructs a new Handler with its dependencies.
func NewHandler(
	ctx context.Context,
	nc *nats.Conn,
	service *braza.Service,
) *Handler {
	return &Handler{
		ctx:     ctx,
		nc:      nc,
		service: service,
		subjects: []string{
			"cmd.lp.quote_request.v1.BRAZA",
			"cmd.lp.trade_execute.v1.BRAZA",
		},
	}
}

// Start subscribes to NATS subjects and begins processing incoming messages.
func (h *Handler) Start() error {
	for _, subj := range h.subjects {
		if _, err := h.nc.QueueSubscribe(subj, "braza-adapter-workers", h.handleMessage); err != nil {
			return fmt.Errorf("subscribe %s: %w", subj, err)
		}
		slog.Info("subscribed to NATS subject", "subject", subj)
	}
	return nil
}

// handleMessage routes a message to the correct command handler.
func (h *Handler) handleMessage(msg *nats.Msg) {
	start := time.Now()

	var env model.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		slog.Warn("invalid envelope", "error", err)
		return
	}

	switch env.EventType {
	case "cmd.lp.quote_request.v1.BRAZA":
		h.onQuoteRequest(env)
	case "cmd.lp.trade_execute.v1.BRAZA":
		h.onTradeExecute(env)
	default:
		slog.Warn("unknown event type", "event_type", env.EventType)
	}

	slog.Debug("message handled",
		"event_type", env.EventType,
		"latency", time.Since(start),
	)
}

// onQuoteRequest handles a quote request command for Braza.
func (h *Handler) onQuoteRequest(env model.Envelope) {
	ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()

	var req model.QuoteRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		slog.Warn("invalid quote request payload", "error", err)
		h.service.PublishErrorEvent(env, err, "BAD_PAYLOAD", false)
		return
	}

	slog.Info("processing quote request",
		"tenant_id", env.TenantID,
		"client_id", env.ClientID,
		"instrument", req.Instrument,
		"side", req.Side,
	)

	if err := h.service.HandleQuoteRequest(ctx, env, req); err != nil {
		h.service.PublishErrorEvent(env, err, "QUOTE_FAILED", true)
	}
}

// onTradeExecute handles a trade execution command for Braza.
func (h *Handler) onTradeExecute(env model.Envelope) {
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()

	var cmd model.TradeCommand
	if err := json.Unmarshal(env.Payload, &cmd); err != nil {
		slog.Warn("invalid trade execute payload", "error", err)
		h.service.PublishErrorEvent(env, err, "BAD_PAYLOAD", false)
		return
	}

	slog.Info("processing trade execute",
		"tenant_id", env.TenantID,
		"client_id", env.ClientID,
		"instrument", cmd.Instrument,
		"side", cmd.Side,
		"quantity", cmd.Quantity,
	)

	if err := h.service.HandleTradeExecute(ctx, env, cmd); err != nil {
		h.service.PublishErrorEvent(env, err, "TRADE_FAILED", true)
	}
}
