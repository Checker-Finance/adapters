package rate

import (
	"context"
	"sync"
	"time"
)

// Config defines rate limiting parameters for a client/venue.
type Config struct {
	RequestsPerSecond int
	Burst             int
	Cooldown          time.Duration
}

// Limiter implements a token bucket rate limiter.
type Limiter struct {
	mu        sync.Mutex
	tokens    float64
	last      time.Time
	rate      float64
	burst     float64
	cooldown  time.Duration
	lastBlock time.Time
}

// New creates a new limiter.
func New(cfg Config) *Limiter {
	return &Limiter{
		tokens:   float64(cfg.Burst),
		last:     time.Now(),
		rate:     float64(cfg.RequestsPerSecond),
		burst:    float64(cfg.Burst),
		cooldown: cfg.Cooldown,
	}
}

func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.last = now

	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}

	if l.tokens >= 1 {
		l.tokens -= 1
		return true
	}

	if l.cooldown > 0 {
		l.lastBlock = now
	}
	return false
}

// Wait blocks until a token becomes available or context is canceled.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		if l.Allow() {
			return nil
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Manager holds per-client limiters.
type Manager struct {
	mu       sync.RWMutex
	limiters map[string]*Limiter
	defaults Config
}

func NewManager(defaults Config) *Manager {
	return &Manager{
		limiters: make(map[string]*Limiter),
		defaults: defaults,
	}
}

func (m *Manager) GetLimiter(clientKey string) *Limiter {
	m.mu.RLock()
	if lim, ok := m.limiters[clientKey]; ok {
		m.mu.RUnlock()
		return lim
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if lim, ok := m.limiters[clientKey]; ok {
		return lim
	}
	lim := New(m.defaults)
	m.limiters[clientKey] = lim
	return lim
}

// Wait ensures rate limit compliance for a given key.
func (m *Manager) Wait(ctx context.Context, key string) error {
	lim := m.GetLimiter(key)
	return lim.Wait(ctx)
}
