package alphapoint

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Session manages the AlphaPoint WebSocket session
type Session struct {
	client          *Client
	logger          *zap.Logger
	auth            *AuthenticateUserRequest
	authMu          sync.RWMutex
	messageHandlers map[string]func(*Response)
	handlersMu      sync.RWMutex
}

// NewSession creates a new AlphaPoint session
func NewSession(client *Client, logger *zap.Logger) *Session {
	s := &Session{
		client:          client,
		logger:          logger,
		messageHandlers: make(map[string]func(*Response)),
	}

	// Register ourselves as a handler for all messages
	client.AddHandler(s.handleMessage)

	return s
}

// SetAuth sets the authentication credentials
func (s *Session) SetAuth(auth *AuthenticateUserRequest) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.auth = auth
}

// GetAuth returns the authentication credentials
func (s *Session) GetAuth() *AuthenticateUserRequest {
	s.authMu.RLock()
	defer s.authMu.RUnlock()
	return s.auth
}

// Login sends authentication to AlphaPoint
func (s *Session) Login(ctx context.Context) error {
	s.authMu.RLock()
	auth := s.auth
	s.authMu.RUnlock()

	if auth == nil {
		s.logger.Warn("No authentication credentials set")
		return nil
	}

	s.logger.Info("Logging in to AlphaPoint")
	return s.client.SendMessage(ctx, "AuthenticateUser", auth)
}

// Logout sends logout request to AlphaPoint
func (s *Session) Logout(ctx context.Context) error {
	s.logger.Info("Logging out from AlphaPoint")
	return s.client.SendMessage(ctx, "LogOut", &LogOutRequest{})
}

// RegisterHandler registers a handler for a specific operation
func (s *Session) RegisterHandler(operation string, handler func(*Response)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.messageHandlers[strings.ToLower(operation)] = handler
}

func (s *Session) handleMessage(response *Response) {
	s.handlersMu.RLock()
	handler, ok := s.messageHandlers[strings.ToLower(response.N)]
	s.handlersMu.RUnlock()

	if ok {
		handler(response)
	} else {
		s.logger.Debug("Unhandled message", zap.String("operation", response.N))
	}
}

// ExecuteOrder sends an order to AlphaPoint
func (s *Session) ExecuteOrder(ctx context.Context, order *SendOrderRequest) error {
	s.logger.Info("Executing order",
		zap.Int("clientOrderId", order.ClientOrderID),
		zap.Int("instrumentId", order.InstrumentID),
	)
	return s.client.SendMessage(ctx, "SendOrder", order)
}

// CancelOrder sends a cancel request to AlphaPoint
func (s *Session) CancelOrder(ctx context.Context, cancel *CancelOrderRequest) error {
	s.logger.Info("Canceling order", zap.Int("orderId", cancel.OrderID))
	return s.client.SendMessage(ctx, "CancelOrder", cancel)
}

// CancelAllOrders sends a cancel all orders request to AlphaPoint
func (s *Session) CancelAllOrders(ctx context.Context, cancel *CancelAllOrdersRequest) error {
	s.logger.Info("Canceling all orders", zap.Int("accountId", cancel.AccountID))
	return s.client.SendMessage(ctx, "CancelAllOrders", cancel)
}

// GetOrderStatus requests the status of an order
func (s *Session) GetOrderStatus(ctx context.Context, request *GetOrderStatusRequest) error {
	s.logger.Info("Getting order status", zap.Int("orderId", request.OrderID))
	return s.client.SendMessage(ctx, "GetOrderStatus", request)
}

// GetInstruments requests the list of instruments
func (s *Session) GetInstruments(ctx context.Context, omsID int) error {
	s.logger.Info("Getting instruments", zap.Int("omsId", omsID))
	return s.client.SendMessage(ctx, "GetInstruments", &GetInstrumentsRequest{OmsID: omsID})
}

// GetProducts requests the list of products
func (s *Session) GetProducts(ctx context.Context, omsID int) error {
	s.logger.Info("Getting products", zap.Int("omsId", omsID))
	return s.client.SendMessage(ctx, "GetProducts", &GetProductsRequest{OmsID: omsID})
}

// GetUserAccounts requests the user's accounts
func (s *Session) GetUserAccounts(ctx context.Context, request *GetUserAccountsRequest) error {
	s.logger.Info("Getting user accounts", zap.Int("userId", request.UserID))
	return s.client.SendMessage(ctx, "GetUserAccounts", request)
}

// IsConnected returns whether the session is connected
func (s *Session) IsConnected() bool {
	return s.client.IsConnected()
}

// Connect connects to AlphaPoint
func (s *Session) Connect(ctx context.Context) error {
	return s.client.Connect(ctx)
}

// Close closes the session
func (s *Session) Close() error {
	return s.client.Close()
}
