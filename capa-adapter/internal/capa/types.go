package capa

import "context"

//
// ────────────────────────────────────────────────
//   Client Configuration (per-client, from AWS SM)
// ────────────────────────────────────────────────
//

// CapaClientConfig holds per-client Capa API configuration resolved from AWS Secrets Manager.
// Secret path: {env}/{clientID}/capa → {"api_key": "...", "base_url": "...", "user_id": "...", ...}
type CapaClientConfig struct {
	APIKey           string // Capa partner API key
	BaseURL          string // Capa API base URL (e.g. "https://sandbox.capa.fi")
	UserID           string // Capa partner user ID (UUID)
	WebhookSecret    string // Secret for HMAC-SHA256 webhook signature validation
	WalletAddress    string // Destination wallet address (on-ramp only)
	BlockchainSymbol string // Blockchain symbol (on-ramp only, e.g. "POL")
	TokenSymbol      string // Token symbol (on-ramp only, e.g. "USDC")
	ReceiverID       string // Receiver ID for off-ramp payouts (off-ramp only)
}

// ConfigResolver resolves per-client Capa configuration.
type ConfigResolver interface {
	// Resolve fetches the CapaClientConfig for a given client ID, using cache when available.
	Resolve(ctx context.Context, clientID string) (*CapaClientConfig, error)

	// DiscoverClients lists all client IDs that have Capa secrets configured.
	DiscoverClients(ctx context.Context) ([]string, error)
}

//
// ────────────────────────────────────────────────
//   Transaction Types
// ────────────────────────────────────────────────
//

// TransactionType classifies a Capa transaction based on the currency pair.
type TransactionType string

const (
	// CrossRamp is a fiat-to-fiat conversion (e.g. USD → MXN).
	CrossRamp TransactionType = "cross-ramp"
	// OnRamp is a fiat-to-crypto conversion (e.g. USD → USDC).
	OnRamp TransactionType = "on-ramp"
	// OffRamp is a crypto-to-fiat conversion (e.g. USDC → MXN).
	OffRamp TransactionType = "off-ramp"
)

//
// ────────────────────────────────────────────────
//   Capa API: Cross-Ramp Quote
// ────────────────────────────────────────────────
//

// CapaCrossRampQuoteRequest is the payload for POST /api/partner/v2/cross-ramp/quotes.
type CapaCrossRampQuoteRequest struct {
	UserID              string  `json:"userId"`
	SourceCurrency      string  `json:"sourceCurrency"`
	DestinationCurrency string  `json:"destinationCurrency"`
	Amount              float64 `json:"amount"`
	AmountCurrency      string  `json:"amountCurrency"`
}

//
// ────────────────────────────────────────────────
//   Capa API: On/Off-Ramp Quote
// ────────────────────────────────────────────────
//

// CapaQuoteRequest is the payload for POST /api/partner/v2/quotes (on-ramp and off-ramp).
type CapaQuoteRequest struct {
	UserID         string  `json:"userId"`
	FiatCurrency   string  `json:"fiatCurrency"`
	CryptoCurrency string  `json:"cryptoCurrency"`
	Amount         float64 `json:"amount"`
	AmountCurrency string  `json:"amountCurrency"`
}

//
// ────────────────────────────────────────────────
//   Capa API: Unified Quote Response
// ────────────────────────────────────────────────
//

// CapaQuoteResponse is the response from quote creation endpoints.
type CapaQuoteResponse struct {
	ID                  string  `json:"id"`
	UserID              string  `json:"userId"`
	SourceCurrency      string  `json:"sourceCurrency"`
	DestinationCurrency string  `json:"destinationCurrency"`
	SourceAmount        float64 `json:"sourceAmount"`
	DestinationAmount   float64 `json:"destinationAmount"`
	ExchangeRate        float64 `json:"exchangeRate"`
	ExpiresAt           string  `json:"expiresAt"`
	Status              string  `json:"status"`
	TransactionType     string  `json:"transactionType,omitempty"`
}

//
// ────────────────────────────────────────────────
//   Capa API: Execute Requests
// ────────────────────────────────────────────────
//

// CapaCrossRampExecuteRequest is the payload for POST /api/partner/v2/cross-ramp.
type CapaCrossRampExecuteRequest struct {
	UserID  string `json:"userId"`
	QuoteID string `json:"quoteId"`
}

// CapaOnRampExecuteRequest is the payload for POST /api/partner/v2/on-ramp.
type CapaOnRampExecuteRequest struct {
	UserID           string `json:"userId"`
	QuoteID          string `json:"quoteId"`
	WalletAddress    string `json:"walletAddress"`
	BlockchainSymbol string `json:"blockchainSymbol"`
	TokenSymbol      string `json:"tokenSymbol"`
}

// CapaOffRampExecuteRequest is the payload for POST /api/partner/v2/off-ramp.
type CapaOffRampExecuteRequest struct {
	UserID     string `json:"userId"`
	QuoteID    string `json:"quoteId"`
	ReceiverID string `json:"receiverId"`
}

//
// ────────────────────────────────────────────────
//   Capa API: Transaction
// ────────────────────────────────────────────────
//

// CapaTransaction holds transaction details from the Capa API.
type CapaTransaction struct {
	ID                  string  `json:"id"`
	QuoteID             string  `json:"quoteId"`
	UserID              string  `json:"userId"`
	SourceCurrency      string  `json:"sourceCurrency"`
	DestinationCurrency string  `json:"destinationCurrency"`
	SourceAmount        float64 `json:"sourceAmount"`
	DestinationAmount   float64 `json:"destinationAmount"`
	ExchangeRate        float64 `json:"exchangeRate"`
	Status              string  `json:"status"`
	TransactionType     string  `json:"transactionType,omitempty"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

// CapaExecuteResponse is the unified response from execute endpoints.
type CapaExecuteResponse struct {
	ID          string          `json:"id"`
	QuoteID     string          `json:"quoteId"`
	Transaction CapaTransaction `json:"transaction"`
}

// CapaTransactionResponse is the response from GET /api/partner/v2/transactions/{id}.
type CapaTransactionResponse struct {
	Transaction CapaTransaction `json:"transaction"`
}

//
// ────────────────────────────────────────────────
//   Capa API: Webhook Events
// ────────────────────────────────────────────────
//

// CapaWebhookEvent represents an incoming webhook event from Capa.
type CapaWebhookEvent struct {
	Event         string          `json:"event"`
	TransactionID string          `json:"transactionId"`
	QuoteID       string          `json:"quoteId"`
	UserID        string          `json:"userId"`
	Status        string          `json:"status"`
	Transaction   CapaTransaction `json:"transaction"`
}

// ResolvedTxID returns the transaction ID from the top-level field or the nested transaction.
func (e *CapaWebhookEvent) ResolvedTxID() string {
	if e.TransactionID != "" {
		return e.TransactionID
	}
	return e.Transaction.ID
}

// ResolvedQuoteID returns the quote ID from the top-level field or the nested transaction.
func (e *CapaWebhookEvent) ResolvedQuoteID() string {
	if e.QuoteID != "" {
		return e.QuoteID
	}
	return e.Transaction.QuoteID
}

// ResolvedStatus returns the status from the top-level field or the nested transaction.
func (e *CapaWebhookEvent) ResolvedStatus() string {
	if e.Status != "" {
		return e.Status
	}
	return e.Transaction.Status
}

//
// ────────────────────────────────────────────────
//   Capa API: Error Response
// ────────────────────────────────────────────────
//

// CapaErrorResponse represents an error from the Capa API.
type CapaErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
