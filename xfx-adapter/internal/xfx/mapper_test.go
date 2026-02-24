package xfx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// ─── NormalizeXFXStatus ───────────────────────────────────────────────────────

func TestNormalizeXFXStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Quote statuses
		{"ACTIVE", "ACTIVE", "pending"},
		{"active lowercase", "active", "pending"},
		{"EXECUTED", "EXECUTED", "submitted"},
		{"EXPIRED", "EXPIRED", "cancelled"},

		// Transaction statuses
		{"PENDING", "PENDING", "pending"},
		{"SETTLED", "SETTLED", "filled"},
		{"FAILED", "FAILED", "rejected"},
		{"CANCELLED", "CANCELLED", "cancelled"},

		// Case insensitivity
		{"settled lowercase", "settled", "filled"},
		{"failed mixed", "Failed", "rejected"},
		{"cancelled mixed", "Cancelled", "cancelled"},

		// Whitespace trimming
		{"spaces", "  SETTLED  ", "filled"},

		// Unknown status passes through as lowercase
		{"unknown", "SOME_NEW_STATUS", "some_new_status"},
		{"unknown mixed", "someNewStatus", "somenewstatus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeXFXStatus(tt.input))
		})
	}
}

// ─── IsTerminalStatus ─────────────────────────────────────────────────────────

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		// Non-terminal
		{"ACTIVE", false},
		{"PENDING", false},
		{"EXECUTED", false},
		{"processing", false},

		// Terminal
		{"SETTLED", true},
		{"filled", true},
		{"FAILED", true},
		{"rejected", true},
		{"CANCELLED", true},
		{"cancelled", true},
		{"EXPIRED", true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.terminal, IsTerminalStatus(tt.status))
		})
	}
}

// ─── ToXFXQuoteRequest ────────────────────────────────────────────────────────

func TestMapper_ToXFXQuoteRequest(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		name         string
		req          model.RFQRequest
		wantSymbol   string
		wantSide     string
		wantQuantity float64
	}{
		{
			name: "colon separator normalized",
			req:  model.RFQRequest{CurrencyPair: "USD:MXN", Side: "buy", Amount: 100000},
			wantSymbol: "USD/MXN", wantSide: "BUY", wantQuantity: 100000,
		},
		{
			name: "underscore separator normalized",
			req:  model.RFQRequest{CurrencyPair: "USDT_COP", Side: "sell", Amount: 50000},
			wantSymbol: "USDT/COP", wantSide: "SELL", wantQuantity: 50000,
		},
		{
			name: "already correct slash format",
			req:  model.RFQRequest{CurrencyPair: "USD/MXN", Side: "BUY", Amount: 200000},
			wantSymbol: "USD/MXN", wantSide: "BUY", wantQuantity: 200000,
		},
		{
			name: "lowercase pair uppercased",
			req:  model.RFQRequest{CurrencyPair: "usd/mxn", Side: "buy", Amount: 150000},
			wantSymbol: "USD/MXN", wantSide: "BUY", wantQuantity: 150000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.ToXFXQuoteRequest(tt.req)
			assert.Equal(t, tt.wantSymbol, result.Symbol)
			assert.Equal(t, tt.wantSide, result.Side)
			assert.Equal(t, tt.wantQuantity, result.Quantity)
		})
	}
}

// ─── FromXFXQuote ─────────────────────────────────────────────────────────────

func TestMapper_FromXFXQuote(t *testing.T) {
	m := NewMapper()

	validUntil := "2025-06-01T12:00:00Z"
	expectedExpiry, _ := time.Parse(time.RFC3339, validUntil)

	resp := &XFXQuoteResponse{
		Quote: XFXQuote{
			ID:         "xfx-qt-001",
			Symbol:     "USD/MXN",
			Side:       "buy",
			Quantity:   100000.0,
			Price:      17.5,
			ValidUntil: validUntil,
			Status:     "ACTIVE",
		},
	}

	result := m.FromXFXQuote(resp, "client-123")

	assert.Equal(t, "xfx-qt-001", result.ID)
	assert.Equal(t, "client-123", result.TakerID)
	assert.Equal(t, "USD/MXN", result.Instrument)
	assert.Equal(t, "BUY", result.Side)
	assert.Equal(t, 17.5, result.Price)
	assert.Equal(t, 17.5, result.Bid)
	assert.Equal(t, 17.5, result.Ask)
	assert.Equal(t, 100000.0, result.Quantity)
	assert.Equal(t, "MXN", result.Currency)
	assert.Equal(t, expectedExpiry, result.ExpiresAt)
	assert.Equal(t, "CREATED", result.Status)
	assert.Equal(t, "XFX", result.Venue)
}

func TestMapper_FromXFXQuote_InvalidDate(t *testing.T) {
	m := NewMapper()

	resp := &XFXQuoteResponse{
		Quote: XFXQuote{
			ID:         "xfx-qt-002",
			Symbol:     "USDT/COP",
			Side:       "sell",
			Quantity:   50000.0,
			Price:      4200.0,
			ValidUntil: "not-a-date",
		},
	}

	// Should not panic; ExpiresAt will be zero value
	result := m.FromXFXQuote(resp, "client-456")
	assert.Equal(t, "xfx-qt-002", result.ID)
	assert.True(t, result.ExpiresAt.IsZero(), "invalid date should produce zero ExpiresAt")
}

