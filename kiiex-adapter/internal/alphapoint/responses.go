package alphapoint

// SendOrderResponse represents the response from sending an order
type SendOrderResponse struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"errormsg"`
	OrderID      int64  `json:"OrderId"`
}

// CancelOrderResponse represents the response from canceling an order
type CancelOrderResponse struct {
	Result    bool   `json:"result"`
	Error     string `json:"errormsg"`
	ErrorCode int    `json:"errorcode"`
	Details   string `json:"detail"`
	OrderID   int64  `json:"orderId"`
}

// Order represents an order from AlphaPoint (used in responses)
type Order struct {
	Side             string  `json:"Side"`
	OrderID          int     `json:"OrderId"`
	Price            float64 `json:"Price"`
	Quantity         float64 `json:"Quantity"`
	DisplayQuantity  float64 `json:"DisplayQuantity"`
	Instrument       int     `json:"Instrument"`
	Account          int     `json:"Account"`
	OrderType        string  `json:"OrderType"`
	ClientOrderID    int     `json:"ClientOrderId"`
	OrderState       string  `json:"OrderState"`
	ReceiveTime      int64   `json:"ReceiveTime"`
	ReceiveTimeTicks int64   `json:"ReceiveTimeTicks"`
	OrigQuantity     float64 `json:"OrigQuantity"`
	QuantityExecuted float64 `json:"QuantityExecuted"`
	AvgPrice         float64 `json:"AvgPrice"`
	CounterPartyID   int     `json:"CounterPartyId"`
	ChangeReason     string  `json:"ChangeReason"`
	OrigOrderID      int     `json:"OrigOrderId"`
	OrigClOrdID      int     `json:"OrigClOrdId"`
	EnteredBy        int     `json:"EnteredBy"`
	IsQuote          bool    `json:"IsQuote"`
	InsideAsk        float64 `json:"InsideAsk"`
	InsideAskSize    float64 `json:"InsideAskSize"`
	InsideBid        float64 `json:"InsideBid"`
	InsideBidSize    float64 `json:"InsideBidSize"`
	LastTradePrice   float64 `json:"LastTradePrice"`
	RejectReason     string  `json:"RejectReason"`
	IsLockedIn       bool    `json:"IsLockedIn"`
	CancelReason     string  `json:"CancelReason"`
	OmsID            int     `json:"OMSId"`
}

// GetOrderStatusResponse represents the response from getting order status
type GetOrderStatusResponse struct {
	Orders []Order `json:"orders"`
}

// AuthenticationResponse represents the response from authentication
type AuthenticationResponse struct {
	Authenticated bool   `json:"authenticated"`
	User          *User  `json:"user"`
	Locked        bool   `json:"locked"`
	Requires2FA   bool   `json:"requires2FA"`
	TwoFAType     string `json:"twoFAType"`
	TwoFAToken    string `json:"twoFAToken"`
	ErrorMsg      string `json:"errormsg"`
}

// User represents user details in authentication response
type User struct {
	UserID        int    `json:"userId"`
	UserName      string `json:"userName"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	AccountID     int    `json:"accountId"`
	OmsID         int    `json:"omsId"`
	Use2FA        bool   `json:"use2FA"`
}

// CancelAllOrdersResponse represents the response from canceling all orders
type CancelAllOrdersResponse struct {
	Result    bool   `json:"result"`
	Error     string `json:"errormsg"`
	ErrorCode int    `json:"errorcode"`
	Details   string `json:"detail"`
}

// GetInstrumentsResponse represents the response from getting instruments
type GetInstrumentsResponse struct {
	Instruments []Instrument `json:"instruments"`
}

// Instrument represents an instrument
type Instrument struct {
	InstrumentID int    `json:"InstrumentId"`
	Symbol       string `json:"Symbol"`
	Product1     int    `json:"Product1"`
	Product2     int    `json:"Product2"`
}

// GetProductsResponse represents the response from getting products
type GetProductsResponse struct {
	Products []Product `json:"products"`
}

// Product represents a product
type Product struct {
	ProductID   int    `json:"ProductId"`
	ProductName string `json:"ProductName"`
	ProductType string `json:"ProductType"`
}
