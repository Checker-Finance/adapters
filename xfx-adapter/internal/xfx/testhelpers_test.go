package xfx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
)

// writeJSON encodes v as JSON into w.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("test helper writeJSON: " + err.Error())
	}
}

// mockConfigResolver implements ConfigResolver for tests.
type mockConfigResolver struct {
	cfg     *XFXClientConfig
	err     error
	clients []string
}

func (m *mockConfigResolver) Resolve(_ context.Context, _ string) (*XFXClientConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

func (m *mockConfigResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return m.clients, m.err
}

// seedToken pre-populates the token cache so tests don't hit Auth0.
func seedToken(tm *TokenManager, clientID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cache[clientID] = tokenEntry{
		accessToken: "test-bearer-token",
		expiresAt:   time.Now().Add(24 * time.Hour),
	}
}

// isPolling reports whether the poller has an active goroutine for txID.
func isPolling(p *Poller, txID string) bool {
	_, ok := p.activeTrades.Load(txID)
	return ok
}

// newMockXFXServer returns an httptest.Server that routes XFX API calls to canned responses.
// Pass nil for a response to trigger a 4xx error on that endpoint.
func newMockXFXServer(
	t *testing.T,
	quoteResp *XFXQuoteResponse,
	execResp *XFXExecuteResponse,
	txResp *XFXTransactionResponse,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodPost && path == "/v1/customer/quotes":
			if quoteResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, XFXErrorResponse{Message: "invalid quote request"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, quoteResp)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/execute"):
			if execResp == nil {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, XFXErrorResponse{Message: "quote not found or expired"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, execResp)

		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/customer/transactions/"):
			if txResp == nil {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, XFXErrorResponse{Message: "transaction not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, txResp)

		case r.Method == http.MethodGet && strings.HasPrefix(path, "/v1/customer/quotes/"):
			if quoteResp == nil {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, XFXErrorResponse{Message: "quote not found"})
				return
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, quoteResp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newTestService returns a Service wired to the given mock server URL.
// The token manager is pre-seeded so no Auth0 calls occur.
func newTestService(t *testing.T, serverURL string) *Service {
	t.Helper()
	logger := zap.NewNop()

	tokens := NewTokenManager(logger)
	seedToken(tokens, "test-client-id")

	client := NewClient(logger, nil, tokens)

	resolver := &mockConfigResolver{
		cfg: &XFXClientConfig{
			BaseURL:      serverURL,
			ClientID:     "test-client-id",
			ClientSecret: "test-secret",
		},
		clients: []string{"test-client-id"},
	}

	return &Service{
		ctx:            context.Background(),
		cfg:            config.Config{},
		logger:         logger,
		client:         client,
		configResolver: resolver,
		publisher:      nil,
		mapper:         NewMapper(),
	}
}

// newTestPoller creates a Poller wired to the given service with a short poll interval.
func newTestPoller(t *testing.T, svc *Service, interval time.Duration) *Poller {
	t.Helper()
	return NewPoller(
		zap.NewNop(),
		config.Config{},
		svc,
		nil, // publisher
		nil, // store
		interval,
		nil, // tradeSync
	)
}

// txAt builds an XFXTransaction with the given status and createdAt timestamp.
func txAt(id, quoteID, status, createdAt string) XFXTransaction {
	return XFXTransaction{
		ID:        id,
		QuoteID:   quoteID,
		Symbol:    "USD/MXN",
		Side:      "buy",
		Quantity:  100000.0,
		Price:     17.5,
		Status:    status,
		CreatedAt: createdAt,
	}
}
