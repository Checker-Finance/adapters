package zodia

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// wsTokenDefaultTTL is the assumed lifetime for a Zodia WS auth token.
	// ⚠️ Verify against sandbox — may differ from 24h.
	wsTokenDefaultTTL = 24 * time.Hour

	// wsTokenExpiryBuffer is the margin before expiry at which we pre-fetch a new token.
	wsTokenExpiryBuffer = 30 * time.Minute
)

//
// ────────────────────────────────────────────────
//   HMACSigner — REST request signing
// ────────────────────────────────────────────────
//

// HMACSigner computes HMAC-SHA512 signatures for Zodia REST API requests.
// Every signed request must include:
//   - Header "Rest-Key": api_key
//   - Header "Rest-Sign": hex(HMAC-SHA512(requestBodyBytes, apiSecret))
//   - Body field "tonce": time.Now().UnixMicro()
//
// ⚠️ The exact HMAC formula (body-only vs path+body) needs verification in sandbox.
type HMACSigner struct{}

// NewHMACSigner returns a new HMACSigner.
func NewHMACSigner() *HMACSigner { return &HMACSigner{} }

// Sign computes HMAC-SHA512 of body using apiSecret and returns the hex-encoded result.
func (s *HMACSigner) Sign(body []byte, apiSecret string) string {
	mac := hmac.New(sha512.New, []byte(apiSecret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Tonce returns the current time as Unix microseconds. Used as a nonce in request bodies.
func Tonce() int64 {
	return time.Now().UnixMicro()
}

//
// ────────────────────────────────────────────────
//   WSTokenManager — WebSocket authentication
// ────────────────────────────────────────────────
//

// wsTokenEntry caches a WS bearer token with its expiry time.
type wsTokenEntry struct {
	token     string
	expiresAt time.Time
}

// WSTokenManager fetches and caches Zodia WebSocket auth tokens per client.
// Tokens are obtained by calling POST /ws/auth (HMAC-signed REST endpoint).
// Each client's token is keyed by their api_key.
type WSTokenManager struct {
	logger *zap.Logger
	rest   *RESTClient
	mu     sync.Mutex
	cache  map[string]wsTokenEntry // apiKey → tokenEntry
}

// NewWSTokenManager creates a new WSTokenManager.
func NewWSTokenManager(logger *zap.Logger, rest *RESTClient) *WSTokenManager {
	return &WSTokenManager{
		logger: logger,
		rest:   rest,
		cache:  make(map[string]wsTokenEntry),
	}
}

// GetToken returns a valid WS auth token for the given client config.
// Returns a cached token if still valid; otherwise fetches a new one via POST /ws/auth.
func (m *WSTokenManager) GetToken(ctx context.Context, cfg *ZodiaClientConfig) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := cfg.APIKey
	if entry, ok := m.cache[key]; ok {
		if time.Now().Before(entry.expiresAt.Add(-wsTokenExpiryBuffer)) {
			return entry.token, nil
		}
	}

	token, err := m.rest.GetWSAuthToken(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("zodia: get ws auth token: %w", err)
	}

	m.cache[key] = wsTokenEntry{
		token:     token,
		expiresAt: time.Now().Add(wsTokenDefaultTTL),
	}

	masked := key
	if len(masked) > 8 {
		masked = masked[:8] + "..."
	}
	m.logger.Info("zodia.ws_token_refreshed", zap.String("api_key_prefix", masked))

	return token, nil
}

// InvalidateToken removes the cached WS token for the given api_key,
// forcing a re-fetch on the next GetToken call.
func (m *WSTokenManager) InvalidateToken(apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cache, apiKey)
	m.logger.Debug("zodia.ws_token_invalidated")
}
