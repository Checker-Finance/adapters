package braza

import (
	"testing"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/stretchr/testify/assert"
)

// ─── NormalizeOrderStatus ──────────────────────────────────────────────────────
// Covers the fixed bug where "COMPLETED" → "Rejected" (capital R) was returned
// and raw uppercase pass-through meant "COMPLETED" was not detected as terminal.

func TestNormalizeOrderStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Portuguese Braza statuses
		{"finalizado → filled", "finalizado", "filled"},
		{"executado → filled", "executado", "filled"},
		{"processando → submitted", "processando", "submitted"},
		{"rejeitado → rejected", "rejeitado", "rejected"},
		{"cancelado → cancelled", "cancelado", "cancelled"},
		{"pendente → pending", "pendente", "pending"},
		// English equivalents
		{"completed → filled (was bug: returned Rejected)", "completed", "filled"},
		{"COMPLETED → filled", "COMPLETED", "filled"},
		{"complete → filled", "complete", "filled"},
		{"filled → filled", "filled", "filled"},
		{"FILLED → filled", "FILLED", "filled"},
		{"rejected → rejected (was bug: returned Rejected with capital R)", "rejected", "rejected"},
		{"REJECTED → rejected", "REJECTED", "rejected"},
		{"failed → rejected (was missing: fell through to pass-through)", "failed", "rejected"},
		{"FAILED → rejected", "FAILED", "rejected"},
		{"error → rejected", "error", "rejected"},
		{"erro → rejected", "erro", "rejected"},
		{"cancelled → cancelled", "cancelled", "cancelled"},
		{"canceled → cancelled", "canceled", "cancelled"},
		{"CANCELLED → cancelled", "CANCELLED", "cancelled"},
		{"processing → submitted", "processing", "submitted"},
		{"pending → pending", "pending", "pending"},
		// Edge cases
		{"empty → empty", "", ""},
		{"whitespace → empty", "  ", ""},
		{"unknown → lowercase passthrough", "SOME_STATUS", "some_status"},
		{"unknown lowercase → passthrough", "unknown_state", "unknown_state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeOrderStatus(tt.input))
		})
	}
}

// ─── isTerminalStatus ─────────────────────────────────────────────────────────
// Ensures terminal detection works with normalized statuses (was broken before fix).

func TestIsTerminalStatus_ViaModel(t *testing.T) {
	terminal := []string{model.StatusFilled, model.StatusCancelled, model.StatusRejected}
	nonTerminal := []string{"submitted", model.StatusPending, "unknown", ""}

	for _, s := range terminal {
		if !isTerminalStatus(s) {
			t.Errorf("isTerminalStatus(%q) should be true", s)
		}
	}
	for _, s := range nonTerminal {
		if isTerminalStatus(s) {
			t.Errorf("isTerminalStatus(%q) should be false", s)
		}
	}
}

// ─── Mapper: FromBrazaQuote ───────────────────────────────────────────────────

func TestMapper_FromBrazaQuote(t *testing.T) {
	m := NewMapper()

	resp := BrazaQuoteResponse{
		ID:         "quote-123",
		Quote:      "5.42",
		FinalQuote: "5.50",
		Status:     "ACTIVE",
	}

	q := m.FromBrazaQuote(resp, "client-a")

	assert.Equal(t, "quote-123", q.ID)
	assert.Equal(t, "client-a", q.TakerID)
	assert.InDelta(t, 5.42, q.Price, 0.001)
	assert.InDelta(t, 5.50, q.Bid, 0.001)
	assert.InDelta(t, 5.50, q.Ask, 0.001)
	assert.Equal(t, "ACTIVE", q.Status)
	assert.Equal(t, "BRAZA", q.Venue)
}

func TestMapper_FromBrazaQuote_InvalidPrice(t *testing.T) {
	m := NewMapper()

	resp := BrazaQuoteResponse{
		ID:    "quote-456",
		Quote: "not-a-number",
	}

	q := m.FromBrazaQuote(resp, "client-b")
	assert.Equal(t, float64(0), q.Price, "invalid float should parse to 0")
}

// ─── Mapper: ToBrazaRFQ ──────────────────────────────────────────────────────

func TestMapper_ToBrazaRFQ(t *testing.T) {
	m := NewMapper()

	req := model.RFQRequest{
		ClientID:     "client-a",
		CurrencyPair: "USDC/BRL",
		Side:         "BUY",
		Amount:       1000,
	}

	brazaReq := m.ToBrazaRFQ(req)

	assert.Equal(t, "buy", brazaReq.Side)
	assert.Equal(t, float64(1000), brazaReq.Amount)
	assert.Equal(t, "USDC:BRL", brazaReq.Currency)
}

func TestMapper_ToBrazaRFQ_SellSide(t *testing.T) {
	m := NewMapper()

	req := model.RFQRequest{
		ClientID:     "client-a",
		CurrencyPair: "BTC:BRL",
		Side:         "SELL",
		Amount:       500,
	}

	brazaReq := m.ToBrazaRFQ(req)
	assert.Equal(t, "sell", brazaReq.Side)
}

func TestMapper_ToBrazaRFQ_InvalidSideDefaultsBuy(t *testing.T) {
	m := NewMapper()

	req := model.RFQRequest{
		ClientID:     "client-a",
		CurrencyPair: "USDT/BRL",
		Side:         "UNKNOWN",
		Amount:       100,
	}

	brazaReq := m.ToBrazaRFQ(req)
	assert.Equal(t, "buy", brazaReq.Side, "unknown side should default to buy")
}

func TestMapper_ToBrazaRFQ_WithCurrencyAmount(t *testing.T) {
	m := NewMapper()

	req := model.RFQRequest{
		ClientID:       "client-a",
		CurrencyPair:   "USDC/BRL",
		Side:           "buy",
		Amount:         500,
		CurrencyAmount: "USDC",
	}

	brazaReq := m.ToBrazaRFQ(req)
	assert.Equal(t, "USDC", brazaReq.CurrencyAmount)
}

// ─── Mapper: FromBrazaBalances ────────────────────────────────────────────────

func TestMapper_FromBrazaBalances(t *testing.T) {
	m := NewMapper()

	resp := BrazaBalancesResponse{
		{
			"USDC": BrazaBalanceDetail{
				CanBuy:                 true,
				CanSell:                false,
				AvailableTotalValueDay: 10000,
				TotalValueDay:          12000,
			},
		},
	}

	balances := m.FromBrazaBalances(resp, "client-a")

	assert.Len(t, balances, 1)
	bal := balances[0]
	assert.Equal(t, "client-a", bal.ClientID)
	assert.Equal(t, "USDC", bal.Instrument)
	assert.True(t, bal.CanBuy)
	assert.False(t, bal.CanSell)
	assert.InDelta(t, 10000, bal.Available, 0.001)
	assert.InDelta(t, 12000, bal.Total, 0.001)
}
