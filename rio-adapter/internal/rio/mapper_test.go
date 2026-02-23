package rio

import (
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeRioStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Pending
		{"created", "created", "pending"},
		{"Created uppercase", "CREATED", "pending"},

		// Submitted (in progress)
		{"processing", "processing", "submitted"},
		{"sourcingliquidity", "sourcingLiquidity", "submitted"},
		{"awaiting_payment", "awaiting_payment", "submitted"},
		{"awaitingpayment", "awaitingPayment", "submitted"},
		{"verifying", "verifying", "submitted"},
		{"manual_review", "manual_review", "submitted"},
		{"compliance_review", "complianceReview", "submitted"},

		// Filled (completed)
		{"paid", "paid", "filled"},
		{"filled", "filled", "filled"},
		{"complete", "complete", "filled"},
		{"completed", "completed", "filled"},
		{"settled", "settled", "filled"},
		{"transfer_complete", "transfer_complete", "filled"},

		// Cancelled
		{"cancelled", "cancelled", "cancelled"},
		{"canceled", "canceled", "cancelled"},
		{"expired", "expired", "cancelled"},
		{"user_cancelled", "user_cancelled", "cancelled"},
		{"timeout", "timeout", "cancelled"},
		{"quote_expired", "quoteExpired", "cancelled"},

		// Rejected (failed)
		{"failed", "failed", "rejected"},
		{"rejected", "rejected", "rejected"},
		{"declined", "declined", "rejected"},
		{"payment_failed", "payment_failed", "rejected"},
		{"failedpayment", "failedPayment", "rejected"},
		{"transfer_failed", "transfer_failed", "rejected"},
		{"insufficient_liquidity", "insufficient_liquidity", "rejected"},
		{"compliance_rejected", "complianceRejected", "rejected"},
		{"kyc_failed", "kycFailed", "rejected"},
		{"blocked", "blocked", "rejected"},

		// Refunding
		{"refund", "refund", "refunding"},
		{"refunding", "refunding", "refunding"},
		{"refund_pending", "refund_pending", "refunding"},
		{"refund_processing", "refundProcessing", "refunding"},

		// Refunded
		{"refunded", "refunded", "refunded"},
		{"refund_complete", "refund_complete", "refunded"},

		// Refund failed -> rejected
		{"refund_failed", "refund_failed", "rejected"},

		// Unknown status passes through
		{"unknown", "some_new_status", "some_new_status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeRioStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{"created", false},
		{"processing", false},
		{"pending", false},
		{"verifying", false},

		{"filled", true},
		{"completed", true},
		{"paid", true},

		{"cancelled", true},
		{"expired", true},

		{"rejected", true},
		{"failed", true},

		{"refunded", true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := IsTerminalStatus(tt.status)
			assert.Equal(t, tt.terminal, result)
		})
	}
}

