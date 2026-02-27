package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newExec(retryMax int, client *http.Client) *Executor {
	return New(zap.NewNop(), nil, client, retryMax, "test", nil)
}

// countingHandler returns a handler whose response alternates based on a call counter.
// For calls <= failCount it returns failStatus; afterwards it returns 200 with body.
func countingHandler(failCount int, failStatus int, successBody []byte) (http.Handler, *atomic.Int32) {
	var n atomic.Int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if int(n.Add(1)) <= failCount {
			w.WriteHeader(failStatus)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(successBody)
	}), &n
}

// ─── Basic success ────────────────────────────────────────────────────────────

func TestDoJSON_SuccessFirstAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
	}))
	defer srv.Close()

	exec := newExec(2, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var out map[string]string
	require.NoError(t, exec.DoJSON(context.Background(), req, "k", &out))
	assert.Equal(t, "ok", out["result"])
}

// ─── 5xx retry then success ───────────────────────────────────────────────────

func TestDoJSON_Retries5xxThenSucceeds(t *testing.T) {
	h, count := countingHandler(1, http.StatusServiceUnavailable, []byte(`{"result":"ok"}`))
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := newExec(2, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var out map[string]string
	require.NoError(t, exec.DoJSON(context.Background(), req, "k", &out))
	assert.EqualValues(t, 2, count.Load(), "expected exactly 2 attempts")
	assert.Equal(t, "ok", out["result"])
}

// ─── POST body is re-sent on retry ───────────────────────────────────────────

func TestDoJSON_PostBodyResentOnRetry(t *testing.T) {
	var received []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = append(received, string(b))
		if len(received) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	exec := newExec(1, srv.Client())

	bodyBytes, _ := json.Marshal(map[string]string{"value": "hello"})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	require.NoError(t, exec.DoJSON(context.Background(), req, "k", nil))
	require.Len(t, received, 2, "expected two attempts")
	assert.JSONEq(t, `{"value":"hello"}`, received[0], "first attempt body")
	assert.JSONEq(t, `{"value":"hello"}`, received[1], "retry must re-send the full body")
}

// ─── 4xx: no retry ────────────────────────────────────────────────────────────

func TestDoJSON_4xxNotRetried(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	exec := newExec(2, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	require.Error(t, exec.DoJSON(context.Background(), req, "k", nil))
	assert.EqualValues(t, 1, count.Load(), "4xx must not be retried")
}

// ─── All retries exhausted ────────────────────────────────────────────────────

func TestDoJSON_ExhaustAllRetries(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	exec := newExec(2, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	err := exec.DoJSON(context.Background(), req, "k", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 2 attempts")
	assert.EqualValues(t, 3, count.Load(), "retryMax=2 means 3 total attempts")
}

// ─── retryMax=0: single attempt only ─────────────────────────────────────────

func TestDoJSON_ZeroRetries(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	exec := newExec(0, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	require.Error(t, exec.DoJSON(context.Background(), req, "k", nil))
	assert.EqualValues(t, 1, count.Load(), "retryMax=0 means exactly one attempt")
}

// ─── Custom error handler receives body ──────────────────────────────────────

func TestDoJSON_CustomErrorHandlerCalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"code":"INVALID"}`))
	}))
	defer srv.Close()

	exec := New(zap.NewNop(), nil, srv.Client(), 2, "test", func(status int, body []byte) error {
		return fmt.Errorf("venue %d: %s", status, body)
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, nil)

	err := exec.DoJSON(context.Background(), req, "k", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "422")
	assert.Contains(t, err.Error(), "INVALID")
}

// ─── JSON decode error ────────────────────────────────────────────────────────

func TestDoJSON_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	exec := newExec(0, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var out map[string]string
	err := exec.DoJSON(context.Background(), req, "k", &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode failed")
}

// ─── Two 5xx then success (two retries exercised) ────────────────────────────

func TestDoJSON_TwoFailuresThenSuccess(t *testing.T) {
	h, count := countingHandler(2, http.StatusBadGateway, []byte(`{"v":1}`))
	srv := httptest.NewServer(h)
	defer srv.Close()

	exec := newExec(2, srv.Client())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var out map[string]int
	require.NoError(t, exec.DoJSON(context.Background(), req, "k", &out))
	assert.EqualValues(t, 3, count.Load(), "expected 3 total attempts")
	assert.Equal(t, 1, out["v"])
}
