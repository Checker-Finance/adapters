package xfx

import (
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// xfxSupportedProducts is the static list of XFX supported currency pairs.
// XFX does not expose a products API; the supported pairs are fixed.
// Minimum trade size: $100,000 USD per pair.
var xfxSupportedProducts = []model.Product{
	{VenueCode: "XFX", InstrumentSymbol: "USD/MXN", ProductID: "USD/MXN", ProductName: "USD/MXN", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USDT/MXN", ProductID: "USDT/MXN", ProductName: "USDT/MXN", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USDC/MXN", ProductID: "USDC/MXN", ProductName: "USDC/MXN", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USD/COP", ProductID: "USD/COP", ProductName: "USD/COP", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USDT/COP", ProductID: "USDT/COP", ProductName: "USDT/COP", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USDC/COP", ProductID: "USDC/COP", ProductName: "USDC/COP", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USD/USDT", ProductID: "USD/USDT", ProductName: "USD/USDT", AsOf: time.Time{}},
	{VenueCode: "XFX", InstrumentSymbol: "USD/USDC", ProductID: "USD/USDC", ProductName: "USD/USDC", AsOf: time.Time{}},
}
