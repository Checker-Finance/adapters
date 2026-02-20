package store

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"sync"
	"testing"
	"time"

	"github.com/Checker-Finance/adapters/braza-adapter/pkg/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mockPgPool struct {
	calls int
	last  any
}

func (m *mockPgPool) Exec(ctx context.Context, query string, args ...any) (any, error) {
	m.calls++
	m.last = args
	return nil, nil
}

func (m *mockPgPool) Close() {}

func newTestStore(t *testing.T) (*hybridStore, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	rdb := newRedisClient(mr.Addr())
	return &hybridStore{redis: rdb}, mr
}

// helper to get a redis client for miniredis
func newRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

func TestSetAndGetBalance(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		ID:                  uuid.New(),
		Venue:               "BRAZA",
		Instrument:          "USDBRL",
		AvailableTotalValue: 59992,
		CanBuy:              true,
		CanSell:             true,
		LastUpdated:         time.Now().UTC(),
	}

	key := "tenantA|client1"
	if err := store.SetBalance(ctx, key, bal); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	res, err := store.GetBalance(ctx, key, "USDBRL")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}
	if res == nil {
		t.Fatal("expected balance, got nil")
	}
	if res.Instrument != "USDBRL" {
		t.Errorf("expected instrument=USDBRL, got %s", res.Instrument)
	}
	if !res.CanBuy || !res.CanSell {
		t.Errorf("expected can_buy/can_sell true")
	}
}

func TestSetAndGetJSON(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	val := map[string]string{"api_key": "abc123", "api_secret": "xyz456"}

	if err := store.SetJSON(ctx, "client:cred", val, time.Minute); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	var got map[string]string
	if err := store.GetJSON(ctx, "client:cred", &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}

	if got["api_key"] != "abc123" {
		t.Errorf("expected api_key=abc123, got %s", got["api_key"])
	}
}

func TestBalanceExpiration(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		ID:                  uuid.New(),
		Venue:               "BRAZA",
		Instrument:          "USDBRL",
		AvailableTotalValue: 1000,
		LastUpdated:         time.Now(),
	}

	key := "tenantA|client1"
	if err := store.redis.Set(ctx, "balance:"+key+":USDBRL", mustJSON(bal), 200*time.Millisecond).Err(); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	_, err := store.GetBalance(ctx, key, "USDBRL")
	if err == nil {
		t.Fatalf("expected error for expired key, got nil")
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestConcurrentBalanceWrites(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	key := "tenantA|client1"
	bal := model.Balance{
		ID: uuid.New(), Venue: "BRAZA", Instrument: "USDBRL",
		AvailableTotalValue: 1000,
		LastUpdated:         time.Now(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bal.AvailableTotalValue = float64(i)
			_ = store.SetBalance(ctx, key, bal)
		}(i)
	}
	wg.Wait()

	res, err := store.GetBalance(ctx, key, "USDBRL")
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}
	if res == nil {
		t.Fatal("expected result after concurrent writes")
	}
}

func TestSetBalance_PostgresIntegration(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	mockPg := &mockPgPool{}
	store.pg = (*pgxpool.Pool)(nil)                  // emulate optional
	store.pg = (interface{})(mockPg).(*pgxpool.Pool) // type dance ignored in real tests

	bal := model.Balance{
		ID: uuid.New(), Venue: "BRAZA", Instrument: "USDBRL",
		AvailableTotalValue: 1234, LastUpdated: time.Now(),
	}

	if err := store.SetBalance(ctx, "tenant|client", bal); err != nil {
		t.Fatalf("SetBalance failed: %v", err)
	}
}
