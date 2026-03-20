package api

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
)

// OrderService defines the service method used by the HTTP handler.
type OrderService interface {
	ExecuteOrder(ctx context.Context, cmd *order.SubmitOrderCommand) error
}

// KiiexHandler handles HTTP API requests for Kiiex operations.
type KiiexHandler struct {
	service OrderService
}

// NewKiiexHandler creates a new KiiexHandler.
func NewKiiexHandler(service OrderService) *KiiexHandler {
	return &KiiexHandler{service: service}
}

// ExecuteOrderHandler handles POST /api/v1/orders.
// Returns 202 Accepted immediately; order dispatch is async via WebSocket.
func (h *KiiexHandler) ExecuteOrderHandler(c *fiber.Ctx) error {
	var req OrderExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := req.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	qty, err := decimal.NewFromString(req.Quantity)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid quantity: " + err.Error()})
	}

	var price decimal.Decimal
	if req.Price != "" {
		price, err = decimal.NewFromString(req.Price)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid price: " + err.Error()})
		}
	}

	cmd := &order.SubmitOrderCommand{
		ID:             req.ID,
		OrderID:        req.OrderID,
		ClientOrderID:  req.ClientOrderID,
		ClientID:       req.ClientID,
		InstrumentPair: req.InstrumentPair,
		Quantity:       qty,
		Price:          price,
		Side:           req.Side,
		Type:           req.Type,
	}

	slog.Info("kiiex.execute_order.rest",
		"client", req.ClientID,
		"pair", req.InstrumentPair,
		"side", req.Side,
	)

	if err := h.service.ExecuteOrder(c.Context(), cmd); err != nil {
		slog.Error("kiiex.execute_order.failed",
			"client", req.ClientID,
			"error", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusAccepted).JSON(OrderExecuteResponse{
		OrderID: req.OrderID,
		Status:  "accepted",
	})
}
