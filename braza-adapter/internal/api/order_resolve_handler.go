package api

import (
	"context"
	"net/http"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type OrderResolveHandler struct {
	Logger    *zap.Logger
	Service   *braza.Service
	Store     store.Store
	TradeSync *legacy.TradeSyncWriter
}

func (h *OrderResolveHandler) ResolveOrder(c *fiber.Ctx) error {
	ctx := context.Background()
	quoteID := c.Params("quoteId")

	if quoteID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "quoteId is required",
		})
	}

	// 1. Get Quote
	qrec, err := h.Store.GetQuoteByQuoteID(ctx, quoteID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if qrec == nil {
		return c.Status(404).JSON(fiber.Map{"error": "quote not found"})
	}

	// 2. Get provider order ID from RFQ
	orderID, err := h.Store.GetOrderIDByRFQ(ctx, qrec.RFQID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if orderID == "" {
		return c.Status(404).JSON(fiber.Map{"error": "orderId not yet known for RFQ"})
	}

	// 3. Resolve credentials
	credsMap, err := h.Service.Resolver().Resolve(ctx, qrec.ClientID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to resolve credentials"})
	}

	creds := h.Service.BuildCredentials(credsMap)

	// 4. Fetch Braza status
	orderStatus, err := h.Service.FetchTradeStatus(ctx, qrec.ClientID, qrec.ProviderOrderID, creds)
	if err != nil {
		h.Logger.Error("failed to fetch order status", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch status from Braza"})
	}

	normalized := braza.NormalizeOrderStatus(orderStatus.Status)

	// 5. Build TradeConfirmation + sync to t_order
	trade := h.Service.BuildTradeConfirmationFromOrder(qrec.ClientID, orderID, orderStatus)
	if trade == nil {
		return c.Status(500).JSON(fiber.Map{"error": "could not build trade confirmation"})
	}

	err = h.TradeSync.SyncTradeUpsert(ctx, trade)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"quoteId":    quoteID,
			"rfqId":      qrec.RFQID,
			"orderId":    orderID,
			"status":     normalized,
			"synced":     false,
			"sync_error": err.Error(),
		})
	}

	// 6. Return combined response
	return c.JSON(fiber.Map{
		"quoteId": quoteID,
		"rfqId":   qrec.RFQID,
		"orderId": orderID,
		"status":  normalized,
		"synced":  true,
	})
}
