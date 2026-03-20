package zodia

import (
	"strconv"
	"strings"
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

//
// ────────────────────────────────────────────────
//   Mapper – Converts between Zodia and Canonical
// ────────────────────────────────────────────────
//

// Mapper translates between Zodia-specific payloads and Checker's canonical domain models.
type Mapper struct{}

// NewMapper constructs a Mapper instance.
func NewMapper() *Mapper { return &Mapper{} }

//
// ────────────────────────────────────────────────
//   WS Price → Canonical Quote
// ────────────────────────────────────────────────
//

// MapWSPriceToQuote converts a Zodia WS price update to a canonical Quote.
func (m *Mapper) MapWSPriceToQuote(price WSPricePayload, req model.RFQRequest) *model.Quote {
	expiresAt := time.Time{}
	if price.ExpiresAt > 0 {
		expiresAt = time.Unix(price.ExpiresAt, 0).UTC()
	}

	side := strings.ToUpper(price.Side)
	p := price.Price
	if p == 0 {
		if strings.EqualFold(side, "BUY") {
			p = price.Ask
		} else {
			p = price.Bid
		}
	}

	return &model.Quote{
		ID:         price.QuoteID,
		TakerID:    req.ClientID,
		Instrument: FromZodiaPair(price.Instrument),
		Side:       side,
		Price:      p,
		Bid:        price.Bid,
		Ask:        price.Ask,
		Quantity:   price.Quantity,
		Currency:   quoteFromInstrument(price.Instrument),
		ExpiresAt:  expiresAt,
		Status:     "CREATED",
		Venue:      "ZODIA",
		Timestamp:  time.Now().UTC(),
	}
}

// quoteFromInstrument extracts the quote currency from a Zodia instrument like "USD.MXN" → "MXN".
func quoteFromInstrument(instrument string) string {
	parts := strings.Split(instrument, ".")
	if len(parts) == 2 {
		return parts[1]
	}
	return instrument
}

//
// ────────────────────────────────────────────────
//   WS Order Confirmation → Canonical TradeConfirmation
// ────────────────────────────────────────────────
//

// MapWSOrderToTrade converts a Zodia WS order confirmation to a canonical TradeConfirmation.
func (m *Mapper) MapWSOrderToTrade(confirm WSOrderConfirmPayload, clientID, quoteID string) *model.TradeConfirmation {
	executedAt := time.Now().UTC()
	if confirm.ExecutedAt > 0 {
		executedAt = time.Unix(confirm.ExecutedAt, 0).UTC()
	}

	return &model.TradeConfirmation{
		TradeID:         confirm.TradeID,
		ClientID:        clientID,
		Venue:           "ZODIA",
		Instrument:      FromZodiaPair(confirm.Instrument),
		Side:            strings.ToUpper(confirm.Side),
		Quantity:        confirm.Quantity,
		Price:           confirm.Price,
		Status:          NormalizeTransactionState(confirm.Status),
		ExecutedAt:      executedAt,
		ProviderOrderID: confirm.TradeID,
		ProviderRFQID:   quoteID,
		RawPayload:      "",
	}
}

//
// ────────────────────────────────────────────────
//   Transaction → Canonical TradeConfirmation
// ────────────────────────────────────────────────
//

// MapTransactionToTrade converts a Zodia transaction to a canonical TradeConfirmation.
func (m *Mapper) MapTransactionToTrade(tx *ZodiaTransaction, clientID string) *model.TradeConfirmation {
	if tx == nil {
		return nil
	}

	createdAt := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, tx.CreatedAt); err == nil {
		createdAt = t
	}

	executedAt := createdAt
	if t, err := time.Parse(time.RFC3339, tx.UpdatedAt); err == nil && !t.IsZero() {
		executedAt = t
	}

	return &model.TradeConfirmation{
		TradeID:         tx.TradeID,
		ClientID:        clientID,
		Venue:           "ZODIA",
		Instrument:      FromZodiaPair(tx.Instrument),
		Side:            strings.ToUpper(tx.Side),
		Quantity:        tx.Quantity,
		Price:           tx.Price,
		Status:          NormalizeTransactionState(tx.State),
		ExecutedAt:      executedAt,
		ProviderOrderID: tx.TradeID,
		ProviderRFQID:   "",
		RawPayload:      "",
	}
}

//
// ────────────────────────────────────────────────
//   Account Balances → Canonical Balances
// ────────────────────────────────────────────────
//

// MapAccountToBalances converts a ZodiaAccountResponse to canonical Balances.
func (m *Mapper) MapAccountToBalances(resp *ZodiaAccountResponse, clientID string) []model.Balance {
	balances := make([]model.Balance, 0, len(resp.Result))
	for currency, b := range resp.Result {
		available, _ := strconv.ParseFloat(b.Available, 64)
		orders, _ := strconv.ParseFloat(b.Orders, 64)
		balances = append(balances, model.Balance{
			ClientID:    clientID,
			Venue:       "ZODIA",
			Instrument:  strings.ToUpper(currency),
			Available:   available,
			Held:        orders,
			Total:       available + orders,
			CanBuy:      true,
			CanSell:     true,
			LastUpdated: time.Now().UTC(),
		})
	}
	return balances
}

//
// ────────────────────────────────────────────────
//   Instrument → Canonical Product
// ────────────────────────────────────────────────
//

// MapInstrumentToProduct converts a Zodia instrument to a canonical Product.
func (m *Mapper) MapInstrumentToProduct(instr ZodiaInstrument) model.Product {
	return model.Product{
		VenueCode:        "ZODIA",
		InstrumentSymbol: FromZodiaPair(instr.Symbol),
		ProductID:        instr.Symbol,
		ProductName:      FromZodiaPair(instr.Symbol),
	}
}

//
// ────────────────────────────────────────────────
//   Webhook Event → ZodiaTransaction
// ────────────────────────────────────────────────
//

// WebhookToTransaction converts a ZodiaWebhookEvent to a ZodiaTransaction.
func (m *Mapper) WebhookToTransaction(event *ZodiaWebhookEvent) *ZodiaTransaction {
	if event == nil {
		return nil
	}
	return &ZodiaTransaction{
		UUID:         event.UUID,
		Type:         event.Type,
		State:        event.State,
		TradeID:      event.TradeID,
		Instrument:   event.Instrument,
		Side:         event.Side,
		Quantity:     event.Quantity,
		Price:        event.Price,
		DealtAmount:  event.DealtAmount,
		ContraAmount: event.ContraAmount,
		CreatedAt:    event.CreatedAt,
		UpdatedAt:    event.UpdatedAt,
	}
}
