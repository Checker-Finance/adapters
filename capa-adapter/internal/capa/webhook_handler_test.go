package capa

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/Checker-Finance/adapters/internal/webhooks"
)

func TestValidateWebhookSignature(t *testing.T) {
	secret := "test-webhook-secret"
	body := []byte(`{"event":"transaction.updated","transactionId":"tx-123","status":"COMPLETED"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

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
			name:      "invalid signature",
			secret:    secret,
			signature: "deadbeef",
			body:      body,
			want:      false,
		},
		{
			name:      "wrong secret",
			secret:    "wrong-secret",
			signature: validSig,
			body:      body,
			want:      false,
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

func TestProcessWebhookEvent_NoSignatureValidation(t *testing.T) {
	// When resolver is nil, signature validation is skipped.
	handler := NewWebhookHandler(nil, // publisher
		nil, // store
		nil, // poller
		nil, // tradeSync
		nil, // service
		nil, // resolver — skip validation
	)

	event := &CapaWebhookEvent{
		Event:         "transaction.updated",
		TransactionID: "tx-123",
		Status:        "IN_PROGRESS",
		Transaction: CapaTransaction{
			ID:     "tx-123",
			Status: "IN_PROGRESS",
		},
	}

	err := handler.ProcessWebhookEvent(context.Background(), "client-1", event, "", []byte{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProcessWebhookEvent_InvalidSignature(t *testing.T) {
	resolver := &mockResolver{cfg: &CapaClientConfig{
		APIKey:        "key",
		BaseURL:       "https://sandbox.capa.fi",
		UserID:        "user-1",
		WebhookSecret: "test-secret",
	}}

	handler := NewWebhookHandler(nil, nil, nil, nil, nil,
		resolver,
	)

	event := &CapaWebhookEvent{
		Event:         "transaction.updated",
		TransactionID: "tx-123",
		Status:        "COMPLETED",
	}

	err := handler.ProcessWebhookEvent(context.Background(), "client-1", event, "invalidsig", []byte(`{}`))
	if err != ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestProcessWebhookEvent_ValidSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"event":"transaction.updated","transactionId":"tx-123","status":"COMPLETED"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	resolver := &mockResolver{cfg: &CapaClientConfig{
		APIKey:        "key",
		BaseURL:       "https://sandbox.capa.fi",
		UserID:        "user-1",
		WebhookSecret: secret,
	}}

	handler := NewWebhookHandler(nil, nil, nil, nil, nil,
		resolver,
	)

	event := &CapaWebhookEvent{
		Event:         "transaction.updated",
		TransactionID: "tx-123",
		Status:        "COMPLETED",
		Transaction:   CapaTransaction{ID: "tx-123", Status: "COMPLETED"},
	}

	err := handler.ProcessWebhookEvent(context.Background(), "client-1", event, sig, body)
	if err != nil {
		t.Errorf("unexpected error with valid signature: %v", err)
	}
}

// ─────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────

type mockResolver struct {
	cfg *CapaClientConfig
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (*CapaClientConfig, error) {
	return m.cfg, m.err
}

func (m *mockResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return nil, nil
}
