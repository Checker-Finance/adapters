package b2c2

import (
	"fmt"
	"strings"
)

//
// ────────────────────────────────────────────────────────────
//   Instrument Conversion
// ────────────────────────────────────────────────────────────
//

// ToB2C2Instrument converts a canonical instrument pair to B2C2 format.
// Example: "usd:btc" → "USDBTC.SPOT", "btc:usd" → "BTCUSD.SPOT"
func ToB2C2Instrument(pair string) string {
	// Remove delimiters and uppercase
	clean := strings.ReplaceAll(pair, ":", "")
	clean = strings.ReplaceAll(clean, "/", "")
	clean = strings.ReplaceAll(clean, "_", "")
	return strings.ToUpper(clean) + ".SPOT"
}

//
// ────────────────────────────────────────────────────────────
//   RFQ Mapping
// ────────────────────────────────────────────────────────────
//

// ToRFQRequest converts a SubmitRequestForQuoteCommand to a B2C2 RFQRequest.
func ToRFQRequest(cmd *SubmitRequestForQuoteCommand) *RFQRequest {
	return &RFQRequest{
		Instrument:  ToB2C2Instrument(cmd.InstrumentPair),
		Side:        strings.ToLower(cmd.Side),
		Quantity:    cmd.Quantity,
		ClientRFQID: cmd.ID,
	}
}

// FromRFQResponse converts a B2C2 RFQResponse to a QuoteArrivedEvent.
// The canonical instrumentPair is taken from the original command to avoid
// lossy reverse-mapping from B2C2 instrument names.
func FromRFQResponse(resp *RFQResponse, cmd *SubmitRequestForQuoteCommand) *QuoteArrivedEvent {
	return &QuoteArrivedEvent{
		RequestForQuoteID: cmd.ID,
		ExternalQuoteID:   resp.RFQID,
		Price:             resp.Price,
		Side:              cmd.Side,
		InstrumentPair:    cmd.InstrumentPair,
		Quantity:          resp.Quantity,
		Expiry:            resp.ValidUntil,
		Provider:          "b2c2",
	}
}

//
// ────────────────────────────────────────────────────────────
//   Order Mapping
// ────────────────────────────────────────────────────────────
//

// ToOrderRequest converts a SubmitOrderCommand to a B2C2 OrderRequest.
func ToOrderRequest(cmd *SubmitOrderCommand) *OrderRequest {
	return &OrderRequest{
		Instrument:    ToB2C2Instrument(cmd.InstrumentPair),
		Side:          strings.ToLower(cmd.Side),
		Quantity:      cmd.Quantity,
		Price:         cmd.Price,
		OrderType:     "FOK",
		RFQID:         cmd.RequestForQuoteID,
		ClientOrderID: cmd.ClientOrderID,
	}
}

// FromOrderResponseFilled converts a filled B2C2 OrderResponse to a FillArrivedEvent.
// Called when executed_price != nil (order was filled).
func FromOrderResponseFilled(resp *OrderResponse, cmd *SubmitOrderCommand) *FillArrivedEvent {
	price := ""
	if resp.ExecutedPrice != nil {
		price = *resp.ExecutedPrice
	}
	return &FillArrivedEvent{
		OrderID:            cmd.OrderID,
		FillID:             fmt.Sprintf("%s-fill", resp.OrderID),
		ExternalOrderID:    resp.OrderID,
		InstrumentPair:     cmd.InstrumentPair,
		QuantityFilled:     resp.Quantity,
		QuantityCumulative: resp.Quantity,
		Price:              price,
		Side:               cmd.Side,
		Status:             "FILLED",
		ClientOrderID:      cmd.ClientOrderID,
		RequestForQuoteID:  cmd.RequestForQuoteID,
		Provider:           "b2c2",
		SourceType:         "b2c2",
		ExecutionType:      "trade",
	}
}

// FromOrderResponseCanceled converts a no-liquidity B2C2 OrderResponse to an OrderCanceledEvent.
// Called when executed_price == nil (no liquidity / FOK not filled).
func FromOrderResponseCanceled(resp *OrderResponse, cmd *SubmitOrderCommand) *OrderCanceledEvent {
	return &OrderCanceledEvent{
		OrderID:           cmd.OrderID,
		ClientOrderID:     cmd.ClientOrderID,
		RequestForQuoteID: cmd.RequestForQuoteID,
		Provider:          "b2c2",
		Reason:            "no_liquidity",
	}
}
