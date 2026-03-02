package zodia

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── MapWSPriceToQuote ────────────────────────────────────────────────────────

func TestMapWSPriceToQuote_BuyUsesAsk(t *testing.T) {
	m := NewMapper()
	price := WSPricePayload{
		Action:     "price_update",
		ClientRef:  "ref-1",
		Instrument: "USD.MXN",
		Side:       "BUY",
		Quantity:   100_000,
		Price:      0, // absent — should pick Ask
		Bid:        17.10,
		Ask:        17.20,
		QuoteID:    "zodia-quote-1",
		ExpiresAt:  time.Now().Add(15 * time.Second).Unix(),
	}
	req := model.RFQRequest{ClientID: "client-1", CurrencyPair: "USD:MXN"}

	q := m.MapWSPriceToQuote(price, req)
	require.NotNil(t, q)
	assert.Equal(t, "zodia-quote-1", q.ID)
	assert.Equal(t, "client-1", q.TakerID)
	assert.Equal(t, "USD:MXN", q.Instrument)
	assert.Equal(t, "BUY", q.Side)
	assert.Equal(t, 17.20, q.Price, "BUY with no Price should use Ask")
	assert.Equal(t, 17.10, q.Bid)
	assert.Equal(t, 17.20, q.Ask)
	assert.Equal(t, 100_000.0, q.Quantity)
	assert.Equal(t, "MXN", q.Currency)
	assert.Equal(t, "ZODIA", q.Venue)
	assert.Equal(t, "CREATED", q.Status)
	assert.False(t, q.ExpiresAt.IsZero())
}

func TestMapWSPriceToQuote_SellUsesBid(t *testing.T) {
	m := NewMapper()
	price := WSPricePayload{
		Instrument: "USD.MXN",
		Side:       "SELL",
		Price:      0,
		Bid:        17.10,
		Ask:        17.20,
		QuoteID:    "q2",
		ExpiresAt:  0, // zero → zero time
	}
	req := model.RFQRequest{ClientID: "c2"}

	q := m.MapWSPriceToQuote(price, req)
	assert.Equal(t, 17.10, q.Price, "SELL with no Price should use Bid")
	assert.True(t, q.ExpiresAt.IsZero(), "zero ExpiresAt should remain zero")
}

func TestMapWSPriceToQuote_ExplicitPrice(t *testing.T) {
	m := NewMapper()
	price := WSPricePayload{
		Instrument: "BTC.USDC",
		Side:       "BUY",
		Price:      42_000.5,
		Bid:        41_999,
		Ask:        42_001,
		QuoteID:    "q3",
	}
	q := m.MapWSPriceToQuote(price, model.RFQRequest{})
	assert.Equal(t, 42_000.5, q.Price, "explicit Price should be used as-is")
}

func TestMapWSPriceToQuote_QuoteCurrency(t *testing.T) {
	tests := []struct {
		instrument string
		wantCcy    string
	}{
		{"USD.MXN", "MXN"},
		{"BTC.USDC", "USDC"},
		{"ETH.USD", "USD"},
		{"NODOT", "NODOT"}, // fallback: whole string
	}
	m := NewMapper()
	for _, tt := range tests {
		t.Run(tt.instrument, func(t *testing.T) {
			q := m.MapWSPriceToQuote(WSPricePayload{Instrument: tt.instrument}, model.RFQRequest{})
			assert.Equal(t, tt.wantCcy, q.Currency)
		})
	}
}

// ─── MapWSOrderToTrade ────────────────────────────────────────────────────────

