package zodia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ─── WebSocket test server helpers ───────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// combinedTestServer handles both REST /ws/auth and WebSocket /ws on a single server,
// which lets WSURL(cfg.BaseURL) point at the correct endpoint without extra configuration.
type combinedTestServer struct {
	srv  *httptest.Server
	wsURL string
	rest *RESTClient
}

func makeCombinedServer(t *testing.T, token string, wsHandler func(*websocket.Conn)) *combinedTestServer {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/ws/auth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ZodiaWSAuthResponse{Token: token}) //nolint:errcheck
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck
		wsHandler(conn)
	})

	srv := httptest.NewServer(mux)
	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/ws"
	signer := NewHMACSigner()
	rc := NewRESTClient(zap.NewNop(), nil, signer)

	return &combinedTestServer{srv: srv, wsURL: wsURL, rest: rc}
}

// authSuccessHandler is a WS handler that sends auth_success after receiving the auth message.
func authSuccessHandler(conn *websocket.Conn) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var auth WSAuthMessage
	if err := json.Unmarshal(msg, &auth); err != nil {
		return
	}
	if auth.Action == "auth" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"auth_success"}`))
	}
	time.Sleep(500 * time.Millisecond) // keep alive for readLoop
}

// ─── Session.Connect ─────────────────────────────────────────────────────────

func TestSession_Connect_AuthSuccess(t *testing.T) {
	cs := makeCombinedServer(t, "good-token", authSuccessHandler)
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	cfg := &ZodiaClientConfig{
		APIKey:    "k",
		APISecret: "s",
		BaseURL:   cs.srv.URL, // WSURL(BaseURL) → ws://host/ws
	}
	sess := NewSession(
		SessionConfig{ClientID: "c1", ZodiaCfg: cfg, MaxRetries: 0, RetryDelay: 10 * time.Millisecond},
		NewWSClient(zap.NewNop()),
		tokenMgr,
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, sess.Connect(ctx))
	assert.True(t, sess.connected.Load())
	sess.Close()
}

func TestSession_Connect_AuthFailure(t *testing.T) {
	cs := makeCombinedServer(t, "bad-token", func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","message":"unauthorized"}`))
		time.Sleep(100 * time.Millisecond)
	})
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	sess := NewSession(
		SessionConfig{ClientID: "c-bad", ZodiaCfg: cfg, MaxRetries: 0, RetryDelay: 10 * time.Millisecond},
		NewWSClient(zap.NewNop()),
		tokenMgr,
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := sess.Connect(ctx)
	assert.Error(t, err, "auth error should cause Connect to fail")
}

// ─── Session.RequestPrice ─────────────────────────────────────────────────────

