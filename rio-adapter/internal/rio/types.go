package rio

import "context"

//
// ────────────────────────────────────────────────
//   Client Configuration (per-client, from AWS SM)
// ────────────────────────────────────────────────
//

// RioClientConfig holds per-client Rio API configuration resolved from AWS Secrets Manager.
// Secret format: {"api_key": "...", "base_url": "https://...", "country": "US"}
type RioClientConfig struct {
	BaseURL string // Rio API base URL (e.g. "https://app.sandbox.rio.trade")
	APIKey  string // Rio API key for x-api-key header
	Country string // Country code for Rio operations (US, MX, PE)
}

// rateLimitKey returns a key that isolates rate limits per client,
// derived from the base URL so each Rio endpoint gets its own bucket.
func (c *RioClientConfig) rateLimitKey() string {
	return "rio_api:" + c.BaseURL
}

// ConfigResolver resolves per-client Rio configuration.
type ConfigResolver interface {
	// Resolve fetches the RioClientConfig for a given client ID, using cache when available.
	Resolve(ctx context.Context, clientID string) (*RioClientConfig, error)

	// DiscoverClients lists all client IDs that have Rio secrets configured.
	DiscoverClients(ctx context.Context) ([]string, error)
}

//
// ────────────────────────────────────────────────
//   CANONICAL → RIO  : Quote Request
// ────────────────────────────────────────────────
//

// RioQuoteRequest represents the payload for creating a quote on Rio.
// POST /api/quotes
type RioQuoteRequest struct {
	Crypto               string  `json:"crypto"`                         // USDC, USDT_POLYGON, BTC, etc.
	Fiat                 string  `json:"fiat"`                           // USD, MXN, PEN
	Side                 string  `json:"side"`                           // buy, sell
	Country              string  `json:"country"`                        // MX, PE, US
	AmountFiat           float64 `json:"amountFiat,omitempty"`           // Amount in fiat
	AmountCrypto         float64 `json:"amountCrypto,omitempty"`         // Amount in crypto
	UserID               string  `json:"userId,omitempty"`               // Optional user ID
	USBankTransferMethod string  `json:"USBankTransferMethod,omitempty"` // ach_push, wire (US only)
}

//
// ────────────────────────────────────────────────
//   RIO → CANONICAL  : Quote Response
// ────────────────────────────────────────────────
//

// RioQuoteResponse represents Rio's quote response.
type RioQuoteResponse struct {
	ID           string  `json:"id"`
	UserID       string  `json:"userId"`
	Crypto       string  `json:"crypto"`
	Fiat         string  `json:"fiat"`
	Side         string  `json:"side"`
	Country      string  `json:"country"`
	AmountFiat   float64 `json:"amountFiat"`
	AmountCrypto float64 `json:"amountCrypto"`
	NetPrice     float64 `json:"netPrice"`
	MarketPrice  float64 `json:"marketPrice"`
	ExpiresAt    string  `json:"expiresAt"`
	CreatedAt    string  `json:"createdAt"`
	Fees         RioFees `json:"fees"`
	Type         string  `json:"type,omitempty"` // onramp, offramp
}

// RioFees represents the fee breakdown in a Rio quote/order.
type RioFees struct {
	ProcessingFeeFiat   float64 `json:"processingFeeFiat"`
	TransferFeeFiat     float64 `json:"transferFeeFiat"`
	PlatformFeeFiat     float64 `json:"platformFeeFiat"`
	ProcessingFeeCrypto float64 `json:"processingFeeCrypto,omitempty"`
	TransferFeeCrypto   float64 `json:"transferFeeCrypto,omitempty"`
	PlatformFeeCrypto   float64 `json:"platformFeeCrypto,omitempty"`
}

//
// ────────────────────────────────────────────────
//   CANONICAL → RIO  : Order Request
// ────────────────────────────────────────────────
//

// RioOrderRequest represents the payload for creating an order on Rio.
// POST /api/orders
type RioOrderRequest struct {
	QuoteID             string `json:"quoteId"`
	PayoutBankAccountID string `json:"payoutBankAccountId,omitempty"` // Required for sell (offramp)
	UserAddressID       string `json:"userAddressId,omitempty"`       // Required for buy (onramp)
	ClientReferenceID   string `json:"clientReferenceId,omitempty"`   // Optional external reference
	Notes               string `json:"notes,omitempty"`               // Optional notes
}

//
// ────────────────────────────────────────────────
//   RIO → CANONICAL  : Order Response
// ────────────────────────────────────────────────
//

// RioOrderResponse represents Rio's order response.
// Returned by POST /api/orders and GET /api/orders/{id}
type RioOrderResponse struct {
	ID                  string  `json:"id"`
	QuoteID             string  `json:"quoteId"`
	UserID              string  `json:"userId"`
	Status              string  `json:"status"` // One of 42 possible statuses
	Side                string  `json:"side"`   // buy, sell
	Type                string  `json:"type"`   // onramp, offramp
	Crypto              string  `json:"crypto"`
	Fiat                string  `json:"fiat"`
	Country             string  `json:"country"`
	AmountFiat          float64 `json:"amountFiat"`
	AmountCrypto        float64 `json:"amountCrypto"`
	NetPrice            float64 `json:"netPrice"`
	MarketPrice         float64 `json:"marketPrice"`
	Fees                RioFees `json:"fees"`
	ClientReferenceID   string  `json:"clientReferenceId,omitempty"`
	PayoutBankAccountID string  `json:"payoutBankAccountId,omitempty"`
	UserAddressID       string  `json:"userAddressId,omitempty"`
	TxHash              string  `json:"txHash,omitempty"`      // Blockchain transaction hash
	SettledAt           string  `json:"settledAt,omitempty"`   // When fully settled
	CompletedAt         string  `json:"completedAt,omitempty"` // When completed
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

//
// ────────────────────────────────────────────────
//   RIO → CANONICAL  : Webhook Event
// ────────────────────────────────────────────────
//

// RioOrderWebhookEvent represents the payload sent by Rio webhooks for order status changes.
type RioOrderWebhookEvent struct {
	Event string           `json:"event"` // order.status_changed
	Data  RioOrderResponse `json:"data"`
}

//
// ────────────────────────────────────────────────
//   RIO : Webhook Registration
// ────────────────────────────────────────────────
//

// RioWebhookRegistration represents the payload for registering a webhook.
// POST /api/webhooks/orders
type RioWebhookRegistration struct {
	URL            string `json:"url"`
	RetryOnFailure bool   `json:"retryOnFailure"`
}

// RioWebhookRegistrationResponse represents the response from webhook registration.
type RioWebhookRegistrationResponse struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Type      string `json:"type"` // orders
	CreatedAt string `json:"createdAt"`
}

//
// ────────────────────────────────────────────────
//   RIO : Error Response
// ────────────────────────────────────────────────
//

// RioErrorResponse represents an error response from Rio API.
type RioErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
