package order

import (
	"context"
	"errors"
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

// sessionEntry pairs an AlphaPoint session with the credentials used to create it.
type sessionEntry struct {
	session *alphapoint.Session
	auth    *security.Auth
}

// Service handles order operations
type Service struct {
	mu               sync.RWMutex
	sessions         map[string]sessionEntry
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
		sessions:         make(map[string]sessionEntry),
		resolver:         resolver,
		instrumentMaster: instrumentMaster,
		eventBus:         eventBus,
		wsURL:            wsURL,
		logger:           logger,
	}
}

// getOrCreateSession returns an existing session entry for clientID or creates and connects a new one.
// Auth is resolved from the secret store only when a new session needs to be created.
func (s *Service) getOrCreateSession(ctx context.Context, clientID string) (sessionEntry, error) {
	s.mu.RLock()
	entry, ok := s.sessions[clientID]
	s.mu.RUnlock()
	if ok {
		return entry, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok = s.sessions[clientID]; ok {
		return entry, nil
	}

	auth, err := s.resolver.Resolve(ctx, clientID)
	if err != nil {
		return sessionEntry{}, fmt.Errorf("resolve credentials for client %q: %w", clientID, err)
	}

	client := alphapoint.NewClient(s.wsURL, s.logger)
	if err := client.Connect(ctx); err != nil {
		return sessionEntry{}, fmt.Errorf("connect to AlphaPoint for client %q: %w", clientID, err)
	}

	sess := alphapoint.NewSession(client, s.logger)
	sess.SetAuth(&alphapoint.AuthenticateUserRequest{
		APIKey:    auth.APIKey,
		Signature: auth.Signature,
		UserID:    auth.UserID,
		Nonce:     auth.Nonce,
	})
	sess.RegisterHandler("sendorder", s.handleSendOrderResponse)
	sess.RegisterHandler("cancelorder", s.handleCancelOrderResponse)
	sess.RegisterHandler("getorderstatus", s.handleGetOrderStatusResponse)

	entry = sessionEntry{session: sess, auth: auth}
	s.sessions[clientID] = entry
	s.logger.Info("AlphaPoint session created", zap.String("clientID", clientID))
	return entry, nil
}

// ExecuteOrder submits an order to AlphaPoint for the client identified in the command.
func (s *Service) ExecuteOrder(ctx context.Context, cmd *SubmitOrderCommand) error {
	s.logger.Info("SubmitOrderCommand received", zap.Any("command", cmd))

	entry, err := s.getOrCreateSession(ctx, cmd.ClientID)
	if err != nil {
		return err
	}

	instrumentID, ok := s.instrumentMaster.GetInstrumentID(cmd.InstrumentPair)
	if !ok {
		return fmt.Errorf("unknown instrument pair: %s", cmd.InstrumentPair)
	}

	tradeInfo := TradeInfo{
		ClientID:  cmd.ClientID,
		OmsID:     entry.auth.OmsID,
		AccountID: entry.auth.AccountID,
		OrderID:   int(cmd.ID),
	}

	orderReq := &alphapoint.SendOrderRequest{
		OmsID:              entry.auth.OmsID,
		AccountID:          entry.auth.AccountID,
		ClientOrderID:      int(cmd.ID),
		InstrumentID:       instrumentID,
		Quantity:           cmd.Quantity.InexactFloat64(),
		OrderType:          OrderTypeFromString(cmd.Type).ToInt(),
		Side:               SideFromString(cmd.Side).ToInt(),
		UseDisplayQuantity: false,
		TimeInForce:        TimeInForceFOK.ToInt(),
	}

	if err := entry.session.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := entry.session.ExecuteOrder(ctx, orderReq); err != nil {
		s.logger.Error("Failed to execute order", zap.Error(err))
		return err
	}

	s.eventBus.Publish(&OrderSubmittedEvent{
		TradeInfo: tradeInfo,
		OrderID:   cmd.ClientOrderID,
	})

	return nil
}

// CancelOrder cancels an order for a specific client.
func (s *Service) CancelOrder(ctx context.Context, clientID, orderID string) error {
	s.logger.Info("CancelOrder", zap.String("clientID", clientID), zap.String("orderId", orderID))

	orderIDInt, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("invalid order ID: %w", err)
	}

	entry, err := s.getOrCreateSession(ctx, clientID)
	if err != nil {
		return err
	}

	cancel := &alphapoint.CancelOrderRequest{
		OmsID:     entry.auth.OmsID,
		AccountID: entry.auth.AccountID,
		OrderID:   orderIDInt,
	}

	if err := entry.session.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := entry.session.CancelOrder(ctx, cancel); err != nil {
		s.logger.Error("Failed to cancel order", zap.Error(err))
		return err
	}

	s.eventBus.Publish(&OrderCanceledEvent{
		OrderID: orderID,
	})

	return nil
}

// GetTradeStatus requests the status of a trade via the client's own session.
func (s *Service) GetTradeStatus(ctx context.Context, tradeInfo TradeInfo) error {
	s.logger.Info("GetTradeStatus", zap.Any("tradeInfo", tradeInfo))

	entry, err := s.getOrCreateSession(ctx, tradeInfo.ClientID)
	if err != nil {
		return err
	}

	request := &alphapoint.GetOrderStatusRequest{
		OmsID:     tradeInfo.OmsID,
		AccountID: tradeInfo.AccountID,
		OrderID:   tradeInfo.OrderID,
	}

	return entry.session.GetOrderStatus(ctx, request)
}

// Close closes all per-client AlphaPoint sessions.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error
	for clientID, entry := range s.sessions {
		if err := entry.session.Close(); err != nil {
			s.logger.Error("failed to close session", zap.String("clientID", clientID), zap.Error(err))
			errs = append(errs, fmt.Errorf("close session %q: %w", clientID, err))
		}
	}
	return errors.Join(errs...)
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
