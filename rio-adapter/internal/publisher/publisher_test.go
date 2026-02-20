package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
)

// --- mock types ---

// mockJetStream implements a minimal JetStreamContext for testing
type mockJetStream struct {
	published []*nats.Msg
	fail      bool
}

func (m *mockJetStream) PublishMsg(msg *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if m.fail {
		return nil, errors.New("mock publish error")
	}
	m.published = append(m.published, msg)
	return &nats.PubAck{Stream: "mock-stream"}, nil
}

// Implement remaining JetStreamContext interface methods as no-ops for testing
func (m *mockJetStream) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	return nil, nil
}
func (m *mockJetStream) PublishAsync(subj string, data []byte, opts ...nats.PubOpt) (nats.PubAckFuture, error) {
	return nil, nil
}
func (m *mockJetStream) PublishMsgAsync(msg *nats.Msg, opts ...nats.PubOpt) (nats.PubAckFuture, error) {
	return nil, nil
}
func (m *mockJetStream) PublishAsyncPending() int { return 0 }
func (m *mockJetStream) PublishAsyncComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (m *mockJetStream) CleanupPublisher() {}
func (m *mockJetStream) Subscribe(subj string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) SubscribeSync(subj string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) ChanSubscribe(subj string, ch chan *nats.Msg, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) ChanQueueSubscribe(subj, queue string, ch chan *nats.Msg, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) QueueSubscribe(subj, queue string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) QueueSubscribeSync(subj, queue string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) PullSubscribe(subj, durable string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (m *mockJetStream) AddStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (m *mockJetStream) UpdateStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (m *mockJetStream) DeleteStream(name string, opts ...nats.JSOpt) error { return nil }
func (m *mockJetStream) StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (m *mockJetStream) Streams(opts ...nats.JSOpt) <-chan *nats.StreamInfo {
	ch := make(chan *nats.StreamInfo)
	close(ch)
	return ch
}
func (m *mockJetStream) PurgeStream(name string, opts ...nats.JSOpt) error { return nil }
func (m *mockJetStream) StreamsInfo(opts ...nats.JSOpt) <-chan *nats.StreamInfo {
	ch := make(chan *nats.StreamInfo)
	close(ch)
	return ch
}
func (m *mockJetStream) StreamNames(opts ...nats.JSOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (m *mockJetStream) GetMsg(name string, seq uint64, opts ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	return nil, nil
}
func (m *mockJetStream) GetLastMsg(name, subj string, opts ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	return nil, nil
}
func (m *mockJetStream) DeleteMsg(name string, seq uint64, opts ...nats.JSOpt) error { return nil }
func (m *mockJetStream) SecureDeleteMsg(name string, seq uint64, opts ...nats.JSOpt) error {
	return nil
}
func (m *mockJetStream) AddConsumer(stream string, cfg *nats.ConsumerConfig, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (m *mockJetStream) UpdateConsumer(stream string, cfg *nats.ConsumerConfig, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (m *mockJetStream) DeleteConsumer(stream, consumer string, opts ...nats.JSOpt) error { return nil }
func (m *mockJetStream) ConsumerInfo(stream, name string, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (m *mockJetStream) Consumers(stream string, opts ...nats.JSOpt) <-chan *nats.ConsumerInfo {
	ch := make(chan *nats.ConsumerInfo)
	close(ch)
	return ch
}
func (m *mockJetStream) ConsumersInfo(stream string, opts ...nats.JSOpt) <-chan *nats.ConsumerInfo {
	ch := make(chan *nats.ConsumerInfo)
	close(ch)
	return ch
}
func (m *mockJetStream) ConsumerNames(stream string, opts ...nats.JSOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (m *mockJetStream) AccountInfo(opts ...nats.JSOpt) (*nats.AccountInfo, error) { return nil, nil }
func (m *mockJetStream) StreamNameBySubject(string, ...nats.JSOpt) (string, error) { return "", nil }
func (m *mockJetStream) KeyValue(bucket string) (nats.KeyValue, error)             { return nil, nil }
func (m *mockJetStream) CreateKeyValue(cfg *nats.KeyValueConfig) (nats.KeyValue, error) {
	return nil, nil
}
func (m *mockJetStream) DeleteKeyValue(bucket string) error { return nil }
func (m *mockJetStream) KeyValueStoreNames() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (m *mockJetStream) KeyValueStores() <-chan nats.KeyValueStatus {
	ch := make(chan nats.KeyValueStatus)
	close(ch)
	return ch
}
func (m *mockJetStream) ObjectStore(bucket string) (nats.ObjectStore, error) { return nil, nil }
func (m *mockJetStream) CreateObjectStore(cfg *nats.ObjectStoreConfig) (nats.ObjectStore, error) {
	return nil, nil
}
func (m *mockJetStream) DeleteObjectStore(bucket string) error { return nil }
func (m *mockJetStream) ObjectStoreNames(opts ...nats.ObjectOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (m *mockJetStream) ObjectStores(opts ...nats.ObjectOpt) <-chan nats.ObjectStoreStatus {
	ch := make(chan nats.ObjectStoreStatus)
	close(ch)
	return ch
}

// --- helper ---

func newTestPublisher(fail bool) *Publisher {
	js := &mockJetStream{fail: fail}
	return &Publisher{
		nc:      nil,
		js:      js,
		subject: "evt.test.v1",
		service: "rio-adapter",
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
		Payload:       json.RawMessage(`{"instrument":"USDBRL","available":59992}`),
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
		ID:          1,
		Venue:       "RIO",
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
