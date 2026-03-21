package model

// Canonical trade status constants used across all adapters.
// Adapter-specific normalizers must map venue statuses to these values.
const (
	StatusPending   = "pending"
	StatusFilled    = "filled"
	StatusCancelled = "cancelled"
	StatusRejected  = "rejected"
	StatusRefunded  = "refunded" // Rio-specific terminal state
)

// IsTerminal reports whether a normalized canonical status is a final,
// non-pollable state. The input must already be normalized (lowercase).
func IsTerminal(normalizedStatus string) bool {
	switch normalizedStatus {
	case StatusFilled, StatusCancelled, StatusRejected, StatusRefunded:
		return true
	default:
		return false
	}
}
