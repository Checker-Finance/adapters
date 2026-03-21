package zodia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

type mockZodiaConfigResolver struct {
	cfg *ZodiaClientConfig
	err error
}

func (m *mockZodiaConfigResolver) Resolve(_ context.Context, _ string) (*ZodiaClientConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

func (m *mockZodiaConfigResolver) DiscoverClients(_ context.Context) ([]string, error) {
	return nil, nil
}

// newZodiaTestService builds a minimal Service for tests with the given dependencies.
func newZodiaTestService(
	t *testing.T,
	resolver ConfigResolver,
	restClient *RESTClient,
	sessionMgr *SessionManager,
) *Service {
	t.Helper()
	return &Service{
		ctx:            context.Background(),
		cfg:            config.Config{},
		restClient:     restClient,
		sessionMgr:     sessionMgr,
		configResolver: resolver,
		mapper:         NewMapper(),
	}
}

// wsAuthThenHandler wraps a WS handler with the standard auth handshake.
func wsAuthThenHandler(handler func(*websocket.Conn)) func(*websocket.Conn) {
	return func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var auth WSAuthMessage
		if err := json.Unmarshal(msg, &auth); err != nil || auth.Action != "auth" {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"error","message":"bad auth"}`))
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"action":"auth_success"}`))
		handler(conn)
	}
}

// makePriceServer creates a combined server that handles auth and returns a price_update.
func makePriceServer(t *testing.T, quoteID string, bid, ask float64) *combinedTestServer {
	t.Helper()
	return makeCombinedServer(t, "test-token", wsAuthThenHandler(func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var sub WSSubscribePriceMessage
		if err := json.Unmarshal(msg, &sub); err != nil {
			return
		}
		resp := WSPricePayload{
			Action:     "price_update",
			ClientRef:  sub.ClientRef,
			Instrument: sub.Instrument,
			Side:       sub.Side,
			Quantity:   sub.Quantity,
			Bid:        bid,
			Ask:        ask,
			QuoteID:    quoteID,
			ExpiresAt:  time.Now().Add(15 * time.Second).Unix(),
		}
		data, _ := json.Marshal(resp)
		_ = conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(300 * time.Millisecond) // keep the connection open for readLoop
	}))
}

