package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/secrets"
	"go.uber.org/zap"
)

// Manager orchestrates multi-tenant credential lookup and adapter-specific auth.
type Manager struct {
	logger    *zap.Logger
	secrets   secrets.Provider
	cache     *CacheAdapter
	brazaAuth *BrazaManager
}

// NewManager constructs the multi-tenant auth manager.
func NewManager(secretsProv secrets.Provider, cache *CacheAdapter, logger *zap.Logger, brazaBaseURL string) *Manager {
	return &Manager{
		logger:    logger,
		secrets:   secretsProv,
		cache:     cache,
		brazaAuth: NewBrazaManager(logger, brazaBaseURL),
	}
}

// GetCredentials resolves the username/password for a given tenant/client/venue.
func (m *Manager) GetCredentials(ctx context.Context, cfg config.Config, clientID, venue string) (Credentials, error) {
	key := fmt.Sprintf("%s/%s/%s", cfg.Env, clientID, venue)
	credsMap, err := m.secrets.GetSecret(ctx, key)
	if err != nil {
		m.logger.Warn("failed to fetch credentials", zap.Error(err), zap.String("key", key))
		return Credentials{}, err
	}
	return Credentials{
		Username: credsMap["username"],
		Password: credsMap["password"],
	}, nil
}

// GetValidToken uses cached or refreshed tokens via the venue-specific manager.
func (m *Manager) GetValidToken(ctx context.Context, clientID string, creds Credentials) (string, error) {
	return m.brazaAuth.GetValidToken(ctx, m.cache, clientID, creds)
}

// RefreshAllTokens (optional) periodically refreshes all cached tokens.
func (m *Manager) RefreshAllTokens(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.logger.Info("periodic token refresh tick")
			// iterate over cached keys, refresh if expiring
		case <-ctx.Done():
			return
		}
	}
}
