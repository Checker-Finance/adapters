package api

import (
	"context"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/gofiber/fiber/v2"
)

// ProductLister is satisfied by zodia.Service.
type ProductLister interface {
	ListProducts(ctx context.Context, clientID string) []model.Product
}

// ProductsHandler handles the GET /api/v1/products endpoint.
type ProductsHandler struct {
	service ProductLister
}

// NewProductsHandler creates a new ProductsHandler.
func NewProductsHandler(svc ProductLister) *ProductsHandler {
	return &ProductsHandler{service: svc}
}

// ListProducts returns the list of Zodia supported products.
func (h *ProductsHandler) ListProducts(c *fiber.Ctx) error {
	clientID := c.Query("clientId", "")
	products := h.service.ListProducts(c.Context(), clientID)
	return c.JSON(fiber.Map{
		"count":    len(products),
		"products": products,
	})
}
