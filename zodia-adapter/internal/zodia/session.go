package zodia

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/zodia-adapter/internal/metrics"
)

// SessionConfig holds parameters for a per-client WebSocket session.
type SessionConfig struct {
	ClientID   string
	ZodiaCfg   *ZodiaClientConfig
	MaxRetries int
	RetryDelay time.Duration
}

// priceResult wraps a WS price update or an error for channel dispatch.
type priceResult struct {
	payload WSPricePayload
	err     error
}

// orderResult wraps a WS order confirmation or an error for channel dispatch.
type orderResult struct {
	payload WSOrderConfirmPayload
	err     error
}

// Session manages a per-client WebSocket connection to Zodia, including
// authentication, request dispatch via pending channels, and reconnection.
//
// ⚠️ WS action strings ("auth", "auth_success", "subscribe_price", "price_update",
// "execute_order", "order_confirmation") are inferred. Verify against Zodia sandbox.
type Session struct {
	cfg      SessionConfig
	wsClient *WSClient
	tokenMgr *WSTokenManager
	logger   *zap.Logger

	mu        sync.Mutex
	conn      WSConn
	connected atomic.Bool
	sendMu    sync.Mutex // serialises WS writes

	pendingPrices sync.Map // clientRef → chan priceResult
	pendingOrders sync.Map // clientRef → chan orderResult
}

// NewSession constructs a new Session (not yet connected).
func NewSession(cfg SessionConfig, wsClient *WSClient, tokenMgr *WSTokenManager, logger *zap.Logger) *Session {
	return &Session{
		cfg:      cfg,
		wsClient: wsClient,
		tokenMgr: tokenMgr,
		logger:   logger,
	}
}

// Connect establishes the WebSocket connection and authenticates.
func (s *Session) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connect(ctx)
}

// connect is the internal (lock-held) connection implementation.
func (s *Session) connect(ctx context.Context) error {
	wsURL := WSURL(s.cfg.ZodiaCfg.BaseURL)
	conn, err := s.wsClient.Dial(ctx, wsURL)
	if err != nil {
		return fmt.Errorf("zodia.session.dial: %w", err)
	}

	// Authenticate with WS token
	token, err := s.tokenMgr.GetToken(ctx, s.cfg.ZodiaCfg)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("zodia.session.get_token: %w", err)
	}

	if err := s.wsClient.SendJSON(conn, WSAuthMessage{Action: "auth", Token: token}); err != nil {
		_ = conn.Close()
		return fmt.Errorf("zodia.session.send_auth: %w", err)
	}

	// Wait for auth result
	raw, err := s.wsClient.ReadMessage(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("zodia.session.read_auth_result: %w", err)
	}

	var authResult WSAuthResult
	if err := json.Unmarshal(raw, &authResult); err != nil {
		_ = conn.Close()
		return fmt.Errorf("zodia.session.parse_auth_result: %w", err)
	}

	if authResult.Action != "auth_success" {
		_ = conn.Close()
		// Invalidate cached token on auth failure
		s.tokenMgr.InvalidateToken(s.cfg.ZodiaCfg.APIKey)
		return fmt.Errorf("zodia.session.auth_failed: %s", authResult.Message)
	}

	s.conn = conn
	s.connected.Store(true)
	s.logger.Info("zodia.session.connected",
		zap.String("client_id", s.cfg.ClientID),
		zap.String("url", wsURL))

	// Start read loop in background
	go s.readLoop(ctx)

	return nil
}

// readLoop continuously reads messages from the WebSocket and dispatches them
// to pending request channels. Triggers reconnection on connection errors.
func (s *Session) readLoop(ctx context.Context) {
	s.logger.Debug("zodia.session.read_loop_started", zap.String("client_id", s.cfg.ClientID))
	for {
		if !s.connected.Load() {
			return
		}

		raw, err := s.wsClient.ReadMessage(s.conn)
		if err != nil {
			if !s.connected.Load() {
				return
			}
			s.logger.Error("zodia.session.read_error",
				zap.String("client_id", s.cfg.ClientID),
				zap.Error(err))
			s.connected.Store(false)
			go s.reconnectWithBackoff(ctx)
			return
		}

		s.dispatch(raw)
	}
}

