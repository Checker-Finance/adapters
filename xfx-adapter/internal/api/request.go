package api

// RFQCreateRequest is the payload for creating a quote request.
type RFQCreateRequest struct {
	ID                 string  `json:"quoteId"`
	ClientID           string  `json:"clientId"`
	CurrencyPair       string  `json:"pair"`
	AmountDenomination string  `json:"amountDenomination"`
	Side               string  `json:"orderSide"`
	Amount             float64 `json:"quantity"`
}

// RFQExecuteRequest is the payload for executing a quote.
type RFQExecuteRequest struct {
	OrderID                   string  `json:"orderId"`
	RFQID                     string  `json:"rfqId"`
	QuoteID                   string  `json:"quoteId"`
	ProviderQuoteID           string  `json:"providerQuoteId"`
	ProviderRequestForQuoteID string  `json:"providerRequestForQuoteId"`
	ClientID                  string  `json:"clientId"`
	Pair                      string  `json:"pair"`
	Quantity                  float64 `json:"quantity"`
	Price                     float64 `json:"price"`
	OrderSide                 string  `json:"orderSide"`
}
