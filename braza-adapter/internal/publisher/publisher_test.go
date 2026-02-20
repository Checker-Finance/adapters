package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
)

// --- mock types ---

type mockJetStream struct {
	nats.JetStreamContext
	published []*nats.Msg
	fail      bool
}

func (m *mockJetStream) PublishMsg(msg *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if m.fail {
		return nil, errors.New("mock publish error")
	}
	m.published = append(m.published, msg)
	return &nats.PubAck{Stream: "mock-stream", Sequence: 1}, nil
}

// --- helper ---

func newTestPublisher(fail bool) *Publisher {
	nc, _ := nats.Connect(nats.DefaultURL, nats.Name("mock"))
	js := &mockJetStream{fail: fail}
	return &Publisher{
		nc:      nc,
		js:      js,
		subject: "evt.test.v1",
		service: "braza-adapter",
	}
}

// --- tests ---

func TestPublishEnvelope_Success(t *testing.T) {
	pub := newTestPublisher(false)
	env := &model.Envelope{
		ID:            uuid.New(),
		CorrelationID: uuid.New(),
		TenantID:      "tenant-001",
		ClientID:      "client-001",
		Topic:         "evt.balance.updated.v1",
		EventType:     "balance.updated",
		Version:       "1.0.0",
		Timestamp:     time.Now(),
		Payload:       json.RawMessage(`{"instrument":"USDBRL","available_total_value":59992}`),
	}

	err := pub.PublishEnvelope(context.Background(), "evt.balance.updated.v1", env)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	js := pub.js.(*mockJetStream)
	if len(js.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(js.published))
	}

	msg := js.published[0]
	if msg.Subject != "evt.balance.updated.v1" {
		t.Errorf("unexpected subject: %s", msg.Subject)
	}

	// verify headers
	if msg.Header.Get("event_type") != "balance.updated" {
		t.Errorf("expected header event_type=balance.updated, got %s", msg.Header.Get("event_type"))
	}

	// verify payload round-trip
	var parsed model.Envelope
	if err := json.Unmarshal(msg.Data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if parsed.ClientID != "client-001" {
		t.Errorf("expected client_id=client-001, got %s", parsed.ClientID)
	}
}

func TestPublishEnvelope_Failure(t *testing.T) {
	pub := newTestPublisher(true)
	env := &model.Envelope{
		ID:        uuid.New(),
		EventType: "balance.updated",
	}

	err := pub.PublishEnvelope(context.Background(), "evt.balance.updated.v1", env)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPublishBalanceUpdated(t *testing.T) {
	pub := newTestPublisher(false)
	bal := model.Balance{
		Venue:       "BRAZA",
		Instrument:  "USDBRL",
		Available:   59992,
		CanBuy:      true,
		CanSell:     true,
		LastUpdated: time.Now(),
	}

	err := pub.PublishBalanceUpdated(context.Background(), bal, "tenant-1", "client-1")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	js := pub.js.(*mockJetStream)
	if len(js.published) == 0 {
		t.Fatal("expected at least one published message")
	}

	msg := js.published[0]
	var env model.Envelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	if env.Topic != "evt.balance.updated.v1" {
		t.Errorf("expected topic=evt.balance.updated.v1, got %s", env.Topic)
	}
	if env.EventType != "balance.updated" {
		t.Errorf("expected event_type=balance.updated, got %s", env.EventType)
	}
}
