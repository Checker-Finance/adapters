package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

type Handler struct {
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
		slog.Error("get_balances_failed", "client_id", clientID, "error", err)
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

	slog.Info("attempting to execute quote", "client", req.ClientID, "quoteID", req.QuoteID)
	trade, err := h.Service.ExecuteRFQ(c.Context(), req.ClientID, req.QuoteID)
	if err != nil {
		res.ErrorMsg = err.Error()
		return c.Status(fiber.StatusBadRequest).JSON(res)
	}

	res.Status = trade.StatusOrder
	slog.Info("successfully executed quote",
		"client", req.ClientID,
		"status", res.Status,
		"quoteID", req.QuoteID)

	return c.Status(fiber.StatusOK).JSON(res)
}
