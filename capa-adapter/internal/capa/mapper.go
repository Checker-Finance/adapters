package capa

import (
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

//
// ────────────────────────────────────────────────
//   Known crypto symbols for transaction type detection
// ────────────────────────────────────────────────
//

var knownCryptos = map[string]bool{
	"USDC": true,
	"USDT": true,
	"ETH":  true,
	"BTC":  true,
	"SOL":  true,
	"POL":  true,
	"ARB":  true,
	"OP":   true,
}

//
// ────────────────────────────────────────────────
//   Mapper – Converts between Capa and Canonical
// ────────────────────────────────────────────────
//

// Mapper translates between Capa-specific payloads and Checker's canonical domain models.
type Mapper struct{}

// NewMapper constructs a Mapper instance.
func NewMapper() *Mapper { return &Mapper{} }

//
// ────────────────────────────────────────────────
//   Transaction type detection
// ────────────────────────────────────────────────
//

// DetectTransactionType determines the Capa transaction type from a canonical currency pair.
// Pair format: "BASE:QUOTE" (e.g. "USD:MXN", "USD:USDC", "USDC:MXN").
func DetectTransactionType(pair string) TransactionType {
	parts := strings.SplitN(strings.ToUpper(pair), ":", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(strings.ToUpper(pair), "/", 2)
	}
	if len(parts) != 2 {
		return CrossRamp
	}
	base := parts[0]
	quote := parts[1]
	baseIsCrypto := knownCryptos[base]
	quoteIsCrypto := knownCryptos[quote]

	switch {
	case baseIsCrypto && !quoteIsCrypto:
		return OffRamp
	case !baseIsCrypto && quoteIsCrypto:
		return OnRamp
	default:
		// Both fiat or both crypto → cross-ramp
		return CrossRamp
	}
}

//
// ────────────────────────────────────────────────
//   CANONICAL → CAPA : Quote Requests
// ────────────────────────────────────────────────
//

// ToCrossRampQuoteRequest converts a canonical RFQRequest to a Capa cross-ramp quote request.
func (m *Mapper) ToCrossRampQuoteRequest(r model.RFQRequest, userID string) *CapaCrossRampQuoteRequest {
	base, quote := splitPair(r.CurrencyPair)
	amountCurrency := base
	if strings.ToUpper(r.Side) == "BUY" {
		amountCurrency = quote
	}
	return &CapaCrossRampQuoteRequest{
		UserID:              userID,
		SourceCurrency:      base,
		DestinationCurrency: quote,
		Amount:              r.Amount,
		AmountCurrency:      amountCurrency,
	}
}

// ToOnOffRampQuoteRequest converts a canonical RFQRequest to a Capa on/off-ramp quote request.
func (m *Mapper) ToOnOffRampQuoteRequest(r model.RFQRequest, userID string, txType TransactionType) *CapaQuoteRequest {
	base, quote := splitPair(r.CurrencyPair)
	var fiat, crypto string
	switch txType {
	case OnRamp:
		// fiat → crypto
		fiat = base
		crypto = quote
	case OffRamp:
		// crypto → fiat
		crypto = base
		fiat = quote
	default:
		fiat = base
		crypto = quote
	}
	amountCurrency := fiat
	if strings.ToUpper(r.Side) == "BUY" {
		amountCurrency = crypto
	}
	return &CapaQuoteRequest{
		UserID:         userID,
		FiatCurrency:   fiat,
		CryptoCurrency: crypto,
		Amount:         r.Amount,
		AmountCurrency: amountCurrency,
	}
}

//
// ────────────────────────────────────────────────
//   CANONICAL → CAPA : Execute Requests
// ────────────────────────────────────────────────
//

// ToCrossRampExecuteRequest builds the execute payload for a cross-ramp transaction.
func (m *Mapper) ToCrossRampExecuteRequest(userID, quoteID string) *CapaCrossRampExecuteRequest {
	return &CapaCrossRampExecuteRequest{UserID: userID, QuoteID: quoteID}
}

// ToOnRampExecuteRequest builds the execute payload for an on-ramp transaction.
func (m *Mapper) ToOnRampExecuteRequest(userID, quoteID string, cfg *CapaClientConfig) *CapaOnRampExecuteRequest {
	return &CapaOnRampExecuteRequest{
		UserID:           userID,
		QuoteID:          quoteID,
		WalletAddress:    cfg.WalletAddress,
		BlockchainSymbol: cfg.BlockchainSymbol,
		TokenSymbol:      cfg.TokenSymbol,
	}
}

// ToOffRampExecuteRequest builds the execute payload for an off-ramp transaction.
func (m *Mapper) ToOffRampExecuteRequest(userID, quoteID string, cfg *CapaClientConfig) *CapaOffRampExecuteRequest {
	return &CapaOffRampExecuteRequest{
		UserID:     userID,
		QuoteID:    quoteID,
		ReceiverID: cfg.ReceiverID,
	}
}

//
// ────────────────────────────────────────────────
//   CAPA → CANONICAL : Quote Response
// ────────────────────────────────────────────────
//

// FromCapaQuote converts a Capa quote response to a canonical Quote.
func (m *Mapper) FromCapaQuote(resp *CapaQuoteResponse, clientID string) *model.Quote {
	validUntil, _ := time.Parse(time.RFC3339, resp.ExpiresAt)

	instrument := resp.SourceCurrency + "/" + resp.DestinationCurrency

	return &model.Quote{
		ID:         resp.ID,
		TakerID:    clientID,
		Instrument: instrument,
		Side:       "BUY",
		Price:      resp.ExchangeRate,
		Bid:        resp.ExchangeRate,
		Ask:        resp.ExchangeRate,
		Quantity:   resp.SourceAmount,
		Currency:   resp.DestinationCurrency,
		ExpiresAt:  validUntil,
		Status:     "CREATED",
		Venue:      "CAPA",
		Timestamp:  time.Now().UTC(),
	}
}

//
// ────────────────────────────────────────────────
//   CAPA → CANONICAL : Trade Confirmation
// ────────────────────────────────────────────────
//

// FromCapaExecuteResponse converts a Capa execute response to a canonical TradeConfirmation.
func (m *Mapper) FromCapaExecuteResponse(resp *CapaExecuteResponse, clientID, quoteID string) *model.TradeConfirmation {
	tx := resp.Transaction
	return txToTradeConfirmation(&tx, clientID, quoteID)
}

// FromCapaTransaction converts a Capa transaction to a canonical TradeConfirmation.
func (m *Mapper) FromCapaTransaction(tx *CapaTransaction, clientID string) *model.TradeConfirmation {
	return txToTradeConfirmation(tx, clientID, tx.QuoteID)
}

func txToTradeConfirmation(tx *CapaTransaction, clientID, quoteID string) *model.TradeConfirmation {
	createdAt, _ := time.Parse(time.RFC3339, tx.CreatedAt)
	executedAt := createdAt
	if tx.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, tx.UpdatedAt); err == nil {
			if IsTerminalCapaStatus(tx.Status) {
				executedAt = t
			}
		}
	}

	instrument := tx.SourceCurrency + "/" + tx.DestinationCurrency

	return &model.TradeConfirmation{
		TradeID:         tx.ID,
		ClientID:        clientID,
		Venue:           "CAPA",
		Instrument:      instrument,
		Side:            "BUY",
		Quantity:        tx.SourceAmount,
		Price:           tx.ExchangeRate,
		Status:          NormalizeCapaStatus(tx.Status),
		ExecutedAt:      executedAt,
		ProviderOrderID: tx.ID,
		ProviderRFQID:   quoteID,
		RawPayload:      "",
	}
}

