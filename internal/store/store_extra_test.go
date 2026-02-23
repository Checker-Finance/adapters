package store

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// --- HealthCheck Tests ---

func TestHealthCheck_Success(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	err := store.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestHealthCheck_RedisNil(t *testing.T) {
	store := &HybridStore{redis: nil}
	err := store.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis not initialized")
}

func TestHealthCheck_RedisDown(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &HybridStore{redis: rdb}

	// Close miniredis to simulate failure
	mr.Close()

	err = store.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis ping failed")
}

// --- Close Tests ---

func TestClose_RedisOnly(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	err := store.Close()
	require.NoError(t, err)
}

func TestClose_NilComponents(t *testing.T) {
	store := &HybridStore{}
	err := store.Close()
	require.NoError(t, err)
}

// --- RecordBalanceEvent / UpdateBalanceSnapshot with nil PG ---

func TestRecordBalanceEvent_NilPG(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		ClientID:   "client-001",
		Venue:      "RIO",
		Instrument: "USDC/BRL",
		Available:  1000,
	}

	// Should return nil (no-op) when PG is nil
	err := store.RecordBalanceEvent(context.Background(), bal)
	require.NoError(t, err)
}

func TestUpdateBalanceSnapshot_NilPG(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	bal := model.Balance{
		ClientID:   "client-001",
		Venue:      "RIO",
		Instrument: "USDC/BRL",
		Available:  1000,
	}

	// Should return nil (no-op) when PG is nil
	err := store.UpdateBalanceSnapshot(context.Background(), bal)
	require.NoError(t, err)
}

// --- GetClientBalances with nil PG ---

func TestGetClientBalances_NilPG(t *testing.T) {
	store, mr := newTestStore(t)
	defer mr.Close()

	results, err := store.GetClientBalances(context.Background(), "client-001")
	assert.Nil(t, results)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "postgres unavailable")
}

// --- GetBalance edge cases ---

func TestGetBalance_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	// Store invalid JSON in Redis
	key := "balance:tenantA:client1:RIO:USDBRL"
	require.NoError(t, mr.Set(key, "not-json"))

	bal, err := store.GetBalance(ctx, "tenantA", "client1", "RIO", "USDBRL")
	assert.Nil(t, bal)
	assert.Error(t, err)
}

// --- SetJSON / GetJSON edge cases ---

func TestGetJSON_KeyNotFound(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	var dest map[string]string
	err := store.GetJSON(ctx, "nonexistent:key", &dest)
	assert.Error(t, err)
}

func TestSetJSON_NilValue(t *testing.T) {
	ctx := context.Background()
	store, mr := newTestStore(t)
	defer mr.Close()

	// nil marshals to "null" — should not error
	err := store.SetJSON(ctx, "test:nil", nil, 0)
	require.NoError(t, err)
}

// --- NewHybrid with logger nil ---

func TestNewTestStoreWithLogger(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	// nil logger should default to zap.NewNop
	st, err := NewHybrid(mr.Addr(), 0, "", PGPoolConfig{}, nil)
	require.NoError(t, err)
	require.NotNil(t, st)

	err = st.Close()
	require.NoError(t, err)
}

func TestNewHybrid_WithExplicitLogger(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	st, err := NewHybrid(mr.Addr(), 0, "", PGPoolConfig{}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, st)

	err = st.Close()
	require.NoError(t, err)
}

func TestNewHybrid_InvalidRedis(t *testing.T) {
	_, err := NewHybrid("localhost:1", 0, "", PGPoolConfig{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis ping failed")
}

func TestNewHybrid_InvalidPGURL(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	_, err = NewHybrid(mr.Addr(), 0, "not-a-valid-pg-url", PGPoolConfig{}, nil)
	assert.Error(t, err)
}
