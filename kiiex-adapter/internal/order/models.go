package order

// TradeInfo contains AlphaPoint trade identifiers
type TradeInfo struct {
	OmsID     int `json:"omsId"`
	AccountID int `json:"accountId"`
	OrderID   int `json:"orderId"`
}

// Side represents the order side (Buy/Sell)
type Side int

const (
	SideBuy Side = iota
	SideSell
	SideShort
	SideUnknown
)

// SideFromString converts a string to Side
func SideFromString(s string) Side {
	switch s {
	case "Buy":
		return SideBuy
	case "Sell":
		return SideSell
	case "Short":
		return SideShort
	default:
		return SideUnknown
	}
}

// ToInt returns the integer value for AlphaPoint API
func (s Side) ToInt() int {
	switch s {
	case SideBuy:
		return 0
	case SideSell:
		return 1
	case SideShort:
		return 2
	default:
		return 3
	}
}

// String returns the string representation
func (s Side) String() string {
	switch s {
	case SideBuy:
		return "Buy"
	case SideSell:
		return "Sell"
	case SideShort:
		return "Short"
	default:
		return "Unknown"
	}
}

// OrderType represents the type of order
type OrderType int

const (
	OrderTypeUnknown OrderType = iota
	OrderTypeMarket
	OrderTypeLimit
	OrderTypeStopMarket
	OrderTypeStopLimit
	OrderTypeTrailingStopMarket
	OrderTypeTrailingStopLimit
	OrderTypeBlockTrade
)

// OrderTypeFromString converts a string to OrderType
func OrderTypeFromString(s string) OrderType {
	switch s {
	case "MarketOrder":
		return OrderTypeMarket
	case "Limit":
		return OrderTypeLimit
	case "StopMarket":
		return OrderTypeStopMarket
	case "StopLimit":
		return OrderTypeStopLimit
	case "TrailingStopMarket":
		return OrderTypeTrailingStopMarket
	case "TrailingStopLimit":
		return OrderTypeTrailingStopLimit
	case "BlockTrade":
		return OrderTypeBlockTrade
	default:
		return OrderTypeUnknown
	}
}

// ToInt returns the integer value for AlphaPoint API
func (t OrderType) ToInt() int {
	switch t {
	case OrderTypeMarket:
		return 1
	case OrderTypeLimit:
		return 2
	case OrderTypeStopMarket:
		return 3
	case OrderTypeStopLimit:
		return 4
	case OrderTypeTrailingStopMarket:
		return 5
	case OrderTypeTrailingStopLimit:
		return 6
	case OrderTypeBlockTrade:
		return 7
	default:
		return 0
	}
}

// String returns the string representation
func (t OrderType) String() string {
	switch t {
	case OrderTypeMarket:
		return "MarketOrder"
	case OrderTypeLimit:
		return "Limit"
	case OrderTypeStopMarket:
		return "StopMarket"
	case OrderTypeStopLimit:
		return "StopLimit"
	case OrderTypeTrailingStopMarket:
		return "TrailingStopMarket"
	case OrderTypeTrailingStopLimit:
		return "TrailingStopLimit"
	case OrderTypeBlockTrade:
		return "BlockTrade"
	default:
		return "Unknown"
	}
}

// TimeInForce represents the time in force for an order
type TimeInForce int

const (
	TimeInForceUnknown TimeInForce = iota
	TimeInForceGTC                 // Good Till Canceled
	TimeInForceOPG                 // Opening
	TimeInForceIOC                 // Immediate or Cancel
	TimeInForceFOK                 // Fill or Kill
	TimeInForceGTX                 // Good Till Crossing
	TimeInForceGTD                 // Good Till Date
)

// TimeInForceFromString converts a string to TimeInForce
func TimeInForceFromString(s string) TimeInForce {
	switch s {
	case "GTC":
		return TimeInForceGTC
	case "OPG":
		return TimeInForceOPG
	case "IOC":
		return TimeInForceIOC
	case "FOK":
		return TimeInForceFOK
	case "GTX":
		return TimeInForceGTX
	case "GTD":
		return TimeInForceGTD
	default:
		return TimeInForceUnknown
	}
}

// ToInt returns the integer value for AlphaPoint API
func (t TimeInForce) ToInt() int {
	switch t {
	case TimeInForceGTC:
		return 1
	case TimeInForceOPG:
		return 2
	case TimeInForceIOC:
		return 3
	case TimeInForceFOK:
		return 4
	case TimeInForceGTX:
		return 5
	case TimeInForceGTD:
		return 6
	default:
		return 0
	}
}

// String returns the string representation
func (t TimeInForce) String() string {
	switch t {
	case TimeInForceGTC:
		return "GTC"
	case TimeInForceOPG:
		return "OPG"
	case TimeInForceIOC:
		return "IOC"
	case TimeInForceFOK:
		return "FOK"
	case TimeInForceGTX:
		return "GTX"
	case TimeInForceGTD:
		return "GTD"
	default:
		return "Unknown"
	}
}

// Order represents an order from AlphaPoint
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

// OrderState constants
const (
	OrderStateFilled   = "Filled"
	OrderStateCanceled = "Canceled"
	OrderStateRejected = "Rejected"
	OrderStateWorking  = "Working"
)
