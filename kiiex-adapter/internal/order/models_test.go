package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSideFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected Side
	}{
		{"Buy", SideBuy},
		{"Sell", SideSell},
		{"Short", SideShort},
		{"Unknown", SideUnknown},
		{"invalid", SideUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SideFromString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSideToInt(t *testing.T) {
	tests := []struct {
		input    Side
		expected int
	}{
		{SideBuy, 0},
		{SideSell, 1},
		{SideShort, 2},
		{SideUnknown, 3},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := tt.input.ToInt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOrderTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected OrderType
	}{
		{"MarketOrder", OrderTypeMarket},
		{"Limit", OrderTypeLimit},
		{"StopMarket", OrderTypeStopMarket},
		{"StopLimit", OrderTypeStopLimit},
		{"TrailingStopMarket", OrderTypeTrailingStopMarket},
		{"TrailingStopLimit", OrderTypeTrailingStopLimit},
		{"BlockTrade", OrderTypeBlockTrade},
		{"Unknown", OrderTypeUnknown},
		{"invalid", OrderTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := OrderTypeFromString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOrderTypeToInt(t *testing.T) {
	tests := []struct {
		input    OrderType
		expected int
	}{
		{OrderTypeMarket, 1},
		{OrderTypeLimit, 2},
		{OrderTypeStopMarket, 3},
		{OrderTypeStopLimit, 4},
		{OrderTypeTrailingStopMarket, 5},
		{OrderTypeTrailingStopLimit, 6},
		{OrderTypeBlockTrade, 7},
		{OrderTypeUnknown, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := tt.input.ToInt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeInForceFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected TimeInForce
	}{
		{"GTC", TimeInForceGTC},
		{"OPG", TimeInForceOPG},
		{"IOC", TimeInForceIOC},
		{"FOK", TimeInForceFOK},
		{"GTX", TimeInForceGTX},
		{"GTD", TimeInForceGTD},
		{"Unknown", TimeInForceUnknown},
		{"invalid", TimeInForceUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := TimeInForceFromString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTimeInForceToInt(t *testing.T) {
	tests := []struct {
		input    TimeInForce
		expected int
	}{
		{TimeInForceGTC, 1},
		{TimeInForceOPG, 2},
		{TimeInForceIOC, 3},
		{TimeInForceFOK, 4},
		{TimeInForceGTX, 5},
		{TimeInForceGTD, 6},
		{TimeInForceUnknown, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := tt.input.ToInt()
			assert.Equal(t, tt.expected, result)
		})
	}
}
