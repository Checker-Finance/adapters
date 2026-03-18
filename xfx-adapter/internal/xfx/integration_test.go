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
// Skips the test if required env vars are not set.
func newIntegrationConfig(t *testing.T) *XFXClientConfig {
	t.Helper()

	clientID := os.Getenv("XFX_CLIENT_ID")
	clientSecret := os.Getenv("XFX_CLIENT_SECRET")
	baseURL := os.Getenv("XFX_BASE_URL")
	auth0Endpoint := os.Getenv("XFX_AUTH0_ENDPOINT")
	auth0Audience := os.Getenv("XFX_AUTH0_AUDIENCE")
	if clientID == "" || clientSecret == "" || baseURL == "" || auth0Endpoint == "" || auth0Audience == "" {
		t.Skip("XFX_CLIENT_ID, XFX_CLIENT_SECRET, XFX_BASE_URL, XFX_AUTH0_ENDPOINT, and XFX_AUTH0_AUDIENCE must be set for integration tests")
	}

	return &XFXClientConfig{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		BaseURL:       baseURL,
		Auth0Endpoint: auth0Endpoint,
		Auth0Audience: auth0Audience,
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