func TestSession_RequestPrice_Success(t *testing.T) {
	cs := makeCombinedServer(t, "tok", func(conn *websocket.Conn) {
		// Auth handshake
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var auth WSAuthMessage
		if err := json.Unmarshal(msg, &auth); err != nil || auth.Action != "auth" {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"auth_success"}`))

		// Wait for subscribe_price request
		_, msg, err = conn.ReadMessage()
		if err != nil {
			return
		}
		var sub WSSubscribePriceMessage
		if err := json.Unmarshal(msg, &sub); err != nil || sub.Action != "subscribe_price" {
			return
		}

		// Send back price_update with matching clientRef
		resp := WSPricePayload{
			Action:     "price_update",
			ClientRef:  sub.ClientRef,
			Instrument: sub.Instrument,
			Side:       sub.Side,
			Quantity:   sub.Quantity,
			Bid:        17.10,
			Ask:        17.20,
			QuoteID:    "zodia-q-1",
			ExpiresAt:  time.Now().Add(15 * time.Second).Unix(),
		}
		data, _ := json.Marshal(resp)
		_ = conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(500 * time.Millisecond)
	})
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	sess := NewSession(
		SessionConfig{ClientID: "c2", ZodiaCfg: cfg, MaxRetries: 0, RetryDelay: 0},
		NewWSClient(zap.NewNop()),
		tokenMgr,
		zap.NewNop(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sess.Connect(ctx))

	price, err := sess.RequestPrice(ctx, "USD.MXN", "BUY", 100_000)
	require.NoError(t, err)
	require.NotNil(t, price)
	assert.Equal(t, "zodia-q-1", price.QuoteID)
	assert.Equal(t, 17.20, price.Ask)
}

func TestSession_RequestPrice_NotConnected(t *testing.T) {
	sess := &Session{
		cfg:    SessionConfig{ClientID: "not-connected"},
		logger: zap.NewNop(),
	}
	// connected = false (zero value)
	_, err := sess.RequestPrice(context.Background(), "USD.MXN", "BUY", 100_000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestSession_ExecuteOrder_NotConnected(t *testing.T) {
	sess := &Session{
		cfg:    SessionConfig{ClientID: "not-connected"},
		logger: zap.NewNop(),
	}
	_, err := sess.ExecuteOrder(context.Background(), "quote-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// ─── Session.Close ────────────────────────────────────────────────────────────

func TestSession_Close_Idempotent(t *testing.T) {
	sess := &Session{cfg: SessionConfig{ClientID: "c"}, logger: zap.NewNop()}
	assert.NotPanics(t, func() {
		sess.Close()
		sess.Close()
	})
}

// ─── Session.dispatch ─────────────────────────────────────────────────────────

func TestSession_dispatch_PriceUpdate(t *testing.T) {
	sess := &Session{cfg: SessionConfig{ClientID: "c"}, logger: zap.NewNop()}
	ch := make(chan priceResult, 1)
	sess.pendingPrices.Store("ref-abc", ch)

	raw, _ := json.Marshal(WSPricePayload{
		Action:    "price_update",
		ClientRef: "ref-abc",
		QuoteID:   "q1",
		Ask:       17.25,
	})
	sess.dispatch(raw)

	select {
	case result := <-ch:
		assert.NoError(t, result.err)
		assert.Equal(t, "q1", result.payload.QuoteID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for price dispatch")
	}
}

func TestSession_dispatch_OrderConfirmation(t *testing.T) {
	sess := &Session{cfg: SessionConfig{ClientID: "c"}, logger: zap.NewNop()}
	ch := make(chan orderResult, 1)
	sess.pendingOrders.Store("ref-exec", ch)

	raw, _ := json.Marshal(WSOrderConfirmPayload{
		Action:    "order_confirmation",
		ClientRef: "ref-exec",
		TradeID:   "trade-xyz",
		Status:    "PROCESSED",
	})
	sess.dispatch(raw)

	select {
	case result := <-ch:
		assert.NoError(t, result.err)
		assert.Equal(t, "trade-xyz", result.payload.TradeID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order dispatch")
	}
}

func TestSession_dispatch_ErrorWithClientRef(t *testing.T) {
	sess := &Session{cfg: SessionConfig{ClientID: "c"}, logger: zap.NewNop()}
	ch := make(chan priceResult, 1)
	sess.pendingPrices.Store("ref-err", ch)

	raw, _ := json.Marshal(WSErrorPayload{
		Action:    "error",
		ClientRef: "ref-err",
		Message:   "instrument not found",
	})
	sess.dispatch(raw)

	select {
	case result := <-ch:
		assert.Error(t, result.err)
		assert.Contains(t, result.err.Error(), "instrument not found")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error dispatch")
	}
}

func TestSession_dispatch_UnknownAction(t *testing.T) {
	sess := &Session{cfg: SessionConfig{ClientID: "c"}, logger: zap.NewNop()}
	// Should not panic
	assert.NotPanics(t, func() {
		raw, _ := json.Marshal(WSMessage{Action: "heartbeat"})
		sess.dispatch(raw)
	})
}

// ─── SessionManager ──────────────────────────────────────────────────────────

func TestSessionManager_GetOrCreate_SameInstance(t *testing.T) {
	cs := makeCombinedServer(t, "tok", authSuccessHandler)
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	mgr := NewSessionManager(zap.NewNop(), NewWSClient(zap.NewNop()), tokenMgr, 0)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess1, err1 := mgr.GetOrCreate(ctx, "client-X", cfg)
	require.NoError(t, err1)

	sess2, err2 := mgr.GetOrCreate(ctx, "client-X", cfg)
	require.NoError(t, err2)

	assert.Same(t, sess1, sess2, "same clientID must return the same Session")
}

func TestSessionManager_Close(t *testing.T) {
	cs := makeCombinedServer(t, "tok", authSuccessHandler)
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	mgr := NewSessionManager(zap.NewNop(), NewWSClient(zap.NewNop()), tokenMgr, 0)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := mgr.GetOrCreate(ctx, "client-Z", cfg)
	require.NoError(t, err)

	mgr.Close("client-Z")

	mgr.mu.Lock()
	_, ok := mgr.sessions["client-Z"]
	mgr.mu.Unlock()
	assert.False(t, ok, "session should be removed after Close")
}

func TestSessionManager_CloseAll(t *testing.T) {
	cs := makeCombinedServer(t, "tok", func(conn *websocket.Conn) {
		// handle two concurrent connections
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"auth_success"}`))
		time.Sleep(500 * time.Millisecond)
	})
	defer cs.srv.Close()

	tokenMgr := NewWSTokenManager(zap.NewNop(), cs.rest)
	mgr := NewSessionManager(zap.NewNop(), NewWSClient(zap.NewNop()), tokenMgr, 0)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = mgr.GetOrCreate(ctx, "c1", cfg)
	_, _ = mgr.GetOrCreate(ctx, "c2", cfg)

	mgr.CloseAll()

	mgr.mu.Lock()
	count := len(mgr.sessions)
	mgr.mu.Unlock()
	assert.Equal(t, 0, count, "all sessions should be removed after CloseAll")
}
