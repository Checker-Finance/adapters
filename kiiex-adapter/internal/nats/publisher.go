package nats

import (
	"context"
	"log/slog"

	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

const (
	subjectKiiexFilled    = "evt.trade.filled.v1.KIIEX"
	subjectKiiexCancelled = "evt.trade.cancelled.v1.KIIEX"
)

// NATSPublisher subscribes to the in-process event bus and publishes fill/cancel events to NATS.
type NATSPublisher struct {
	pub      *publisher.Publisher
	eventBus *eventbus.EventBus
}

// NewNATSPublisher creates a NATSPublisher that listens to eventBus and forwards events to NATS.
func NewNATSPublisher(pub *publisher.Publisher, eventBus *eventbus.EventBus) *NATSPublisher {
	p := &NATSPublisher{
		pub:      pub,
		eventBus: eventBus,
	}
	p.subscribeToEvents()
	return p
}

func (p *NATSPublisher) subscribeToEvents() {
	p.eventBus.Subscribe(order.FillArrivedEvent{}, func(event interface{}) {
		fill, ok := event.(*order.FillArrivedEvent)
		if !ok {
			if v, ok2 := event.(order.FillArrivedEvent); ok2 {
				fill = &v
			} else {
				return
			}
		}
		p.publishFillArrived(fill)
	})

	p.eventBus.Subscribe(order.OrderCanceledEvent{}, func(event interface{}) {
		cancel, ok := event.(*order.OrderCanceledEvent)
		if !ok {
			if v, ok2 := event.(order.OrderCanceledEvent); ok2 {
				cancel = &v
			} else {
				return
			}
		}
		p.publishOrderCanceled(cancel)
	})
}

func (p *NATSPublisher) publishFillArrived(event *order.FillArrivedEvent) {
	if event == nil || event.OrderID == "" {
		slog.Error("kiiex.publisher.fill_invalid", "event", event)
		return
	}
	slog.Info("kiiex.publisher.fill",
		"orderId", event.OrderID,
		"instrumentPair", event.InstrumentPair,
		"status", event.Status,
	)
	if err := p.pub.Publish(context.Background(), subjectKiiexFilled, event); err != nil {
		slog.Error("kiiex.publisher.fill_failed", "error", err)
	}
}

func (p *NATSPublisher) publishOrderCanceled(event *order.OrderCanceledEvent) {
	if event == nil || event.OrderID == "" {
		slog.Error("kiiex.publisher.cancel_invalid", "event", event)
		return
	}
	slog.Info("kiiex.publisher.cancel",
		"orderId", event.OrderID,
	)
	if err := p.pub.Publish(context.Background(), subjectKiiexCancelled, event); err != nil {
		slog.Error("kiiex.publisher.cancel_failed", "error", err)
	}
}
