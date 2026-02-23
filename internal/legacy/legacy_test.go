package legacy

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewRFQSweeper(t *testing.T) {
	logger := zap.NewNop()
	interval := 5 * time.Minute
	ttl := 10 * time.Minute

	sweeper := NewRFQSweeper(nil, logger, interval, ttl)

	if sweeper == nil {
		t.Fatal("expected non-nil sweeper")
	}
	if sweeper.interval != interval {
		t.Errorf("expected interval %v, got %v", interval, sweeper.interval)
	}
	if sweeper.ttl != ttl {
		t.Errorf("expected ttl %v, got %v", ttl, sweeper.ttl)
	}
	if sweeper.logger != logger {
		t.Error("expected logger to match")
	}
}

func TestNewTradeSyncWriter(t *testing.T) {
	logger := zap.NewNop()

	writer := NewTradeSyncWriter(nil, logger, "test-adapter")

	if writer == nil {
		t.Fatal("expected non-nil writer")
	}
	if writer.logger != logger {
		t.Error("expected logger to match")
	}
	if writer.source != "test-adapter" {
		t.Errorf("expected source=test-adapter, got %s", writer.source)
	}
}

func TestTradeSyncWriter_SyncTradeUpsert_NilTrade(t *testing.T) {
	logger := zap.NewNop()
	writer := NewTradeSyncWriter(nil, logger, "test-adapter")

	// Nil trade should be a no-op
	err := writer.SyncTradeUpsert(t.Context(), nil)
	if err != nil {
		t.Fatalf("expected nil error for nil trade, got: %v", err)
	}
}

func TestRFQSweeper_Queries(t *testing.T) {
	// Verify that the two queries target different tables
	if rfqQuery == quoteQuery {
		t.Fatal("rfqQuery and quoteQuery should not be identical")
	}

	// Verify rfqQuery targets t_request_for_quote
	if !containsSubstring(rfqQuery, "activity.t_request_for_quote") {
		t.Error("rfqQuery should target activity.t_request_for_quote")
	}

	// Verify quoteQuery targets t_quote
	if !containsSubstring(quoteQuery, "activity.t_quote") {
		t.Error("quoteQuery should target activity.t_quote")
	}

	// Both should set status to EXPIRED
	if !containsSubstring(rfqQuery, "EXPIRED") {
		t.Error("rfqQuery should set status to EXPIRED")
	}
	if !containsSubstring(quoteQuery, "EXPIRED") {
		t.Error("quoteQuery should set status to EXPIRED")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
