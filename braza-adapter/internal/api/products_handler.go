package api

import (
	"context"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/gofiber/fiber/v2"
)

type ProductsHandler struct {
	Service *braza.Service
	cfg     *config.Config
}

func NewProductsHandler(svc *braza.Service, cfg *config.Config) *ProductsHandler {
	return &ProductsHandler{Service: svc, cfg: cfg}
}

// GET /api/v1/products
func (h *ProductsHandler) ListProducts(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
	defer cancel()

	venue := c.Query("venue", "braza")

	products, err := h.Service.ListProducts(ctx, h.cfg.ClientInstrumentID, venue)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.JSON(fiber.Map{
		"count":    len(products),
		"products": products,
	})
}
