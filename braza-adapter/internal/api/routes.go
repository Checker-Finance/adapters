package api

import "github.com/gofiber/fiber/v2"

func RegisterRoutes(app *fiber.App, h *Handler, ph *ProductsHandler, oh *OrderResolveHandler) {
	app.Post("/quotes", h.CreateRFQHandler)
	app.Post("/orders", h.ExecuteRFQHandler)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).SendString("ok")
	})
	v1 := app.Group("/api/v1")
	v1.Get("/balances/:client_id", h.GetBalances)
	v1.Post("/quotes", h.CreateRFQHandler)
	v1.Post("/quotes/:quotation_id/execute", h.ExecuteRFQHandler)
	v1.Post("/resolve-order/:quoteId", oh.ResolveOrder)
	v1.Get("/products", ph.ListProducts)
}
