package tracking

import (
	"context"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

// TradeStatusService tracks trade statuses and polls for updates
type TradeStatusService struct {
	tradeMap     map[string]order.TradeInfo // checkerID -> AlphaPoint TradeInfo
	mu           sync.RWMutex
	orderService OrderService
	eventBus     *eventbus.EventBus
	logger       *zap.Logger
	pollInterval time.Duration
	done         chan struct{}
}

// OrderService defines the order service interface for trade status
type OrderService interface {
	GetTradeStatus(ctx context.Context, tradeInfo order.TradeInfo) error
}

// NewTradeStatusService creates a new trade status service
func NewTradeStatusService(
	orderService OrderService,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *TradeStatusService {
	s := &TradeStatusService{
		tradeMap:     make(map[string]order.TradeInfo),
		orderService: orderService,
		eventBus:     eventBus,
		logger:       logger,
		pollInterval: 5 * time.Minute,
		done:         make(chan struct{}),
	}

	// Subscribe to events
	s.subscribeToEvents()

	return s
}

func (s *TradeStatusService) subscribeToEvents() {
	// Subscribe to OrderSubmittedEvent
	s.eventBus.Subscribe(order.OrderSubmittedEvent{}, func(event interface{}) {
		if submitted, ok := event.(*order.OrderSubmittedEvent); ok {
			s.handleOrderSubmitted(submitted)
		} else if submitted, ok := event.(order.OrderSubmittedEvent); ok {
			s.handleOrderSubmitted(&submitted)
		}
	})

	// Subscribe to AttemptedCancelEvent
	s.eventBus.Subscribe(order.AttemptedCancelEvent{}, func(event interface{}) {
		if canceled, ok := event.(*order.AttemptedCancelEvent); ok {
			s.handleAttemptedCancel(canceled)
		} else if canceled, ok := event.(order.AttemptedCancelEvent); ok {
			s.handleAttemptedCancel(&canceled)
		}
	})

	// Subscribe to FillArrivedEvent
	s.eventBus.Subscribe(order.FillArrivedEvent{}, func(event interface{}) {
		if fill, ok := event.(*order.FillArrivedEvent); ok {
			s.handleFillArrived(fill)
		} else if fill, ok := event.(order.FillArrivedEvent); ok {
			s.handleFillArrived(&fill)
		}
	})
}

func (s *TradeStatusService) handleOrderSubmitted(event *order.OrderSubmittedEvent) {
	s.logger.Info("Order submitted", zap.Any("tradeInfo", event.TradeInfo))

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tradeMap[event.OrderID] = event.TradeInfo
}

func (s *TradeStatusService) handleAttemptedCancel(event *order.AttemptedCancelEvent) {
	s.logger.Info("Attempted cancel", zap.Int("orderId", event.OrderID))

	if event.OrderID != 0 {
		if tradeID, ok := s.markFilled(event.OrderID); ok {
			s.eventBus.Publish(&order.OrderCanceledEvent{
				OrderID: tradeID,
			})
		}
	}
}

func (s *TradeStatusService) handleFillArrived(event *order.FillArrivedEvent) {
	s.logger.Info("Fill arrived", zap.Any("event", event))

	quantityLeaves, err := decimal.NewFromString(event.QuantityLeaves)
	if err != nil {
		s.logger.Error("Failed to parse quantityLeaves", zap.Error(err))
		return
	}

	if quantityLeaves.IsZero() {
		// Order is fully filled, remove from tracking
		if event.OrderID != "" {
			s.mu.Lock()
			delete(s.tradeMap, event.OrderID)
			s.mu.Unlock()
		}
	}
}

func (s *TradeStatusService) markFilled(tradeID int) (string, bool) {
	s.logger.Info("MarkTradeFilled", zap.Int("tradeId", tradeID))

	s.mu.Lock()
	defer s.mu.Unlock()

	for key, tradeInfo := range s.tradeMap {
		if tradeInfo.OrderID == tradeID {
			delete(s.tradeMap, key)
			return key, true
		}
	}

	return "", false
}

// Start starts the polling goroutine
func (s *TradeStatusService) Start(ctx context.Context) {
	s.logger.Info("Starting trade status polling", zap.Duration("interval", s.pollInterval))

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollTradeStatuses(ctx)
		}
	}
}

func (s *TradeStatusService) pollTradeStatuses(ctx context.Context) {
	s.mu.RLock()
	trades := make(map[string]order.TradeInfo, len(s.tradeMap))
	for k, v := range s.tradeMap {
		trades[k] = v
	}
	s.mu.RUnlock()

	for _, tradeInfo := range trades {
		s.logger.Info("Requesting status of trade", zap.Any("trade", tradeInfo))
		if err := s.orderService.GetTradeStatus(ctx, tradeInfo); err != nil {
			s.logger.Error("Failed to get trade status", zap.Error(err))
		}
	}
}

// Stop stops the polling goroutine
func (s *TradeStatusService) Stop() {
	close(s.done)
}

// GetTrackedTrades returns a copy of the tracked trades
func (s *TradeStatusService) GetTrackedTrades() map[string]order.TradeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]order.TradeInfo, len(s.tradeMap))
	for k, v := range s.tradeMap {
		result[k] = v
	}
	return result
}

// TradeCount returns the number of tracked trades
func (s *TradeStatusService) TradeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tradeMap)
}
