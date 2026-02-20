package model

import "time"

// Product represents a normalized venue-specific product definition
// stored in reference.venue_products and shared across the platform.
type Product struct {
	VenueCode        string    `json:"venue_code"`           // e.g. "BRAZA"
	InstrumentSymbol string    `json:"instrument_symbol"`    // e.g. "USDC/BRL"
	ProductID        string    `json:"product_id"`           // 190
	SecondaryID      string    `json:"product_secondary_id"` // 50396
	ProductName      string    `json:"product_name"`         // "CRYPTO D0/D0"
	IsBlocked        bool      `json:"is_blocked"`           // true if blocked
	AsOf             time.Time `json:"as_of"`                // last update
}
