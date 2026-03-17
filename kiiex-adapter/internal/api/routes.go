package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes registers all HTTP routes on the Fiber app.
func RegisterRoutes(app *fiber.App, h *KiiexHandler, nc *nats.Conn) {
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	app.Get("/health", func(c *fiber.Ctx) error {
		checks := map[string]string{"nats": "ok"}
		status := "ok"
		code := fiber.StatusOK

		if nc == nil || !nc.IsConnected() {
			checks["nats"] = "disconnected"
			status = "degraded"
			code = fiber.StatusServiceUnavailable
		} else if err := nc.FlushTimeout(1 * time.Second); err != nil {
			checks["nats"] = err.Error()
			status = "degraded"
			code = fiber.StatusServiceUnavailable
		}

		return c.Status(code).JSON(fiber.Map{
			"status": status,
			"checks": checks,
		})
	})

	v1 := app.Group("/api/v1")
	v1.Post("/orders", h.ExecuteOrderHandler)
}
