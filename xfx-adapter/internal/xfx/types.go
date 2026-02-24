package xfx

import "context"

//
// ────────────────────────────────────────────────
//   Client Configuration (per-client, from AWS SM)
// ────────────────────────────────────────────────
//

// XFXClientConfig holds per-client XFX API configuration resolved from AWS Secrets Manager.
// Secret format: {"client_id": "...", "client_secret": "...", "base_url": "https://api.xfx.io"}
type XFXClientConfig struct {
	BaseURL      string // XFX API base URL (e.g. "https://dev-api.xfx.io")
	ClientID     string // OAuth2 client_id for Auth0 token request
	ClientSecret string // OAuth2 client_secret for Auth0 token request
}


// ConfigResolver resolves per-client XFX configuration.
type ConfigResolver interface {
	// Resolve fetches the XFXClientConfig for a given client ID, using cache when available.
	Resolve(ctx context.Context, clientID string) (*XFXClientConfig, error)

	// DiscoverClients lists all client IDs that have XFX secrets configured.
	DiscoverClients(ctx context.Context) ([]string, error)
}

//
// ────────────────────────────────────────────────
//   Auth0 Token Types
// ────────────────────────────────────────────────
//

// Auth0TokenRequest is the payload for the Auth0 client credentials flow.
type Auth0TokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Audience     string `json:"audience"`
	GrantType    string `json:"grant_type"`
}

// Auth0TokenResponse is the response from Auth0 token endpoint.
type Auth0TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

//
// ────────────────────────────────────────────────
//   XFX API: Quote Request / Response
// ────────────────────────────────────────────────
//

// XFXQuoteRequest is the payload for POST /v1/customer/quotes.
type XFXQuoteRequest struct {
	Symbol            string  `json:"symbol"`                      // e.g. "USD/MXN"
	Side              string  `json:"side"`                        // "BUY" or "SELL"
	Quantity          float64 `json:"quantity"`                    // Amount in base currency
	CustomerAccountID int     `json:"customerAccountId,omitempty"` // Optional account ID
	Metadata          *XFXQuoteMetadata `json:"metadata,omitempty"`
}

// XFXQuoteMetadata holds optional metadata for a quote.
type XFXQuoteMetadata struct {
	Reference string `json:"reference,omitempty"`
}

// XFXQuoteResponse is the response from POST /v1/customer/quotes or GET /v1/customer/quotes/{id}.
type XFXQuoteResponse struct {
	Success       bool     `json:"success"`
	Quote         XFXQuote `json:"quote"`
	PayoutAccountID string `json:"payoutAccountId,omitempty"`
	Message       string   `json:"message,omitempty"`
}

// XFXQuote contains the quote details.
type XFXQuote struct {
	ID         string  `json:"id"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	Quantity   float64 `json:"quantity"`
	Price      float64 `json:"price"`
	ValidUntil string  `json:"validUntil"`
	Status     string  `json:"status"` // ACTIVE, EXPIRED, EXECUTED, CANCELLED
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

//
// ────────────────────────────────────────────────
//   XFX API: Execute Trade Request / Response
// ────────────────────────────────────────────────
//

// XFXExecuteResponse is the response from POST /v1/customer/quotes/{quoteId}/execute.
type XFXExecuteResponse struct {
	Success     bool           `json:"success"`
	Message     string         `json:"message,omitempty"`
	Transaction XFXTransaction `json:"transaction"`
}

//
// ────────────────────────────────────────────────
//   XFX API: Transaction Types
// ────────────────────────────────────────────────
//

// XFXTransactionResponse is the response from GET /v1/customer/transactions/{id}.
type XFXTransactionResponse struct {
	Success     bool           `json:"success"`
	Transaction XFXTransaction `json:"transaction"`
}

// XFXTransaction holds the transaction details.
type XFXTransaction struct {
	ID        string  `json:"id"`
	QuoteID   string  `json:"quoteId"`
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Quantity  float64 `json:"quantity"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"` // PENDING, SETTLED, FAILED, CANCELLED
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	SettledAt string  `json:"settledAt,omitempty"`
}

// XFXListTransactionsResponse is the response from GET /v1/customer/transactions.
type XFXListTransactionsResponse struct {
	Success      bool             `json:"success"`
	Transactions []XFXTransaction `json:"transactions"`
	Total        int              `json:"total,omitempty"`
	Page         int              `json:"page,omitempty"`
	PageSize     int              `json:"pageSize,omitempty"`
}

//
// ────────────────────────────────────────────────
//   XFX API: Error Response
// ────────────────────────────────────────────────
//

// XFXErrorResponse represents an error response from the XFX API.
type XFXErrorResponse struct {
	Success bool      `json:"success"`
	Error   XFXError  `json:"error,omitempty"`
	Message string    `json:"message,omitempty"`
}

// XFXError holds the structured error detail.
type XFXError struct {
	Code    string   `json:"code,omitempty"`
	Message string   `json:"message,omitempty"`
	Details []string `json:"details,omitempty"`
}
