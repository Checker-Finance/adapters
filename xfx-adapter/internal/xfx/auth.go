package xfx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// auth0TokenEndpoint is the Auth0 token URL for XFX.
	auth0TokenEndpoint = "https://dev-er8o7vv4aka08m70.us.auth0.com/oauth/token"
	// auth0Audience is the audience for XFX API tokens.
	auth0Audience = "https://api.xfx.io/trading"
	// tokenExpiryBuffer is the margin before actual expiry at which we pre-fetch a new token.
	tokenExpiryBuffer = 5 * time.Minute
)

// tokenEntry caches a bearer token with its expiry time.
type tokenEntry struct {
	accessToken string
	expiresAt   time.Time
}

// TokenManager fetches and caches OAuth2 client credentials tokens per client.
// Each client ID maps to a separate cached token derived from its own client_id/client_secret.
type TokenManager struct {
	logger  *zap.Logger
	client  *http.Client
	mu      sync.Mutex
	cache   map[string]tokenEntry // clientID → tokenEntry
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(logger *zap.Logger) *TokenManager {
	return &TokenManager{
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
		cache:  make(map[string]tokenEntry),
	}
}

// GetToken returns a valid bearer token for the given client config.
// Returns cached token if still valid; otherwise fetches a new one from Auth0.
func (m *TokenManager) GetToken(ctx context.Context, cfg *XFXClientConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.cache[cfg.ClientID]; ok {
		if time.Now().Before(entry.expiresAt.Add(-tokenExpiryBuffer)) {
			return entry.accessToken, nil
		}
	}

	// Fetch new token from Auth0
	token, err := m.fetchToken(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("xfx auth: fetch token for client %q: %w", cfg.ClientID, err)
	}

	m.cache[cfg.ClientID] = tokenEntry{
		accessToken: token.AccessToken,
		expiresAt:   time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}

	m.logger.Info("xfx.auth.token_refreshed",
		zap.String("client_id", cfg.ClientID),
		zap.Int64("expires_in_sec", token.ExpiresIn))

	return token.AccessToken, nil
}

// fetchToken requests a new access token from Auth0.
func (m *TokenManager) fetchToken(ctx context.Context, cfg *XFXClientConfig) (*Auth0TokenResponse, error) {
	payload := Auth0TokenRequest{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Audience:     auth0Audience,
		GrantType:    "client_credentials",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, auth0TokenEndpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth0 returned %d", resp.StatusCode)
	}

	var tokenResp Auth0TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode auth0 response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("auth0 returned empty access_token")
	}

	return &tokenResp, nil
}
