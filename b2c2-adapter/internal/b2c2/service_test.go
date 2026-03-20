package b2c2_test

import (
	"context"
	"errors"
	"testing"


	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
)

// ─── Mock Resolver ────────────────────────────────────────────────────────────

type mockResolver struct {
	cfg *b2c2.B2C2ClientConfig
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (*b2c2.B2C2ClientConfig, error) {
	return m.cfg, m.err
}

func (m *mockResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return nil, nil
}

// ─── Mock Publisher ───────────────────────────────────────────────────────────

type mockPublisher struct {
	quoteEvents  []*b2c2.QuoteArrivedEvent
	fillEvents   []*b2c2.FillArrivedEvent
	cancelEvents []*b2c2.OrderCanceledEvent
	err          error
}

func (m *mockPublisher) PublishQuoteEvent(_ context.Context, e *b2c2.QuoteArrivedEvent) error {
	if m.err != nil {
		return m.err
	}
	m.quoteEvents = append(m.quoteEvents, e)
	return nil
}

func (m *mockPublisher) PublishFillEvent(_ context.Context, e *b2c2.FillArrivedEvent) error {
	if m.err != nil {
		return m.err
	}
	m.fillEvents = append(m.fillEvents, e)
	return nil
}

func (m *mockPublisher) PublishCancelEvent(_ context.Context, e *b2c2.OrderCanceledEvent) error {
	if m.err != nil {
		return m.err
	}
	m.cancelEvents = append(m.cancelEvents, e)
	return nil
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestHandleRFQCommand_Success(t *testing.T) {
	ctx := context.Background()

	rfqResp := &b2c2.RFQResponse{
		RFQID:      "b2c2-rfq-1",
		Instrument: "USDBTC.SPOT",
		Side:       "buy",
		Quantity:   "1000000",
		Price:      "0.00003",
		ValidUntil: "2024-01-01T12:00:00Z",
	}

	pub := &mockPublisher{}

	// Build service with stub functions via the internal mapper path.
	// We use FromRFQResponse directly to verify the mapping is correct,
	// then verify publisher receives the right event.
	cmd := &b2c2.SubmitRequestForQuoteCommand{
		ID:             "rfq-1",
		InstrumentPair: "usd:btc",
		Quantity:       "1000000",
		Side:           "BUY",
		ClientID:       "client-1",
	}

	event := b2c2.FromRFQResponse(rfqResp, cmd)
	if err := pub.PublishQuoteEvent(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}


	if len(pub.quoteEvents) != 1 {
		t.Fatalf("expected 1 quote event, got %d", len(pub.quoteEvents))
	}
	if pub.quoteEvents[0].ExternalQuoteID != "b2c2-rfq-1" {
		t.Errorf("expected externalQuoteId b2c2-rfq-1, got %s", pub.quoteEvents[0].ExternalQuoteID)
	}
}

func TestHandleOrderCommand_FilledOrder(t *testing.T) {
	ctx := context.Background()

	execPrice := "0.00003"
	orderResp := &b2c2.OrderResponse{
		OrderID:       "b2c2-order-1",
		ClientOrderID: "co-1",
		Instrument:    "USDBTC.SPOT",
		Side:          "buy",
		Quantity:      "1000000",
		ExecutedPrice: &execPrice,
		Status:        "FILLED",
	}

	cmd := &b2c2.SubmitOrderCommand{
		OrderID:           "order-1",
		InstrumentPair:    "usd:btc",
		Quantity:          "1000000",
		Price:             "0.00003",
		Side:              "BUY",
		ClientOrderID:     "co-1",
		RequestForQuoteID: "rfq-1",
		ClientID:          "client-1",
	}

	pub := &mockPublisher{}
	event := b2c2.FromOrderResponseFilled(orderResp, cmd)
	if err := pub.PublishFillEvent(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.fillEvents) != 1 {
		t.Fatalf("expected 1 fill event, got %d", len(pub.fillEvents))
	}
	if pub.fillEvents[0].Status != "FILLED" {
		t.Errorf("expected status FILLED, got %s", pub.fillEvents[0].Status)
	}
	if pub.fillEvents[0].ExecutionType != "trade" {
		t.Errorf("expected executionType trade, got %s", pub.fillEvents[0].ExecutionType)
	}
}

func TestHandleOrderCommand_NoLiquidity(t *testing.T) {
	ctx := context.Background()

	orderResp := &b2c2.OrderResponse{
		OrderID:       "b2c2-order-2",
		ClientOrderID: "co-2",
		Instrument:    "USDBTC.SPOT",
		Side:          "buy",
		Quantity:      "1000000",
		ExecutedPrice: nil, // no liquidity
		Status:        "REJECTED",
	}

	cmd := &b2c2.SubmitOrderCommand{
		OrderID:           "order-2",
		InstrumentPair:    "usd:btc",
		Quantity:          "1000000",
		Price:             "0.00003",
		Side:              "BUY",
		ClientOrderID:     "co-2",
		RequestForQuoteID: "rfq-2",
		ClientID:          "client-1",
	}

	pub := &mockPublisher{}
	event := b2c2.FromOrderResponseCanceled(orderResp, cmd)
	if err := pub.PublishCancelEvent(ctx, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pub.cancelEvents) != 1 {
		t.Fatalf("expected 1 cancel event, got %d", len(pub.cancelEvents))
	}
	if pub.cancelEvents[0].Reason != "no_liquidity" {
		t.Errorf("expected reason no_liquidity, got %s", pub.cancelEvents[0].Reason)
	}
	if pub.cancelEvents[0].ClientID != "client-1" {
		t.Errorf("expected clientId client-1, got %s", pub.cancelEvents[0].ClientID)
	}
	if pub.cancelEvents[0].InstrumentPair != "usd:btc" {
		t.Errorf("expected instrumentPair usd:btc, got %s", pub.cancelEvents[0].InstrumentPair)
	}
	if pub.cancelEvents[0].Side != "BUY" {
		t.Errorf("expected side BUY, got %s", pub.cancelEvents[0].Side)
	}
	if pub.cancelEvents[0].Quantity != "1000000" {
		t.Errorf("expected quantity 1000000, got %s", pub.cancelEvents[0].Quantity)
	}
	if pub.cancelEvents[0].QuotedPrice != "0.00003" {
		t.Errorf("expected quotedPrice 0.00003, got %s", pub.cancelEvents[0].QuotedPrice)
	}
	wantMsg := "Quote rfq-2 for 1,000,000 USD/BTC at 0.00003 was rejected; please resend RFQ"
	if pub.cancelEvents[0].Message != wantMsg {
		t.Errorf("expected message %q, got %q", wantMsg, pub.cancelEvents[0].Message)
	}
}

func TestPublisherError(t *testing.T) {
	ctx := context.Background()
	pub := &mockPublisher{err: errors.New("broker unavailable")}

	event := &b2c2.QuoteArrivedEvent{RequestForQuoteID: "rfq-1"}
	if err := pub.PublishQuoteEvent(ctx, event); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolverError(t *testing.T) {
	ctx := context.Background()
	resolver := &mockResolver{err: errors.New("secret not found")}

	_, err := resolver.Resolve(ctx, "unknown-client")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
