package capa

import (
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// capaSupportedProducts is the static list of Capa supported currency pairs.
// Capa supports LATAM fiat/crypto conversions across MXN, DOP, USD, EUR and
// multiple blockchains. This list covers the cross-ramp, on-ramp, and off-ramp flows.
var capaSupportedProducts = []model.Product{
	// Cross-ramp (fiat ↔ fiat)
	{VenueCode: "CAPA", InstrumentSymbol: "USD/MXN", ProductID: "USD/MXN", ProductName: "USD/MXN", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USD/DOP", ProductID: "USD/DOP", ProductName: "USD/DOP", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "EUR/MXN", ProductID: "EUR/MXN", ProductName: "EUR/MXN", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "EUR/DOP", ProductID: "EUR/DOP", ProductName: "EUR/DOP", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "MXN/DOP", ProductID: "MXN/DOP", ProductName: "MXN/DOP", AsOf: time.Time{}},
	// On-ramp (fiat → crypto)
	{VenueCode: "CAPA", InstrumentSymbol: "USD/USDC", ProductID: "USD/USDC", ProductName: "USD/USDC", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USD/USDT", ProductID: "USD/USDT", ProductName: "USD/USDT", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "MXN/USDC", ProductID: "MXN/USDC", ProductName: "MXN/USDC", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "MXN/USDT", ProductID: "MXN/USDT", ProductName: "MXN/USDT", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "DOP/USDC", ProductID: "DOP/USDC", ProductName: "DOP/USDC", AsOf: time.Time{}},
	// Off-ramp (crypto → fiat)
	{VenueCode: "CAPA", InstrumentSymbol: "USDC/MXN", ProductID: "USDC/MXN", ProductName: "USDC/MXN", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USDT/MXN", ProductID: "USDT/MXN", ProductName: "USDT/MXN", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USDC/USD", ProductID: "USDC/USD", ProductName: "USDC/USD", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USDT/USD", ProductID: "USDT/USD", ProductName: "USDT/USD", AsOf: time.Time{}},
	{VenueCode: "CAPA", InstrumentSymbol: "USDC/DOP", ProductID: "USDC/DOP", ProductName: "USDC/DOP", AsOf: time.Time{}},
}
