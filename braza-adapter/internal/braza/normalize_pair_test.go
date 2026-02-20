package braza

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizePairForBraza(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "standard slash pair",
			input:    "usdt/brl",
			expected: "USDT:BRL",
		},
		{
			name:     "already uppercase slash",
			input:    "USDT/BRL",
			expected: "USDT:BRL",
		},
		{
			name:     "underscore instead of slash",
			input:    "usdt_brl",
			expected: "USDT:BRL",
		},
		{
			name:     "multiple delimiters",
			input:    "usd_t_brl",
			expected: "USD:T:BRL",
		},
		{
			name:     "already normalized",
			input:    "USDT:BRL",
			expected: "USDT:BRL",
		},
		{
			name:     "mixed punctuation",
			input:    "u_s_d/t_b_r_l",
			expected: "U:S:D:T:B:R:L",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePairForBraza(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
