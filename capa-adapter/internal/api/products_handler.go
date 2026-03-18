package api

import (
	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/gofiber/fiber/v2"
)

// ProductLister is satisfied by capa.Service.
type ProductLister interface {
	ListProducts() []model.Product
}

// ProductsHandler handles the GET /api/v1/products endpoint.
type ProductsHandler struct {
	service ProductLister
}

// NewProductsHandler creates a new ProductsHandler.
func NewProductsHandler(svc ProductLister) *ProductsHandler {
	return &ProductsHandler{service: svc}
}

// ListProducts returns the static list of Capa supported products.
func (h *ProductsHandler) ListProducts(c *fiber.Ctx) error {
	products := h.service.ListProducts()
	return c.JSON(fiber.Map{
		"count":    len(products),
		"products": products,
	})
}