func TestMapWSOrderToTrade_Basic(t *testing.T) {
	m := NewMapper()
	now := time.Now().Unix()
	confirm := WSOrderConfirmPayload{
		Action:     "order_confirmation",
		ClientRef:  "ref-exec-1",
		TradeID:    "trade-xyz",
		Instrument: "USD.MXN",
		Side:       "buy",
		Quantity:   50_000,
		Price:      17.15,
		Status:     "PROCESSED",
		ExecutedAt: now,
	}

	trade := m.MapWSOrderToTrade(confirm, "client-A", "quote-id-1")
	require.NotNil(t, trade)
	assert.Equal(t, "trade-xyz", trade.TradeID)
	assert.Equal(t, "client-A", trade.ClientID)
	assert.Equal(t, "ZODIA", trade.Venue)
	assert.Equal(t, "USD:MXN", trade.Instrument)
	assert.Equal(t, "BUY", trade.Side, "side should be upper-cased")
	assert.Equal(t, 50_000.0, trade.Quantity)
	assert.Equal(t, 17.15, trade.Price)
	assert.Equal(t, "filled", trade.Status, "PROCESSED → filled via NormalizeTransactionState")
	assert.Equal(t, "trade-xyz", trade.ProviderOrderID)
	assert.Equal(t, "quote-id-1", trade.ProviderRFQID)
	assert.Equal(t, time.Unix(now, 0).UTC(), trade.ExecutedAt)
}

func TestMapWSOrderToTrade_ZeroExecutedAt(t *testing.T) {
	m := NewMapper()
	confirm := WSOrderConfirmPayload{ExecutedAt: 0}
	before := time.Now()
	trade := m.MapWSOrderToTrade(confirm, "c", "q")
	after := time.Now()
	assert.True(t, !trade.ExecutedAt.Before(before) && !trade.ExecutedAt.After(after),
		"zero ExecutedAt should default to now")
}

// ─── MapTransactionToTrade ────────────────────────────────────────────────────

func TestMapTransactionToTrade_Nil(t *testing.T) {
	m := NewMapper()
	assert.Nil(t, m.MapTransactionToTrade(nil, "client"))
}

func TestMapTransactionToTrade_ProcessedState(t *testing.T) {
	m := NewMapper()
	tx := &ZodiaTransaction{
		TradeID:    "trade-1",
		Instrument: "USD.MXN",
		Side:       "sell",
		Quantity:   200_000,
		Price:      17.30,
		State:      "PROCESSED",
		CreatedAt:  "2024-01-15T10:00:00Z",
		UpdatedAt:  "2024-01-15T10:01:00Z",
	}

	trade := m.MapTransactionToTrade(tx, "client-B")
	require.NotNil(t, trade)
	assert.Equal(t, "trade-1", trade.TradeID)
	assert.Equal(t, "client-B", trade.ClientID)
	assert.Equal(t, "USD:MXN", trade.Instrument)
	assert.Equal(t, "SELL", trade.Side)
	assert.Equal(t, 200_000.0, trade.Quantity)
	assert.Equal(t, 17.30, trade.Price)
	assert.Equal(t, "filled", trade.Status)
	// ExecutedAt should be from UpdatedAt
	assert.Equal(t, "2024-01-15T10:01:00Z", trade.ExecutedAt.UTC().Format(time.RFC3339))
}

func TestMapTransactionToTrade_InvalidTimestamps(t *testing.T) {
	m := NewMapper()
	tx := &ZodiaTransaction{
		TradeID:   "t",
		State:     "PENDING",
		CreatedAt: "not-a-time",
		UpdatedAt: "also-not-a-time",
	}
	before := time.Now()
	trade := m.MapTransactionToTrade(tx, "c")
	after := time.Now()
	// Should fall back to time.Now()
	assert.True(t, !trade.ExecutedAt.Before(before) && !trade.ExecutedAt.After(after))
}

// ─── MapAccountToBalances ─────────────────────────────────────────────────────

