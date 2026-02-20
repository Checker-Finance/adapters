package model

import (
	"fmt"
	"time"
)

// CanonicalSettlement defines normalized settlement information
// for an instrument or trade, independent of venue format.
type CanonicalSettlement struct {
	ValueDate         time.Time      `json:"value_date"`          // The settlement date (value date)
	TenorDays         int            `json:"tenor_days"`          // Number of days from trade date to value date
	AllowedWindowDays int            `json:"allowed_window_days"` // Typically derived from "available_slip_day"
	MaxWindowDays     int            `json:"max_window_days"`     // Derived from "max_slip_day"
	Type              SettlementType `json:"type"`                // e.g. SPOT, TOM, FORWARD, NDF, etc.
	Instrument        string         `json:"instrument"`          // e.g. "USDBRL"
	Venue             string         `json:"venue"`               // e.g. "BRAZA"
	UpdatedAt         time.Time      `json:"updated_at"`          // Timestamp when this settlement was last refreshed
}

// NewSettlement creates a normalized CanonicalSettlement
// given trade date, tenor, and metadata.
func NewSettlement(venue, instrument string, tradeDate time.Time, tenorDays, allowed, max int) CanonicalSettlement {
	valDate := tradeDate.AddDate(0, 0, tenorDays)
	return CanonicalSettlement{
		Venue:             venue,
		Instrument:        instrument,
		ValueDate:         valDate,
		TenorDays:         tenorDays,
		AllowedWindowDays: allowed,
		MaxWindowDays:     max,
		Type:              FromTenorDays(tenorDays),
		UpdatedAt:         time.Now().UTC(),
	}
}

// DaysUntil returns the number of days from now until settlement date.
func (s CanonicalSettlement) DaysUntil() int {
	d := int(s.ValueDate.Sub(time.Now().UTC()).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// IsExpired returns true if the value date is before the current date.
func (s CanonicalSettlement) IsExpired() bool {
	return time.Now().UTC().After(s.ValueDate)
}

// EffectiveWindow checks whether the settlement window is still open.
func (s CanonicalSettlement) EffectiveWindow() bool {
	return s.DaysUntil() <= s.AllowedWindowDays
}

// Normalize ensures fields are internally consistent.
func (s *CanonicalSettlement) Normalize() {
	if !s.Type.Valid() {
		s.Type = FromTenorDays(s.TenorDays)
	}
	if s.ValueDate.IsZero() && s.TenorDays > 0 {
		s.ValueDate = time.Now().UTC().AddDate(0, 0, s.TenorDays)
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now().UTC()
	}
}

// Valid returns an error if settlement parameters are inconsistent or invalid.
func (s CanonicalSettlement) Valid() error {
	if s.Venue == "" {
		return fmt.Errorf("missing venue")
	}
	if s.Instrument == "" {
		return fmt.Errorf("missing instrument")
	}
	if s.TenorDays < 0 {
		return fmt.Errorf("invalid tenor_days: %d", s.TenorDays)
	}
	if !s.Type.Valid() {
		return fmt.Errorf("invalid settlement type: %s", s.Type)
	}
	return nil
}

func (s CanonicalSettlement) IsSpot() bool {
	return s.Type == SettlementTypeSPOT
}

func (s CanonicalSettlement) IsForward() bool {
	return s.Type == SettlementTypeFORWARD || s.Type == SettlementTypeNDF
}

func (s CanonicalSettlement) IsSwap() bool {
	return s.Type == SettlementTypeSWAP
}

func (s CanonicalSettlement) IsTomNext() bool {
	return s.Type == SettlementTypeTOM || s.Type == SettlementTypeTOD
}

func (s CanonicalSettlement) String() string {
	return fmt.Sprintf("[%s %s] %s (%s, T+%d, allowed=%d, max=%d, value=%s)",
		s.Venue,
		s.Instrument,
		s.Type,
		s.ValueDate.Format("2006-01-02"),
		s.TenorDays,
		s.AllowedWindowDays,
		s.MaxWindowDays,
		s.UpdatedAt.Format(time.RFC3339),
	)
}
