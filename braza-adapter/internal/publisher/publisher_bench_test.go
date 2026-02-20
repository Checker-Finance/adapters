package publisher

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// --- mockJetStream for benchmarks (non-blocking, no error) ---

type benchJetStream struct{}

func (b *benchJetStream) PublishMsg(msg *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error) {
	return &nats.PubAck{Stream: "mock-stream", Seq: 1}, nil
}

// --- setup helper ---

func newBenchPublisher() *Publisher {
	nc, _ := nats.Connect(nats.DefaultURL, nats.Name("bench"))
	js := &benchJetStream{}
	return &Publisher{
		nc:      nc,
		js:      js,
		subject: "evt.test.v1",
		service: "braza-adapter",
	}
}

// --- benchmarks ---

func BenchmarkPublishEnvelope(b *testing.B) {
	pub := newBenchPublisher()
	env := &model.Envelope{
		ID:            uuid.New(),
		CorrelationID: uuid.New(),
		TenantID:      "tenantA",
		ClientID:      "client1",
		Topic:         "evt.balance.updated.v1",
		EventType:     "balance.updated",
		Version:       "1.0.0",
		Timestamp:     time.Now(),
		Payload:       json.RawMessage(`{"instrument":"USDBRL","available_total_value":59992}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := pub.PublishEnvelope(context.Background(), "", env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublishBalanceUpdated(b *testing.B) {
	pub := newBenchPublisher()

	bal := model.Balance{
		ID:                  uuid.New(),
		Venue:               "BRAZA",
		Instrument:          "USDBRL",
		AvailableTotalValue: 59992,
		CanBuy:              true,
		CanSell:             true,
		LastUpdated:         time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientID := "client" + strconv.Itoa(i%100)
		tenantID := "tenant" + strconv.Itoa(i%10)
		if err := pub.PublishBalanceUpdated(context.Background(), bal, tenantID, clientID); err != nil {
			b.Fatal(err)
		}
	}
}
