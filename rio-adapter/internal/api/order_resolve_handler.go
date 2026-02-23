package api

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// OrderResolverService defines the interface for fetching and mapping order data.
type OrderResolverService interface {
	FetchTradeStatus(ctx context.Context, clientID, orderID string) (*rio.RioOrderResponse, error)
	BuildTradeConfirmationFromOrder(clientID, orderID string, order *rio.RioOrderResponse) *model.TradeConfirmation
}

// TradeSync defines the interface for syncing trades to the legacy database.
type TradeSync interface {
	SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error
}

// OrderResolveHandler resolves order status by quoteId, fetches live
// status from Rio, and syncs the result to the legacy database.
type OrderResolveHandler struct {
	Logger    *zap.Logger
	Service   OrderResolverService
	Store     store.Store
	TradeSync TradeSync
}

// ResolveOrder handles POST /api/v1/resolve-order/:quoteId.
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
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if qrec == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "quote not found"})
	}

	// 2. Get provider order ID from RFQ
	orderID, err := h.Store.GetOrderIDByRFQ(ctx, qrec.RFQID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if orderID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "orderId not yet known for RFQ"})
	}

	// 3. Fetch live status from Rio (no credential resolution needed; API key auth)
	orderResp, err := h.Service.FetchTradeStatus(ctx, qrec.ClientID, qrec.ProviderOrderID)
	if err != nil {
		h.Logger.Error("failed to fetch order status from Rio",
			zap.String("quote_id", quoteID),
			zap.String("provider_order_id", qrec.ProviderOrderID),
			zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch status from Rio"})
	}

	normalized := rio.NormalizeRioStatus(orderResp.Status)

	// 4. Build TradeConfirmation + sync to legacy t_order
	trade := h.Service.BuildTradeConfirmationFromOrder(qrec.ClientID, orderID, orderResp)
	if trade == nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "could not build trade confirmation"})
	}

	if err := h.TradeSync.SyncTradeUpsert(ctx, trade); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"quoteId":    quoteID,
			"rfqId":      qrec.RFQID,
			"orderId":    orderID,
			"status":     normalized,
			"synced":     false,
			"sync_error": err.Error(),
		})
	}

	// 5. Return resolved status
	return c.JSON(fiber.Map{
		"quoteId": quoteID,
		"rfqId":   qrec.RFQID,
		"orderId": orderID,
		"status":  normalized,
		"synced":  true,
	})
}