// dispatch parses a raw WS message and routes it to the appropriate pending channel.
func (s *Session) dispatch(raw []byte) {
	var msg WSMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		s.logger.Warn("zodia.session.dispatch_parse_failed", zap.Error(err))
		return
	}

	switch msg.Action {
	case "price_update":
		var payload WSPricePayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			s.logger.Warn("zodia.session.price_parse_failed", zap.Error(err))
			return
		}
		if ch, ok := s.pendingPrices.Load(payload.ClientRef); ok {
			ch.(chan priceResult) <- priceResult{payload: payload}
		}

	case "order_confirmation":
		var payload WSOrderConfirmPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			s.logger.Warn("zodia.session.order_parse_failed", zap.Error(err))
			return
		}
		if ch, ok := s.pendingOrders.Load(payload.ClientRef); ok {
			ch.(chan orderResult) <- orderResult{payload: payload}
		}

	case "error":
		var payload WSErrorPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			s.logger.Warn("zodia.session.error_parse_failed", zap.Error(err))
			return
		}
		s.logger.Warn("zodia.session.ws_error",
			zap.String("client_id", s.cfg.ClientID),
			zap.String("code", payload.Code),
			zap.String("message", payload.Message))

		errMsg := fmt.Errorf("zodia ws error: %s", payload.Message)
		if payload.ClientRef != "" {
			if ch, ok := s.pendingPrices.Load(payload.ClientRef); ok {
				ch.(chan priceResult) <- priceResult{err: errMsg}
			}
			if ch, ok := s.pendingOrders.Load(payload.ClientRef); ok {
				ch.(chan orderResult) <- orderResult{err: errMsg}
			}
		}

	default:
		s.logger.Debug("zodia.session.unknown_action",
			zap.String("action", msg.Action),
			zap.String("client_id", s.cfg.ClientID))
	}
}

// reconnectWithBackoff attempts to reconnect with exponential backoff.
func (s *Session) reconnectWithBackoff(ctx context.Context) {
	delay := time.Second
	maxDelay := 60 * time.Second

	for i := 0; i < s.cfg.MaxRetries; i++ {
		select {
		case <-ctx.Done():
			s.logger.Info("zodia.session.reconnect_cancelled",
				zap.String("client_id", s.cfg.ClientID))
			return
		case <-time.After(delay):
		}

		s.logger.Info("zodia.session.reconnecting",
			zap.String("client_id", s.cfg.ClientID),
			zap.Int("attempt", i+1),
			zap.Duration("delay", delay))

		s.mu.Lock()
		err := s.connect(ctx)
		s.mu.Unlock()

		if err != nil {
			s.logger.Error("zodia.session.reconnect_failed",
				zap.String("client_id", s.cfg.ClientID),
				zap.Int("attempt", i+1),
				zap.Error(err))
			delay = min(delay*2, maxDelay)
			continue
		}

		metrics.IncWSReconnect(s.cfg.ClientID)
		s.logger.Info("zodia.session.reconnected",
			zap.String("client_id", s.cfg.ClientID),
			zap.Int("attempts", i+1))
		return
	}

	s.logger.Error("zodia.session.max_retries_exceeded",
		zap.String("client_id", s.cfg.ClientID),
		zap.Int("max_retries", s.cfg.MaxRetries))
}