//
// ────────────────────────────────────────────────
//   Status Normalization
// ────────────────────────────────────────────────
//

// NormalizeCapaStatus maps Capa transaction/event statuses to canonical statuses.
func NormalizeCapaStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "COMPLETED_ON_RAMP", "COMPLETED_OFF_RAMP":
		return "filled"
	case "CANCELLED":
		return "cancelled"
	case "FAILED", "REJECTED":
		return "rejected"
	default:
		// PENDING_FUNDS, FUNDS_RECEIVED, IN_PROGRESS, CREATED_*, FIAT_RECEIVED_*,
		// CRYPTO_RECEIVED_*, AWAITING_FUND_TRANSFER, etc.
		return "pending"
	}
}

// IsTerminalCapaStatus returns true if the raw Capa status represents a final state.
func IsTerminalCapaStatus(status string) bool {
	return model.IsTerminal(NormalizeCapaStatus(status))
}

// IsTerminalStatus is an alias kept for consistency with XFX adapter naming.
func IsTerminalStatus(status string) bool {
	return IsTerminalCapaStatus(status)
}

//
// ────────────────────────────────────────────────
//   Helpers
// ────────────────────────────────────────────────
//

// splitPair splits a canonical pair like "USD:MXN" or "USD/MXN" into (base, quote).
func splitPair(pair string) (base, quote string) {
	pair = strings.ToUpper(pair)
	if idx := strings.Index(pair, ":"); idx != -1 {
		return pair[:idx], pair[idx+1:]
	}
	if idx := strings.Index(pair, "/"); idx != -1 {
		return pair[:idx], pair[idx+1:]
	}
	return pair, ""
}
