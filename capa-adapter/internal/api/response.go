package api

// RFQResponse represents the quotation preview response.
type RFQResponse struct {
	QuoteID         string  `json:"quoteId"`
	ProviderQuoteId string  `json:"providerQuoteId"`
	Price           float64 `json:"price"`
	ExpireAt        int64   `json:"expireAt"`
	ErrorMsg        string  `json:"errorMessage,omitempty"`
}

// RFQExecutionResponse represents an executed quote result.
type RFQExecutionResponse struct {
	OrderID         string  `json:"orderId"`
	ProviderOrderID string  `json:"providerOrderId"`
	Status          string  `json:"status"`
	Price           float64 `json:"price"`
	ExecutedAt      int64   `json:"executedAt"`
	ErrorMsg        string  `json:"errorMessage,omitempty"`
}
