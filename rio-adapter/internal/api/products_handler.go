package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/gofiber/fiber/v2"
)

// ProductsHandler handles the GET /api/v1/products endpoint.
type ProductsHandler struct {
	store store.Store
	venue string
}

// NewProductsHandler creates a new ProductsHandler.
func NewProductsHandler(st store.Store, venue string) *ProductsHandler {
	return &ProductsHandler{store: st, venue: venue}
}

// ListProducts returns the products catalog for the configured venue.
func (h *ProductsHandler) ListProducts(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
	defer cancel()

	products, err := h.store.ListProducts(ctx, h.venue)
	if err != nil {
		slog.Error("rio.list_products.failed", "venue", h.venue, "error", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"count":    len(products),
		"products": products,
	})
}
