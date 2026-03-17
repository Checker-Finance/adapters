package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// ProductsHandler handles the GET /api/v1/products endpoint.
type ProductsHandler struct {
	store  store.Store
	venue  string
	logger *zap.Logger
}

// NewProductsHandler creates a new ProductsHandler.
func NewProductsHandler(st store.Store, venue string, logger *zap.Logger) *ProductsHandler {
	return &ProductsHandler{store: st, venue: venue, logger: logger}
}

// ListProducts returns the products catalog for the configured venue.
func (h *ProductsHandler) ListProducts(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
	defer cancel()

	products, err := h.store.ListProducts(ctx, h.venue)
	if err != nil {
		h.logger.Error("rio.list_products.failed", zap.String("venue", h.venue), zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"count":    len(products),
		"products": products,
	})
}