func TestMapAccountToBalances_ParsesDecimal(t *testing.T) {
	m := NewMapper()
	resp := &ZodiaAccountResponse{
		Result: map[string]ZodiaAccountBalance{
			"USD": {Available: "100000.50", Orders: "5000.25"},
			"MXN": {Available: "2000000.00", Orders: "0"},
		},
	}

	balances := m.MapAccountToBalances(resp, "client-C")
	assert.Len(t, balances, 2)

	byInstrument := make(map[string]model.Balance)
	for _, b := range balances {
		byInstrument[b.Instrument] = b
	}

	usd := byInstrument["USD"]
	assert.Equal(t, "client-C", usd.ClientID)
	assert.Equal(t, "ZODIA", usd.Venue)
	assert.InDelta(t, 100_000.50, usd.Available, 0.001)
	assert.InDelta(t, 5_000.25, usd.Held, 0.001)
	assert.InDelta(t, 105_000.75, usd.Total, 0.001)
	assert.True(t, usd.CanBuy)
	assert.True(t, usd.CanSell)

	mxn := byInstrument["MXN"]
	assert.InDelta(t, 2_000_000.0, mxn.Available, 0.001)
	assert.InDelta(t, 0.0, mxn.Held, 0.001)
}

func TestMapAccountToBalances_InvalidDecimal(t *testing.T) {
	m := NewMapper()
	resp := &ZodiaAccountResponse{
		Result: map[string]ZodiaAccountBalance{
			"XYZ": {Available: "not-a-number", Orders: "also-bad"},
		},
	}
	balances := m.MapAccountToBalances(resp, "c")
	assert.Len(t, balances, 1)
	assert.Equal(t, 0.0, balances[0].Available, "invalid decimal should parse to 0")
	assert.Equal(t, 0.0, balances[0].Held)
}

// ─── MapInstrumentToProduct ───────────────────────────────────────────────────

func TestMapInstrumentToProduct(t *testing.T) {
	m := NewMapper()
	instr := ZodiaInstrument{
		Symbol:  "USD.MXN",
		Base:    "USD",
		Quote:   "MXN",
		Status:  "active",
		MinSize: 100_000,
	}

	prod := m.MapInstrumentToProduct(instr)
	assert.Equal(t, "ZODIA", prod.VenueCode)
	assert.Equal(t, "USD:MXN", prod.InstrumentSymbol)
	assert.Equal(t, "USD.MXN", prod.ProductID)
	assert.Equal(t, "USD:MXN", prod.ProductName)
}

// ─── WebhookToTransaction ─────────────────────────────────────────────────────

func TestWebhookToTransaction_Nil(t *testing.T) {
	m := NewMapper()
	assert.Nil(t, m.WebhookToTransaction(nil))
}

func TestWebhookToTransaction_AllFields(t *testing.T) {
	m := NewMapper()
	event := &ZodiaWebhookEvent{
		UUID:         "evt-uuid-1",
		Type:         "OTCTRADE",
		State:        "PROCESSED",
		TradeID:      "trade-wh-1",
		Instrument:   "USD.MXN",
		Side:         "BUY",
		Quantity:     100_000,
		Price:        17.25,
		DealtAmount:  100_000,
		ContraAmount: 1_725_000,
		CreatedAt:    "2024-01-15T10:00:00Z",
		UpdatedAt:    "2024-01-15T10:01:00Z",
	}

	tx := m.WebhookToTransaction(event)
	require.NotNil(t, tx)
	assert.Equal(t, "evt-uuid-1", tx.UUID)
	assert.Equal(t, "OTCTRADE", tx.Type)
	assert.Equal(t, "PROCESSED", tx.State)
	assert.Equal(t, "trade-wh-1", tx.TradeID)
	assert.Equal(t, "USD.MXN", tx.Instrument)
	assert.Equal(t, "BUY", tx.Side)
	assert.Equal(t, 100_000.0, tx.Quantity)
	assert.Equal(t, 17.25, tx.Price)
	assert.Equal(t, 100_000.0, tx.DealtAmount)
	assert.Equal(t, 1_725_000.0, tx.ContraAmount)
	assert.Equal(t, "2024-01-15T10:00:00Z", tx.CreatedAt)
	assert.Equal(t, "2024-01-15T10:01:00Z", tx.UpdatedAt)
}
