package b2c2

import "context"

//
// ────────────────────────────────────────────────────────────
//   Per-Client Configuration (resolved from AWS Secrets Manager)
// ────────────────────────────────────────────────────────────
//

// B2C2ClientConfig holds per-client B2C2 API configuration.
// Secret path: {env}/{clientId}/b2c2
// Secret JSON format: {"api_token":"...","base_url":"https://api.b2c2.net"}
type B2C2ClientConfig struct {
	APIToken string // Static API token for Authorization header
	BaseURL  string // B2C2 API base URL (prod or UAT)
}

// ConfigResolver resolves per-client B2C2 configuration.
type ConfigResolver interface {
	Resolve(ctx context.Context, clientID string) (*B2C2ClientConfig, error)
	DiscoverClients(ctx context.Context) ([]string, error)
}

//
// ────────────────────────────────────────────────────────────
//   B2C2 API: RFQ Request / Response
// ────────────────────────────────────────────────────────────
//

// RFQRequest is the payload for POST /request_for_quote/.
type RFQRequest struct {
	Instrument   string `json:"instrument"`              // e.g. "BTCUSD.SPOT"
	Side         string `json:"side"`                    // "buy" or "sell"
	Quantity     string `json:"quantity"`                // string decimal
	ClientRFQID  string `json:"client_rfq_id"`           // idempotency key
	ExecutingUnit string `json:"executing_unit,omitempty"` // optional
}

// RFQResponse is the response from POST /request_for_quote/.
type RFQResponse struct {
	RFQID       string `json:"rfq_id"`
	ClientRFQID string `json:"client_rfq_id"`
	Instrument  string `json:"instrument"`
	Side        string `json:"side"`
	Quantity    string `json:"quantity"`
	Price       string `json:"price"`
	ValidUntil  string `json:"valid_until"`
	IsIndicative bool  `json:"is_indicative"`
	Created     string `json:"created"`
}

//
// ────────────────────────────────────────────────────────────
//   B2C2 API: Order Request / Response
// ────────────────────────────────────────────────────────────
//

// OrderRequest is the payload for POST /v2/order/.
type OrderRequest struct {
	Instrument    string `json:"instrument"`               // e.g. "BTCUSD.SPOT"
	Side          string `json:"side"`                     // "buy" or "sell"
	Quantity      string `json:"quantity"`                 // string decimal
	Price         string `json:"price"`                    // limit price
	OrderType     string `json:"order_type"`               // always "FOK"
	RFQID         string `json:"rfq_id,omitempty"`         // associated RFQ
	ClientOrderID string `json:"client_order_id"`          // idempotency key
	ExecutingUnit string `json:"executing_unit,omitempty"` // optional
}

// OrderResponse is the response from POST /v2/order/.
type OrderResponse struct {
	OrderID       string  `json:"order_id"`
	ClientOrderID string  `json:"client_order_id"`
	RFQID         string  `json:"rfq_id"`
	Instrument    string  `json:"instrument"`
	Side          string  `json:"side"`
	Quantity      string  `json:"quantity"`
	ExecutedPrice *string `json:"executed_price"` // null if no liquidity / cancelled
	OrderType     string  `json:"order_type"`
	Status        string  `json:"status"` // e.g. "FILLED", "REJECTED"
	Created       string  `json:"created"`
	ExecutingUnit string  `json:"executing_unit,omitempty"`
}

//
// ────────────────────────────────────────────────────────────
//   B2C2 API: Balance Response
// ────────────────────────────────────────────────────────────
//

// BalanceResponse maps currency code → string amount.
type BalanceResponse map[string]string

//
// ────────────────────────────────────────────────────────────
//   B2C2 API: Instruments Response
// ────────────────────────────────────────────────────────────
//

// Instrument represents a single B2C2 trading instrument.
type Instrument struct {
	Name               string `json:"name"`
	UnderlyingCurrency string `json:"underlying_currency"`
	QuotedCurrency     string `json:"quoted_currency"`
	IsActive           bool   `json:"is_active"`
	MaxQuantity        string `json:"max_quantity"`
	MinQuantity        string `json:"min_quantity"`
}

//
// ────────────────────────────────────────────────────────────
//   B2C2 API: Error Response
// ────────────────────────────────────────────────────────────
//

