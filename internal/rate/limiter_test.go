package rate

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 10,
		Burst:             5,
		Cooldown:          100 * time.Millisecond,
	})

	// Should allow up to burst count immediately
	allowed := 0
	for i := 0; i < 10; i++ {
		if lim.Allow() {
			allowed++
		}
	}

	if allowed != 5 {
		t.Errorf("expected 5 allowed from burst, got %d", allowed)
	}
}

func TestLimiter_Refill(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 100, // refills fast
		Burst:             2,
		Cooldown:          0,
	})

	// Drain the bucket
	for lim.Allow() {
	}

	// Wait for tokens to refill
	time.Sleep(50 * time.Millisecond)

	if !lim.Allow() {
		t.Error("expected token to be available after refill period")
	}
}

func TestLimiter_BurstCap(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 1000,
		Burst:             3,
		Cooldown:          0,
	})

	// Even after a long sleep, tokens should not exceed burst
	time.Sleep(100 * time.Millisecond)

	allowed := 0
	for i := 0; i < 10; i++ {
		if lim.Allow() {
			allowed++
		}
	}

	if allowed > 3 {
		t.Errorf("burst cap exceeded: got %d allowed, want <= 3", allowed)
	}
}

func TestLimiter_Wait_Success(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 100,
		Burst:             1,
		Cooldown:          0,
	})

	// Drain the single token
	lim.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := lim.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected Wait to succeed, got: %v", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Wait took too long: %v", elapsed)
	}
}

func TestLimiter_Wait_ContextCanceled(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 1,
		Burst:             1,
		Cooldown:          0,
	})

	// Drain the token
	lim.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := lim.Wait(ctx)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	lim := New(Config{
		RequestsPerSecond: 1000,
		Burst:             100,
		Cooldown:          0,
	})

	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow() {
				mu.Lock()
				total++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if total == 0 {
		t.Error("expected some requests to be allowed")
	}
	if total > 100 {
		t.Errorf("allowed more than burst: %d", total)
	}
}

func TestManager_GetLimiter(t *testing.T) {
	mgr := NewManager(Config{
		RequestsPerSecond: 10,
		Burst:             5,
		Cooldown:          0,
	})

	l1 := mgr.GetLimiter("client-a")
	l2 := mgr.GetLimiter("client-a")
	l3 := mgr.GetLimiter("client-b")

	if l1 != l2 {
		t.Error("same key should return the same limiter instance")
	}
	if l1 == l3 {
		t.Error("different keys should return different limiter instances")
	}
}

func TestManager_Wait(t *testing.T) {
	mgr := NewManager(Config{
		RequestsPerSecond: 100,
		Burst:             5,
		Cooldown:          0,
	})

	ctx := context.Background()
	if err := mgr.Wait(ctx, "client-x"); err != nil {
		t.Fatalf("expected Wait to succeed, got: %v", err)
	}
}

func TestManager_ConcurrentGetLimiter(t *testing.T) {
	mgr := NewManager(Config{
		RequestsPerSecond: 10,
		Burst:             5,
		Cooldown:          0,
	})

	var wg sync.WaitGroup
	limiters := make([]*Limiter, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			limiters[idx] = mgr.GetLimiter("shared-key")
		}(i)
	}
	wg.Wait()

	// All should be the same instance
	for i := 1; i < 20; i++ {
		if limiters[i] != limiters[0] {
			t.Fatalf("limiter at index %d differs from index 0", i)
		}
	}
}
