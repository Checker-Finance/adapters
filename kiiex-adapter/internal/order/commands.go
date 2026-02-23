package order

import (
	"time"

	"github.com/shopspring/decimal"
)

// SubmitOrderCommand represents an order submission request from RabbitMQ
type SubmitOrderCommand struct {
	ID                 int64           `json:"id,omitempty"`
	OrderID            string          `json:"orderId,omitempty"`
	InstrumentPair     string          `json:"instrumentPair,omitempty"`
	Quantity           decimal.Decimal `json:"quantity,omitempty"`
	Price              decimal.Decimal `json:"price,omitempty"`
	Side               string          `json:"side,omitempty"`
	Status             string          `json:"status,omitempty"`
	Type               string          `json:"type,omitempty"`
	ClientID           string          `json:"clientId,omitempty"`
	Date               *time.Time      `json:"date,omitempty"`
	ClientOrderID      string          `json:"clientOrderId,omitempty"`
	RequestForQuoteID  string          `json:"requestForQuoteId,omitempty"`
	Provider           string          `json:"provider,omitempty"`
	Notes              string          `json:"notes,omitempty"`
	BlockChain         string          `json:"blockChain,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	Source             string          `json:"source,omitempty"`
}

// CancelOrderCommand represents an order cancellation request from RabbitMQ
type CancelOrderCommand struct {
	OrderID string `json:"orderId"`
}

// AmendOrderCommand represents an order amendment request
type AmendOrderCommand struct {
	OrderID  string          `json:"orderId"`
	Quantity decimal.Decimal `json:"quantity,omitempty"`
	Price    decimal.Decimal `json:"price,omitempty"`
}
