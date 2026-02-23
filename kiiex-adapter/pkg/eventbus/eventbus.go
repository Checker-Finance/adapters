package eventbus

import (
	"reflect"
	"sync"
)

// Handler is a function that handles an event
type Handler func(event interface{})

// EventBus provides in-process pub/sub
type EventBus struct {
	handlers map[reflect.Type][]Handler
	mu       sync.RWMutex
}

// New creates a new EventBus
func New() *EventBus {
	return &EventBus{
		handlers: make(map[reflect.Type][]Handler),
	}
}

// Subscribe registers a handler for a specific event type
func (e *EventBus) Subscribe(eventType interface{}, handler Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()

	t := reflect.TypeOf(eventType)
	e.handlers[t] = append(e.handlers[t], handler)
}

// SubscribeFunc registers a typed handler function
// The handler function should have the signature: func(EventType)
func (e *EventBus) SubscribeFunc(handler interface{}) {
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()

	if handlerType.Kind() != reflect.Func {
		panic("handler must be a function")
	}

	if handlerType.NumIn() != 1 {
		panic("handler must have exactly one argument")
	}

	eventType := handlerType.In(0)

	e.mu.Lock()
	defer e.mu.Unlock()

	wrappedHandler := func(event interface{}) {
		eventValue := reflect.ValueOf(event)
		if eventValue.Type().AssignableTo(eventType) {
			handlerValue.Call([]reflect.Value{eventValue})
		} else if eventValue.Type().Kind() == reflect.Ptr && eventValue.Elem().Type().AssignableTo(eventType) {
			handlerValue.Call([]reflect.Value{eventValue.Elem()})
		} else if eventType.Kind() == reflect.Ptr && reflect.PtrTo(eventValue.Type()).AssignableTo(eventType) {
			ptr := reflect.New(eventValue.Type())
			ptr.Elem().Set(eventValue)
			handlerValue.Call([]reflect.Value{ptr})
		}
	}

	e.handlers[eventType] = append(e.handlers[eventType], wrappedHandler)
}

// Publish publishes an event to all subscribers
func (e *EventBus) Publish(event interface{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	eventType := reflect.TypeOf(event)

	// Try exact type match
	if handlers, ok := e.handlers[eventType]; ok {
		for _, handler := range handlers {
			go handler(event)
		}
	}

	// If event is a pointer, also try the element type
	if eventType.Kind() == reflect.Ptr {
		elemType := eventType.Elem()
		if handlers, ok := e.handlers[elemType]; ok {
			for _, handler := range handlers {
				go handler(reflect.ValueOf(event).Elem().Interface())
			}
		}
	}

	// If event is not a pointer, also try the pointer type
	if eventType.Kind() != reflect.Ptr {
		ptrType := reflect.PtrTo(eventType)
		if handlers, ok := e.handlers[ptrType]; ok {
			ptr := reflect.New(eventType)
			ptr.Elem().Set(reflect.ValueOf(event))
			for _, handler := range handlers {
				go handler(ptr.Interface())
			}
		}
	}
}

// PublishSync publishes an event synchronously to all subscribers
func (e *EventBus) PublishSync(event interface{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	eventType := reflect.TypeOf(event)

	if handlers, ok := e.handlers[eventType]; ok {
		for _, handler := range handlers {
			handler(event)
		}
	}

	// If event is a pointer, also try the element type
	if eventType.Kind() == reflect.Ptr {
		elemType := eventType.Elem()
		if handlers, ok := e.handlers[elemType]; ok {
			for _, handler := range handlers {
				handler(reflect.ValueOf(event).Elem().Interface())
			}
		}
	}
}

// HasSubscribers returns true if there are subscribers for the event type
func (e *EventBus) HasSubscribers(eventType interface{}) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	t := reflect.TypeOf(eventType)
	handlers, ok := e.handlers[t]
	return ok && len(handlers) > 0
}

// SubscriberCount returns the number of subscribers for an event type
func (e *EventBus) SubscriberCount(eventType interface{}) int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	t := reflect.TypeOf(eventType)
	return len(e.handlers[t])
}
