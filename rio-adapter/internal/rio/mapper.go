package rio

import (
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
)

//
// ────────────────────────────────────────────────
//   Mapper – Converts between Rio and Canonical
// ────────────────────────────────────────────────
//

// Mapper translates between Rio-specific payloads and Checker's canonical domain models.
type Mapper struct{}

// NewMapper constructs a Mapper instance.
func NewMapper() *Mapper { return &Mapper{} }

//
// ────────────────────────────────────────────────
//   CANONICAL → RIO : Quote Request
// ────────────────────────────────────────────────
//

// ToRioQuoteRequest converts a canonical RFQRequest to Rio's quote request format.
func (m *Mapper) ToRioQuoteRequest(r model.RFQRequest, country string) *RioQuoteRequest {
	crypto, fiat := parsePair(r.CurrencyPair)

	req := &RioQuoteRequest{
		Crypto:  strings.ToUpper(crypto),
		Fiat:    strings.ToUpper(fiat),
		Side:    strings.ToLower(r.Side),
		Country: country,
	}

	// Determine if amount is in fiat or crypto based on CurrencyAmount
	if r.CurrencyAmount != "" {
		amtCurrency := strings.ToUpper(r.CurrencyAmount)
		if amtCurrency == req.Fiat {
			req.AmountFiat = r.Amount
		} else {
			req.AmountCrypto = r.Amount
		}
	} else {
		// Default: amount is in fiat
		req.AmountFiat = r.Amount
	}

	return req
}

// parsePair splits a currency pair like "usdc/usd" or "USDC:USD" into base and quote.
func parsePair(pair string) (crypto, fiat string) {
	pair = strings.ToUpper(pair)
	pair = strings.ReplaceAll(pair, ":", "/")
	pair = strings.ReplaceAll(pair, "_", "/")

	parts := strings.Split(pair, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return pair, ""
}

//
// ────────────────────────────────────────────────
//   RIO → CANONICAL : Quote Response
// ────────────────────────────────────────────────
//

// FromRioQuote converts a Rio quote response to a canonical Quote.
func (m *Mapper) FromRioQuote(resp *RioQuoteResponse, clientID string) *model.Quote {
	expiresAt, _ := time.Parse(time.RFC3339, resp.ExpiresAt)

	rate := canonicalRate(resp.AmountFiat, resp.AmountCrypto)

	return &model.Quote{
		ID:         resp.ID,
		TakerID:    clientID,
		Instrument: formatPair(resp.Crypto, resp.Fiat),
		Side:       strings.ToUpper(resp.Side),
		Price:      rate,
		Bid:        rate,
		Ask:        rate,
		Quantity:   resp.AmountCrypto,
		Currency:   resp.Fiat,
		ExpiresAt:  expiresAt,
		Status:     "CREATED",
		Venue:      "RIO",
		Timestamp:  time.Now().UTC(),
	}
}

// canonicalRate computes the canonical quote-per-base rate from the provider's
// fiat and crypto amounts. For a pair like USDC/MXN this gives MXN-per-1-USDC
// (e.g. 88187 / 5000 = 17.637). This is convention-agnostic: it works
// regardless of how the provider quotes NetPrice internally.
// Returns 0 if amountCrypto is zero to avoid division by zero.
func canonicalRate(amountFiat, amountCrypto float64) float64 {
	if amountCrypto == 0 {
		return 0
	}
	return amountFiat / amountCrypto
}

// formatPair creates a normalized pair string.
func formatPair(crypto, fiat string) string {
	return strings.ToUpper(crypto) + "/" + strings.ToUpper(fiat)
}

//
// ────────────────────────────────────────────────
//   RIO → CANONICAL : Order Response
// ────────────────────────────────────────────────
//

// FromRioOrder converts a Rio order response to a canonical TradeConfirmation.
func (m *Mapper) FromRioOrder(resp *RioOrderResponse, clientID string) *model.TradeConfirmation {
	executedAt, _ := time.Parse(time.RFC3339, resp.CreatedAt)
	if resp.CompletedAt != "" {
		executedAt, _ = time.Parse(time.RFC3339, resp.CompletedAt)
	}

	return &model.TradeConfirmation{
		TradeID:         resp.ID,
		ClientID:        clientID,
		Venue:           "RIO",
		Instrument:      formatPair(resp.Crypto, resp.Fiat),
		Side:            strings.ToUpper(resp.Side),
		Quantity:        resp.AmountCrypto,
		Price:           canonicalRate(resp.AmountFiat, resp.AmountCrypto),
		Status:          NormalizeRioStatus(resp.Status),
		ExecutedAt:      executedAt,
		ProviderOrderID: resp.ID,
		ProviderRFQID:   resp.QuoteID,
		RawPayload:      "", // Can be populated if needed
	}
}

//
// ────────────────────────────────────────────────
//   Status Normalization
// ────────────────────────────────────────────────
//
// Rio has ~42 different order statuses. We normalize them to canonical statuses:
// - pending: Order created, awaiting action
// - submitted: Order in progress, being processed
// - filled: Order completed successfully
// - cancelled: Order was cancelled
// - rejected: Order failed
// - refunded: Order is being refunded
//

// NormalizeRioStatus maps Rio's 42+ statuses to canonical statuses.
func NormalizeRioStatus(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))

	switch s {
	// ─── Pending / Created ───
	case "created":
		return "pending"

	// ─── In Progress / Submitted ───
	case "processing",
		"sourcingliquidity",
		"sourcingpartners",
		"liquiditysourced",
		"partnersourced",
		"awaiting_payment",
		"awaitingpayment",
		"payment_pending",
		"paymentpending",
		"awaiting_transfer",
		"awaitingtransfer",
		"transfer_pending",
		"transferpending",
		"initiating_transfer",
		"initiatingtransfer",
		"transfer_initiated",
		"transferinitiated",
		"verifying",
		"verification_pending",
		"verificationpending",
		"pending_compliance",
		"pendingcompliance",
		"compliance_review",
		"compliancereview",
		"manual_review",
		"manualreview",
		"awaiting_confirmation",
		"awaitingconfirmation":
		return "submitted"

	// ─── Filled / Completed ───
	case "paid",
		"filled",
		"complete",
		"completed",
		"settled",
		"transfer_complete",
		"transfercomplete",
		"payment_received",
		"paymentreceived",
		"payment_complete",
		"paymentcomplete":
		return "filled"

	// ─── Cancelled ───
	case "cancelled",
		"canceled",
		"expired",
		"user_cancelled",
		"usercancelled",
		"user_canceled",
		"usercanceled",
		"system_cancelled",
		"systemcancelled",
		"timeout",
		"quote_expired",
		"quoteexpired":
		return "cancelled"

	// ─── Rejected / Failed ───
	case "failed",
		"rejected",
		"declined",
		"payment_failed",
		"paymentfailed",
		"failed_payment",
		"failedpayment",
		"transfer_failed",
		"transferfailed",
		"failed_transfer",
		"failedtransfer",
		"failed_fill",
		"failedfill",
		"fill_failed",
		"fillfailed",
		"failed_unknown",
		"failedunknown",
		"insufficient_liquidity",
		"insufficientliquidity",
		"insufficient_liquidity_failed",
		"insufficientliquidityfailed",
		"compliance_rejected",
		"compliancerejected",
		"verification_failed",
		"verificationfailed",
		"kyc_failed",
		"kycfailed",
		"aml_failed",
		"amlfailed",
		"blocked":
		return "rejected"

	// ─── Refund in Progress ───
	case "refund",
		"refunding",
		"refund_pending",
		"refundpending",
		"refund_initiated",
		"refundinitiated",
		"refund_processing",
		"refundprocessing":
		return "refunding"

	// ─── Refunded (completed refund) ───
	case "refunded",
		"refund_complete",
		"refundcomplete",
		"refund_completed",
		"refundcompleted":
		return "refunded"

	// ─── Refund Failed ───
	case "refund_failed",
		"refundfailed":
		return "rejected" // Treat failed refunds as rejected

	default:
		// Pass through unknown statuses as-is for debugging
		return status
	}
}

