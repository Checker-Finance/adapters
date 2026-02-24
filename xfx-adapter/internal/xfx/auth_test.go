package xfx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockTransport is an http.RoundTripper that delegates to a handler function.
type mockTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.fn(req)
}

// auth0Response builds a fake *http.Response with the given status and JSON body.
func auth0Response(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// newTokenManagerWithTransport creates a TokenManager with a custom HTTP transport.
func newTokenManagerWithTransport(t *testing.T, fn func(*http.Request) (*http.Response, error)) *TokenManager {
	t.Helper()
	tm := NewTokenManager(zap.NewNop())
	tm.client = &http.Client{Transport: &mockTransport{fn: fn}}
	return tm
}

func testCfg(clientID string) *XFXClientConfig {
	return &XFXClientConfig{
		ClientID:     clientID,
		ClientSecret: "secret-" + clientID,
		BaseURL:      "https://api.test.xfx.io",
	}
}

// ─── GetToken: cache miss → fetches from Auth0 ────────────────────────────────

func TestTokenManager_GetToken_FetchesOnCacheMiss(t *testing.T) {
	tokenResp, _ := json.Marshal(Auth0TokenResponse{
		AccessToken: "new-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   86400,
	})

	callCount := 0
	tm := newTokenManagerWithTransport(t, func(req *http.Request) (*http.Response, error) {
		callCount++
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		assert.Equal(t, http.MethodPost, req.Method)
		return auth0Response(http.StatusOK, string(tokenResp)), nil
	})

	token, err := tm.GetToken(context.Background(), testCfg("client-a"))
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", token)
	assert.Equal(t, 1, callCount, "should call Auth0 exactly once on cache miss")
}

// ─── GetToken: cache hit → no HTTP call ──────────────────────────────────────

func TestTokenManager_GetToken_ReturnsCachedToken(t *testing.T) {
	callCount := 0
	tm := newTokenManagerWithTransport(t, func(*http.Request) (*http.Response, error) {
		callCount++
		return nil, nil
	})

	// Pre-populate cache with a valid token
	tm.mu.Lock()
	tm.cache["client-b"] = tokenEntry{
		accessToken: "cached-token",
		expiresAt:   time.Now().Add(24 * time.Hour),
	}
	tm.mu.Unlock()

	token, err := tm.GetToken(context.Background(), testCfg("client-b"))
	require.NoError(t, err)
	assert.Equal(t, "cached-token", token)
	assert.Equal(t, 0, callCount, "should NOT call Auth0 when token is cached and valid")
}

// ─── GetToken: within 5-minute buffer → refreshes ────────────────────────────

func TestTokenManager_GetToken_RefreshesWhenNearExpiry(t *testing.T) {
	tokenResp, _ := json.Marshal(Auth0TokenResponse{
		AccessToken: "refreshed-token",
		ExpiresIn:   86400,
	})

	callCount := 0
	tm := newTokenManagerWithTransport(t, func(*http.Request) (*http.Response, error) {
		callCount++
		return auth0Response(http.StatusOK, string(tokenResp)), nil
	})

	// Token expires in 3 minutes — within the 5-minute buffer
	tm.mu.Lock()
	tm.cache["client-c"] = tokenEntry{
		accessToken: "expiring-soon-token",
		expiresAt:   time.Now().Add(3 * time.Minute),
	}
	tm.mu.Unlock()

	token, err := tm.GetToken(context.Background(), testCfg("client-c"))
	require.NoError(t, err)
	assert.Equal(t, "refreshed-token", token)
	assert.Equal(t, 1, callCount, "should refresh token when within 5-minute buffer")
}

// ─── GetToken: multiple clients have independent caches ──────────────────────

func TestTokenManager_GetToken_IndependentCachePerClient(t *testing.T) {
	tm := newTokenManagerWithTransport(t, func(req *http.Request) (*http.Response, error) {
		var payload Auth0TokenRequest
		_ = json.NewDecoder(req.Body).Decode(&payload)
		resp, _ := json.Marshal(Auth0TokenResponse{
			AccessToken: "token-for-" + payload.ClientID,
			ExpiresIn:   86400,
		})
		return auth0Response(http.StatusOK, string(resp)), nil
	})

	tokenA, err := tm.GetToken(context.Background(), testCfg("client-x"))
	require.NoError(t, err)
	assert.Equal(t, "token-for-client-x", tokenA)

	tokenB, err := tm.GetToken(context.Background(), testCfg("client-y"))
	require.NoError(t, err)
	assert.Equal(t, "token-for-client-y", tokenB)

	// Second call for client-x should use cache
	tokenA2, err := tm.GetToken(context.Background(), testCfg("client-x"))
	require.NoError(t, err)
	assert.Equal(t, "token-for-client-x", tokenA2)
}

// ─── GetToken: Auth0 returns non-200 ─────────────────────────────────────────

func TestTokenManager_GetToken_Auth0NonOKStatus(t *testing.T) {
	tm := newTokenManagerWithTransport(t, func(*http.Request) (*http.Response, error) {
		return auth0Response(http.StatusUnauthorized, `{"error":"unauthorized"}`), nil
	})

	_, err := tm.GetToken(context.Background(), testCfg("client-d"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch token")
}

// ─── GetToken: Auth0 returns empty access_token ───────────────────────────────

func TestTokenManager_GetToken_EmptyAccessToken(t *testing.T) {
	tokenResp, _ := json.Marshal(Auth0TokenResponse{
		AccessToken: "", // empty
		ExpiresIn:   86400,
	})

	tm := newTokenManagerWithTransport(t, func(*http.Request) (*http.Response, error) {
		return auth0Response(http.StatusOK, string(tokenResp)), nil
	})

	_, err := tm.GetToken(context.Background(), testCfg("client-e"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty access_token")
}

// ─── GetToken: Auth0 returns invalid JSON ─────────────────────────────────────

func TestTokenManager_GetToken_InvalidJSON(t *testing.T) {
	tm := newTokenManagerWithTransport(t, func(*http.Request) (*http.Response, error) {
		return auth0Response(http.StatusOK, `{not valid json`), nil
	})

	_, err := tm.GetToken(context.Background(), testCfg("client-f"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode auth0 response")
}

// ─── GetToken: Auth0TokenRequest payload fields ───────────────────────────────

func TestTokenManager_GetToken_SendsCorrectPayload(t *testing.T) {
	var capturedPayload Auth0TokenRequest

	tokenResp, _ := json.Marshal(Auth0TokenResponse{
		AccessToken: "ok-token",
		ExpiresIn:   3600,
	})

	tm := newTokenManagerWithTransport(t, func(req *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(req.Body).Decode(&capturedPayload)
		return auth0Response(http.StatusOK, string(tokenResp)), nil
	})

	cfg := &XFXClientConfig{
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		BaseURL:      "https://api.xfx.io",
	}
	_, err := tm.GetToken(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, "my-client-id", capturedPayload.ClientID)
	assert.Equal(t, "my-client-secret", capturedPayload.ClientSecret)
	assert.Equal(t, auth0Audience, capturedPayload.Audience)
	assert.Equal(t, "client_credentials", capturedPayload.GrantType)
}
