package braza

//
// ────────────────────────────────────────────────
//   BRAZA → CANONICAL  : Balances
// ────────────────────────────────────────────────
//

type BrazaBalancesResponse []map[string]BrazaBalanceDetail

type BrazaBalanceDetail struct {
	CanBuy                 bool    `json:"can_buy"`
	CanSell                bool    `json:"can_sell"`
	MaxSlipDay             float64 `json:"max_slip_day"`
	MinValueOrder          float64 `json:"min_value_order"`
	MaxValueOrder          float64 `json:"max_value_order"`
	TotalValueDay          float64 `json:"total_value_day"`
	AvailableSlipDay       float64 `json:"available_slip_day"`
	AvailableTotalValueDay float64 `json:"available_total_value_day"`
}

//
// ────────────────────────────────────────────────
//   CANONICAL → BRAZA  : RFQ (Preview Quotation)
// ────────────────────────────────────────────────
//

type BrazaRFQRequest struct {
	CurrencyAmount string  `json:"currency_amount"` // e.g. "USDC"
	Amount         float64 `json:"amount"`          // e.g. 1000
	Currency       string  `json:"currency"`        // e.g. "USDC:BRL"
	Side           string  `json:"side"`            // "buy" | "sell"
	ProductID      int     `json:"product_id"`      // e.g. 24
}

//
// ────────────────────────────────────────────────
//   BRAZA → CANONICAL  : Quote Preview Response
// ────────────────────────────────────────────────
//

type BrazaQuoteResponse struct {
	ID            string `json:"id"`
	ForeignAmount string `json:"fgn_qty_client"` // or fgn_quantity
	BRLAmount     string `json:"brl_quantity"`
	Quote         string `json:"quote"`
	FinalQuote    string `json:"final_quotation"`
	IOF           string `json:"iof"`
	VET           string `json:"vet"`
	Fees          string `json:"fees_amount"`
	Parity        string `json:"parity"`
	Status        string `json:"status,omitempty"`
}

//
// ────────────────────────────────────────────────
//   BRAZA → CANONICAL  : Execute Order (lightweight)
// ────────────────────────────────────────────────
//
// Returned by POST /rates-ttl/v2/order/{quotation_id}/execute-order
// Only indicates order submission, not final execution.
//

type BrazaExecuteResponse struct {
	StatusOrder string `json:"status_order"` // e.g. "Processing"
}

//
// ────────────────────────────────────────────────
//   BRAZA → CANONICAL  : Order / Trade Status
// ────────────────────────────────────────────────
//
// Returned by GET /trader-api/order/{id}
// Provides the actual fill info when ready.
//

type BrazaOrderStatus struct {
	ID             int     `json:"id"`
	UUID           string  `json:"uuid"`
	Status         string  `json:"status"`
	Side           string  `json:"side"`
	Instrument     string  `json:"par"`
	ExecutionPrice float64 `json:"execution_price"`
	Qty            float64 `json:"qty"`
	Timestamp      string  `json:"timestamp"`
}

type BrazaProductListResponse struct {
	Count   int               `json:"count"`
	Results []BrazaProductDef `json:"results"`
}

type BrazaProductDef struct {
	ID               int    `json:"id"`
	Par              string `json:"par"`                // e.g. "USDCBRL"
	Nome             string `json:"nome"`               // e.g. "CRYPTO D0/D0 TEMP"
	IDProductCompany int    `json:"id_product_company"` // internal Braza ID
	FlagPedirData    int    `json:"flag_pedir_data"`
	FlagBloqueado    int    `json:"flag_bloqueado"`
}
