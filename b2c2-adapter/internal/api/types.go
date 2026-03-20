package api

import "fmt"

// RFQCreateRequest is the payload for POST /api/v1/quotes.
type RFQCreateRequest struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
	Pair     string `json:"pair"`      // canonical format, e.g. "usd:btc"
	Side     string `json:"orderSide"` // "buy" or "sell"
	Quantity string `json:"quantity"`
}

// Validate checks that RFQCreateRequest has all required fields.
func (r RFQCreateRequest) Validate() error {
	if r.ClientID == "" {
		return fmt.Errorf("clientId is required")
	}
	if r.Pair == "" {
		return fmt.Errorf("pair is required")
	}
	if r.Side == "" {
		return fmt.Errorf("orderSide is required")
	}
	if r.Quantity == "" {
		return fmt.Errorf("quantity is required")
	}
	return nil
}

// RFQCreateResponse is the response for POST /api/v1/quotes.
type RFQCreateResponse struct {
	QuoteID         string `json:"quoteId"`
	ProviderQuoteID string `json:"providerQuoteId"`
	Price           string `json:"price"`
	ExpireAt        string `json:"expireAt"`
	ErrorMsg        string `json:"errorMessage,omitempty"`
}

// OrderExecuteRequest is the payload for POST /api/v1/orders.
type OrderExecuteRequest struct {
	OrderID       string `json:"orderId"`
	ClientID      string `json:"clientId"`
	Pair          string `json:"pair"` // canonical format, e.g. "usd:btc"
	Side          string `json:"side"` // "buy" or "sell"
	Quantity      string `json:"quantity"`
	Price         string `json:"price"`
	RFQID         string `json:"rfqId"`
	ClientOrderID string `json:"clientOrderId"`
}

// Validate checks that OrderExecuteRequest has all required fields.
func (r OrderExecuteRequest) Validate() error {
	if r.ClientID == "" {
		return fmt.Errorf("clientId is required")
	}
	if r.Pair == "" {
		return fmt.Errorf("pair is required")
	}
	if r.Side == "" {
		return fmt.Errorf("side is required")
	}
	if r.Quantity == "" {
		return fmt.Errorf("quantity is required")
	}
	return nil
}

// OrderExecuteResponse is the response for POST /api/v1/orders.
type OrderExecuteResponse struct {
	OrderID         string `json:"orderId"`
	ProviderOrderID string `json:"providerOrderId"`
	Status          string `json:"status"`
	Price           string `json:"price,omitempty"`
	ExecutedAt      string `json:"executedAt,omitempty"`
	ErrorMsg        string `json:"errorMessage,omitempty"`
}
