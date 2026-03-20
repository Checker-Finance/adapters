//go:build integration

package capa

import (
	"context"
	"os"
	"testing"
	"time"


	"github.com/Checker-Finance/adapters/internal/rate"
)

func TestIntegration_CrossRampQuote(t *testing.T) {
	apiKey := os.Getenv("CAPA_API_KEY")
	baseURL := os.Getenv("CAPA_BASE_URL")
	userID := os.Getenv("CAPA_USER_ID")

	if apiKey == "" || baseURL == "" || userID == "" {
		t.Skip("CAPA_API_KEY, CAPA_BASE_URL, CAPA_USER_ID env vars required for integration tests")
	}

	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})
	client := NewClient(rateMgr)

	cfg := &CapaClientConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		UserID:  userID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req := &CapaCrossRampQuoteRequest{
		UserID:              userID,
		SourceCurrency:      "USD",
		DestinationCurrency: "MXN",
		Amount:              1000.0,
		AmountCurrency:      "USD",
	}

	resp, err := client.GetCrossRampQuote(ctx, cfg, req)
	if err != nil {
		t.Fatalf("GetCrossRampQuote failed: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty quote ID")
	}
	if resp.ExchangeRate <= 0 {
		t.Errorf("expected positive exchange rate, got %f", resp.ExchangeRate)
	}

	t.Logf("Quote ID: %s, Rate: %f, Source: %s, Destination: %s",
		resp.ID, resp.ExchangeRate, resp.SourceCurrency, resp.DestinationCurrency)
}

func TestIntegration_CrossRampExecute(t *testing.T) {
	apiKey := os.Getenv("CAPA_API_KEY")
	baseURL := os.Getenv("CAPA_BASE_URL")
	userID := os.Getenv("CAPA_USER_ID")

	if apiKey == "" || baseURL == "" || userID == "" {
		t.Skip("CAPA_API_KEY, CAPA_BASE_URL, CAPA_USER_ID env vars required for integration tests")
	}

	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})
	client := NewClient(rateMgr)

	cfg := &CapaClientConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		UserID:  userID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Get quote
	quoteReq := &CapaCrossRampQuoteRequest{
		UserID:              userID,
		SourceCurrency:      "USD",
		DestinationCurrency: "MXN",
		Amount:              1000.0,
		AmountCurrency:      "USD",
	}

	quote, err := client.GetCrossRampQuote(ctx, cfg, quoteReq)
	if err != nil {
		t.Fatalf("GetCrossRampQuote failed: %v", err)
	}
	t.Logf("Got quote: ID=%s, Rate=%f", quote.ID, quote.ExchangeRate)

	// Step 2: Execute the quote
	execReq := &CapaCrossRampExecuteRequest{
		UserID:  userID,
		QuoteID: quote.ID,
	}

	execResp, err := client.CreateCrossRamp(ctx, cfg, execReq)
	if err != nil {
		t.Fatalf("CreateCrossRamp failed: %v", err)
	}

	t.Logf("Transaction created: ID=%s, Status=%s",
		execResp.Transaction.ID, execResp.Transaction.Status)

	if execResp.Transaction.ID == "" {
		t.Error("expected non-empty transaction ID")
	}
}
