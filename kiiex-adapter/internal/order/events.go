package order

// OrderSubmittedEvent is published when an order is submitted to AlphaPoint
type OrderSubmittedEvent struct {
	TradeInfo TradeInfo
	OrderID   string
}

// FillArrivedEvent is published when a fill is received from AlphaPoint
type FillArrivedEvent struct {
	OrderID                   string `json:"orderId,omitempty"`
	FillID                    string `json:"fillId,omitempty"`
	ExternalOrderID           string `json:"externalOrderId,omitempty"`
	InstrumentPair            string `json:"instrumentPair,omitempty"`
	QuantityFilled            string `json:"quantityFilled,omitempty"`
	QuantityCumulative        string `json:"quantityCumulative,omitempty"`
	QuantityLeaves            string `json:"quantityLeaves,omitempty"`
	Price                     string `json:"price,omitempty"`
	Side                      string `json:"side,omitempty"`
	Status                    string `json:"status,omitempty"`
	Type                      string `json:"type,omitempty"`
	ClientID                  string `json:"clientId,omitempty"`
	Date                      string `json:"date,omitempty"`
	ClientOrderID             string `json:"clientOrderId,omitempty"`
	RequestForQuoteID         string `json:"requestForQuoteId,omitempty"`
	ExternalRequestForQuoteID string `json:"externalRequestForQuoteId,omitempty"`
	Provider                  string `json:"provider,omitempty"`
	Source                    string `json:"source,omitempty"`
	SourceType                string `json:"sourceType,omitempty"`
	ExecutionType             string `json:"executionType"`
}

// NewFillArrivedEvent creates a new FillArrivedEvent with executionType set to "trade"
func NewFillArrivedEvent() *FillArrivedEvent {
	return &FillArrivedEvent{
		ExecutionType: "trade",
	}
}

// OrderCanceledEvent is published when an order is canceled
type OrderCanceledEvent struct {
	OrderID string `json:"orderId"`
}

// AttemptedCancelEvent is published when a cancel attempt is made
type AttemptedCancelEvent struct {
	OrderID int `json:"orderId"`
}
