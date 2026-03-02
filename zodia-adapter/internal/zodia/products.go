package zodia

import (
	"time"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// zodiaSupportedProducts is the static list of Zodia Markets supported currency pairs.
// These are the RFS-eligible instruments. Verify and expand from GET /zm/rest/available-instruments.
var zodiaSupportedProducts = []model.Product{
	{VenueCode: "ZODIA", InstrumentSymbol: "USD:MXN", ProductID: "USD.MXN", ProductName: "USD/MXN", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "USD:COP", ProductID: "USD.COP", ProductName: "USD/COP", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "USD:BRL", ProductID: "USD.BRL", ProductName: "USD/BRL", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "BTC:USD", ProductID: "BTC.USD", ProductName: "BTC/USD", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "ETH:USD", ProductID: "ETH.USD", ProductName: "ETH/USD", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "BTC:USDC", ProductID: "BTC.USDC", ProductName: "BTC/USDC", AsOf: time.Time{}},
	{VenueCode: "ZODIA", InstrumentSymbol: "ETH:USDC", ProductID: "ETH.USDC", ProductName: "ETH/USDC", AsOf: time.Time{}},
}
