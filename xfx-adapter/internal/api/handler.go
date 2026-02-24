package api

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// RFQService defines the interface for RFQ operations used by the handler.
type RFQService interface {
	CreateRFQ(ctx context.Context, req model.RFQRequest) (*model.Quote, error)
	ExecuteRFQ(ctx context.Context, clientID, quoteID string) (*model.TradeConfirmation, error)
}

// ClientValidator checks whether a client ID is configured and allowed.
type ClientValidator interface {
	IsKnownClient(ctx context.Context, clientID string) bool
}

// XFXHandler handles HTTP API requests for XFX operations.
type XFXHandler struct {
	logger    *zap.Logger
	service   RFQService
	validator ClientValidator
}

// NewXFXHandler creates a new XFXHandler.
func NewXFXHandler(logger *zap.Logger, service RFQService, validator ClientValidator) *XFXHandler {
	return &XFXHandler{
		logger:    logger,
		service:   service,
		validator: validator,
	}
}

// CreateRFQHandler handles quote creation requests.
func (h *XFXHandler) CreateRFQHandler(c *fiber.Ctx) error {
	var req RFQCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := req.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.validator != nil && !h.validator.IsKnownClient(c.Context(), req.ClientID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "unknown or unauthorized clientId"})
	}

	r := toRFQRequest(req)

	quote, err := h.service.CreateRFQ(c.Context(), r)
	if err != nil {
		h.logger.Error("xfx.create_rfq.failed",
			zap.String("client", req.ClientID),
			zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(RFQResponse{
			QuoteID:  req.ID,
			ErrorMsg: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(RFQResponse{
		QuoteID:         req.ID,
		ProviderQuoteId: quote.ID,
		Price:           quote.Price,
		ExpireAt:        quote.ExpiresAt.Unix(),
	})
}

// ExecuteRFQHandler handles quote execution requests.
func (h *XFXHandler) ExecuteRFQHandler(c *fiber.Ctx) error {
	var req RFQExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err := req.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if h.validator != nil && !h.validator.IsKnownClient(c.Context(), req.ClientID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "unknown or unauthorized clientId"})
	}

	// Resolve quote ID from URL param or body fields
	quoteID := c.Params("quotation_id")
	if quoteID == "" {
		quoteID = req.QuoteID
	}
	if quoteID == "" {
		quoteID = req.ProviderQuoteID
	}
	if quoteID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "quoteId is required"})
	}

	h.logger.Info("xfx.execute_rfq",
		zap.String("client", req.ClientID),
		zap.String("quote_id", quoteID))

	trade, err := h.service.ExecuteRFQ(c.Context(), req.ClientID, quoteID)
	if err != nil {
		h.logger.Error("xfx.execute_rfq.failed",
			zap.String("client", req.ClientID),
			zap.String("quote_id", quoteID),
			zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(RFQExecutionResponse{
			OrderID:  req.OrderID,
			ErrorMsg: err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(RFQExecutionResponse{
		OrderID:         req.OrderID,
		ProviderOrderID: trade.TradeID,
		Status:          trade.Status,
		Price:           trade.Price,
		ExecutedAt:      trade.ExecutedAt.Unix(),
	})
}

// toRFQRequest converts an API request to a canonical RFQRequest.
func toRFQRequest(req RFQCreateRequest) model.RFQRequest {
	return model.RFQRequest{
		ClientID:       req.ClientID,
		Side:           req.Side,
		CurrencyPair:   req.CurrencyPair,
		Amount:         req.Amount,
		CurrencyAmount: req.AmountDenomination,
	}
}
