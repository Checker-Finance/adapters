package api

import (
	"testing"
)

func TestRFQCreateRequest_Validate(t *testing.T) {
	valid := RFQCreateRequest{
		ID:                 "q-1",
		ClientID:           "client-1",
		CurrencyPair:       "USD:BRL",
		AmountDenomination: "BRL",
		Side:               "buy",
		Amount:             1000,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(r *RFQCreateRequest)
		wantErr string
	}{
		{
			name:    "missing quoteId",
			mutate:  func(r *RFQCreateRequest) { r.ID = "" },
			wantErr: "quoteId is required",
		},
		{
			name:    "whitespace quoteId",
			mutate:  func(r *RFQCreateRequest) { r.ID = "   " },
			wantErr: "quoteId is required",
		},
		{
			name:    "missing clientId",
			mutate:  func(r *RFQCreateRequest) { r.ClientID = "" },
			wantErr: "clientId is required",
		},
		{
			name:    "missing pair",
			mutate:  func(r *RFQCreateRequest) { r.CurrencyPair = "" },
			wantErr: "pair is required",
		},
		{
			name:    "missing amountDenomination",
			mutate:  func(r *RFQCreateRequest) { r.AmountDenomination = "" },
			wantErr: "amountDenomination is required",
		},
		{
			name:    "invalid side",
			mutate:  func(r *RFQCreateRequest) { r.Side = "hold" },
			wantErr: "orderSide must be 'buy' or 'sell'",
		},
		{
			name:    "empty side",
			mutate:  func(r *RFQCreateRequest) { r.Side = "" },
			wantErr: "orderSide must be 'buy' or 'sell'",
		},
		{
			name:    "zero amount",
			mutate:  func(r *RFQCreateRequest) { r.Amount = 0 },
			wantErr: "quantity must be greater than 0",
		},
		{
			name:    "negative amount",
			mutate:  func(r *RFQCreateRequest) { r.Amount = -100 },
			wantErr: "quantity must be greater than 0",
		},
		{
			name:   "sell side accepted",
			mutate: func(r *RFQCreateRequest) { r.Side = "sell" },
		},
		{
			name:   "BUY uppercase accepted",
			mutate: func(r *RFQCreateRequest) { r.Side = "BUY" },
		},
		{
			name:   "SELL uppercase accepted",
			mutate: func(r *RFQCreateRequest) { r.Side = "SELL" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid // copy
			tt.mutate(&r)
			err := r.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestRFQExecuteRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     RFQExecuteRequest
		wantErr string
	}{
		{
			name:    "valid request",
			req:     RFQExecuteRequest{ClientID: "client-1"},
			wantErr: "",
		},
		{
			name:    "missing clientId",
			req:     RFQExecuteRequest{ClientID: ""},
			wantErr: "clientId is required",
		},
		{
			name:    "whitespace clientId",
			req:     RFQExecuteRequest{ClientID: "   "},
			wantErr: "clientId is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
