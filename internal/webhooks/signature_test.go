package webhooks_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/Checker-Finance/adapters/internal/webhooks"
)

func computeSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestValidateHMACSHA256(t *testing.T) {
	secret := "test-webhook-secret"
	body := []byte(`{"event":"order.filled","orderId":"ord-123","status":"COMPLETED"}`)
	validSig := computeSig(secret, body)

	tests := []struct {
		name      string
		secret    string
		signature string
		body      []byte
		want      bool
	}{
		{
			name:      "valid signature",
			secret:    secret,
			signature: validSig,
			body:      body,
			want:      true,
		},
		{
			name:      "valid signature with sha256= prefix",
			secret:    secret,
			signature: "sha256=" + validSig,
			body:      body,
			want:      true,
		},
		{
			name:      "valid signature with SHA256= prefix (uppercase)",
			secret:    secret,
			signature: "SHA256=" + validSig,
			body:      body,
			want:      true,
		},
		{
			name:      "valid signature with whitespace",
			secret:    secret,
			signature: "  " + validSig + "  ",
			body:      body,
			want:      true,
		},
		{
			name:      "tampered body",
			secret:    secret,
			signature: validSig,
			body:      []byte(`{"event":"order.filled","orderId":"ord-999","status":"COMPLETED"}`),
			want:      false,
		},
		{
			name:      "wrong secret",
			secret:    "wrong-secret",
			signature: validSig,
			body:      body,
			want:      false,
		},
		{
			name:      "empty signature",
			secret:    secret,
			signature: "",
			body:      body,
			want:      false,
		},
		{
			name:      "invalid hex in signature",
			secret:    secret,
			signature: "not-valid-hex!",
			body:      body,
			want:      false,
		},
		{
			name:      "empty body with matching sig",
			secret:    secret,
			signature: computeSig(secret, []byte{}),
			body:      []byte{},
			want:      true,
		},
		{
			name:      "empty secret",
			secret:    "",
			signature: computeSig("", body),
			body:      body,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := webhooks.ValidateHMACSHA256(tt.secret, tt.signature, tt.body)
			if got != tt.want {
				t.Errorf("ValidateHMACSHA256() = %v, want %v", got, tt.want)
			}
		})
	}
}
