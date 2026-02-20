package api

import (
	"context"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/store"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func RegisterRoutes(app *fiber.App, nc *nats.Conn, st store.Store,
	rioHandler *RioHandler,
	orderResolveHandler *OrderResolveHandler,
	webhookHandler *rio.WebhookHandler,
) {
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
	v1.Post("/quotes", rioHandler.CreateRFQHandler)
	v1.Post("/orders", rioHandler.ExecuteRFQHandler)
	v1.Post("/resolve-order/:quoteId", orderResolveHandler.ResolveOrder)

	// Webhook route
	app.Post("/webhooks/rio/orders", webhookHandler.HandleOrderWebhook)
}
