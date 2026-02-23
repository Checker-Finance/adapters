package alphapoint

// SendOrderRequest represents a request to send an order
type SendOrderRequest struct {
	InstrumentID       int     `json:"InstrumentId"`
	OmsID              int     `json:"OMSId"`
	AccountID          int     `json:"AccountId"`
	TimeInForce        int     `json:"TimeInForce"`
	ClientOrderID      int     `json:"ClientOrderId"`
	OrderIDOCO         int     `json:"OrderIdOCO"`
	UseDisplayQuantity bool    `json:"UseDisplayQuantity"`
	Side               int     `json:"Side"`
	Quantity           float64 `json:"quantity"`
	OrderType          int     `json:"OrderType"`
	PegPriceType       int     `json:"PegPriceType"`
	LimitPrice         int     `json:"LimitPrice"`
}

// CancelOrderRequest represents a request to cancel an order
type CancelOrderRequest struct {
	OmsID     int `json:"OMSId"`
	AccountID int `json:"AccountId"`
	ClOrderID int `json:"ClOrderId"`
	OrderID   int `json:"OrderId"`
}

// CancelAllOrdersRequest represents a request to cancel all orders
type CancelAllOrdersRequest struct {
	OmsID     int `json:"OMSId"`
	AccountID int `json:"AccountId"`
}

// AuthenticateUserRequest represents an authentication request
type AuthenticateUserRequest struct {
	APIKey    string `json:"APIKey"`
	Signature string `json:"Signature"`
	UserID    int    `json:"UserId"`
	Nonce     string `json:"Nonce"`
}

// GetOrderStatusRequest represents a request to get order status
type GetOrderStatusRequest struct {
	OmsID     int `json:"omsId"`
	AccountID int `json:"accountId"`
	OrderID   int `json:"orderId"`
}

// GetInstrumentsRequest represents a request to get instruments
type GetInstrumentsRequest struct {
	OmsID int `json:"OMSId"`
}

// GetProductsRequest represents a request to get products
type GetProductsRequest struct {
	OmsID int `json:"OMSId"`
}

// GetUserAccountsRequest represents a request to get user accounts
type GetUserAccountsRequest struct {
	OmsID    int    `json:"omsId"`
	UserID   int    `json:"userId"`
	Username string `json:"username"`
}

// LogOutRequest represents a logout request
type LogOutRequest struct{}

// GetOpenOrdersRequest represents a request to get open orders
type GetOpenOrdersRequest struct {
	OmsID     int `json:"OMSId"`
	AccountID int `json:"AccountId"`
}

// GetOpenQuotesRequest represents a request to get open quotes
type GetOpenQuotesRequest struct {
	OmsID        int `json:"OMSId"`
	AccountID    int `json:"AccountId"`
	InstrumentID int `json:"InstrumentId"`
}
