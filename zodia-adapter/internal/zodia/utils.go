package zodia

import "strings"

// NormalizeTransactionState maps Zodia transaction states to canonical status strings.
// Known Zodia states: "PENDING", "PROCESSED"
// ⚠️ Failure/cancellation states (e.g. "REJECTED", "CANCELLED", "FAILED") need
// verification against sandbox — add them here when confirmed.
func NormalizeTransactionState(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PENDING":
		return "pending"
	case "PROCESSED":
		return "filled"
	case "FAILED":
		return "rejected"
	case "REJECTED":
		return "rejected"
	case "CANCELLED":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(state))
	}
}

// IsTerminalState returns true if the Zodia transaction state represents a final state
// from which no further transitions are expected.
func IsTerminalState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PROCESSED", "FAILED", "REJECTED", "CANCELLED":
		return true
	default:
		return false
	}
}

// ToZodiaPair converts a canonical currency pair to Zodia's dot notation.
// Example: "USD:MXN" → "USD.MXN", "USD/MXN" → "USD.MXN"
func ToZodiaPair(canonicalPair string) string {
	p := strings.ToUpper(canonicalPair)
	p = strings.ReplaceAll(p, ":", ".")
	p = strings.ReplaceAll(p, "/", ".")
	p = strings.ReplaceAll(p, "_", ".")
	return p
}

// FromZodiaPair converts a Zodia dot-notation pair back to the canonical colon notation.
// Example: "USD.MXN" → "USD:MXN"
func FromZodiaPair(zodiaPair string) string {
	return strings.ReplaceAll(strings.ToUpper(zodiaPair), ".", ":")
}

// statusLabel returns "ok" or "error" for use as a Prometheus label.
func statusLabel(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
