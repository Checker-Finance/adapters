package zodia

import "context"

//
// ────────────────────────────────────────────────
//   Client Configuration (per-client, from AWS SM)
// ────────────────────────────────────────────────
//

// ZodiaClientConfig holds per-client Zodia API configuration resolved from AWS Secrets Manager.
// Secret format: {"api_key": "...", "api_secret": "...", "base_url": "https://trade-uk.zodiamarkets.com"}
type ZodiaClientConfig struct {
	APIKey    string // Zodia REST-Key (HMAC auth)
	APISecret string // Zodia API secret (used for HMAC signing)
	BaseURL   string // Zodia API base URL (region-specific)
}

// ConfigResolver resolves per-client Zodia configuration.
type ConfigResolver interface {
	// Resolve fetches the ZodiaClientConfig for a given client ID, using cache when available.
	Resolve(ctx context.Context, clientID string) (*ZodiaClientConfig, error)

	// DiscoverClients lists all client IDs that have Zodia secrets configured.
	DiscoverClients(ctx context.Context) ([]string, error)
}

//
// ────────────────────────────────────────────────
//   Zodia REST API: Account / Balance Types
// ────────────────────────────────────────────────
//

// ZodiaAccountRequest is the signed payload for POST /api/3/account.
type ZodiaAccountRequest struct {
	Tonce int64 `json:"tonce"`
}

// ZodiaAccountResponse is the response from POST /api/3/account.
// The result is a map of currency code → balance detail.
// ⚠️ Verify exact structure against sandbox.
type ZodiaAccountResponse struct {
	Result map[string]ZodiaAccountBalance `json:"result"`
}

// ZodiaAccountBalance holds the available and locked balance for a currency.
type ZodiaAccountBalance struct {
	Available string `json:"available"` // decimal string
	Orders    string `json:"orders"`    // amount locked in open orders
}

//
// ────────────────────────────────────────────────
//   Zodia REST API: Instruments Types
// ────────────────────────────────────────────────
//

// ZodiaInstrumentsResponse is the response from GET /zm/rest/available-instruments.
// ⚠️ Auth requirements unknown — may or may not require HMAC signing.
type ZodiaInstrumentsResponse struct {
	Instruments []ZodiaInstrument `json:"instruments"`
}

// ZodiaInstrument represents a tradeable currency pair on Zodia.
type ZodiaInstrument struct {
	Symbol  string  `json:"symbol"`  // e.g. "USD.MXN"
	Base    string  `json:"base"`    // e.g. "USD"
	Quote   string  `json:"quote"`   // e.g. "MXN"
	Status  string  `json:"status"`  // "active", "inactive"
	MinSize float64 `json:"minSize"` // minimum trade size in base currency
}

//
// ────────────────────────────────────────────────
//   Zodia REST API: Transaction Types
// ────────────────────────────────────────────────
//

// ZodiaTransactionFilter is the signed payload for POST /api/3/transaction/list.
type ZodiaTransactionFilter struct {
	Tonce     int64  `json:"tonce"`
	Type      string `json:"type,omitempty"`      // e.g. "OTCTRADE", "RFSTRADE"
	State     string `json:"state,omitempty"`     // e.g. "PENDING", "PROCESSED"
	TradeID   string `json:"tradeId,omitempty"`   // filter by specific trade
	StartTime string `json:"startTime,omitempty"` // RFC3339
	EndTime   string `json:"endTime,omitempty"`   // RFC3339
}

// ZodiaTransactionListResponse is the response from POST /api/3/transaction/list.
type ZodiaTransactionListResponse struct {
	Result []ZodiaTransaction `json:"result"`
}

// ZodiaTransaction represents a single Zodia trade transaction.
type ZodiaTransaction struct {
	UUID          string  `json:"uuid"`
	Type          string  `json:"type"`          // "OTCTRADE", "RFSTRADE"
	State         string  `json:"state"`         // "PENDING", "PROCESSED"
	TradeID       string  `json:"tradeId"`       // Zodia trade identifier
	Instrument    string  `json:"instrument"`    // e.g. "USD.MXN"
	Side          string  `json:"side"`          // "BUY", "SELL"
	Quantity      float64 `json:"quantity"`      // amount in base currency
	Price         float64 `json:"price"`         // execution price
	DealtCurrency string  `json:"dealtCurrency"` // currency of quantity
	DealtAmount   float64 `json:"dealtAmount"`   // dealt amount
	ContraAmount  float64 `json:"contraAmount"`  // contra amount
	CreatedAt     string  `json:"createdAt"`     // RFC3339
	UpdatedAt     string  `json:"updatedAt"`     // RFC3339
}

//
// ────────────────────────────────────────────────
//   Zodia REST API: WS Auth Types
// ────────────────────────────────────────────────
//

