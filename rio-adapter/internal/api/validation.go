package api

import (
	"fmt"
	"strings"
)

func (r RFQCreateRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("quoteId is required")
	}
	if strings.TrimSpace(r.ClientID) == "" {
		return fmt.Errorf("clientId is required")
	}
	if strings.TrimSpace(r.CurrencyPair) == "" {
		return fmt.Errorf("pair is required")
	}
	if strings.TrimSpace(r.AmountDenomination) == "" {
		return fmt.Errorf("amountDenomination is required")
	}
	side := strings.ToLower(strings.TrimSpace(r.Side))
	if side != "buy" && side != "sell" {
		return fmt.Errorf("orderSide must be 'buy' or 'sell'")
	}
	if r.Amount <= 0 {
		return fmt.Errorf("quantity must be greater than 0")
	}
	return nil
}

func (r RFQExecuteRequest) Validate() error {
	if strings.TrimSpace(r.ClientID) == "" {
		return fmt.Errorf("clientId is required")
	}
	return nil
}
