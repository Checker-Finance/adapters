package api

import (
	"context"
	"time"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes registers all HTTP routes on the Fiber app.
func RegisterRoutes(app *fiber.App, nc *nats.Conn, st store.Store, xfxHandler *XFXHandler, resolveHandler *OrderResolveHandler) {
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		checks := map[string]string{
			"nats":  "ok",
			"store": "ok",
		}
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

		healthCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := st.HealthCheck(healthCtx); err != nil {
			checks["store"] = err.Error()
			status = "degraded"
			code = fiber.StatusServiceUnavailable
		}

		return c.Status(code).JSON(fiber.Map{
			"status": status,
			"checks": checks,
		})
	})

	// API routes
	v1 := app.Group("/api/v1")
	v1.Post("/quotes", xfxHandler.CreateRFQHandler)
	v1.Post("/orders", xfxHandler.ExecuteRFQHandler)
	v1.Post("/resolve-order/:quoteId", resolveHandler.ResolveOrder)
}
