package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

type Handler struct {
	Logger  *zap.Logger
	Service *braza.Service
	Store   store.Store
}

// GetBalances godoc
func (h *Handler) GetBalances(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "missing client_id"})
	}

	ctx := context.Background()
	rows, err := h.Store.GetClientBalances(ctx, clientID)
	if err != nil {
		h.Logger.Error("get_balances_failed", zap.String("client_id", clientID), zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusOK).JSON(rows)
}

// CreateRFQ godoc
func (h *Handler) CreateRFQHandler(c *fiber.Ctx) error {
	var req RFQCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	r := model.RFQRequest{
		ClientID:       req.ClientID,
		Side:           req.Side,
		CurrencyPair:   req.CurrencyPair,
		Amount:         req.Amount,
		CurrencyAmount: req.AmountDenomination,
	}

	res := RFQResponse{
		QuoteID:  req.ID,
		ExpireAt: time.Now().Unix(),
	}

	rfq, err := h.Service.CreateRFQ(c.Context(), r)
	if err != nil {
		res.ErrorMsg = err.Error()
		return c.Status(fiber.StatusBadRequest).JSON(res)
	}

	res.ProviderQuoteId = rfq.ID
	res.Price = rfq.Price
	res.ExpireAt = time.Now().Add(time.Second * 15).Unix()
	return c.Status(fiber.StatusCreated).JSON(res)
}

func (h *Handler) ExecuteRFQHandler(c *fiber.Ctx) error {
	var req RFQExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res := RFQExecutionResponse{
		OrderID:         req.OrderID,
		ProviderOrderID: req.QuoteID,
		RemainingAmount: req.Quantity,
		FilledAmount:    0.0,
		Price:           req.Price,
		ExecutedAt:      time.Now().Unix(),
	}

	h.Logger.Info("attempting to execute quote", zap.String("client", req.ClientID), zap.String("quoteID", req.QuoteID))
	trade, err := h.Service.ExecuteRFQ(c.Context(), req.ClientID, req.QuoteID)
	if err != nil {
		res.ErrorMsg = err.Error()
		return c.Status(fiber.StatusBadRequest).JSON(res)
	}

	res.Status = trade.StatusOrder
	h.Logger.Info("successfully executed quote",
		zap.String("client", req.ClientID),
		zap.String("status", res.Status),
		zap.String("quoteID", req.QuoteID))
	
	return c.Status(fiber.StatusOK).JSON(res)
}
