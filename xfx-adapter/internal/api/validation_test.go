package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── RFQCreateRequest.Validate ────────────────────────────────────────────────

func TestRFQCreateRequest_Validate_Valid(t *testing.T) {
	req := RFQCreateRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD/MXN",
		Side:         "buy",
		Amount:       100000,
	}
	assert.NoError(t, req.Validate())
}

func TestRFQCreateRequest_Validate_MissingClientID(t *testing.T) {
	req := RFQCreateRequest{
		CurrencyPair: "USD/MXN",
		Side:         "buy",
		Amount:       100000,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clientId is required")
}

func TestRFQCreateRequest_Validate_MissingPair(t *testing.T) {
	req := RFQCreateRequest{
		ClientID: "client-001",
		Side:     "buy",
		Amount:   100000,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pair is required")
}

func TestRFQCreateRequest_Validate_MissingSide(t *testing.T) {
	req := RFQCreateRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD/MXN",
		Amount:       100000,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "orderSide is required")
}

func TestRFQCreateRequest_Validate_ZeroQuantity(t *testing.T) {
	req := RFQCreateRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD/MXN",
		Side:         "buy",
		Amount:       0,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quantity must be positive")
}

func TestRFQCreateRequest_Validate_NegativeQuantity(t *testing.T) {
	req := RFQCreateRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD/MXN",
		Side:         "buy",
		Amount:       -100,
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quantity must be positive")
}

// ─── RFQExecuteRequest.Validate ───────────────────────────────────────────────

func TestRFQExecuteRequest_Validate_Valid(t *testing.T) {
	req := RFQExecuteRequest{
		ClientID: "client-001",
		QuoteID:  "qt-001",
	}
	assert.NoError(t, req.Validate())
}

func TestRFQExecuteRequest_Validate_MissingClientID(t *testing.T) {
	req := RFQExecuteRequest{
		QuoteID: "qt-001",
	}
	err := req.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clientId is required")
}
