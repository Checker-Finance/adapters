package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/Checker-Finance/adapters/internal/store"
)

// BalanceHandler handles GET /api/v1/balances/:client_id.
type BalanceHandler struct {
	store store.Store
}

// NewBalanceHandler constructs a BalanceHandler.
func NewBalanceHandler(st store.Store) *BalanceHandler {
	return &BalanceHandler{store: st}
}

// GetBalances returns the cached balance snapshot for a client.
func (h *BalanceHandler) GetBalances(c *fiber.Ctx) error {
	clientID := c.Params("client_id")
	if clientID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "missing client_id"})
	}

	rows, err := h.store.GetClientBalances(context.Background(), clientID)
	if err != nil {
		slog.Error("zodia.get_balances.failed",
			"client_id", clientID,
			"error", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusOK).JSON(rows)
}
