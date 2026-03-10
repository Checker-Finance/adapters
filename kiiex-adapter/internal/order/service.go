package order

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/alphapoint"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

// ConfigResolver resolves per-client Kiiex credentials from a secret store.
type ConfigResolver interface {
	Resolve(ctx context.Context, clientID string) (*security.Auth, error)
}

// Service handles order operations
type Service struct {
	mu               sync.RWMutex
	sessions         map[string]*alphapoint.Session
	resolver         ConfigResolver
	instrumentMaster *instruments.Master
	eventBus         *eventbus.EventBus
	wsURL            string
	logger           *zap.Logger
}

// NewService creates a new order service
func NewService(
	resolver ConfigResolver,
	instrumentMaster *instruments.Master,
	eventBus *eventbus.EventBus,
	wsURL string,
	logger *zap.Logger,
) *Service {
	return &Service{
		sessions:         make(map[string]*alphapoint.Session),
		resolver:         resolver,
		instrumentMaster: instrumentMaster,
		eventBus:         eventBus,
		wsURL:            wsURL,
		logger:           logger,
	}
}

// getOrCreateSession returns an existing session for clientID or creates and connects a new one.
func (s *Service) getOrCreateSession(ctx context.Context, clientID string, auth *security.Auth) (*alphapoint.Session, error) {
	s.mu.RLock()
	sess, ok := s.sessions[clientID]
	s.mu.RUnlock()
	if ok {
		return sess, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok = s.sessions[clientID]; ok {
		return sess, nil
	}

	client := alphapoint.NewClient(s.wsURL, s.logger)
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect to AlphaPoint for client %q: %w", clientID, err)
	}

	sess = alphapoint.NewSession(client, s.logger)
	sess.SetAuth(&alphapoint.AuthenticateUserRequest{
		APIKey:    auth.APIKey,
		Signature: auth.Signature,
		UserID:    auth.UserID,
		Nonce:     auth.Nonce,
	})
	sess.RegisterHandler("sendorder", s.handleSendOrderResponse)
	sess.RegisterHandler("cancelorder", s.handleCancelOrderResponse)
	sess.RegisterHandler("getorderstatus", s.handleGetOrderStatusResponse)

	s.sessions[clientID] = sess
	s.logger.Info("AlphaPoint session created", zap.String("clientID", clientID))
	return sess, nil
}

// ExecuteOrder submits an order to AlphaPoint for the client identified in the command.
func (s *Service) ExecuteOrder(ctx context.Context, cmd *SubmitOrderCommand) error {
	s.logger.Info("SubmitOrderCommand received", zap.Any("command", cmd))

	auth, err := s.resolver.Resolve(ctx, cmd.ClientID)
	if err != nil {
		return fmt.Errorf("resolve credentials for client %q: %w", cmd.ClientID, err)
	}

	instrumentID, ok := s.instrumentMaster.GetInstrumentID(cmd.InstrumentPair)
	if !ok {
		return fmt.Errorf("unknown instrument pair: %s", cmd.InstrumentPair)
	}

	tradeInfo := TradeInfo{
		ClientID:  cmd.ClientID,
		OmsID:     auth.OmsID,
		AccountID: auth.AccountID,
		OrderID:   int(cmd.ID),
	}

	orderReq := &alphapoint.SendOrderRequest{
		OmsID:              auth.OmsID,
		AccountID:          auth.AccountID,
		ClientOrderID:      int(cmd.ID),
		InstrumentID:       instrumentID,
		Quantity:           cmd.Quantity.InexactFloat64(),
		OrderType:          OrderTypeFromString(cmd.Type).ToInt(),
		Side:               SideFromString(cmd.Side).ToInt(),
		UseDisplayQuantity: false,
		TimeInForce:        TimeInForceFOK.ToInt(),
	}

	sess, err := s.getOrCreateSession(ctx, cmd.ClientID, auth)
	if err != nil {
		return err
	}

	s.eventBus.Publish(&OrderSubmittedEvent{
		TradeInfo: tradeInfo,
		OrderID:   cmd.ClientOrderID,
	})

	if err := sess.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := sess.ExecuteOrder(ctx, orderReq); err != nil {
		s.logger.Error("Failed to execute order", zap.Error(err))
		return err
	}

	return nil
}

