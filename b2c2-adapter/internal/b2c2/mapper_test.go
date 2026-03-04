package b2c2

import (
	"testing"
)

func TestToB2C2Instrument(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"usd:btc", "USDBTC.SPOT"},
		{"btc:usd", "BTCUSD.SPOT"},
		{"eth:usd", "ETHUSD.SPOT"},
		{"usd/btc", "USDBTC.SPOT"},
		{"USD:BTC", "USDBTC.SPOT"},
	}

	for _, tt := range tests {
		got := ToB2C2Instrument(tt.input)
		if got != tt.want {
			t.Errorf("ToB2C2Instrument(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToRFQRequest(t *testing.T) {
	cmd := &SubmitRequestForQuoteCommand{
		ID:             "rfq-123",
		InstrumentPair: "usd:btc",
		Quantity:       "1000000",
		Side:           "BUY",
		ClientID:       "client-1",
	}

	req := ToRFQRequest(cmd)

	if req.Instrument != "USDBTC.SPOT" {
		t.Errorf("expected instrument USDBTC.SPOT, got %s", req.Instrument)
	}
	if req.Side != "buy" {
		t.Errorf("expected side buy, got %s", req.Side)
	}
	if req.Quantity != "1000000" {
		t.Errorf("expected quantity 1000000, got %s", req.Quantity)
	}
	if req.ClientRFQID != "rfq-123" {
		t.Errorf("expected client_rfq_id rfq-123, got %s", req.ClientRFQID)
	}
}

func TestFromRFQResponse(t *testing.T) {
	cmd := &SubmitRequestForQuoteCommand{
		ID:             "rfq-123",
		InstrumentPair: "usd:btc",
		Side:           "BUY",
		Quantity:       "1000000",
		ClientID:       "client-1",
	}
	resp := &RFQResponse{
		RFQID:      "b2c2-rfq-456",
		Instrument: "USDBTC.SPOT",
		Side:       "buy",
		Quantity:   "1000000",
		Price:      "0.00003123",
		ValidUntil: "2024-01-01T12:00:00Z",
	}

	event := FromRFQResponse(resp, cmd)

	if event.RequestForQuoteID != "rfq-123" {
		t.Errorf("expected requestForQuoteId rfq-123, got %s", event.RequestForQuoteID)
	}
	if event.ExternalQuoteID != "b2c2-rfq-456" {
		t.Errorf("expected externalQuoteId b2c2-rfq-456, got %s", event.ExternalQuoteID)
	}
	if event.Price != "0.00003123" {
		t.Errorf("expected price 0.00003123, got %s", event.Price)
	}
	if event.InstrumentPair != "usd:btc" {
		t.Errorf("expected instrumentPair usd:btc, got %s", event.InstrumentPair)
	}
	if event.Provider != "b2c2" {
		t.Errorf("expected provider b2c2, got %s", event.Provider)
	}
}

func TestFromOrderResponseFilled(t *testing.T) {
	execPrice := "0.00003123"
	cmd := &SubmitOrderCommand{
		OrderID:           "order-123",
		InstrumentPair:    "usd:btc",
		Quantity:          "1000000",
		Price:             "0.00003123",
		Side:              "BUY",
		ClientOrderID:     "client-order-123",
		RequestForQuoteID: "rfq-123",
		ClientID:          "client-1",
	}
	resp := &OrderResponse{
		OrderID:       "b2c2-order-456",
		ClientOrderID: "client-order-123",
		Instrument:    "USDBTC.SPOT",
		Side:          "buy",
		Quantity:      "1000000",
		ExecutedPrice: &execPrice,
		Status:        "FILLED",
	}

	event := FromOrderResponseFilled(resp, cmd)

	if event.OrderID != "order-123" {
		t.Errorf("expected orderId order-123, got %s", event.OrderID)
	}
	if event.ExternalOrderID != "b2c2-order-456" {
		t.Errorf("expected externalOrderId b2c2-order-456, got %s", event.ExternalOrderID)
	}
	if event.Price != "0.00003123" {
		t.Errorf("expected price 0.00003123, got %s", event.Price)
	}
	if event.Status != "FILLED" {
		t.Errorf("expected status FILLED, got %s", event.Status)
	}
	if event.ExecutionType != "trade" {
		t.Errorf("expected executionType trade, got %s", event.ExecutionType)
	}
	if event.Provider != "b2c2" {
		t.Errorf("expected provider b2c2, got %s", event.Provider)
	}
}

func TestFromOrderResponseCanceled(t *testing.T) {
	cmd := &SubmitOrderCommand{
		OrderID:           "order-123",
		InstrumentPair:    "usd:btc",
		Quantity:          "1000000",
		Price:             "0.00003123",
		Side:              "BUY",
		ClientOrderID:     "client-order-123",
		RequestForQuoteID: "rfq-123",
		ClientID:          "client-1",
	}
	resp := &OrderResponse{
		OrderID:       "b2c2-order-456",
		ClientOrderID: "client-order-123",
		Instrument:    "USDBTC.SPOT",
		Side:          "buy",
		Quantity:      "1000000",
		ExecutedPrice: nil, // no liquidity
		Status:        "REJECTED",
	}

	event := FromOrderResponseCanceled(resp, cmd)

	if event.OrderID != "order-123" {
		t.Errorf("expected orderId order-123, got %s", event.OrderID)
	}
	if event.Reason != "no_liquidity" {
		t.Errorf("expected reason no_liquidity, got %s", event.Reason)
	}
	if event.Provider != "b2c2" {
		t.Errorf("expected provider b2c2, got %s", event.Provider)
	}
}

func TestToOrderRequest(t *testing.T) {
	cmd := &SubmitOrderCommand{
		OrderID:           "order-123",
		InstrumentPair:    "btc:usd",
		Quantity:          "0.5",
		Price:             "50000",
		Side:              "SELL",
		ClientOrderID:     "client-order-123",
		RequestForQuoteID: "rfq-123",
	}

	req := ToOrderRequest(cmd)

	if req.Instrument != "BTCUSD.SPOT" {
		t.Errorf("expected instrument BTCUSD.SPOT, got %s", req.Instrument)
	}
	if req.Side != "sell" {
		t.Errorf("expected side sell, got %s", req.Side)
	}
	if req.OrderType != "FOK" {
		t.Errorf("expected order_type FOK, got %s", req.OrderType)
	}
	if req.RFQID != "rfq-123" {
		t.Errorf("expected rfq_id rfq-123, got %s", req.RFQID)
	}
}

func TestEffectiveClientID(t *testing.T) {
	// Prefers ClientID over IssuerId
	cmd := &SubmitRequestForQuoteCommand{
		ClientID: "client-from-clientId",
		IssuerId: "client-from-issuerId",
	}
	if got := cmd.EffectiveClientID(); got != "client-from-clientId" {
		t.Errorf("expected client-from-clientId, got %s", got)
	}

	// Falls back to IssuerId when ClientID is empty
	cmd2 := &SubmitRequestForQuoteCommand{
		IssuerId: "client-from-issuerId",
	}
	if got := cmd2.EffectiveClientID(); got != "client-from-issuerId" {
		t.Errorf("expected client-from-issuerId, got %s", got)
	}
}