// IsTerminalStatus returns true if the status represents a final state.
func IsTerminalStatus(status string) bool {
	normalized := NormalizeRioStatus(status)
	switch normalized {
	case "filled", "cancelled", "rejected", "refunded":
		return true
	default:
		return false
	}
}

//
// ────────────────────────────────────────────────
//   API Response Mappers
// ────────────────────────────────────────────────
//

// ToAPIQuoteResponse formats a canonical Quote for API response.
func (m *Mapper) ToAPIQuoteResponse(q *model.Quote) map[string]any {
	return map[string]any{
		"id":         q.ID,
		"tenant_id":  q.TenantID,
		"instrument": q.Instrument,
		"side":       q.Side,
		"price":      q.Price,
		"quantity":   q.Quantity,
		"status":     q.Status,
		"venue":      q.Venue,
		"expires_at": q.ExpiresAt,
		"timestamp":  q.Timestamp,
	}
}

// ToAPITradeResponse formats a canonical TradeConfirmation for API response.
func (m *Mapper) ToAPITradeResponse(t *model.TradeConfirmation) map[string]any {
	return map[string]any{
		"trade_id":          t.TradeID,
		"client_id":         t.ClientID,
		"venue":             t.Venue,
		"instrument":        t.Instrument,
		"side":              t.Side,
		"quantity":          t.Quantity,
		"price":             t.Price,
		"status":            t.Status,
		"executed_at":       t.ExecutedAt,
		"provider_order_id": t.ProviderOrderID,
	}
}