func TestMapper_FromXFXQuote_CurrencyExtraction(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		symbol   string
		wantCurr string
	}{
		{"USD/MXN", "MXN"},
		{"USDT/COP", "COP"},
		{"USD/USDT", "USDT"},
		{"NOSLASH", "NOSLASH"}, // no slash → symbol returned as-is
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			resp := &XFXQuoteResponse{Quote: XFXQuote{Symbol: tt.symbol}}
			result := m.FromXFXQuote(resp, "c")
			assert.Equal(t, tt.wantCurr, result.Currency)
		})
	}
}

// ─── FromXFXExecute ───────────────────────────────────────────────────────────

func TestMapper_FromXFXExecute(t *testing.T) {
	m := NewMapper()

	createdAt := "2025-06-01T10:00:00Z"
	expectedTime, _ := time.Parse(time.RFC3339, createdAt)

	resp := &XFXExecuteResponse{
		Transaction: XFXTransaction{
			ID:        "tx-001",
			QuoteID:   "qt-001",
			Symbol:    "USD/MXN",
			Side:      "buy",
			Quantity:  100000.0,
			Price:     17.5,
			Status:    "PENDING",
			CreatedAt: createdAt,
		},
	}

	result := m.FromXFXExecute(resp, "client-abc", "qt-001")

	assert.Equal(t, "tx-001", result.TradeID)
	assert.Equal(t, "client-abc", result.ClientID)
	assert.Equal(t, "XFX", result.Venue)
	assert.Equal(t, "USD/MXN", result.Instrument)
	assert.Equal(t, "BUY", result.Side)
	assert.Equal(t, 100000.0, result.Quantity)
	assert.Equal(t, 17.5, result.Price)
	assert.Equal(t, "pending", result.Status) // PENDING → pending
	assert.Equal(t, expectedTime, result.ExecutedAt)
	assert.Equal(t, "tx-001", result.ProviderOrderID)
	assert.Equal(t, "qt-001", result.ProviderRFQID)
}

// ─── FromXFXTransaction ───────────────────────────────────────────────────────

func TestMapper_FromXFXTransaction_WithSettledAt(t *testing.T) {
	m := NewMapper()

	createdAt := "2025-06-01T10:00:00Z"
	settledAt := "2025-06-01T10:05:00Z"
	expectedSettled, _ := time.Parse(time.RFC3339, settledAt)

	resp := &XFXTransactionResponse{
		Transaction: XFXTransaction{
			ID:        "tx-002",
			QuoteID:   "qt-002",
			Symbol:    "USDT/MXN",
			Side:      "sell",
			Quantity:  200000.0,
			Price:     17.4,
			Status:    "SETTLED",
			CreatedAt: createdAt,
			SettledAt: settledAt,
		},
	}

	result := m.FromXFXTransaction(resp, "client-xyz")

	assert.Equal(t, "tx-002", result.TradeID)
	assert.Equal(t, "client-xyz", result.ClientID)
	assert.Equal(t, "filled", result.Status)
	assert.Equal(t, "SELL", result.Side)
	assert.Equal(t, expectedSettled, result.ExecutedAt, "should use SettledAt when available")
	assert.Equal(t, "qt-002", result.ProviderRFQID)
}

func TestMapper_FromXFXTransaction_WithoutSettledAt(t *testing.T) {
	m := NewMapper()

	createdAt := "2025-06-01T10:00:00Z"
	expectedCreated, _ := time.Parse(time.RFC3339, createdAt)

	resp := &XFXTransactionResponse{
		Transaction: XFXTransaction{
			ID:        "tx-003",
			QuoteID:   "qt-003",
			Symbol:    "USD/COP",
			Side:      "buy",
			Status:    "PENDING",
			CreatedAt: createdAt,
			SettledAt: "", // no settled time
		},
	}

	result := m.FromXFXTransaction(resp, "client-xyz")

	assert.Equal(t, "pending", result.Status)
	assert.Equal(t, expectedCreated, result.ExecutedAt, "should fall back to CreatedAt when SettledAt empty")
}

func TestMapper_FromXFXTransaction_TerminalStatuses(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		xfxStatus      string
		canonicalStatus string
	}{
		{"SETTLED", "filled"},
		{"FAILED", "rejected"},
		{"CANCELLED", "cancelled"},
		{"PENDING", "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.xfxStatus, func(t *testing.T) {
			resp := &XFXTransactionResponse{
				Transaction: XFXTransaction{
					ID:        "tx-status",
					Status:    tt.xfxStatus,
					CreatedAt: "2025-06-01T10:00:00Z",
				},
			}
			result := m.FromXFXTransaction(resp, "c")
			assert.Equal(t, tt.canonicalStatus, result.Status)
		})
	}
}
