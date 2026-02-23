package order

import (
	"fmt"
	"strings"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/alphapoint"
)

// FillAdapter converts AlphaPoint orders to fill events
type FillAdapter struct{}

// Adapt converts an alphapoint.Order to a FillArrivedEvent
func (a *FillAdapter) Adapt(o *alphapoint.Order) *FillArrivedEvent {
	event := NewFillArrivedEvent()
	event.FillID = fmt.Sprintf("%d", o.OrderID)
	event.Price = fmt.Sprintf("%f", o.Price)
	event.QuantityFilled = fmt.Sprintf("%f", o.QuantityExecuted)
	event.QuantityLeaves = fmt.Sprintf("%f", o.OrigQuantity-o.QuantityExecuted)
	event.Side = strings.ToLower(o.Side)
	event.Status = strings.ToLower(o.OrderState)
	event.Type = strings.ToLower(o.OrderType)
	event.ClientID = fmt.Sprintf("%d", o.Account)
	event.ClientOrderID = fmt.Sprintf("%d", o.ClientOrderID)
	event.Provider = fmt.Sprintf("%d", o.CounterPartyID)
	event.Source = fmt.Sprintf("%d", o.OmsID)
	event.Date = fmt.Sprintf("%d", o.ReceiveTime)
	return event
}

// AdaptOrder is a package-level function for adapting orders
func AdaptOrder(o *alphapoint.Order) *FillArrivedEvent {
	adapter := &FillAdapter{}
	return adapter.Adapt(o)
}
