package api

import "fmt"

// Validate checks that RFQCreateRequest has all required fields.
func (r *RFQCreateRequest) Validate() error {
	if r.ClientID == "" {
		return fmt.Errorf("clientId is required")
	}
	if r.CurrencyPair == "" {
		return fmt.Errorf("pair is required")
	}
	if r.Side == "" {
		return fmt.Errorf("orderSide is required")
	}
	if r.Amount <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	return nil
}

// Validate checks that RFQExecuteRequest has all required fields.
func (r *RFQExecuteRequest) Validate() error {
	if r.ClientID == "" {
		return fmt.Errorf("clientId is required")
	}
	return nil
}
