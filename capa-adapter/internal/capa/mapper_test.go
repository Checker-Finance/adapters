package capa

import (
	"testing"
)

func TestDetectTransactionType(t *testing.T) {
	tests := []struct {
		pair string
		want TransactionType
	}{
		// Cross-ramp (fiat ↔ fiat)
		{"USD:MXN", CrossRamp},
		{"USD/MXN", CrossRamp},
		{"EUR:DOP", CrossRamp},
		{"MXN:USD", CrossRamp},

		// On-ramp (fiat → crypto)
		{"USD:USDC", OnRamp},
		{"MXN:USDT", OnRamp},
		{"USD:ETH", OnRamp},
		{"DOP:USDC", OnRamp},

		// Off-ramp (crypto → fiat)
		{"USDC:MXN", OffRamp},
		{"USDT:USD", OffRamp},
		{"ETH:USD", OffRamp},
		{"USDC:DOP", OffRamp},
	}

	for _, tt := range tests {
		t.Run(tt.pair, func(t *testing.T) {
			got := DetectTransactionType(tt.pair)
			if got != tt.want {
				t.Errorf("DetectTransactionType(%q) = %q, want %q", tt.pair, got, tt.want)
			}
		})
	}
}

func TestNormalizeCapaStatus(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"COMPLETED", "filled"},
		{"COMPLETED_ON_RAMP", "filled"},
		{"COMPLETED_OFF_RAMP", "filled"},
		{"CANCELLED", "cancelled"},
		{"FAILED", "rejected"},
		{"REJECTED", "rejected"},
		{"PENDING_FUNDS", "pending"},
		{"FUNDS_RECEIVED", "pending"},
		{"IN_PROGRESS", "pending"},
		{"CREATED_ON_RAMP", "pending"},
		{"FIAT_RECEIVED_ON_RAMP", "pending"},
		{"CRYPTO_RECEIVED_OFF_RAMP", "pending"},
		{"AWAITING_FUND_TRANSFER", "pending"},
		{"", "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := NormalizeCapaStatus(tt.raw)
			if got != tt.want {
				t.Errorf("NormalizeCapaStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	terminal := []string{"COMPLETED", "COMPLETED_ON_RAMP", "COMPLETED_OFF_RAMP", "CANCELLED", "FAILED", "REJECTED"}
	nonTerminal := []string{"PENDING_FUNDS", "FUNDS_RECEIVED", "IN_PROGRESS", "CREATED_ON_RAMP", "AWAITING_FUND_TRANSFER"}

	for _, s := range terminal {
		if !IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) should be true", s)
		}
	}
	for _, s := range nonTerminal {
		if IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) should be false", s)
		}
	}
}

func TestFromCapaQuote(t *testing.T) {
	m := NewMapper()
	resp := &CapaQuoteResponse{
		ID:                  "quote-123",
		UserID:              "user-abc",
		SourceCurrency:      "USD",
		DestinationCurrency: "MXN",
		SourceAmount:        1000.0,
		DestinationAmount:   18500.0,
		ExchangeRate:        18.5,
		ExpiresAt:           "2026-01-01T00:00:00Z",
		Status:              "CREATED",
	}

	quote := m.FromCapaQuote(resp, "client-1")
	if quote.ID != "quote-123" {
		t.Errorf("expected ID=quote-123, got %s", quote.ID)
	}
	if quote.Price != 18.5 {
		t.Errorf("expected Price=18.5, got %f", quote.Price)
	}
	if quote.Instrument != "USD/MXN" {
		t.Errorf("expected Instrument=USD/MXN, got %s", quote.Instrument)
	}
	if quote.Venue != "CAPA" {
		t.Errorf("expected Venue=CAPA, got %s", quote.Venue)
	}
	if quote.TakerID != "client-1" {
		t.Errorf("expected TakerID=client-1, got %s", quote.TakerID)
	}
}

func TestFromCapaTransaction(t *testing.T) {
	m := NewMapper()
	tx := &CapaTransaction{
		ID:                  "tx-456",
		QuoteID:             "quote-123",
		UserID:              "user-abc",
		SourceCurrency:      "USDC",
		DestinationCurrency: "MXN",
		SourceAmount:        1000.0,
		DestinationAmount:   18500.0,
		ExchangeRate:        18.5,
		Status:              "COMPLETED_OFF_RAMP",
		CreatedAt:           "2026-01-01T00:00:00Z",
		UpdatedAt:           "2026-01-01T00:01:00Z",
	}

	trade := m.FromCapaTransaction(tx, "client-1")
	if trade.TradeID != "tx-456" {
		t.Errorf("expected TradeID=tx-456, got %s", trade.TradeID)
	}
	if trade.Status != "filled" {
		t.Errorf("expected Status=filled, got %s", trade.Status)
	}
	if trade.Venue != "CAPA" {
		t.Errorf("expected Venue=CAPA, got %s", trade.Venue)
	}
	if trade.Instrument != "USDC/MXN" {
		t.Errorf("expected Instrument=USDC/MXN, got %s", trade.Instrument)
	}
}
