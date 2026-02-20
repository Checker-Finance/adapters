package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope is the canonical event/command envelope.
// All messages published to or consumed from NATS must follow this format.
type Envelope struct {
	ID            uuid.UUID       `json:"id"`
	CorrelationID uuid.UUID       `json:"correlation_id"`
	TenantID      string          `json:"tenant_id"`
	ClientID      string          `json:"client_id"`
	Topic         string          `json:"topic"`
	EventType     string          `json:"event_type"`
	Version       string          `json:"version"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
	Context       Context         `json:"context,omitempty"`
}

type Context struct {
	Instrument string  `json:"instrument,omitempty"`
	Side       string  `json:"side,omitempty"`
	Quantity   float64 `json:"quantity,omitempty"`
	DeskID     string  `json:"desk_id,omitempty"`
	Original   string  `json:"original,omitempty"`
}

type Quote struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TakerID    string    `json:"taker_id"`
	MakerID    string    `json:"maker_id"`
	DeskID     string    `json:"desk_id,omitempty"`
	Instrument string    `json:"instrument"`
	Side       string    `json:"side"`
	Price      float64   `json:"price"` // normalized rate (e.g. quote price)
	Bid        float64   `json:"bid"`
	Ask        float64   `json:"ask"`
	Quantity   float64   `json:"quantity"` // notional or amount in base currency
	Currency   string    `json:"currency"` // e.g. "USD" or "BTC"
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"` // CREATED | EXPIRED | REPLACED | ACCEPTED | REJECTED
	Venue      string    `json:"venue"`
	Timestamp  time.Time `json:"timestamp"`
}

type RFQRequest struct {
	// Identifiers
	TenantID string `json:"tenant_id"`
	ClientID string `json:"client_id"`
	DeskID   string `json:"desk_id,omitempty"`

	// RFQ details
	CurrencyPair   string  `json:"currency_pair"` // e.g. "USDCBRL" or "BTCUSD"
	Side           string  `json:"side"`          // BUY or SELL
	Amount         float64 `json:"amount"`        // requested notional or quantity
	ProductID      int     `json:"product_id,omitempty"`
	CurrencyAmount string  `json:"currency_amount,omitempty"` // optional: amount in source currency

	// Context
	Source      string    `json:"source,omitempty"`       // e.g. "SLACK", "WHATSAPP"
	RequestTime time.Time `json:"request_time,omitempty"` // UTC time the RFQ was issued

	// Meta
	CorrelationID string `json:"correlation_id,omitempty"` // internal tracking ID
	Comment       string `json:"comment,omitempty"`
}

type QuoteRequest struct {
	RequestID   uuid.UUID `json:"request_id"`
	Instrument  string    `json:"instrument"`
	Side        string    `json:"side"` // BUY or SELL
	Quantity    float64   `json:"quantity"`
	Counterpart string    `json:"counterpart,omitempty"`
	Settlement  string    `json:"settlement,omitempty"` // e.g. SPOT, TOM
	Timestamp   time.Time `json:"timestamp"`
}

type QuoteResponse struct {
	ID             string    `json:"id"`
	QuoteRequestID string    `json:"quote_request_id,omitempty"`
	Instrument     string    `json:"instrument"`
	Venue          string    `json:"venue"`
	Side           string    `json:"side"`
	BidPrice       float64   `json:"bid_price,omitempty"`
	AskPrice       float64   `json:"ask_price,omitempty"`
	Quantity       float64   `json:"quantity"`
	TTL            int       `json:"ttl_seconds"`
	ReceivedAt     time.Time `json:"received_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	RawPayload     string    `json:"raw_payload,omitempty"`
}

type TradeCommand struct {
	TenantID   string    `json:"tenant_id"`
	ClientID   string    `json:"client_id"`
	CommandID  string    `json:"command_id"`
	QuoteID    string    `json:"quote_id"`
	Instrument string    `json:"instrument"`
	Side       string    `json:"side"`
	Quantity   float64   `json:"quantity"`
	Price      float64   `json:"price"`
	Venue      string    `json:"venue"`
	Settlement string    `json:"settlement,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	ExternalID string    `json:"external_id,omitempty"`
}

type TradeConfirmation struct {
	ID              string    `json:"id"`
	TradeID         string    `json:"trade_id"`
	TenantID        string    `json:"tenant_id"`
	ClientID        string    `json:"client_id"`
	OrderID         string    `json:"order_id"`
	Instrument      string    `json:"instrument"`
	Side            string    `json:"side"`
	Quantity        float64   `json:"quantity"`
	Price           float64   `json:"price"`
	Venue           string    `json:"venue"`
	Status          string    `json:"status"` // e.g. FILLED, REJECTED, PARTIAL
	ExecutedAt      time.Time `json:"executed_at"`
	SettlementAt    time.Time `json:"settlement_at"`
	RawPayload      string    `json:"raw_payload,omitempty"`
	ExecutionTime   time.Time `json:"execution_time"`
	TradeReference  string    `json:"trade_reference"`
	Message         string    `json:"message,omitempty"`
	RFQID           string    `json:"rfq_id"`
	Notes           string    `json:"notes,omitempty"`
	ProviderOrderID string    `json:"provider_order_id,omitempty"`
	ProviderRFQID   string    `json:"provider_rfq_id,omitempty"`
}

type Balance struct {
	ID          int64  `json:"id"`
	ClientID    string `json:"client_id"`
	Venue       string `json:"venue"`
	AccountType string `json:"account_type,omitempty"` // e.g. CASH, MARGIN
	Instrument  string `json:"instrument"`             // currency or pair, e.g. USDBRL

	// Amount fields
	Available float64 `json:"available"` // immediately tradable or withdrawable
	Held      float64 `json:"held"`      // reserved for open orders / settlements
	Total     float64 `json:"total"`     // Available + Held (should match LP total)

	// Metadata
	Currency      string    `json:"currency,omitempty"` // redundant, but useful for display
	LastUpdated   time.Time `json:"last_updated"`
	AsOf          time.Time `json:"as_of,omitempty"`
	Source        string    `json:"source,omitempty"` // e.g. BRAZA API
	SlipDaysAvail int       `json:"slip_days_avail,omitempty"`
	SlipDaysMax   int       `json:"slip_days_max,omitempty"`
	CanBuy        bool      `json:"can_buy"`
	CanSell       bool      `json:"can_sell"`
	Version       int       `json:"version,omitempty"`
}

type Order struct {
	ID         uuid.UUID `json:"id"`
	Instrument string    `json:"instrument"`
	Side       string    `json:"side"`
	Quantity   float64   `json:"quantity"`
	Price      float64   `json:"price"`
}

func NewUUID() uuid.UUID {
	return uuid.New()
}
