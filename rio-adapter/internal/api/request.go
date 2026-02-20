package api

// RFQCreateRequest is the payload to initiate an RFQ quotation request.
type RFQCreateRequest struct {
	ID                 string  `json:"quoteId"`
	ClientID           string  `json:"clientId" example:"client-demo-01"`
	CurrencyPair       string  `json:"pair" example:"USD:BRL"`
	AmountDenomination string  `json:"amountDenomination" example:"BRL"`
	Side               string  `json:"orderSide" example:"buy"`
	Amount             float64 `json:"quantity" example:"1000.00"`
}

// RFQExecuteRequest represents the payload for executing a quotation.
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

// TradeCreateRequest defines an order/trade creation payload.
type TradeCreateRequest struct {
	TenantID     string  `json:"tenant_id" example:"checker"`
	ClientID     string  `json:"client_id" example:"client-demo-01"`
	DeskID       string  `json:"desk_id" example:"desk-default"`
	Instrument   string  `json:"instrument" example:"USD:BRL"`
	Side         string  `json:"side" example:"buy"`
	Quantity     float64 `json:"quantity" example:"1000.00"`
	Price        float64 `json:"price,omitempty" example:"5.01"`
	QuoteID      string  `json:"quote_id,omitempty"`
	ExternalID   string  `json:"external_id,omitempty"`
	ExecutionRef string  `json:"execution_ref,omitempty"`
}