// makeExecuteServer creates a combined server that handles auth + execute_order.
func makeExecuteServer(t *testing.T, tradeID, status string) *combinedTestServer {
	t.Helper()
	return makeCombinedServer(t, "test-token", wsAuthThenHandler(func(conn *websocket.Conn) {
		// Consume the subscribe_price (sent by CreateRFQ step — not used here, but ExecuteRFQ
		// doesn't call RequestPrice; we skip directly to the execute message).
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var raw map[string]any
		_ = json.Unmarshal(msg, &raw)
		action, _ := raw["action"].(string)

		if action == "execute_order" {
			var exec WSExecuteOrderMessage
			_ = json.Unmarshal(msg, &exec)
			resp := WSOrderConfirmPayload{
				Action:    "order_confirmation",
				ClientRef: exec.ClientRef,
				TradeID:   tradeID,
				Status:    status,
			}
			data, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
		time.Sleep(300 * time.Millisecond)
	}))
}

// makeTransactionServer creates an httptest.Server serving POST /api/3/transaction/list.
func makeTransactionServer(t *testing.T, tx *ZodiaTransaction) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "transaction") {
			w.Header().Set("Content-Type", "application/json")
			var result []ZodiaTransaction
			if tx != nil {
				result = []ZodiaTransaction{*tx}
			}
			_ = json.NewEncoder(w).Encode(ZodiaTransactionListResponse{Result: result})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// ─── CreateRFQ ────────────────────────────────────────────────────────────────

func TestZodiaService_CreateRFQ_ConfigError(t *testing.T) {
	svc := newZodiaTestService(t,
		&mockZodiaConfigResolver{err: errors.New("secret missing")},
		nil, nil,
	)

	_, err := svc.CreateRFQ(context.Background(), model.RFQRequest{
		ClientID:     "unknown-client",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100_000,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve client config")
}

func TestZodiaService_CreateRFQ_SessionError(t *testing.T) {
	// WS server that immediately rejects auth → GetOrCreate / Connect fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/auth":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ZodiaWSAuthResponse{Token: "tok"})
		case "/ws":
			up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			conn, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close() //nolint:errcheck
			_, _, _ = conn.ReadMessage()
			_ = conn.WriteMessage(websocket.TextMessage,
				[]byte(`{"action":"error","message":"auth rejected"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: srv.URL}
	signer := NewHMACSigner()
	rc := NewRESTClient(nil, signer)
	tokenMgr := NewWSTokenManager(rc)
	sessionMgr := NewSessionManager(NewWSClient(), tokenMgr, 0 /* maxRetries */)
	svc := newZodiaTestService(t, &mockZodiaConfigResolver{cfg: cfg}, rc, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := svc.CreateRFQ(ctx, model.RFQRequest{
		ClientID:     "client-err",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100_000,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zodia")
}

func TestZodiaService_CreateRFQ_Success(t *testing.T) {
	cs := makePriceServer(t, "zodia-q-001", 17.10, 17.20)
	defer cs.srv.Close()

	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	tokenMgr := NewWSTokenManager(cs.rest)
	sessionMgr := NewSessionManager(NewWSClient(), tokenMgr, 0)
	svc := newZodiaTestService(t, &mockZodiaConfigResolver{cfg: cfg}, cs.rest, sessionMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quote, err := svc.CreateRFQ(ctx, model.RFQRequest{
		ClientID:     "client-001",
		CurrencyPair: "USD:MXN",
		Side:         "BUY",
		Amount:       100_000,
	})
	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Equal(t, "zodia-q-001", quote.ID)
	assert.Equal(t, "ZODIA", quote.Venue)
	assert.InDelta(t, 17.10, quote.Bid, 0.001)
	assert.InDelta(t, 17.20, quote.Ask, 0.001)
}

// ─── ExecuteRFQ ──────────────────────────────────────────────────────────────

func TestZodiaService_ExecuteRFQ_ConfigError(t *testing.T) {
	svc := newZodiaTestService(t,
		&mockZodiaConfigResolver{err: errors.New("config unavailable")},
		nil, nil,
	)

	_, err := svc.ExecuteRFQ(context.Background(), "unknown-client", "qt-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve client config")
}

func TestZodiaService_ExecuteRFQ_Terminal(t *testing.T) {
	// Server handles auth then responds to execute_order with PROCESSED status.
	cs := makeExecuteServer(t, "trade-002", "PROCESSED")
	defer cs.srv.Close()

	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	tokenMgr := NewWSTokenManager(cs.rest)
	sessionMgr := NewSessionManager(NewWSClient(), tokenMgr, 0)
	svc := newZodiaTestService(t, &mockZodiaConfigResolver{cfg: cfg}, cs.rest, sessionMgr)

	// Pre-create the session so it's in Connected state before ExecuteRFQ is called.
	clientCfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	sess, err := sessionMgr.GetOrCreate(context.Background(), "client-002", clientCfg)
	require.NoError(t, err)
	require.True(t, sess.IsConnected())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trade, err := svc.ExecuteRFQ(ctx, "client-002", "zodia-q-002")
	require.NoError(t, err)
	require.NotNil(t, trade)
	assert.Equal(t, "trade-002", trade.TradeID)
	assert.Equal(t, model.StatusFilled, trade.Status, "PROCESSED should normalize to filled")
	assert.Equal(t, "ZODIA", trade.Venue)
}

func TestZodiaService_ExecuteRFQ_NonTerminal_StartsPoller(t *testing.T) {
	cs := makeExecuteServer(t, "trade-003", "PENDING")
	defer cs.srv.Close()

	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: cs.srv.URL}
	tokenMgr := NewWSTokenManager(cs.rest)
	sessionMgr := NewSessionManager(NewWSClient(), tokenMgr, 0)

	serviceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := newZodiaTestService(t, &mockZodiaConfigResolver{cfg: cfg}, cs.rest, sessionMgr)
	svc.ctx = serviceCtx

	poller := NewPoller(config.Config{}, svc, nil, nil, 10*time.Millisecond, nil)
	svc.SetPoller(poller)

	// Pre-create session.
	_, err := sessionMgr.GetOrCreate(context.Background(), "client-003", cfg)
	require.NoError(t, err)

	ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	trade, err := svc.ExecuteRFQ(ctx, "client-003", "zodia-q-003")
	require.NoError(t, err)
	require.NotNil(t, trade)
	assert.Equal(t, model.StatusPending, trade.Status, "PENDING should normalize to pending")

	// Give poller goroutine time to register.
	time.Sleep(5 * time.Millisecond)
	_, active := poller.activeTrades.Load("trade-003")
	assert.True(t, active, "poller should track trade-003")

	poller.Stop()
}

// ─── FetchTransactionStatus ──────────────────────────────────────────────────

func TestZodiaService_FetchTransactionStatus_Success(t *testing.T) {
	tx := &ZodiaTransaction{
		UUID:    "uuid-001",
		TradeID: "tx-fetch-001",
		State:   "PROCESSED",
		Type:    "RFSTRADE",
	}

	srv := makeTransactionServer(t, tx)
	defer srv.Close()

	signer := NewHMACSigner()
	rc := NewRESTClient(nil, signer)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: srv.URL}
	resolver := &mockZodiaConfigResolver{cfg: cfg}
	svc := newZodiaTestService(t, resolver, rc, nil)

	result, err := svc.FetchTransactionStatus(context.Background(), "client-001", "tx-fetch-001")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "tx-fetch-001", result.TradeID)
	assert.Equal(t, "PROCESSED", result.State)
}

func TestZodiaService_FetchTransactionStatus_NotFound(t *testing.T) {
	// Server returns empty result list.
	srv := makeTransactionServer(t, nil)
	defer srv.Close()

	signer := NewHMACSigner()
	rc := NewRESTClient(nil, signer)
	cfg := &ZodiaClientConfig{APIKey: "k", APISecret: "s", BaseURL: srv.URL}
	svc := newZodiaTestService(t, &mockZodiaConfigResolver{cfg: cfg}, rc, nil)

	_, err := svc.FetchTransactionStatus(context.Background(), "client-001", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ─── syncTerminalTrade ────────────────────────────────────────────────────────

func TestZodiaService_SyncTerminalTrade_NilPublisher(t *testing.T) {
	svc := &Service{
		publisher:       nil,
		tradeSyncWriter: nil,
	}
	trade := &model.TradeConfirmation{
		TradeID:  "trade-nil-pub",
		ClientID: "client-001",
		Status:   model.StatusFilled,
	}
	// Should not panic.
	svc.syncTerminalTrade(context.Background(), trade)
}
