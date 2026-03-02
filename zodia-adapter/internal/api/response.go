package api

// RFQResponse is the HTTP response for a quote creation request.
type RFQResponse struct {
	QuoteID         string  `json:"quoteId"`
	ProviderQuoteId string  `json:"providerQuoteId,omitempty"`
	Price           float64 `json:"price,omitempty"`
	ExpireAt        int64   `json:"expireAt,omitempty"`
	ErrorMsg        string  `json:"error,omitempty"`
}

// RFQExecutionResponse is the HTTP response for a quote execution request.
type RFQExecutionResponse struct {
	OrderID         string  `json:"orderId"`
	ProviderOrderID string  `json:"providerOrderId,omitempty"`
	Status          string  `json:"status,omitempty"`
	Price           float64 `json:"price,omitempty"`
	ExecutedAt      int64   `json:"executedAt,omitempty"`
	ErrorMsg        string  `json:"error,omitempty"`
}
