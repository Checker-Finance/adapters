package api

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/model"
)

// OrderResolverService fetches and normalizes a live transaction from XFX.
type OrderResolverService interface {
	ResolveTransaction(ctx context.Context, clientID, txID string) (*model.TradeConfirmation, error)
}

// TradeSync syncs a trade to the legacy database.
type TradeSync interface {
	SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error
}

// OrderResolveHandler resolves order status by quoteId, fetches live status
// from XFX, and syncs the result to the legacy database.
type OrderResolveHandler struct {
	logger    *zap.Logger
	service   OrderResolverService
	store     store.Store
	tradeSync TradeSync
}

// NewOrderResolveHandler creates a new OrderResolveHandler.
func NewOrderResolveHandler(
	logger *zap.Logger,
	service OrderResolverService,
	st store.Store,
	tradeSync TradeSync,
) *OrderResolveHandler {
	return &OrderResolveHandler{
		logger:    logger,
		service:   service,
		store:     st,
		tradeSync: tradeSync,
	}
}

// ResolveOrder handles POST /api/v1/resolve-order/:quoteId.
// It looks up the quote in the store, fetches the current transaction status
// from XFX, syncs the result to the legacy database, and returns the resolved state.
func (h *OrderResolveHandler) ResolveOrder(c *fiber.Ctx) error {
	ctx := context.Background()
	quoteID := c.Params("quoteId")

	if quoteID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "quoteId is required"})
	}

	// 1. Look up quote record in the store.
	qrec, err := h.store.GetQuoteByQuoteID(ctx, quoteID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if qrec == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "quote not found"})
	}

	// 2. Resolve the provider transaction ID via the RFQ.
	txID, err := h.store.GetOrderIDByRFQ(ctx, qrec.RFQID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if txID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "transaction ID not yet known for this quote"})
	}

	// 3. Fetch live status from XFX and map to canonical model.
	trade, err := h.service.ResolveTransaction(ctx, qrec.ClientID, txID)
	if err != nil {
		h.logger.Error("xfx.resolve_order.fetch_failed",
			zap.String("quote_id", quoteID),
			zap.String("tx_id", txID),
			zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch status from XFX"})
	}

	// 4. Sync to legacy database.
	if err := h.tradeSync.SyncTradeUpsert(ctx, trade); err != nil {
		h.logger.Warn("xfx.resolve_order.sync_failed",
			zap.String("quote_id", quoteID),
			zap.String("tx_id", txID),
			zap.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"quoteId":    quoteID,
			"rfqId":      qrec.RFQID,
			"txId":       txID,
			"status":     trade.Status,
			"synced":     false,
			"sync_error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"quoteId": quoteID,
		"rfqId":   qrec.RFQID,
		"txId":    txID,
		"status":  trade.Status,
		"synced":  true,
	})
}
