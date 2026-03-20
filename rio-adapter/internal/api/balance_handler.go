package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/gofiber/fiber/v2"
)

// BalanceHandler handles the GET /api/v1/balances/:client_id endpoint.
type BalanceHandler struct {
	store store.Store
}

// NewBalanceHandler creates a new BalanceHandler.
func NewBalanceHandler(st store.Store) *BalanceHandler {
	return &BalanceHandler{store: st}
}

// GetBalances returns the balances for the given client.
func (h *BalanceHandler) GetBalances(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "missing client_id"})
	}

	ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
	defer cancel()

	balances, err := h.store.GetClientBalances(ctx, clientID)
	if err != nil {
		slog.Error("rio.get_balances.failed", "client_id", clientID, "error", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(balances)
}