// CancelOrder cancels an order for a specific client.
func (s *Service) CancelOrder(ctx context.Context, clientID, orderID string) error {
	s.logger.Info("CancelOrder", zap.String("clientID", clientID), zap.String("orderId", orderID))

	auth, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return fmt.Errorf("resolve credentials for client %q: %w", clientID, err)
	}

	orderIDInt, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("invalid order ID: %w", err)
	}

	cancel := &alphapoint.CancelOrderRequest{
		OmsID:     auth.OmsID,
		AccountID: auth.AccountID,
		OrderID:   orderIDInt,
	}

	sess, err := s.getOrCreateSession(ctx, clientID, auth)
	if err != nil {
		return err
	}

	s.eventBus.Publish(&OrderCanceledEvent{
		OrderID: orderID,
	})

	if err := sess.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := sess.CancelOrder(ctx, cancel); err != nil {
		s.logger.Error("Failed to cancel order", zap.Error(err))
		return err
	}

	return nil
}

// GetTradeStatus requests the status of a trade via the client's own session.
func (s *Service) GetTradeStatus(ctx context.Context, tradeInfo TradeInfo) error {
	s.logger.Info("GetTradeStatus", zap.Any("tradeInfo", tradeInfo))

	auth, err := s.resolver.Resolve(ctx, tradeInfo.ClientID)
	if err != nil {
		return fmt.Errorf("resolve credentials for client %q: %w", tradeInfo.ClientID, err)
	}

	sess, err := s.getOrCreateSession(ctx, tradeInfo.ClientID, auth)
	if err != nil {
		return err
	}

	request := &alphapoint.GetOrderStatusRequest{
		OmsID:     tradeInfo.OmsID,
		AccountID: tradeInfo.AccountID,
		OrderID:   tradeInfo.OrderID,
	}

	return sess.GetOrderStatus(ctx, request)
}

// Close closes all per-client AlphaPoint sessions.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for clientID, sess := range s.sessions {
		if err := sess.Close(); err != nil {
			s.logger.Error("failed to close session", zap.String("clientID", clientID), zap.Error(err))
			lastErr = err
		}
	}
	return lastErr
}

func (s *Service) handleSendOrderResponse(response *alphapoint.Response) {
	s.logger.Info("Order submitted response", zap.String("payload", response.O))

	var sendOrderResp alphapoint.SendOrderResponse
	if err := response.ParsePayload(&sendOrderResp); err != nil {
		s.logger.Error("Failed to parse SendOrderResponse", zap.Error(err))
		return
	}

	if strings.ToLower(sendOrderResp.Status) == "rejected" {
		s.logger.Warn("Order rejected", zap.String("error", sendOrderResp.ErrorMessage))
		s.eventBus.Publish(&AttemptedCancelEvent{
			OrderID: int(sendOrderResp.OrderID),
		})
	}
}

func (s *Service) handleCancelOrderResponse(response *alphapoint.Response) {
	s.logger.Info("Cancel submitted response", zap.String("payload", response.O))

	var cancelResp alphapoint.CancelOrderResponse
	if err := response.ParsePayload(&cancelResp); err != nil {
		s.logger.Error("Failed to parse CancelOrderResponse", zap.Error(err))
		return
	}

	s.eventBus.Publish(&AttemptedCancelEvent{
		OrderID: int(cancelResp.OrderID),
	})
}

func (s *Service) handleGetOrderStatusResponse(response *alphapoint.Response) {
	s.logger.Info("Order status response", zap.String("payload", response.O))

	var statusResp alphapoint.GetOrderStatusResponse
	if err := response.ParsePayload(&statusResp); err != nil {
		s.logger.Error("Failed to parse GetOrderStatusResponse", zap.Error(err))
		return
	}

	for _, order := range statusResp.Orders {
		if order.OrderID == 0 {
			s.logger.Error("Received order with null orderId", zap.Any("order", order))
			continue
		}

		orderState := strings.ToLower(order.OrderState)

		if orderState == "canceled" || orderState == "rejected" {
			s.logger.Warn("Order canceled or rejected", zap.Int("orderId", order.OrderID))
			s.eventBus.Publish(&AttemptedCancelEvent{
				OrderID: order.OrderID,
			})
			continue
		}

		if orderState == "filled" {
			event := AdaptOrder(&order)
			if symbol, ok := s.instrumentMaster.GetSymbol(order.Instrument); ok {
				event.InstrumentPair = symbol
			}
			s.eventBus.Publish(event)
		}
	}
}
