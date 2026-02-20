package publisher

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/metrics"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/logger"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// Publisher wraps a NATS connection and provides helpers for publishing canonical events.
type Publisher struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	subject string
	service string
}

// New creates a new Publisher with JetStream enabled if available.
func New(nc *nats.Conn, subject, service string) (*Publisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	return &Publisher{
		nc:      nc,
		js:      js,
		subject: subject,
		service: service,
	}, nil
}

// PublishEnvelope serializes and publishes a canonical event envelope to NATS.
func (p *Publisher) PublishEnvelope(ctx context.Context, subject string, env *model.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		logger.S().Errorw("publisher.marshal_failed",
			"subject", subject,
			"event_type", env.EventType,
			"error", err,
		)
		metrics.IncError("publisher", "marshal_failed")
		return err
	}

	if subject == "" {
		subject = p.subject
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header: nats.Header{
			"event_type":     []string{env.EventType},
			"correlation_id": []string{env.CorrelationID.String()},
			"service":        []string{p.service},
			"content_type":   []string{"application/json"},
			"tenant_id":      []string{env.TenantID},
			"client_id":      []string{env.ClientID},
		},
	}

	start := time.Now()
	_, err = p.js.PublishMsg(msg)
	metrics.ObserveDuration(metrics.NATSMessageLatency, start, subject)

	if err != nil {
		logger.S().Errorw("publisher.publish_failed",
			"subject", subject,
			"event_type", env.EventType,
			"client_id", env.ClientID,
			"error", err,
		)
		metrics.IncNATSMessage(subject, "error")
		return err
	}

	logger.S().Infow("publisher.publish_success",
		"subject", subject,
		"event_type", env.EventType,
		"client_id", env.ClientID,
	)

	metrics.IncNATSMessage(subject, "ok")
	return nil
}

// PublishBalanceUpdated emits canonical balance.updated events.
func (p *Publisher) PublishBalanceUpdated(ctx context.Context, bal model.Balance, tenantID, clientID string) error {
	env := &model.Envelope{
		ID:            uuid.New(),
		CorrelationID: uuid.New(),
		TenantID:      tenantID,
		ClientID:      clientID,
		Topic:         "evt.balance.updated.v1",
		EventType:     "balance.updated",
		Version:       "1.0.0",
		Timestamp:     time.Now().UTC(),
	}

	data, _ := json.Marshal(bal)
	env.Payload = data

	return p.PublishEnvelope(ctx, "evt.balance.updated.v1", env)
}

// Publish publishes raw JSON payloads (for non-canonical internal events).
func (p *Publisher) Publish(ctx context.Context, subject string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		metrics.IncError("publisher", "marshal_failed")
		return err
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{"source": []string{p.service}},
	}

	start := time.Now()
	_, err = p.js.PublishMsg(msg)
	metrics.ObserveDuration(metrics.NATSMessageLatency, start, subject)

	if err != nil {
		metrics.IncNATSMessage(subject, "error")
		return err
	}

	metrics.IncNATSMessage(subject, "ok")
	return nil
}

func (p *Publisher) Close() {
	if p.nc != nil && p.nc.IsConnected() {
		p.nc.Close()
	}
}