// RequestPrice sends a subscribe_price message and waits for the price_update response.
func (s *Session) RequestPrice(ctx context.Context, instrument, side string, quantity float64) (*WSPricePayload, error) {
	if !s.connected.Load() {
		return nil, fmt.Errorf("zodia.session: not connected (client: %s)", s.cfg.ClientID)
	}

	clientRef := uuid.New().String()
	ch := make(chan priceResult, 1)
	s.pendingPrices.Store(clientRef, ch)
	defer s.pendingPrices.Delete(clientRef)

	msg := WSSubscribePriceMessage{
		Action:     "subscribe_price",
		ClientRef:  clientRef,
		Instrument: instrument,
		Side:       side,
		Quantity:   quantity,
	}

	s.sendMu.Lock()
	err := s.wsClient.SendJSON(s.conn, msg)
	s.sendMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("zodia.session.request_price.send: %w", err)
	}

	select {
	case result := <-ch:
		if result.err != nil {
			return nil, result.err
		}
		return &result.payload, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("zodia.session.request_price: %w", ctx.Err())
	}
}

// ExecuteOrder sends an execute_order message and waits for the order_confirmation response.
func (s *Session) ExecuteOrder(ctx context.Context, quoteID string) (*WSOrderConfirmPayload, error) {
	if !s.connected.Load() {
		return nil, fmt.Errorf("zodia.session: not connected (client: %s)", s.cfg.ClientID)
	}

	clientRef := uuid.New().String()
	ch := make(chan orderResult, 1)
	s.pendingOrders.Store(clientRef, ch)
	defer s.pendingOrders.Delete(clientRef)

	msg := WSExecuteOrderMessage{
		Action:    "execute_order",
		ClientRef: clientRef,
		QuoteID:   quoteID,
	}

	s.sendMu.Lock()
	err := s.wsClient.SendJSON(s.conn, msg)
	s.sendMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("zodia.session.execute_order.send: %w", err)
	}

	select {
	case result := <-ch:
		if result.err != nil {
			return nil, result.err
		}
		return &result.payload, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("zodia.session.execute_order: %w", ctx.Err())
	}
}

// Close closes the underlying WebSocket connection gracefully.
func (s *Session) Close() {
	s.connected.Store(false)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// IsConnected returns true if the session has an active WS connection.
func (s *Session) IsConnected() bool {
	return s.connected.Load()
}

//
// ────────────────────────────────────────────────
//   SessionManager — one session per client
// ────────────────────────────────────────────────
//

// SessionManager maintains one WebSocket session per client ID.
type SessionManager struct {
	mu         sync.Mutex
	sessions   map[string]*Session
	wsClient   *WSClient
	tokenMgr   *WSTokenManager
	logger     *zap.Logger
	maxRetries int
}

// NewSessionManager constructs a new SessionManager.
func NewSessionManager(logger *zap.Logger, wsClient *WSClient, tokenMgr *WSTokenManager, maxRetries int) *SessionManager {
	return &SessionManager{
		sessions:   make(map[string]*Session),
		wsClient:   wsClient,
		tokenMgr:   tokenMgr,
		logger:     logger,
		maxRetries: maxRetries,
	}
}

// GetOrCreate returns the existing session for clientID or creates and connects a new one.
func (m *SessionManager) GetOrCreate(ctx context.Context, clientID string, cfg *ZodiaClientConfig) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[clientID]; ok && sess.IsConnected() {
		return sess, nil
	}

	sess := NewSession(SessionConfig{
		ClientID:   clientID,
		ZodiaCfg:   cfg,
		MaxRetries: m.maxRetries,
		RetryDelay: time.Second,
	}, m.wsClient, m.tokenMgr, m.logger)

	if err := sess.Connect(ctx); err != nil {
		return nil, fmt.Errorf("zodia.session_manager.connect: %w", err)
	}

	m.sessions[clientID] = sess
	return sess, nil
}

// Close closes the session for the given clientID.
func (m *SessionManager) Close(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sess, ok := m.sessions[clientID]; ok {
		sess.Close()
		delete(m.sessions, clientID)
	}
}

// CloseAll closes all active sessions.
func (m *SessionManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for clientID, sess := range m.sessions {
		sess.Close()
		delete(m.sessions, clientID)
	}
}
