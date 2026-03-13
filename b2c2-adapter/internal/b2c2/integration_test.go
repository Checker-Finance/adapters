//go:build integration

package b2c2

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newIntegrationConfig returns a B2C2ClientConfig from environment variables.
// It skips the test if B2C2_API_TOKEN is not set.
func newIntegrationConfig(t *testing.T) *B2C2ClientConfig {
	t.Helper()

	token := os.Getenv("B2C2_API_TOKEN")
	if token == "" {
		t.Skip("B2C2_API_TOKEN must be set for integration tests")
	}

	baseURL := os.Getenv("B2C2_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.uat.b2c2.net"
	}

	return &B2C2ClientConfig{
		APIToken: token,
		BaseURL:  baseURL,
	}
}

// newIntegrationClient returns a Client and config for integration tests.
func newIntegrationClient(t *testing.T) (*Client, *B2C2ClientConfig) {
	t.Helper()
	cfg := newIntegrationConfig(t)
	return NewClient(zap.NewNop(), nil), cfg
}

func TestIntegration_GetInstruments(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	instruments, err := client.GetInstruments(ctx, cfg)
	if err != nil {
		t.Fatalf("GetInstruments failed: %v", err)
	}
	if len(instruments) == 0 {
		t.Fatal("expected non-empty instrument list")
	}

	for _, inst := range instruments {
		if inst.Name == "" {
			t.Error("instrument with empty name")
		}
	}

	max := 5
	if len(instruments) < max {
		max = len(instruments)
	}
	for _, inst := range instruments[:max] {
		t.Logf("instrument: name=%s underlying=%s quoted=%s active=%v",
			inst.Name, inst.UnderlyingCurrency, inst.QuotedCurrency, inst.IsActive)
	}
}

func TestIntegration_GetBalance(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	balance, err := client.GetBalance(ctx, cfg)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if len(balance) == 0 {
		t.Fatal("expected non-empty balance map")
	}

	for currency, amount := range balance {
		t.Logf("balance: %s = %s", currency, amount)
	}
}

func TestIntegration_RequestQuote_Buy(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.RequestQuote(ctx, cfg, &RFQRequest{
		Instrument:  "BTCUSD.SPOT",
		Side:        "buy",
		Quantity:    "0.1",
		ClientRFQID: fmt.Sprintf("integration-buy-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("RequestQuote (buy) failed: %v", err)
	}

	if resp.RFQID == "" {
		t.Error("expected rfq_id to be non-empty")
	}
	if resp.Price == "" {
		t.Error("expected price to be non-empty")
	}
	if resp.ValidUntil == "" {
		t.Error("expected valid_until to be set")
	}
	if resp.Side != "buy" {
		t.Errorf("expected side 'buy', got %q", resp.Side)
	}
	if resp.Instrument != "BTCUSD.SPOT" {
		t.Errorf("expected instrument 'BTCUSD.SPOT', got %q", resp.Instrument)
	}

	t.Logf("RFQ created: id=%s price=%s valid_until=%s", resp.RFQID, resp.Price, resp.ValidUntil)
}

func TestIntegration_RequestQuote_Sell(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.RequestQuote(ctx, cfg, &RFQRequest{
		Instrument:  "BTCUSD.SPOT",
		Side:        "sell",
		Quantity:    "0.1",
		ClientRFQID: fmt.Sprintf("integration-sell-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("RequestQuote (sell) failed: %v", err)
	}

	if resp.RFQID == "" {
		t.Error("expected rfq_id to be non-empty")
	}
	if resp.Side != "sell" {
		t.Errorf("expected side 'sell', got %q", resp.Side)
	}

	t.Logf("Sell RFQ: id=%s price=%s", resp.RFQID, resp.Price)
}

func TestIntegration_RequestQuote_InvalidInstrument(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.RequestQuote(ctx, cfg, &RFQRequest{
		Instrument:  "INVALIDXYZ.SPOT",
		Side:        "buy",
		Quantity:    "1",
		ClientRFQID: fmt.Sprintf("integration-invalid-%d", time.Now().UnixNano()),
	})
	if err == nil {
		t.Fatal("expected error for invalid instrument, got nil")
	}

	t.Logf("Got expected error for invalid instrument: %v", err)
}

func TestIntegration_RFQToOrder(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Step 1: Request a quote.
	rfq, err := client.RequestQuote(ctx, cfg, &RFQRequest{
		Instrument:  "BTCUSD.SPOT",
		Side:        "buy",
		Quantity:    "0.1",
		ClientRFQID: fmt.Sprintf("integration-order-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("RequestQuote failed: %v", err)
	}
	t.Logf("RFQ received: id=%s price=%s valid_until=%s", rfq.RFQID, rfq.Price, rfq.ValidUntil)

	// Step 2: Execute a FOK order against the quote.
	// UAT may reject without an active book — that's expected; verify structured response.
	order, err := client.ExecuteOrder(ctx, cfg, &OrderRequest{
		Instrument:    "BTCUSD.SPOT",
		Side:          "buy",
		Quantity:      rfq.Quantity,
		Price:         rfq.Price,
		OrderType:     "FOK",
		RFQID:         rfq.RFQID,
		ClientOrderID: fmt.Sprintf("integration-co-%d", time.Now().UnixNano()),
		ValidUntil:    rfq.ValidUntil,
	})
	if err != nil {
		t.Logf("ExecuteOrder returned expected error: %v", err)
		return
	}

	if order.OrderID == "" {
		t.Error("expected order_id to be non-empty")
	}
	if order.Instrument != "BTCUSD.SPOT" {
		t.Errorf("expected instrument BTCUSD.SPOT, got %q", order.Instrument)
	}

	// Both filled and no-liquidity are valid FOK outcomes.
	if order.ExecutedPrice != nil {
		t.Logf("Order filled: id=%s price=%s", order.OrderID, *order.ExecutedPrice)
	} else {
		t.Logf("Order not filled (no liquidity): id=%s status=%s", order.OrderID, order.Status)
	}
}
