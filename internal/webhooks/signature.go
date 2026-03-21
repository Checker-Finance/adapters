// Package webhooks provides shared utilities for webhook processing.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ValidateHMACSHA256 verifies that the given HMAC-SHA256 signature matches
// the provided body using the shared secret. The signature may optionally
// carry a "sha256=" prefix (e.g. as sent by GitHub/common webhook frameworks).
func ValidateHMACSHA256(secret, signature string, body []byte) bool {
	normalized := strings.TrimSpace(signature)
	const prefix = "sha256="
	if strings.HasPrefix(strings.ToLower(normalized), prefix) {
		normalized = normalized[len(prefix):]
	}
	expected, err := hex.DecodeString(normalized)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	actual := mac.Sum(nil)
	return hmac.Equal(actual, expected)
}