func TestMapper_ToRioQuoteRequest(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		name     string
		req      model.RFQRequest
		country  string
		expected *RioQuoteRequest
	}{
		{
			name: "buy with fiat amount",
			req: model.RFQRequest{
				ClientID:       "client-123",
				CurrencyPair:   "usdc/usd",
				Side:           "BUY",
				Amount:         1000.0,
				CurrencyAmount: "USD",
			},
			country: "US",
			expected: &RioQuoteRequest{
				Crypto:     "USDC",
				Fiat:       "USD",
				Side:       "buy",
				Country:    "US",
				AmountFiat: 1000.0,
			},
		},
		{
			name: "sell with crypto amount",
			req: model.RFQRequest{
				ClientID:       "client-456",
				CurrencyPair:   "USDT:MXN",
				Side:           "sell",
				Amount:         500.0,
				CurrencyAmount: "USDT",
			},
			country: "MX",
			expected: &RioQuoteRequest{
				Crypto:       "USDT",
				Fiat:        "MXN",
				Side:        "sell",
				Country:     "MX",
				AmountCrypto: 500.0,
			},
		},
		{
			name: "default to fiat amount when not specified",
			req: model.RFQRequest{
				ClientID:     "client-789",
				CurrencyPair: "btc/pen",
				Side:         "buy",
				Amount:       2000.0,
			},
			country: "PE",
			expected: &RioQuoteRequest{
				Crypto:     "BTC",
				Fiat:       "PEN",
				Side:       "buy",
				Country:    "PE",
				AmountFiat: 2000.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.ToRioQuoteRequest(tt.req, tt.country)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapper_FromRioQuote(t *testing.T) {
	m := NewMapper()

	resp := &RioQuoteResponse{
		ID:           "quote-123",
		UserID:       "user-456",
		Crypto:       "USDC",
		Fiat:         "USD",
		Side:         "buy",
		AmountFiat:   1000.0,
		AmountCrypto: 999.5,
		NetPrice:     1.0005,
		MarketPrice:  1.0003,
		ExpiresAt:    "2024-01-15T10:30:00Z",
		CreatedAt:    "2024-01-15T10:29:00Z",
	}

	result := m.FromRioQuote(resp, "client-123")

	assert.Equal(t, "quote-123", result.ID)
	assert.Equal(t, "client-123", result.TakerID)
	assert.Equal(t, "USDC/USD", result.Instrument)
	assert.Equal(t, "BUY", result.Side)
	// canonical rate = AmountFiat / AmountCrypto = 1000 / 999.5 ≈ 1.0005
	assert.InDelta(t, 1000.0/999.5, result.Price, 1e-6, "Price should be AmountFiat/AmountCrypto")
	assert.InDelta(t, 1000.0/999.5, result.Bid, 1e-6, "Bid should match canonical rate")
	assert.InDelta(t, 1000.0/999.5, result.Ask, 1e-6, "Ask should match canonical rate")
	assert.Equal(t, 999.5, result.Quantity)
	assert.Equal(t, "CREATED", result.Status)
	assert.Equal(t, "RIO", result.Venue)
}

func TestMapper_FromRioOrder(t *testing.T) {
	m := NewMapper()

	resp := &RioOrderResponse{
		ID:                "order-789",
		QuoteID:           "quote-123",
		UserID:            "user-456",
		Status:            "completed",
		Side:              "buy",
		Crypto:            "USDC",
		Fiat:              "MXN",
		AmountFiat:        20000.0,
		AmountCrypto:      1000.0,
		NetPrice:          20.0,
		ClientReferenceID: "client-ref-123",
		CreatedAt:         "2024-01-15T10:30:00Z",
		CompletedAt:       "2024-01-15T10:35:00Z",
	}

	result := m.FromRioOrder(resp, "client-123")

	assert.Equal(t, "order-789", result.TradeID)
	assert.Equal(t, "client-123", result.ClientID)
	assert.Equal(t, "RIO", result.Venue)
	assert.Equal(t, "USDC/MXN", result.Instrument)
	assert.Equal(t, "BUY", result.Side)
	assert.Equal(t, 1000.0, result.Quantity)
	// canonical rate = AmountFiat / AmountCrypto = 20000 / 1000 = 20.0
	assert.Equal(t, 20.0, result.Price, "Price should be AmountFiat/AmountCrypto")
	assert.Equal(t, "filled", result.Status)
	assert.Equal(t, "order-789", result.ProviderOrderID)
	assert.Equal(t, "quote-123", result.ProviderRFQID)

	// Check that CompletedAt is used when available
	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-15T10:35:00Z")
	assert.Equal(t, expectedTime, result.ExecutedAt)
}

func TestCanonicalRate(t *testing.T) {
	tests := []struct {
		name         string
		amountFiat   float64
		amountCrypto float64
		want         float64
		tol          float64
	}{
		{"USDC/MXN", 88187.11, 5000, 17.637422, 0.001},
		{"BTC/USD", 21000, 0.5, 42000, 0},
		{"USDC/BRL near parity", 5000, 1000, 5.0, 0},
		{"zero crypto returns zero", 1000, 0, 0, 0},
		{"zero fiat returns zero", 0, 1000, 0, 0},
		{"both zero returns zero", 0, 0, 0, 0},
		{"small crypto amount", 42000, 0.001, 42000000, 0},
		{"rate close to 1", 1000, 999.5, 1.0005, 0.0001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalRate(tt.amountFiat, tt.amountCrypto)
			if tt.tol == 0 {
				assert.Equal(t, tt.want, got)
			} else {
				assert.InDelta(t, tt.want, got, tt.tol)
			}
		})
	}
}

func TestMapper_FromRioQuote_ZeroAmounts(t *testing.T) {
	m := NewMapper()

	resp := &RioQuoteResponse{
		ID:           "quote-zero",
		Crypto:       "USDC",
		Fiat:         "MXN",
		Side:         "buy",
		AmountFiat:   0,
		AmountCrypto: 0,
		NetPrice:     0.056443,
		MarketPrice:  0.056,
		ExpiresAt:    "2024-01-15T10:30:00Z",
	}

	result := m.FromRioQuote(resp, "client-123")

	assert.Equal(t, 0.0, result.Price, "Zero amounts should produce zero canonical price")
	assert.Equal(t, 0.0, result.Bid)
	assert.Equal(t, 0.0, result.Ask)
}

func TestMapper_FromRioQuote_BTCUSD(t *testing.T) {
	m := NewMapper()

	// BTC/USD: NetPrice might be 42000 (already canonical) or 0.0000238 (inverted).
	// canonicalRate ignores NetPrice entirely — it uses AmountFiat/AmountCrypto.
	resp := &RioQuoteResponse{
		ID:           "quote-btc",
		Crypto:       "BTC",
		Fiat:         "USD",
		Side:         "buy",
		AmountFiat:   21000,
		AmountCrypto: 0.5,
		NetPrice:     42000, // doesn't matter — we derive from amounts
		MarketPrice:  41900,
		ExpiresAt:    "2024-01-15T10:30:00Z",
	}

	result := m.FromRioQuote(resp, "client-123")

	// canonical = 21000 / 0.5 = 42000 USD per 1 BTC
	assert.Equal(t, 42000.0, result.Price, "BTC/USD should be 42000 derived from amounts")
	assert.Equal(t, "BTC/USD", result.Instrument)
}

func TestParsePair(t *testing.T) {
	tests := []struct {
		pair          string
		expectedBase  string
		expectedQuote string
	}{
		{"usdc/usd", "USDC", "USD"},
		{"USDT:MXN", "USDT", "MXN"},
		{"btc_pen", "BTC", "PEN"},
		{"ETH/EUR", "ETH", "EUR"},
		{"invalid", "INVALID", ""},
	}

	for _, tt := range tests {
		t.Run(tt.pair, func(t *testing.T) {
			base, quote := parsePair(tt.pair)
			assert.Equal(t, tt.expectedBase, base)
			assert.Equal(t, tt.expectedQuote, quote)
		})
	}
}
