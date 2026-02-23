package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SettlementType defines the settlement convention for an instrument or trade.
// It captures how many days after trade date the value date occurs.
type SettlementType string

const (
	SettlementTypeTOD     SettlementType = "TOD"     // Today (T+0)
	SettlementTypeTOM     SettlementType = "TOM"     // Tomorrow (T+1)
	SettlementTypeSPOT    SettlementType = "SPOT"    // Standard spot (usually T+2)
	SettlementTypeFORWARD SettlementType = "FORWARD" // Any settlement beyond spot
	SettlementTypeNDF     SettlementType = "NDF"     // Non-Deliverable Forward
	SettlementTypeSWAP    SettlementType = "SWAP"    // FX swap or related leg
)

// Valid returns true if the settlement type is one of the known constants.
func (t SettlementType) Valid() bool {
	switch t {
	case SettlementTypeTOD,
		SettlementTypeTOM,
		SettlementTypeSPOT,
		SettlementTypeFORWARD,
		SettlementTypeNDF,
		SettlementTypeSWAP:
		return true
	default:
		return false
	}
}

func (t SettlementType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(t))
}

func (t *SettlementType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "TOD":
		*t = SettlementTypeTOD
	case "TOM":
		*t = SettlementTypeTOM
	case "SPOT":
		*t = SettlementTypeSPOT
	case "FORWARD":
		*t = SettlementTypeFORWARD
	case "NDF":
		*t = SettlementTypeNDF
	case "SWAP":
		*t = SettlementTypeSWAP
	default:
		return fmt.Errorf("invalid settlement type: %s", s)
	}
	return nil
}

func (t SettlementType) String() string {
	return string(t)
}

// FromTenorDays derives a SettlementType from tenor days (T+N).
func FromTenorDays(days int) SettlementType {
	switch {
	case days <= 0:
		return SettlementTypeTOD
	case days == 1:
		return SettlementTypeTOM
	case days == 2:
		return SettlementTypeSPOT
	default:
		return SettlementTypeFORWARD
	}
}

// FromValueDate derives a SettlementType from trade and value dates.
func FromValueDate(tradeDate, valueDate time.Time) SettlementType {
	days := int(valueDate.Sub(tradeDate).Hours() / 24)
	return FromTenorDays(days)
}

func (t SettlementType) IsSpotOrFwd() bool {
	return t == SettlementTypeSPOT || t == SettlementTypeFORWARD
}

func (t SettlementType) IsSameDay() bool {
	return t == SettlementTypeTOD
}
