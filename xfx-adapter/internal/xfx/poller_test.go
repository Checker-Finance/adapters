package xfx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusSequenceServer returns a mock server where each GET /v1/customer/transactions/{id}
// call returns the next status in the provided sequence.
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
			createdAt := time.Now().UTC().Format(time.RFC3339)
			resp := XFXTransactionResponse{
				Success: true,
				Transaction: XFXTransaction{
					ID:        "tx-seq-001",
					QuoteID:   "qt-seq-001",
					Symbol:    "USD/MXN",
					Side:      "buy",
					Quantity:  100000.0,
					Price:     17.5,
					Status:    statuses[idx],
					CreatedAt: createdAt,
				},
			}
			w.WriteHeader(http.StatusOK)
			writeJSON(w, resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// txErrorServer returns a mock server that always returns 500 for transaction status.
func txErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "internal error"})
	}))
}

// ─── PollTradeStatus: reaches terminal ───────────────────────────────────────

func TestPoller_PollTradeStatus_ReachesTerminal(t *testing.T) {
	// PENDING → PENDING → SETTLED (terminal)
	server := statusSequenceServer(t, []string{"PENDING", "PENDING", "SETTLED"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	poller.PollTradeStatus(context.Background(), "test-client-id", "qt-seq-001", "tx-seq-001")

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-seq-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"poller should stop after reaching SETTLED (terminal) status")

	poller.Stop()
}

func TestPoller_PollTradeStatus_FailedStatus_IsTerminal(t *testing.T) {
	server := statusSequenceServer(t, []string{"PENDING", "FAILED"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	poller.PollTradeStatus(context.Background(), "test-client-id", "qt-seq-001", "tx-seq-001")

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-seq-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"poller should stop after FAILED status")

	poller.Stop()
}

func TestPoller_PollTradeStatus_CancelledStatus_IsTerminal(t *testing.T) {
	server := statusSequenceServer(t, []string{"CANCELLED"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	poller.PollTradeStatus(context.Background(), "test-client-id", "qt-seq-001", "tx-seq-001")

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-seq-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"poller should stop after CANCELLED status")

	poller.Stop()
}

// ─── DuplicatePrevention ─────────────────────────────────────────────────────

func TestPoller_DuplicatePrevention(t *testing.T) {
	// Server always returns non-terminal so poller stays active
	server := statusSequenceServer(t, []string{"PENDING"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()

	// Start first poller
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-dup-001")
	time.Sleep(5 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-dup-001"), "first poll should be active")

	// Attempt duplicate — should be silently ignored
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-dup-001")
	time.Sleep(5 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-dup-001"), "poller should still be active")

	poller.Stop()
}

// ─── Context cancellation stops polling ──────────────────────────────────────

func TestPoller_ContextCancellation(t *testing.T) {
	server := statusSequenceServer(t, []string{"PENDING"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-ctx-001")

	time.Sleep(10 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-ctx-001"))

	cancel()

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-ctx-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"poller should stop after parent context is cancelled")

	poller.Stop()
}

// ─── HTTP errors continue polling (retry behavior) ───────────────────────────

func TestPoller_ErrorContinuesPolling(t *testing.T) {
	// 500 errors are retried by the executor → slows things down, but eventually
	// the poller continues to the next tick. We use a separate error server that
	// only ever returns 500 to verify the poller stays alive despite errors.
	server := txErrorServer(t)
	defer server.Close()

	svc := newTestService(t, server.URL)
	// Use a longer interval so retries don't race with the ticker
	poller := newTestPoller(t, svc, 100*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-err-001")

	// Give it time to attempt polling — poller should still be active
	time.Sleep(50 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-err-001"),
		"poller should continue despite HTTP errors")

	poller.Stop()
}

// ─── Stop: graceful shutdown of all active goroutines ────────────────────────

func TestPoller_Stop_GracefulShutdown(t *testing.T) {
	server := statusSequenceServer(t, []string{"PENDING"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()

	// Start two independent pollers
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-stop-001")
	poller.PollTradeStatus(ctx, "test-client-id", "qt-002", "tx-stop-002")

	time.Sleep(10 * time.Millisecond)
	assert.True(t, isPolling(poller, "tx-stop-001"))
	assert.True(t, isPolling(poller, "tx-stop-002"))

	poller.Stop()

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-stop-001") && !isPolling(poller, "tx-stop-002")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"all goroutines should stop after Stop()")
}

// ─── Status change detection ──────────────────────────────────────────────────

func TestPoller_StatusChangeDetection(t *testing.T) {
	// PENDING → EXECUTED → SETTLED: poller should detect each change and reach terminal
	server := statusSequenceServer(t, []string{"PENDING", "EXECUTED", "SETTLED"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	ctx := context.Background()
	poller.PollTradeStatus(ctx, "test-client-id", "qt-001", "tx-seq-001")

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-seq-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"poller should stop after SETTLED terminal status through status progression")

	poller.Stop()
}

// ─── Active trade cleanup ─────────────────────────────────────────────────────

func TestPoller_ActiveTradeRemovedAfterTerminal(t *testing.T) {
	server := statusSequenceServer(t, []string{"SETTLED"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 5*time.Millisecond)

	poller.PollTradeStatus(context.Background(), "test-client-id", "qt-001", "tx-cleanup-001")

	require.Eventually(t, func() bool {
		return !isPolling(poller, "tx-cleanup-001")
	}, 300*time.Millisecond, 5*time.Millisecond,
		"activeTrades entry should be removed after terminal status")

	poller.Stop()
}

// ─── Concurrent polling of multiple distinct trades ──────────────────────────

func TestPoller_ConcurrentIndependentTrades(t *testing.T) {
	server := statusSequenceServer(t, []string{"PENDING"})
	defer server.Close()

	svc := newTestService(t, server.URL)
	poller := newTestPoller(t, svc, 50*time.Millisecond)

	ctx := context.Background()

	// Start 3 different trades
	for i := range 3 {
		txID := "tx-concurrent-00" + string(rune('1'+i))
		poller.PollTradeStatus(ctx, "test-client-id", "qt-001", txID)
	}

	time.Sleep(10 * time.Millisecond)

	assert.True(t, isPolling(poller, "tx-concurrent-001"))
	assert.True(t, isPolling(poller, "tx-concurrent-002"))
	assert.True(t, isPolling(poller, "tx-concurrent-003"))

	poller.Stop()
}
