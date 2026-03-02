package zodia

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ─── NormalizeTransactionState ────────────────────────────────────────────────

func TestNormalizeTransactionState(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"PENDING lowercase", "PENDING", "pending"},
		{"pending lowercase", "pending", "pending"},
		{"PROCESSED → filled", "PROCESSED", "filled"},
		{"processed lowercase", "processed", "filled"},
		{"FAILED → rejected", "FAILED", "rejected"},
		{"REJECTED → rejected", "REJECTED", "rejected"},
		{"CANCELLED → cancelled", "CANCELLED", "cancelled"},
		{"cancelled lowercase", "cancelled", "cancelled"},
		{"whitespace trimmed", "  PROCESSED  ", "filled"},
		{"mixed case", "Processed", "filled"},
		{"unknown passthrough", "SOME_NEW_STATE", "some_new_state"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeTransactionState(tt.input))
		})
	}
}

// ─── IsTerminalState ──────────────────────────────────────────────────────────

func TestIsTerminalState(t *testing.T) {
	tests := []struct {
		state    string
		terminal bool
	}{
		// Terminal
		{"PROCESSED", true},
		{"processed", true},
		{"FAILED", true},
		{"REJECTED", true},
		{"CANCELLED", true},
		{"cancelled", true},

		// Non-terminal
		{"PENDING", false},
		{"pending", false},
		{"", false},
		{"UNKNOWN", false},
		{"IN_PROGRESS", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			assert.Equal(t, tt.terminal, IsTerminalState(tt.state))
		})
	}
}

// ─── ToZodiaPair ──────────────────────────────────────────────────────────────

func TestToZodiaPair(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"USD:MXN", "USD.MXN"},
		{"USD/MXN", "USD.MXN"},
		{"USD_MXN", "USD.MXN"},
		{"usd:mxn", "USD.MXN"},
		{"BTC:USDC", "BTC.USDC"},
		{"ETH/USD", "ETH.USD"},
		{"USD.MXN", "USD.MXN"}, // already in dot notation
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, ToZodiaPair(tt.input))
		})
	}
}

// ─── FromZodiaPair ────────────────────────────────────────────────────────────

func TestFromZodiaPair(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"USD.MXN", "USD:MXN"},
		{"BTC.USDC", "BTC:USDC"},
		{"ETH.USD", "ETH:USD"},
		{"usd.mxn", "USD:MXN"},
		{"USD:MXN", "USD:MXN"}, // no dot → no change
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, FromZodiaPair(tt.input))
		})
	}
}

// ─── Pair round-trip ──────────────────────────────────────────────────────────

func TestPairRoundTrip(t *testing.T) {
	pairs := []string{"USD:MXN", "BTC:USDC", "ETH:USD", "USD:COP"}
	for _, pair := range pairs {
		t.Run(pair, func(t *testing.T) {
			zodia := ToZodiaPair(pair)
			canonical := FromZodiaPair(zodia)
			assert.Equal(t, pair, canonical, "round-trip should be idempotent")
		})
	}
}

// ─── statusLabel ─────────────────────────────────────────────────────────────

func TestStatusLabel(t *testing.T) {
	assert.Equal(t, "ok", statusLabel(nil))
	assert.Equal(t, "error", statusLabel(assert.AnError))
}
