package publisher

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// --- benchJetStream for benchmarks (non-blocking, no error) ---

type benchJetStream struct{}

func (b *benchJetStream) PublishMsg(msg *nats.Msg, opts ...nats.PubOpt) (*nats.PubAck, error) {
	return &nats.PubAck{Stream: "mock-stream"}, nil
}

// Implement remaining JetStreamContext interface methods as no-ops for benchmarking
func (b *benchJetStream) Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	return nil, nil
}
func (b *benchJetStream) PublishAsync(subj string, data []byte, opts ...nats.PubOpt) (nats.PubAckFuture, error) {
	return nil, nil
}
func (b *benchJetStream) PublishMsgAsync(msg *nats.Msg, opts ...nats.PubOpt) (nats.PubAckFuture, error) {
	return nil, nil
}
func (b *benchJetStream) PublishAsyncPending() int { return 0 }
func (b *benchJetStream) PublishAsyncComplete() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (b *benchJetStream) CleanupPublisher() {}
func (b *benchJetStream) Subscribe(subj string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) SubscribeSync(subj string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) ChanSubscribe(subj string, ch chan *nats.Msg, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) ChanQueueSubscribe(subj, queue string, ch chan *nats.Msg, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) QueueSubscribe(subj, queue string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) QueueSubscribeSync(subj, queue string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) PullSubscribe(subj, durable string, opts ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, nil
}
func (b *benchJetStream) AddStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (b *benchJetStream) UpdateStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (b *benchJetStream) DeleteStream(name string, opts ...nats.JSOpt) error { return nil }
func (b *benchJetStream) StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, nil
}
func (b *benchJetStream) Streams(opts ...nats.JSOpt) <-chan *nats.StreamInfo {
	ch := make(chan *nats.StreamInfo)
	close(ch)
	return ch
}
func (b *benchJetStream) PurgeStream(name string, opts ...nats.JSOpt) error { return nil }
func (b *benchJetStream) StreamsInfo(opts ...nats.JSOpt) <-chan *nats.StreamInfo {
	ch := make(chan *nats.StreamInfo)
	close(ch)
	return ch
}
func (b *benchJetStream) StreamNames(opts ...nats.JSOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (b *benchJetStream) GetMsg(name string, seq uint64, opts ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	return nil, nil
}
func (b *benchJetStream) GetLastMsg(name, subj string, opts ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	return nil, nil
}
func (b *benchJetStream) DeleteMsg(name string, seq uint64, opts ...nats.JSOpt) error { return nil }
func (b *benchJetStream) SecureDeleteMsg(name string, seq uint64, opts ...nats.JSOpt) error {
	return nil
}
func (b *benchJetStream) AddConsumer(stream string, cfg *nats.ConsumerConfig, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (b *benchJetStream) UpdateConsumer(stream string, cfg *nats.ConsumerConfig, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (b *benchJetStream) DeleteConsumer(stream, consumer string, opts ...nats.JSOpt) error {
	return nil
}
func (b *benchJetStream) ConsumerInfo(stream, name string, opts ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, nil
}
func (b *benchJetStream) Consumers(stream string, opts ...nats.JSOpt) <-chan *nats.ConsumerInfo {
	ch := make(chan *nats.ConsumerInfo)
	close(ch)
	return ch
}
func (b *benchJetStream) ConsumersInfo(stream string, opts ...nats.JSOpt) <-chan *nats.ConsumerInfo {
	ch := make(chan *nats.ConsumerInfo)
	close(ch)
	return ch
}
func (b *benchJetStream) ConsumerNames(stream string, opts ...nats.JSOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (b *benchJetStream) AccountInfo(opts ...nats.JSOpt) (*nats.AccountInfo, error) { return nil, nil }
func (b *benchJetStream) StreamNameBySubject(string, ...nats.JSOpt) (string, error) { return "", nil }
func (b *benchJetStream) KeyValue(bucket string) (nats.KeyValue, error)             { return nil, nil }
func (b *benchJetStream) CreateKeyValue(cfg *nats.KeyValueConfig) (nats.KeyValue, error) {
	return nil, nil
}
func (b *benchJetStream) DeleteKeyValue(bucket string) error { return nil }
func (b *benchJetStream) KeyValueStoreNames() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (b *benchJetStream) KeyValueStores() <-chan nats.KeyValueStatus {
	ch := make(chan nats.KeyValueStatus)
	close(ch)
	return ch
}
func (b *benchJetStream) ObjectStore(bucket string) (nats.ObjectStore, error) { return nil, nil }
func (b *benchJetStream) CreateObjectStore(cfg *nats.ObjectStoreConfig) (nats.ObjectStore, error) {
	return nil, nil
}
func (b *benchJetStream) DeleteObjectStore(bucket string) error { return nil }
func (b *benchJetStream) ObjectStoreNames(opts ...nats.ObjectOpt) <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
func (b *benchJetStream) ObjectStores(opts ...nats.ObjectOpt) <-chan nats.ObjectStoreStatus {
	ch := make(chan nats.ObjectStoreStatus)
	close(ch)
	return ch
}

// --- setup helper ---

func newBenchPublisher() *Publisher {
	js := &benchJetStream{}
	return &Publisher{
		nc:      nil,
		js:      js,
		subject: "evt.test.v1",
		service: "rio-adapter",
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
		Payload:       json.RawMessage(`{"instrument":"USDBRL","available":59992}`),
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
		ID:          1,
		Venue:       "RIO",
		Instrument:  "USDBRL",
		Available:   59992,
		CanBuy:      true,
		CanSell:     true,
		LastUpdated: time.Now(),
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
