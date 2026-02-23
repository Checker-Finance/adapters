package braza

import (
	"strconv"
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
	"github.com/gofiber/fiber/v2/log"
)

//
// ────────────────────────────────────────────────
//   Mapper – Converts between Braza and Canonical
// ────────────────────────────────────────────────
//

// Mapper translates between Braza-specific payloads and Checker’s canonical domain models.
type Mapper struct{}

// NewMapper constructs a Mapper instance.
func NewMapper() *Mapper { return &Mapper{} }

//
// ────────────────────────────────────────────────
//   Helper functions
// ────────────────────────────────────────────────
//

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

//
// ────────────────────────────────────────────────
//   Balances
// ────────────────────────────────────────────────
//
// Braza → Canonical
// Braza’s balance response contains a list of currencies with totals and availables.
// We normalize them to Checker’s canonical model.Balance.
//

func (m *Mapper) FromBrazaBalances(resp BrazaBalancesResponse, clientID string) []model.Balance {
	balances := make([]model.Balance, 0)

	for _, item := range resp {
		for symbol, data := range item {
			balances = append(balances, model.Balance{
				ClientID:    clientID,
				Instrument:  symbol,
				CanBuy:      data.CanBuy,
				CanSell:     data.CanSell,
				Available:   data.AvailableTotalValueDay,
				Held:        0.00, // Braza doesn't return this directly
				Total:       data.TotalValueDay,
				Venue:       "braza",
				LastUpdated: time.Now().UTC(),
			})
		}
	}

	return balances
}

//
// ────────────────────────────────────────────────
//   RFQ (Preview Quotation)
// ────────────────────────────────────────────────
//
// Canonical → Braza
// Checker’s canonical RFQRequest is mapped into Braza’s `preview-quotation` request format.
//

func (m *Mapper) ToBrazaRFQ(req model.RFQRequest) BrazaRFQRequest {
	side := strings.ToLower(req.Side)
	if side != "buy" && side != "sell" {
		side = "buy" // default fallback
	}

	pair := NormalizePairForBraza(req.CurrencyPair)
	var currency string
	if req.CurrencyAmount != "" {
		currency = req.CurrencyAmount
	} else {
		parts := strings.Split(pair, ":")
		if len(parts) != 2 {
			log.Info("Invalid currency pair")
		}
		currency = parts[1]
		log.Infof("normalized currency pair to: %s:%s", parts[0], parts[1])
	}

	return BrazaRFQRequest{
		CurrencyAmount: currency,                    // e.g. "USDC"
		Amount:         req.Amount,                  // e.g. 1000
		Currency:       NormalizePairForBraza(pair), // "USDC:BRL"
		Side:           side,                        // "buy" or "sell"
	}
}

func NormalizePairForBraza(pair string) string {
	// "usdt/brl" → "USDT:BRL"
	if pair == "" {
		return ""
	}

	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(pair, "/", ":"), "_", ":"))
}

func (m *Mapper) FromBrazaProduct(product BrazaProductDef) model.Product {
	p := model.Product{
		VenueCode:        "braza",
		ProductID:        parseIntString(product.ID),
		InstrumentSymbol: product.Par,
		ProductName:      product.Nome,
		SecondaryID:      parseIntString(product.IDProductCompany),
		IsBlocked:        false,
		AsOf:             time.Now(),
	}

	return p
}

//
// Braza → Canonical
// Braza returns a quote preview response with values as strings.
// We normalize that into model.Quote.
//

func (m *Mapper) FromBrazaQuote(resp BrazaQuoteResponse, clientID string) model.Quote {
	return model.Quote{
		ID:        resp.ID,
		TakerID:   clientID,
		Price:     parseFloat(resp.Quote),
		Bid:       parseFloat(resp.FinalQuote),
		Ask:       parseFloat(resp.FinalQuote),
		Status:    resp.Status,
		Timestamp: time.Now().UTC(),
		Venue:     "BRAZA",
	}
}

//
// ────────────────────────────────────────────────
//   Executions / Trades
// ────────────────────────────────────────────────
//
// Braza → Canonical
// Braza’s trade execution response (ExecuteResponse) maps to Checker’s canonical TradeConfirmation.
//

func (m *Mapper) FromBrazaExecution(resp BrazaExecuteResponse, clientID string) model.TradeConfirmation {
	return model.TradeConfirmation{
		ClientID:      clientID,
		ExecutionTime: time.Now().UTC(),
		Venue:         "BRAZA",
	}
}

//
// ────────────────────────────────────────────────
//   API Presentation Mappers
// ────────────────────────────────────────────────
//
// Canonical → API Response
// Used by Fiber REST handlers to shape canonical data into API-facing JSON.
//

func (m *Mapper) ToAPIQuoteResponse(q model.Quote) map[string]any {
	return map[string]any{
		"id":         q.ID,
		"tenant_id":  q.TenantID,
		"instrument": q.Instrument,
		"side":       q.Side,
		"status":     q.Status,
		"venue":      q.Venue,
		"timestamp":  q.Timestamp,
	}
}

func NormalizeOrderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "processing", "processando":
		return "submitted"
	case "finalizado", "completed", "complete", "executado":
		return "filled"
	case "rejeitado", "rejected":
		return "Rejected"
	case "cancelado", "cancelled", "canceled":
		return "cancelled"
	case "erro", "error":
		return "error"
	case "pendente", "pending":
		return "pending"
	default:
		return status // pass through unknown values
	}
}
