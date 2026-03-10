//go:build integration

package xfx

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/rate"
)

// newIntegrationConfig returns an XFXClientConfig from environment variables.
// Skips the test if XFX_CLIENT_ID, XFX_CLIENT_SECRET, or XFX_BASE_URL are not set.
func newIntegrationConfig(t *testing.T) *XFXClientConfig {
	t.Helper()

	clientID := os.Getenv("XFX_CLIENT_ID")
	clientSecret := os.Getenv("XFX_CLIENT_SECRET")
	baseURL := os.Getenv("XFX_BASE_URL")
	if clientID == "" || clientSecret == "" || baseURL == "" {
		t.Skip("XFX_CLIENT_ID, XFX_CLIENT_SECRET, and XFX_BASE_URL must be set for integration tests")
	}

	return &XFXClientConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		BaseURL:      baseURL,
	}
}

func newIntegrationClient(t *testing.T) (*Client, *XFXClientConfig) {
	t.Helper()
	cfg := newIntegrationConfig(t)
	logger, _ := zap.NewDevelopment()
	tokens := NewTokenManager(logger)
	return NewClient(logger, rate.NewManager(rate.Config{RequestsPerSecond: 10, Burst: 20}), tokens), cfg
}

// TestIntegration_Auth verifies that Auth0 client credentials flow succeeds.
func TestIntegration_Auth(t *testing.T) {
	cfg := newIntegrationConfig(t)
	logger, _ := zap.NewDevelopment()
	tokens := NewTokenManager(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := tokens.GetToken(ctx, cfg)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty access token")
	}

	t.Logf("Auth0 token obtained successfully (length=%d)", len(token))
}

// TestIntegration_GetAccounts verifies connectivity to the XFX API and retrieves account balances.
func TestIntegration_GetAccounts(t *testing.T) {
	client, cfg := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.GetAccounts(ctx, cfg)
	if err != nil {
		t.Fatalf("GetAccounts failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("GetAccounts returned success=false")
	}

	t.Logf("Accounts retrieved: count=%d", len(resp.Accounts))
	for _, acc := range resp.Accounts {
		t.Logf("  account id=%d currency=%s available=%f total=%f canBuy=%v canSell=%v",
			acc.ID, acc.Currency, acc.Available, acc.Total, acc.CanBuy, acc.CanSell)
	}
}

// TestIntegration_RequestQuote verifies that a quote can be requested from XFX.
// Uses the minimum notional (100,000 USD) on USD/MXN.
func TestIntegration_RequestQuote(t *testing.T) {
	client, cfg := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.RequestQuote(ctx, cfg, &XFXQuoteRequest{
		Symbol:   "USD/MXN",
		Side:     "BUY",
		Quantity: 100000,
	})
	if err != nil {
		t.Fatalf("RequestQuote failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("RequestQuote returned success=false: %s", resp.Message)
	}
	if resp.Quote.ID == "" {
		t.Fatal("expected non-empty quote ID")
	}
	if resp.Quote.Price <= 0 {
		t.Errorf("expected positive price, got %f", resp.Quote.Price)
	}

	t.Logf("Quote obtained: id=%s symbol=%s side=%s quantity=%f price=%f validUntil=%s",
		resp.Quote.ID, resp.Quote.Symbol, resp.Quote.Side,
		resp.Quote.Quantity, resp.Quote.Price, resp.Quote.ValidUntil)
}
