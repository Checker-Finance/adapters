package api

import "time"

// BalanceResponse represents a client's balance snapshot for API responses.
type BalanceResponse struct {
	Venue      string    `json:"venue"`
	Instrument string    `json:"instrument"`
	Available  float64   `json:"available"`
	Held       float64   `json:"held"`
	CanBuy     bool      `json:"can_buy"`
	CanSell    bool      `json:"can_sell"`
	LastUpdate time.Time `json:"last_update"`
}

// RFQResponse represents Braza's quotation preview response.
type RFQResponse struct {
	QuoteID         string  `json:"quoteId"`
	ProviderQuoteId string  `json:"providerQuoteId"`
	Price           float64 `json:"price"`
	ExpireAt        int64   `json:"expireAt"`
	ErrorMsg        string  `json:"errorMessage"`
}

// RFQExecutionResponse represents an executed RFQ result.
type RFQExecutionResponse struct {
	OrderID         string  `json:"orderId"`
	ProviderOrderID string  `json:"providerOrderId"`
	Status          string  `json:"status"`
	Price           float64 `json:"price"`
	RemainingAmount float64 `json:"remainingAmount"`
	FilledAmount    float64 `json:"filledAmount"`
	ExecutedAt      int64   `json:"executedAt"`
	ErrorMsg        string  `json:"errorMessage"`
}

// TradeResponse represents a trade execution result from Braza.
type TradeResponse struct {
	OrderID         string  `json:"order_id"`
	Instrument      string  `json:"instrument"`
	Side            string  `json:"side"`
	Quantity        float64 `json:"quantity"`
	ExecutionPrice  float64 `json:"execution_price"`
	Status          string  `json:"status"`
	TransactionHash string  `json:"transaction_hash,omitempty"`
}

type BrazaProduct struct {
	ID               int    `json:"id"`
	Par              string `json:"par"`                // e.g. "USDCBRL"
	Nome             string `json:"nome"`               // e.g. "CRYPTO D0/D0 TEMP"
	IDProductCompany int    `json:"id_product_company"` // company-level id
	FlagPedirData    int    `json:"flag_pedir_data"`    // 0/1
	FlagBloqueado    int    `json:"flag_bloqueado"`     // 0/1
}

type BrazaProductListResponse struct {
	Count   int            `json:"count"`
	Results []BrazaProduct `json:"results"`
}