// ZodiaWSAuthRequest is the signed payload for POST /ws/auth.
type ZodiaWSAuthRequest struct {
	Tonce int64 `json:"tonce"`
}

// ZodiaWSAuthResponse is the response from POST /ws/auth.
type ZodiaWSAuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt,omitempty"` // RFC3339; may be absent
}

//
// ────────────────────────────────────────────────
//   Zodia REST API: Error Response
// ────────────────────────────────────────────────
//

// ZodiaErrorResponse represents an error response from the Zodia REST API.
type ZodiaErrorResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

//
// ────────────────────────────────────────────────
//   Zodia WebSocket: Message Types
// ────────────────────────────────────────────────
//
// ⚠️ WS action strings and field names are inferred.
// Verify against Zodia sandbox or full WS API docs before production use.
//

// WSMessage is a generic WebSocket message used for action dispatch.
type WSMessage struct {
	Action    string `json:"action"`
	ClientRef string `json:"clientRef,omitempty"`
}

// WSAuthMessage is sent to the WS server immediately after connecting.
type WSAuthMessage struct {
	Action string `json:"action"` // "auth"
	Token  string `json:"token"`
}

// WSAuthResult is received after sending WSAuthMessage.
type WSAuthResult struct {
	Action  string `json:"action"`            // "auth_success" or "error"
	Message string `json:"message,omitempty"` // error detail if action == "error"
}

// WSSubscribePriceMessage requests a streaming price for an instrument.
type WSSubscribePriceMessage struct {
	Action     string  `json:"action"`     // "subscribe_price"
	ClientRef  string  `json:"clientRef"`  // client-generated UUID for correlation
	Instrument string  `json:"instrument"` // e.g. "USD.MXN"
	Side       string  `json:"side"`       // "BUY" or "SELL"
	Quantity   float64 `json:"quantity"`   // amount in base currency
}

// WSPricePayload is received as a price_update message from the server.
type WSPricePayload struct {
	Action     string  `json:"action"`    // "price_update"
	ClientRef  string  `json:"clientRef"` // matches the subscribe request
	Instrument string  `json:"instrument"`
	Side       string  `json:"side"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price"`
	Bid        float64 `json:"bid"`
	Ask        float64 `json:"ask"`
	QuoteID    string  `json:"quoteId"`   // Zodia-assigned quote identifier
	ExpiresAt  int64   `json:"expiresAt"` // Unix timestamp
}

// WSExecuteOrderMessage requests execution of a quoted price.
type WSExecuteOrderMessage struct {
	Action    string `json:"action"`    // "execute_order"
	ClientRef string `json:"clientRef"` // client-generated UUID for correlation
	QuoteID   string `json:"quoteId"`   // from WSPricePayload.QuoteID
}

// WSOrderConfirmPayload is received as an order_confirmation message from the server.
type WSOrderConfirmPayload struct {
	Action     string  `json:"action"`    // "order_confirmation"
	ClientRef  string  `json:"clientRef"` // matches the execute request
	TradeID    string  `json:"tradeId"`   // Zodia trade identifier
	Instrument string  `json:"instrument"`
	Side       string  `json:"side"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price"`
	Status     string  `json:"status"`     // e.g. "PENDING", "PROCESSED"
	ExecutedAt int64   `json:"executedAt"` // Unix timestamp
}

// WSErrorPayload is received when the server returns an error.
type WSErrorPayload struct {
	Action    string `json:"action"`              // "error"
	ClientRef string `json:"clientRef,omitempty"` // set if error is correlated to a request
	Code      string `json:"code,omitempty"`
	Message   string `json:"message"`
}

//
// ────────────────────────────────────────────────
//   Zodia Webhook Types
// ────────────────────────────────────────────────
//

// ZodiaWebhookEvent is the payload of incoming webhook notifications.
// Zodia sends these for transaction state changes.
// ⚠️ Exact field names need verification against webhook docs.
type ZodiaWebhookEvent struct {
	UUID         string  `json:"uuid"`         // unique event identifier (use for dedup)
	Type         string  `json:"type"`         // "OTCTRADE", "RFSTRADE"
	State        string  `json:"state"`        // "PENDING", "PROCESSED"
	TradeID      string  `json:"tradeId"`      // Zodia trade identifier
	Instrument   string  `json:"instrument"`   // e.g. "USD.MXN"
	Side         string  `json:"side"`         // "BUY", "SELL"
	Quantity     float64 `json:"quantity"`     // amount in base currency
	Price        float64 `json:"price"`        // execution price
	DealtAmount  float64 `json:"dealtAmount"`  // dealt amount
	ContraAmount float64 `json:"contraAmount"` // contra amount
	CreatedAt    string  `json:"createdAt"`    // RFC3339
	UpdatedAt    string  `json:"updatedAt"`    // RFC3339
}
