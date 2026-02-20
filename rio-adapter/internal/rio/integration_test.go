//go:build integration

package rio

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newIntegrationConfig returns a RioClientConfig from environment variables.
// It skips the test if RIO_BASE_URL or RIO_API_KEY are not set.
func newIntegrationConfig(t *testing.T) *RioClientConfig {
	t.Helper()

	baseURL := os.Getenv("RIO_BASE_URL")
	apiKey := os.Getenv("RIO_API_KEY")
	country := os.Getenv("RIO_COUNTRY")
	if baseURL == "" || apiKey == "" {
		t.Skip("RIO_BASE_URL and RIO_API_KEY must be set for integration tests")
	}
	if country == "" {
		country = "MX"
	}

	return &RioClientConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Country: country,
	}
}

// newIntegrationClient returns a Client for integration tests.
func newIntegrationClient(t *testing.T) (*Client, *RioClientConfig) {
	t.Helper()
	cfg := newIntegrationConfig(t)
	return NewClient(zap.NewNop(), nil), cfg
}

func TestIntegration_CreateQuote_Buy(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.CreateQuote(ctx, cfg, &RioQuoteRequest{
		Crypto:     "USDC",
		Fiat:       "MXN",
		Side:       "buy",
		Country:    cfg.Country,
		AmountFiat: 1000,
	})
	if err != nil {
		t.Fatalf("CreateQuote (buy) failed: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected quote ID to be non-empty")
	}
	if resp.NetPrice <= 0 {
		t.Errorf("expected positive net price, got %f", resp.NetPrice)
	}
	if resp.ExpiresAt == "" {
		t.Error("expected ExpiresAt to be set")
	}
	if resp.Side != "buy" {
		t.Errorf("expected side 'buy', got %q", resp.Side)
	}

	t.Logf("Quote created: id=%s price=%f expires=%s fees=%+v",
		resp.ID, resp.NetPrice, resp.ExpiresAt, resp.Fees)
}

func TestIntegration_CreateQuote_Sell(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.CreateQuote(ctx, cfg, &RioQuoteRequest{
		Crypto:       "USDC",
		Fiat:         "MXN",
		Side:         "sell",
		Country:      cfg.Country,
		AmountCrypto: 50,
	})
	if err != nil {
		t.Fatalf("CreateQuote (sell) failed: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected quote ID to be non-empty")
	}
	if resp.Side != "sell" {
		t.Errorf("expected side 'sell', got %q", resp.Side)
	}

	t.Logf("Sell quote created: id=%s price=%f amount_fiat=%f",
		resp.ID, resp.NetPrice, resp.AmountFiat)
}

func TestIntegration_CreateQuote_InvalidCrypto(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.CreateQuote(ctx, cfg, &RioQuoteRequest{
		Crypto:     "INVALIDXYZ",
		Fiat:       "MXN",
		Side:       "buy",
		Country:    cfg.Country,
		AmountFiat: 1000,
	})
	if err == nil {
		t.Fatal("expected error for invalid crypto symbol, got nil")
	}

	t.Logf("Got expected error for invalid crypto: %v", err)
}

func TestIntegration_GetOrder_NotFound(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := client.GetOrder(ctx, cfg, "nonexistent-order-id-12345")
	if err == nil {
		t.Fatal("expected error for non-existent order, got nil")
	}

	t.Logf("Got expected error for missing order: %v", err)
}

func TestIntegration_QuoteToOrder(t *testing.T) {
	client, cfg := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Step 1: Create a quote.
	quote, err := client.CreateQuote(ctx, cfg, &RioQuoteRequest{
		Crypto:     "USDC",
		Fiat:       "MXN",
		Side:       "buy",
		Country:    cfg.Country,
		AmountFiat: 1000,
	})
	if err != nil {
		t.Fatalf("CreateQuote failed: %v", err)
	}
	t.Logf("Quote created: id=%s", quote.ID)

	// Step 2: Attempt to create an order from the quote.
	// Sandbox may reject this without bank/wallet setup — that's fine,
	// we just verify we get a structured error rather than a panic.
	order, err := client.CreateOrder(ctx, cfg, &RioOrderRequest{
		QuoteID: quote.ID,
	})
	if err != nil {
		// Expected: sandbox likely rejects without bank/wallet config.
		t.Logf("CreateOrder returned expected error: %v", err)
		return
	}

	// If the sandbox allows it, verify the order references the quote.
	if order.QuoteID != quote.ID {
		t.Errorf("expected order.QuoteID=%s, got %s", quote.ID, order.QuoteID)
	}
	if order.Status == "" {
		t.Error("expected order status to be non-empty")
	}

	t.Logf("Order created: id=%s status=%s quote=%s", order.ID, order.Status, order.QuoteID)
}
