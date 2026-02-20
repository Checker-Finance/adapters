package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
)

// --- RFQ Execution Mapping ---
func mapFromCanonicalTradeConfirmation(t model.TradeConfirmation) RFQExecutionResponse {
	return RFQExecutionResponse{
		Status:  t.Status,
		OrderID: t.OrderID,
	}
}

// --- Trade Mapping ---
func mapToCanonicalTrade(req TradeCreateRequest) model.TradeCommand {
	return model.TradeCommand{
		TenantID:   req.TenantID,
		ClientID:   req.ClientID,
		Instrument: req.Instrument,
		Side:       req.Side,
		Quantity:   req.Quantity,
		Price:      req.Price,
		QuoteID:    req.QuoteID,
		ExternalID: req.ExternalID,
	}
}

func mapFromCanonicalTrade(t model.TradeConfirmation) TradeResponse {
	return TradeResponse{
		OrderID:        t.OrderID,
		Instrument:     t.Instrument,
		Side:           t.Side,
		Quantity:       t.Quantity,
		ExecutionPrice: t.Price,
		Status:         t.Status,
	}
}

func fromBrazaProduct(p BrazaProduct) model.Product {
	instr := strings.ToUpper(p.Par)
	if len(instr) >= 6 {
		instr = instr[:3] + "_" + instr[3:]
	}
	return model.Product{
		VenueCode:        "braza",
		InstrumentSymbol: instr,
		ProductID:        parseIntString(p.ID),
		ProductName:      p.Nome,
		SecondaryID:      parseIntString(p.IDProductCompany),
		IsBlocked:        p.FlagBloqueado == 1,
		AsOf:             time.Now().UTC(),
	}
}

func parseInt(s string) int64 {
	f, _ := strconv.ParseInt(strings.TrimSpace(s), 64, 8)
	return f
}

func parseIntString(i int) string {
	str := strconv.FormatInt(int64(i), 10)
	return str
}