// ErrorResponse is the structured error from B2C2 API.
type ErrorResponse struct {
	Errors map[string]any `json:"errors"`
}

//
// ────────────────────────────────────────────────────────────
//   Canonical Message Types (RabbitMQ inbound / outbound)
// ────────────────────────────────────────────────────────────
//

// SubmitRequestForQuoteCommand is consumed from outbound.rfqs.created.b2c2.
type SubmitRequestForQuoteCommand struct {
	ID             string `json:"id"`
	InstrumentPair string `json:"instrumentPair"` // canonical e.g. "usd:btc"
	Quantity       string `json:"quantity"`
	Side           string `json:"side"`
	IssuerId       string `json:"issuerId,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

// effectiveClientID returns the client ID from either ClientID or IssuerId.
func (c *SubmitRequestForQuoteCommand) EffectiveClientID() string {
	if c.ClientID != "" {
		return c.ClientID
	}
	return c.IssuerId
}

// SubmitOrderCommand is consumed from outbound.orders.created.b2c2.
type SubmitOrderCommand struct {
	OrderID           string `json:"orderId"`
	InstrumentPair    string `json:"instrumentPair"` // canonical e.g. "usd:btc"
	Quantity          string `json:"quantity"`
	Price             string `json:"price"`
	Side              string `json:"side"`
	ClientOrderID     string `json:"clientOrderId"`
	RequestForQuoteID string `json:"requestForQuoteId"`
	ClientID          string `json:"clientId"`
	Provider          string `json:"provider,omitempty"`
}

// CancelOrderCommand is consumed from outbound.orders.canceled.b2c2.
type CancelOrderCommand struct {
	OrderID           string `json:"orderId"`
	ClientOrderID     string `json:"clientOrderId,omitempty"`
	RequestForQuoteID string `json:"requestForQuoteId,omitempty"`
	ClientID          string `json:"clientId,omitempty"`
	Provider          string `json:"provider,omitempty"`
}

// QuoteArrivedEvent is published to exchange.outbound.rfq / inbound.quotes.creates.
type QuoteArrivedEvent struct {
	RequestForQuoteID string `json:"requestForQuoteId"`
	ExternalQuoteID   string `json:"externalQuoteId"`
	Price             string `json:"price"`
	Side              string `json:"side"`
	InstrumentPair    string `json:"instrumentPair"`
	Quantity          string `json:"quantity"`
	Expiry            string `json:"expiry"`
	Provider          string `json:"provider"`
}

// FillArrivedEvent is published to exchange.outbound.orders / inbound.fills.creates.
type FillArrivedEvent struct {
	OrderID           string `json:"orderId"`
	FillID            string `json:"fillId"`
	ExternalOrderID   string `json:"externalOrderId"`
	InstrumentPair    string `json:"instrumentPair"`
	QuantityFilled    string `json:"quantityFilled"`
	QuantityCumulative string `json:"quantityCumulative"`
	Price             string `json:"price"`
	Side              string `json:"side"`
	Status            string `json:"status"`
	ClientOrderID     string `json:"clientOrderId"`
	RequestForQuoteID string `json:"requestForQuoteId"`
	Provider          string `json:"provider"`
	SourceType        string `json:"sourceType"`
	ExecutionType     string `json:"executionType"`
}

// OrderCanceledEvent is published to exchange.outbound.orders / returned.orders.canceled.
type OrderCanceledEvent struct {
	OrderID           string `json:"orderId"`
	ClientOrderID     string `json:"clientOrderId"`
	RequestForQuoteID string `json:"requestForQuoteId"`
	Provider          string `json:"provider"`
	Reason            string `json:"reason"`
	Message           string `json:"message"`
	ClientID          string `json:"clientId"`
	InstrumentPair    string `json:"instrumentPair"`
	Side              string `json:"side"`
	Quantity          string `json:"quantity"`
	QuotedPrice       string `json:"quotedPrice"`
}

//
// ────────────────────────────────────────────────────────────
//   Publisher Interface
// ────────────────────────────────────────────────────────────
//

// Publisher publishes canonical events to RabbitMQ.
type Publisher interface {
	PublishQuoteEvent(ctx context.Context, event *QuoteArrivedEvent) error
	PublishFillEvent(ctx context.Context, event *FillArrivedEvent) error
	PublishCancelEvent(ctx context.Context, event *OrderCanceledEvent) error
}
