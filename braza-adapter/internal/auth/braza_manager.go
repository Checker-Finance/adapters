package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// BrazaManager handles JWT authentication and refresh lifecycle for the Braza API.
// It is called by the multi-tenant auth.Manager for a specific tenant/client/venue.
type BrazaManager struct {
	logger   *zap.Logger
	baseURL  string
	client   *http.Client
}

// NewBrazaManager creates a new Braza-specific auth manager for the given base URL.
func NewBrazaManager(logger *zap.Logger, baseURL string) *BrazaManager {
	return &BrazaManager{
		logger:  logger,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// GetValidToken retrieves a valid access token (from cache, refresh, or login).
func (m *BrazaManager) GetValidToken(
	ctx context.Context,
	cache *CacheAdapter,
	clientID string,
	creds Credentials,
) (string, error) {
	key := fmt.Sprintf("braza:token:%s:", clientID)

	// 1. Attempt to reuse cached token if valid
	val, _ := cache.Get(ctx, key)
	if val != "" {
		var tb TokenBundle
		if err := json.Unmarshal([]byte(val), &tb); err == nil {
			if time.Now().Unix() < tb.Exp-300 {
				// Still valid for >5min
				return tb.AccessToken, nil
			}
			// Try refresh before expiring
			if tb.RefreshToken != "" {
				if newTok, err := m.refresh(ctx, tb.RefreshToken); err == nil {
					m.saveToken(ctx, cache, key, newTok)
					return newTok.AccessToken, nil
				}
				m.logger.Warn("braza.refresh_failed", zap.Error(err))
			}
		}
	}

	// 2. Fallback to login
	newTok, err := m.login(ctx, creds)
	if err != nil {
		m.logger.Error("braza.login_failed", zap.Error(err))
		return "", err
	}
	m.saveToken(ctx, cache, key, newTok)
	return newTok.AccessToken, nil
}

// login authenticates with Braza /auth/ to obtain new tokens.
func (m *BrazaManager) login(ctx context.Context, creds Credentials) (TokenBundle, error) {
	url := fmt.Sprintf("%s/auth/", m.baseURL)
	body := map[string]string{"username": creds.Username, "password": creds.Password}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return TokenBundle{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return TokenBundle{}, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return TokenBundle{}, fmt.Errorf("braza login failed: %d", resp.StatusCode)
	}

	var tr TokenBundle
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return TokenBundle{}, err
	}

	// If no expiry provided, assume 1 hour validity
	if tr.Exp == 0 {
		tr.Exp = time.Now().Add(time.Hour).Unix()
	}

	m.logger.Info("braza.login_success", zap.String("user", creds.Username))
	return tr, nil
}

// refresh exchanges a refresh_token for a new access_token.
func (m *BrazaManager) refresh(ctx context.Context, refreshToken string) (TokenBundle, error) {
	url := fmt.Sprintf("%s/auth/refresh/", m.baseURL)
	body := map[string]string{"refresh_token": refreshToken}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return TokenBundle{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return TokenBundle{}, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return TokenBundle{}, fmt.Errorf("braza refresh failed: %d", resp.StatusCode)
	}

	var tr TokenBundle
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return TokenBundle{}, err
	}

	if tr.Exp == 0 {
		tr.Exp = time.Now().Add(time.Hour).Unix()
	}

	m.logger.Info("braza.refresh_success")
	return tr, nil
}

// saveToken stores a token bundle into cache.
func (m *BrazaManager) saveToken(ctx context.Context, cache *CacheAdapter, key string, tb TokenBundle) {
	b, _ := json.Marshal(tb)
	_ = cache.SetWithTTL(ctx, key, string(b), time.Until(time.Unix(tb.Exp, 0)))
}
