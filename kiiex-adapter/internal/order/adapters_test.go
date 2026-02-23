package order

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/alphapoint"
)

func TestFillAdapter_Adapt(t *testing.T) {
	o := &alphapoint.Order{
		OrderID:          12345,
		Price:            100.50,
		QuantityExecuted: 10.0,
		OrigQuantity:     15.0,
		Side:             "Buy",
		OrderState:       "Filled",
		OrderType:        "Limit",
		Account:          100,
		ClientOrderID:    999,
		CounterPartyID:   200,
		OmsID:            1,
		ReceiveTime:      1234567890,
	}

	adapter := &FillAdapter{}
	event := adapter.Adapt(o)

	assert.Equal(t, "12345", event.FillID)
	assert.Equal(t, "100.500000", event.Price)
	assert.Equal(t, "10.000000", event.QuantityFilled)
	assert.Equal(t, "5.000000", event.QuantityLeaves)
	assert.Equal(t, "buy", event.Side)
	assert.Equal(t, "filled", event.Status)
	assert.Equal(t, "limit", event.Type)
	assert.Equal(t, "100", event.ClientID)
	assert.Equal(t, "999", event.ClientOrderID)
	assert.Equal(t, "200", event.Provider)
	assert.Equal(t, "1", event.Source)
	assert.Equal(t, "1234567890", event.Date)
	assert.Equal(t, "trade", event.ExecutionType)
}

func TestAdaptOrder(t *testing.T) {
	o := &alphapoint.Order{
		OrderID:          54321,
		Price:            200.75,
		QuantityExecuted: 5.0,
		OrigQuantity:     5.0,
		Side:             "Sell",
		OrderState:       "Filled",
		OrderType:        "MarketOrder",
		Account:          200,
		ClientOrderID:    888,
		CounterPartyID:   300,
		OmsID:            2,
		ReceiveTime:      9876543210,
	}

	event := AdaptOrder(o)

	assert.Equal(t, "54321", event.FillID)
	assert.Equal(t, "sell", event.Side)
	assert.Equal(t, "0.000000", event.QuantityLeaves) // Fully filled
	assert.Equal(t, "trade", event.ExecutionType)
}

func TestNewFillArrivedEvent(t *testing.T) {
	event := NewFillArrivedEvent()
	assert.Equal(t, "trade", event.ExecutionType)
}
