package eventbus

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type TestEvent struct {
	Message string
}

type AnotherEvent struct {
	Value int
}

func TestEventBus_Subscribe_And_Publish(t *testing.T) {
	bus := New()

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(TestEvent{}, func(event interface{}) {
		if e, ok := event.(TestEvent); ok {
			received = e
			wg.Done()
		}
	})

	bus.Publish(TestEvent{Message: "hello"})

	// Wait for async handler
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, "hello", received.Message)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBus_PublishSync(t *testing.T) {
	bus := New()

	var received TestEvent

	bus.Subscribe(TestEvent{}, func(event interface{}) {
		if e, ok := event.(TestEvent); ok {
			received = e
		}
	})

	bus.PublishSync(TestEvent{Message: "sync"})

	assert.Equal(t, "sync", received.Message)
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := New()

	var count int
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(3)

	handler := func(event interface{}) {
		mu.Lock()
		count++
		mu.Unlock()
		wg.Done()
	}

	bus.Subscribe(TestEvent{}, handler)
	bus.Subscribe(TestEvent{}, handler)
	bus.Subscribe(TestEvent{}, handler)

	bus.Publish(TestEvent{Message: "test"})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		assert.Equal(t, 3, count)
		mu.Unlock()
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for events")
	}
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	bus := New()

	var receivedTest bool
	var receivedAnother bool
	var wg sync.WaitGroup
	wg.Add(2)

	bus.Subscribe(TestEvent{}, func(event interface{}) {
		receivedTest = true
		wg.Done()
	})

	bus.Subscribe(AnotherEvent{}, func(event interface{}) {
		receivedAnother = true
		wg.Done()
	})

	bus.Publish(TestEvent{Message: "test"})
	bus.Publish(AnotherEvent{Value: 42})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, receivedTest)
		assert.True(t, receivedAnother)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for events")
	}
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := New()

	// Should not panic
	bus.Publish(TestEvent{Message: "no subscribers"})
}

func TestEventBus_HasSubscribers(t *testing.T) {
	bus := New()

	assert.False(t, bus.HasSubscribers(TestEvent{}))

	bus.Subscribe(TestEvent{}, func(event interface{}) {})

	assert.True(t, bus.HasSubscribers(TestEvent{}))
	assert.False(t, bus.HasSubscribers(AnotherEvent{}))
}

func TestEventBus_SubscriberCount(t *testing.T) {
	bus := New()

	assert.Equal(t, 0, bus.SubscriberCount(TestEvent{}))

	bus.Subscribe(TestEvent{}, func(event interface{}) {})
	assert.Equal(t, 1, bus.SubscriberCount(TestEvent{}))

	bus.Subscribe(TestEvent{}, func(event interface{}) {})
	assert.Equal(t, 2, bus.SubscriberCount(TestEvent{}))
}
