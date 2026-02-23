package order

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/alphapoint"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

// Service handles order operations
type Service struct {
	session          *alphapoint.Session
	instrumentMaster *instruments.Master
	eventBus         *eventbus.EventBus
	auth             *security.Auth
	logger           *zap.Logger
}

// NewService creates a new order service
func NewService(
	session *alphapoint.Session,
	instrumentMaster *instruments.Master,
	eventBus *eventbus.EventBus,
	auth *security.Auth,
	logger *zap.Logger,
) *Service {
	s := &Service{
		session:          session,
		instrumentMaster: instrumentMaster,
		eventBus:         eventBus,
		auth:             auth,
		logger:           logger,
	}

	// Register message handlers
	session.RegisterHandler("sendorder", s.handleSendOrderResponse)
	session.RegisterHandler("cancelorder", s.handleCancelOrderResponse)
	session.RegisterHandler("getorderstatus", s.handleGetOrderStatusResponse)

	return s
}

// ExecuteOrder submits an order to AlphaPoint
func (s *Service) ExecuteOrder(ctx context.Context, cmd *SubmitOrderCommand) error {
	s.logger.Info("SubmitOrderCommand received", zap.Any("command", cmd))

	instrumentID, ok := s.instrumentMaster.GetInstrumentID(cmd.InstrumentPair)
	if !ok {
		return fmt.Errorf("unknown instrument pair: %s", cmd.InstrumentPair)
	}

	tradeInfo := TradeInfo{
		OmsID:     s.auth.OmsID,
		AccountID: s.auth.AccountID,
		OrderID:   int(cmd.ID),
	}

	order := &alphapoint.SendOrderRequest{
		OmsID:              s.auth.OmsID,
		AccountID:          s.auth.AccountID,
		ClientOrderID:      int(cmd.ID),
		InstrumentID:       instrumentID,
		Quantity:           cmd.Quantity.InexactFloat64(),
		OrderType:          OrderTypeFromString(cmd.Type).ToInt(),
		Side:               SideFromString(cmd.Side).ToInt(),
		UseDisplayQuantity: false,
		TimeInForce:        TimeInForceFOK.ToInt(),
	}

	// Publish order submitted event
	s.eventBus.Publish(&OrderSubmittedEvent{
		TradeInfo: tradeInfo,
		OrderID:   cmd.ClientOrderID,
	})

	// Login and execute order
	if err := s.session.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := s.session.ExecuteOrder(ctx, order); err != nil {
		s.logger.Error("Failed to execute order", zap.Error(err))
		return err
	}

	return nil
}

// CancelOrder cancels an order
func (s *Service) CancelOrder(ctx context.Context, orderID string) error {
	s.logger.Info("CancelOrder", zap.String("orderId", orderID))

	orderIDInt, err := strconv.Atoi(orderID)
	if err != nil {
		return fmt.Errorf("invalid order ID: %w", err)
	}

	cancel := &alphapoint.CancelOrderRequest{
		OmsID:     s.auth.OmsID,
		AccountID: s.auth.AccountID,
		OrderID:   orderIDInt,
	}

	// Publish order canceled event
	s.eventBus.Publish(&OrderCanceledEvent{
		OrderID: orderID,
	})

	// Login and cancel order
	if err := s.session.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	if err := s.session.CancelOrder(ctx, cancel); err != nil {
		s.logger.Error("Failed to cancel order", zap.Error(err))
		return err
	}

	return nil
}

// CancelAllOrders cancels all orders
func (s *Service) CancelAllOrders(ctx context.Context) error {
	s.logger.Info("CancelAllOrders")

	cancel := &alphapoint.CancelAllOrdersRequest{
		OmsID:     s.auth.OmsID,
		AccountID: s.auth.AccountID,
	}

	if err := s.session.Login(ctx); err != nil {
		s.logger.Error("Failed to login", zap.Error(err))
		return err
	}

	return s.session.CancelAllOrders(ctx, cancel)
}

// GetTradeStatus requests the status of a trade
func (s *Service) GetTradeStatus(ctx context.Context, tradeInfo TradeInfo) error {
	s.logger.Info("GetTradeStatus", zap.Any("tradeInfo", tradeInfo))

	request := &alphapoint.GetOrderStatusRequest{
		OmsID:     tradeInfo.OmsID,
		AccountID: tradeInfo.AccountID,
		OrderID:   tradeInfo.OrderID,
	}

	return s.session.GetOrderStatus(ctx, request)
}

// GetInstruments requests the list of instruments
func (s *Service) GetInstruments(ctx context.Context) error {
	s.logger.Info("GetInstruments")
	return s.session.GetInstruments(ctx, s.auth.OmsID)
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
