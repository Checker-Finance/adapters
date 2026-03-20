//go:build integration

package xfx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"


	"github.com/Checker-Finance/adapters/internal/rate"
	intsecrets "github.com/Checker-Finance/adapters/internal/secrets"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// newIntegrationConfig resolves XFXClientConfig from AWS Secrets Manager,
// exactly as the production service does via AWSResolver.
//
// Required env vars:
//   - XFX_TEST_CLIENT_ID — the client ID whose secret to fetch
//
// Optional env vars:
//   - ENV        — environment prefix (default: "dev")
//   - AWS_REGION — AWS region (default: "us-east-2")
func newIntegrationConfig(t *testing.T) *XFXClientConfig {
	t.Helper()

	clientID := os.Getenv("XFX_TEST_CLIENT_ID")
	if clientID == "" {
		t.Skip("XFX_TEST_CLIENT_ID must be set for integration tests")
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "dev"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}


	provider, err := pkgsecrets.NewAWSProvider(region)
	if err != nil {
		t.Fatalf("failed to create AWS provider: %v", err)
	}

	cache := pkgsecrets.NewCache[XFXClientConfig](24 * time.Hour)
	resolver := intsecrets.NewAWSResolver[XFXClientConfig](env, "xfx", provider, cache)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := resolver.Resolve(ctx, clientID, func(m map[string]string) (XFXClientConfig, error) {
		c := XFXClientConfig{
			ClientID:      m["client_id"],
			ClientSecret:  m["client_secret"],
			BaseURL:       m["base_url"],
			Auth0Endpoint: m["auth0_endpoint"],
			Auth0Audience: m["auth0_audience"],
		}
		if c.ClientID == "" || c.ClientSecret == "" || c.BaseURL == "" || c.Auth0Endpoint == "" || c.Auth0Audience == "" {
			return XFXClientConfig{}, fmt.Errorf("secret missing required fields (got: client_id=%q base_url=%q auth0_endpoint=%q)", c.ClientID, c.BaseURL, c.Auth0Endpoint)
		}
		return c, nil
	})
	if err != nil {
		t.Fatalf("failed to resolve XFX config from AWS SM (%s/%s/xfx): %v", env, clientID, err)
	}

	t.Logf("Resolved config from AWS SM: env=%s clientID=%s baseURL=%s", env, cfg.ClientID, cfg.BaseURL)
	return &cfg
}

func newIntegrationClient(t *testing.T) (*Client, *XFXClientConfig) {
	t.Helper()
	cfg := newIntegrationConfig(t)
	tokens := NewTokenManager()
	return NewClient(rate.NewManager(rate.Config{RequestsPerSecond: 10, Burst: 20}), tokens), cfg
}

// TestIntegration_Auth verifies that Auth0 client credentials flow succeeds.
func TestIntegration_Auth(t *testing.T) {
	cfg := newIntegrationConfig(t)
	tokens := NewTokenManager()

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

// TestIntegration_RequestQuote_OverCreditLimit intentionally exceeds the daily credit limit
// to observe the raw error format XFX returns.
func TestIntegration_RequestQuote_OverCreditLimit(t *testing.T) {
	client, cfg := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.RequestQuote(ctx, cfg, &XFXQuoteRequest{
		Symbol:   "USD/MXN",
		Side:     "BUY",
		Quantity: 2_000_000,
	})
	t.Logf("err=%v", err)
	if resp != nil {
		t.Logf("resp.Success=%v resp.Message=%q", resp.Success, resp.Message)
		t.Logf("resp.Quote=%+v", resp.Quote)
	}
}

// TestIntegration_RequestQuote_USDT_MXN_2M sends a 2,000,000 USDT/MXN BUY quote and logs
// the full request, raw XFX response, and adapter result.
func TestIntegration_RequestQuote_USDT_MXN_2M(t *testing.T) {
	client, cfg := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req := &XFXQuoteRequest{
		Symbol:   "USDT/MXN",
		Side:     "BUY",
		Quantity: 2_000_000,
	}

	reqJSON, _ := json.Marshal(req)
	t.Logf("=== OUR REQUEST ===")
	t.Logf("POST %s/v1/customer/quotes", cfg.BaseURL)
	t.Logf("%s", reqJSON)

	resp, err := client.RequestQuote(ctx, cfg, req)

	t.Logf("=== XFX RESPONSE ===")
	if resp != nil {
		respJSON, _ := json.Marshal(resp)
		t.Logf("%s", respJSON)
	} else {
		t.Logf("(nil — transport or auth error)")
	}

	t.Logf("=== ADAPTER RESPONSE ===")
	if err != nil {
		t.Logf("error: %v", err)
	} else {
		t.Logf("ok: quoteID=%s price=%f validUntil=%s", resp.Quote.ID, resp.Quote.Price, resp.Quote.ValidUntil)
	}
}

// TestIntegration_RequestQuote_USDT_MXN verifies that a USDT/MXN quote can be requested from XFX.
func TestIntegration_RequestQuote_USDT_MXN(t *testing.T) {
	client, cfg := newIntegrationClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.RequestQuote(ctx, cfg, &XFXQuoteRequest{
		Symbol:   "USDT/MXN",
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
