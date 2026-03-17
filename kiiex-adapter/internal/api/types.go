package api

import "fmt"

// OrderExecuteRequest is the payload for POST /api/v1/orders.
type OrderExecuteRequest struct {
	ID             int64  `json:"id,omitempty"`
	ClientID       string `json:"clientId"`
	InstrumentPair string `json:"instrumentPair"`
	Quantity       string `json:"quantity"`
	Price          string `json:"price,omitempty"`
	Side           string `json:"side"`
	Type           string `json:"type"`
	OrderID        string `json:"orderId,omitempty"`
	ClientOrderID  string `json:"clientOrderId,omitempty"`
}

// Validate checks that OrderExecuteRequest has all required fields.
func (r OrderExecuteRequest) Validate() error {
	if r.ClientID == "" {
		return fmt.Errorf("clientId is required")
	}
	if r.InstrumentPair == "" {
		return fmt.Errorf("instrumentPair is required")
	}
	if r.Quantity == "" {
		return fmt.Errorf("quantity is required")
	}
	if r.Side == "" {
		return fmt.Errorf("side is required")
	}
	return nil
}

// OrderExecuteResponse is the response for POST /api/v1/orders (202 Accepted).
type OrderExecuteResponse struct {
	OrderID string `json:"orderId"`
	Status  string `json:"status"` // always "accepted"
}
