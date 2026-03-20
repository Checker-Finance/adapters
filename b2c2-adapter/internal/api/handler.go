package api

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
)

// B2CService defines the service methods used by the HTTP handler.
type B2CService interface {
	CreateRFQ(ctx context.Context, clientID, pair, side, quantity, clientRFQID string) (*b2c2.RFQResponse, error)
	ExecuteRFQ(ctx context.Context, clientID, pair, side, quantity, price, rfqID, clientOrderID string) (*b2c2.OrderResponse, error)
	GetBalance(ctx context.Context, clientID string) (b2c2.BalanceResponse, error)
	GetProducts(ctx context.Context, clientID string) ([]b2c2.Instrument, error)
}

// B2C2Handler handles HTTP API requests for B2C2 operations.
type B2C2Handler struct {
	service B2CService
}

// NewB2C2Handler creates a new B2C2Handler.
func NewB2C2Handler(service B2CService) *B2C2Handler {
	return &B2C2Handler{service: service}
}

// CreateRFQHandler handles POST /api/v1/quotes.
func (h *B2C2Handler) CreateRFQHandler(c *fiber.Ctx) error {
	var req RFQCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := req.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	resp, err := h.service.CreateRFQ(c.Context(), req.ClientID, req.Pair, req.Side, req.Quantity, req.ID)
	if err != nil {
		slog.Error("b2c2.create_rfq.failed",
			"client", req.ClientID,
			"error", err)
		return c.Status(fiber.StatusBadRequest).JSON(RFQCreateResponse{
			QuoteID:  req.ID,
			ErrorMsg: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(RFQCreateResponse{
		QuoteID:         req.ID,
		ProviderQuoteID: resp.RFQID,
		Price:           resp.Price,
		ExpireAt:        resp.ValidUntil,
	})
}

// ExecuteOrderHandler handles POST /api/v1/orders.
func (h *B2C2Handler) ExecuteOrderHandler(c *fiber.Ctx) error {
	var req OrderExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := req.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	slog.Info("b2c2.execute_order",
		"client", req.ClientID,
		"pair", req.Pair,
		"rfqId", req.RFQID,
	)

	resp, err := h.service.ExecuteRFQ(c.Context(), req.ClientID, req.Pair, req.Side, req.Quantity, req.Price, req.RFQID, req.ClientOrderID)
	if err != nil {
		slog.Error("b2c2.execute_order.failed",
			"client", req.ClientID,
			"error", err)
		return c.Status(fiber.StatusBadRequest).JSON(OrderExecuteResponse{
			OrderID:  req.OrderID,
			ErrorMsg: err.Error(),
		})
	}

	price := ""
	if resp.ExecutedPrice != nil {
		price = *resp.ExecutedPrice
	}
	return c.Status(fiber.StatusOK).JSON(OrderExecuteResponse{
		OrderID:         req.OrderID,
		ProviderOrderID: resp.OrderID,
		Status:          resp.Status,
		Price:           price,
		ExecutedAt:      resp.Created,
	})
}

// GetProductsHandler handles GET /api/v1/products.
func (h *B2C2Handler) GetProductsHandler(c *fiber.Ctx) error {
	clientID := c.Query("clientId")
	if clientID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "clientId query param is required"})
	}

	instruments, err := h.service.GetProducts(c.Context(), clientID)
	if err != nil {
		slog.Error("b2c2.get_products.failed", "client", clientID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"count":    len(instruments),
		"products": instruments,
	})
}

// GetBalancesHandler handles GET /api/v1/balances/:client_id.
func (h *B2C2Handler) GetBalancesHandler(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing client_id"})
	}

	balances, err := h.service.GetBalance(c.Context(), clientID)
	if err != nil {
		slog.Error("b2c2.get_balances.failed", "client", clientID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(balances)
}
