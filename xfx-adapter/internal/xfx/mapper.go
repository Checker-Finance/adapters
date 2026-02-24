package xfx

import (
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

//
// ────────────────────────────────────────────────
//   Mapper – Converts between XFX and Canonical
// ────────────────────────────────────────────────
//

// Mapper translates between XFX-specific payloads and Checker's canonical domain models.
type Mapper struct{}

// NewMapper constructs a Mapper instance.
func NewMapper() *Mapper { return &Mapper{} }

//
// ────────────────────────────────────────────────
//   CANONICAL → XFX : Quote Request
// ────────────────────────────────────────────────
//

// ToXFXQuoteRequest converts a canonical RFQRequest to XFX's quote request format.
// XFX uses symbol (e.g. "USD/MXN"), side ("BUY"/"SELL"), and quantity (base currency amount).
func (m *Mapper) ToXFXQuoteRequest(r model.RFQRequest) *XFXQuoteRequest {
	symbol := normalizeSymbol(r.CurrencyPair)
	side := strings.ToUpper(r.Side)

	return &XFXQuoteRequest{
		Symbol:   symbol,
		Side:     side,
		Quantity: r.Amount,
	}
}

// normalizeSymbol converts currency pair formats to XFX's expected "BASE/QUOTE" format.
func normalizeSymbol(pair string) string {
	pair = strings.ToUpper(pair)
	pair = strings.ReplaceAll(pair, ":", "/")
	pair = strings.ReplaceAll(pair, "_", "/")
	return pair
}

//
// ────────────────────────────────────────────────
//   XFX → CANONICAL : Quote Response
// ────────────────────────────────────────────────
//

// FromXFXQuote converts an XFX quote to a canonical Quote.
func (m *Mapper) FromXFXQuote(resp *XFXQuoteResponse, clientID string) *model.Quote {
	q := resp.Quote
	validUntil, _ := time.Parse(time.RFC3339, q.ValidUntil)

	return &model.Quote{
		ID:         q.ID,
		TakerID:    clientID,
		Instrument: q.Symbol,
		Side:       strings.ToUpper(q.Side),
		Price:      q.Price,
		Bid:        q.Price,
		Ask:        q.Price,
		Quantity:   q.Quantity,
		Currency:   quoteFromSymbol(q.Symbol),
		ExpiresAt:  validUntil,
		Status:     "CREATED",
		Venue:      "XFX",
		Timestamp:  time.Now().UTC(),
	}
}

// quoteFromSymbol extracts the quote currency from a symbol like "USD/MXN" → "MXN".
func quoteFromSymbol(symbol string) string {
	parts := strings.Split(symbol, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return symbol
}

//
// ────────────────────────────────────────────────
//   XFX → CANONICAL : Execute Response
// ────────────────────────────────────────────────
//

// FromXFXExecute converts an XFX execute response to a canonical TradeConfirmation.
func (m *Mapper) FromXFXExecute(resp *XFXExecuteResponse, clientID, quoteID string) *model.TradeConfirmation {
	tx := resp.Transaction
	return txToTradeConfirmation(&tx, clientID, quoteID)
}

// FromXFXTransaction converts an XFX transaction to a canonical TradeConfirmation.
func (m *Mapper) FromXFXTransaction(resp *XFXTransactionResponse, clientID string) *model.TradeConfirmation {
	tx := resp.Transaction
	return txToTradeConfirmation(&tx, clientID, tx.QuoteID)
}

func txToTradeConfirmation(tx *XFXTransaction, clientID, quoteID string) *model.TradeConfirmation {
	createdAt, _ := time.Parse(time.RFC3339, tx.CreatedAt)
	executedAt := createdAt
	if tx.SettledAt != "" {
		if t, err := time.Parse(time.RFC3339, tx.SettledAt); err == nil {
			executedAt = t
		}
	}

	return &model.TradeConfirmation{
		TradeID:         tx.ID,
		ClientID:        clientID,
		Venue:           "XFX",
		Instrument:      tx.Symbol,
		Side:            strings.ToUpper(tx.Side),
		Quantity:        tx.Quantity,
		Price:           tx.Price,
		Status:          NormalizeXFXStatus(tx.Status),
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

// NormalizeXFXStatus maps XFX transaction/quote statuses to canonical statuses.
// XFX quote statuses: ACTIVE, EXPIRED, EXECUTED, CANCELLED
// XFX transaction statuses: PENDING, SETTLED, FAILED, CANCELLED
func NormalizeXFXStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	// Quote statuses
	case "ACTIVE":
		return "pending"
	case "EXECUTED":
		return "submitted"
	case "EXPIRED":
		return "cancelled"

	// Transaction statuses
	case "PENDING":
		return "pending"
	case "SETTLED":
		return "filled"
	case "FAILED":
		return "rejected"
	case "CANCELLED":
		return "cancelled"

	default:
		return strings.ToLower(status)
	}
}

// IsTerminalStatus returns true if the status represents a final state.
func IsTerminalStatus(status string) bool {
	switch NormalizeXFXStatus(status) {
	case "filled", "cancelled", "rejected":
		return true
	default:
		return false
	}
}
