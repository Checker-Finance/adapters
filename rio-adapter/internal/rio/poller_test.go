package rio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
)

// --- Test Helpers ---

// statusSequenceServer returns a mock server where each GET /api/orders/{id}
// call returns the next status in the given sequence.
// writeJSON is a test helper that encodes v as JSON into w.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("test helper writeJSON: " + err.Error())
	}
}

func statusSequenceServer(t *testing.T, statuses []string) *httptest.Server {
	t.Helper()
	var callCount int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet {
			idx := atomic.AddInt64(&callCount, 1) - 1
			if int(idx) >= len(statuses) {
				idx = int64(len(statuses) - 1)
			}

			resp := RioOrderResponse{
				ID:        "ord-test-001",
				QuoteID:   "qt-test-001",
				Status:    statuses[idx],
				Side:      "buy",
				Crypto:    "USDC",
				Fiat:      "BRL",
				NetPrice:  5.0,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

// errorServer returns a mock server that always returns an error.
func errorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "internal error"})
	}))
}

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

// --- Tests ---

func TestPoller_PollTradeStatus_ReachesTerminal(t *testing.T) {
	// Status progression: processing -> processing -> completed (terminal)
	server := statusSequenceServer(t, []string{"processing", "processing", "completed"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-test-001")

	// Wait for the poller to reach terminal status
	require.Eventually(t, func() bool {
		return !poller.IsPolling("ord-test-001")
	}, 200*time.Millisecond, 5*time.Millisecond,
		"poller should stop after reaching terminal status")

	poller.Stop()
}

func TestPoller_DuplicatePrevention(t *testing.T) {
	// Server always returns non-terminal so poller stays active
	server := statusSequenceServer(t, []string{"processing"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()

	// Start first poller
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-dup-001")
	time.Sleep(5 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-dup-001"))

	// Try to start duplicate — should be ignored
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-dup-001")
	time.Sleep(5 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-dup-001"))

	poller.Stop()

	// After stop, polling should eventually clear
	time.Sleep(20 * time.Millisecond)
}

func TestPoller_CancelPolling(t *testing.T) {
	server := statusSequenceServer(t, []string{"processing"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-cancel-001")

	time.Sleep(10 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-cancel-001"))

	// Cancel via webhook
	poller.CancelPolling("ord-cancel-001")

	// Should stop polling
	require.Eventually(t, func() bool {
		return !poller.IsPolling("ord-cancel-001")
	}, 100*time.Millisecond, 5*time.Millisecond,
		"poller should stop after CancelPolling")

	poller.Stop()
}

func TestPoller_ContextCancellation(t *testing.T) {
	server := statusSequenceServer(t, []string{"processing"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-ctx-001")

	time.Sleep(10 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-ctx-001"))

	// Cancel the parent context
	cancel()

	require.Eventually(t, func() bool {
		return !poller.IsPolling("ord-ctx-001")
	}, 200*time.Millisecond, 5*time.Millisecond,
		"poller should stop after context cancellation")

	poller.Stop()
}

func TestPoller_ErrorContinuesPolling(t *testing.T) {
	// Server returns errors — poller should continue retrying
	server := errorServer(t)
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-err-001")

	// Wait a bit — poller should still be active despite errors
	time.Sleep(30 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-err-001"),
		"poller should continue polling despite errors")

	poller.Stop()
	time.Sleep(20 * time.Millisecond)
}

func TestPoller_Stop_GracefulShutdown(t *testing.T) {
	server := statusSequenceServer(t, []string{"processing"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()

	// Start multiple pollers
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-stop-001")
	poller.PollTradeStatus(ctx, "client-002", "qt-002", "ord-stop-002")

	time.Sleep(10 * time.Millisecond)
	assert.True(t, poller.IsPolling("ord-stop-001"))
	assert.True(t, poller.IsPolling("ord-stop-002"))

	// Stop all
	poller.Stop()

	require.Eventually(t, func() bool {
		return !poller.IsPolling("ord-stop-001") && !poller.IsPolling("ord-stop-002")
	}, 200*time.Millisecond, 5*time.Millisecond,
		"all pollers should stop after Stop()")
}

func TestPoller_StatusChangeDetection(t *testing.T) {
	// Verify the poller detects status changes (processing -> submitted -> filled)
	server := statusSequenceServer(t, []string{"processing", "awaiting_payment", "completed"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "client-001", "qt-001", "ord-status-001")

	// Should eventually reach terminal and stop
	require.Eventually(t, func() bool {
		return !poller.IsPolling("ord-status-001")
	}, 200*time.Millisecond, 5*time.Millisecond,
		"poller should stop after terminal status through status progression")

	poller.Stop()
}

func TestPoller_IsPolling_Nonexistent(t *testing.T) {
	poller := &Poller{
		logger: zap.NewNop(),
		stopCh: make(chan struct{}),
	}
	assert.False(t, poller.IsPolling("nonexistent-order"))
}

func TestPoller_CancelPolling_Nonexistent(t *testing.T) {
	poller := &Poller{
		logger: zap.NewNop(),
		stopCh: make(chan struct{}),
	}
	// Should not panic
	poller.CancelPolling("nonexistent-order")
}
