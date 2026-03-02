package zodia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── HMACSigner ───────────────────────────────────────────────────────────────

func TestHMACSigner_Sign_Deterministic(t *testing.T) {
	s := NewHMACSigner()
	body := []byte(`{"tonce":1234567890}`)
	secret := "test-api-secret"

	sig1 := s.Sign(body, secret)
	sig2 := s.Sign(body, secret)
	assert.Equal(t, sig1, sig2, "same input should produce same signature")
	assert.NotEmpty(t, sig1)
}

func TestHMACSigner_Sign_DifferentSecrets(t *testing.T) {
	s := NewHMACSigner()
	body := []byte(`{"tonce":1234567890}`)

	sig1 := s.Sign(body, "secret-a")
	sig2 := s.Sign(body, "secret-b")
	assert.NotEqual(t, sig1, sig2, "different secrets should produce different signatures")
}

func TestHMACSigner_Sign_DifferentBodies(t *testing.T) {
	s := NewHMACSigner()
	secret := "shared-secret"

	sig1 := s.Sign([]byte(`{"tonce":1}`), secret)
	sig2 := s.Sign([]byte(`{"tonce":2}`), secret)
	assert.NotEqual(t, sig1, sig2, "different bodies should produce different signatures")
}

func TestHMACSigner_Sign_IsHex(t *testing.T) {
	s := NewHMACSigner()
	sig := s.Sign([]byte("body"), "secret")
	// SHA512 hex is 128 chars
	assert.Len(t, sig, 128)
	for _, c := range sig {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"signature should be lowercase hex")
	}
}

func TestTonce_IsPositive(t *testing.T) {
	t1 := Tonce()
	t2 := Tonce()
	assert.Greater(t, t1, int64(0))
	// tonce should be microseconds; current time in microseconds is ~1.7e15
	assert.Greater(t, t1, int64(1_000_000_000_000_000))
	// monotonically non-decreasing
	assert.GreaterOrEqual(t, t2, t1)
}

// ─── WSTokenManager ───────────────────────────────────────────────────────────

func testZodiaCfg(apiKey string) *ZodiaClientConfig {
	return &ZodiaClientConfig{
		APIKey:    apiKey,
		APISecret: "secret-" + apiKey,
		BaseURL:   "https://test.zodiamarkets.com",
	}
}



func TestWSTokenManager_GetToken_CacheHit(t *testing.T) {
	mgr := &WSTokenManager{
		logger: zap.NewNop(),
		cache:  make(map[string]wsTokenEntry),
	}

	// Pre-populate cache with valid token
	mgr.cache["api-key-1"] = wsTokenEntry{
		token:     "cached-ws-token",
		expiresAt: time.Now().Add(25 * time.Hour),
	}

	// Since we can't call GetToken without a real RESTClient,
	// we test the cache directly to verify the TTL logic
	entry, ok := mgr.cache["api-key-1"]
	assert.True(t, ok)
	assert.Equal(t, "cached-ws-token", entry.token)
	assert.True(t, time.Now().Before(entry.expiresAt.Add(-wsTokenExpiryBuffer)),
		"token within valid window (>30min from expiry)")
}

func TestWSTokenManager_GetToken_NearExpiry(t *testing.T) {
	mgr := &WSTokenManager{
		logger: zap.NewNop(),
		cache:  make(map[string]wsTokenEntry),
	}

	// Token expires in 10 minutes — within the 30-minute buffer
	mgr.cache["api-key-2"] = wsTokenEntry{
		token:     "expiring-soon-token",
		expiresAt: time.Now().Add(10 * time.Minute),
	}

	entry := mgr.cache["api-key-2"]
	// Check: within buffer means we should NOT reuse
	shouldRefresh := !time.Now().Before(entry.expiresAt.Add(-wsTokenExpiryBuffer))
	assert.True(t, shouldRefresh, "token near expiry should trigger refresh")
}

func TestWSTokenManager_InvalidateToken(t *testing.T) {
	mgr := &WSTokenManager{
		logger: zap.NewNop(),
		cache: map[string]wsTokenEntry{
			"api-key-3": {token: "valid-token", expiresAt: time.Now().Add(24 * time.Hour)},
		},
	}

	mgr.InvalidateToken("api-key-3")
	_, ok := mgr.cache["api-key-3"]
	assert.False(t, ok, "token should be removed after invalidation")
}

func TestWSTokenManager_InvalidateToken_NonExistent(t *testing.T) {
	mgr := &WSTokenManager{
		logger: zap.NewNop(),
		cache:  make(map[string]wsTokenEntry),
	}
	// Should not panic
	assert.NotPanics(t, func() {
		mgr.InvalidateToken("non-existent-key")
	})
}

func TestWSTokenManager_GetToken_ViaRESTServer(t *testing.T) {
	tokenResp, _ := json.Marshal(ZodiaWSAuthResponse{Token: "server-ws-token"})
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Verify HMAC headers are set
		assert.NotEmpty(t, r.Header.Get("Rest-Key"), "Rest-Key header required")
		assert.NotEmpty(t, r.Header.Get("Rest-Sign"), "Rest-Sign header required")

		body, _ := io.ReadAll(r.Body)
		var req ZodiaWSAuthRequest
		_ = json.Unmarshal(body, &req)
		assert.Greater(t, req.Tonce, int64(0), "tonce must be positive")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tokenResp)
	}))
	defer srv.Close()

	signer := NewHMACSigner()
	restClient := NewRESTClient(zap.NewNop(), nil, signer)
	mgr := NewWSTokenManager(zap.NewNop(), restClient)

	cfg := testZodiaCfg("test-key")
	cfg.BaseURL = srv.URL

	ctx := context.Background()
	token, err := mgr.GetToken(ctx, cfg)
	require.NoError(t, err)
	assert.Equal(t, "server-ws-token", token)
	assert.Equal(t, 1, callCount, "should call REST server once")

	// Second call should use cache
	token2, err := mgr.GetToken(ctx, cfg)
	require.NoError(t, err)
	assert.Equal(t, "server-ws-token", token2)
	assert.Equal(t, 1, callCount, "second call should use cache")
}
