package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the core runtime configuration for a service instance.
// It supports environment-based initialization, with sensible defaults.
type Config struct {
	ServiceName  string // e.g. "braza-adapter"
	Env          string // e.g. "dev", "uat", "prod"
	Venue        string
	DatabaseURL  string
	PollInterval time.Duration
	NATSURL      string // e.g. nats://localhost:4222
	RedisURL     string // e.g. redis://localhost:6379 or redis://:pass@host:6379/1
	AWSRegion    string // for AWS SDK client
	LogLevel     string // "debug", "info", etc.
	Port         int    // service HTTP or metrics port

	CacheTTL         time.Duration // TTL for secret cache
	CleanupFreq      time.Duration // frequency for cache cleanup goroutine
	SettlementCutOff time.Time

	// Optional per-service paths or topics
	InboundSubject  string // NATS subject for commands
	OutboundSubject string // NATS subject for events

	BrazaBaseURL string

	ClientBalancesIDs  string
	ClientInstrumentID string
}

// Load loads configuration from environment variables, then overlays any values
// found in the service-level AWS Secrets Manager secret at {env}/{service-name}.
func Load(ctx context.Context) *Config {
	// load .env silently (no error if missing)
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:         pkgconfig.GetEnv("SERVICE_NAME", "braza-adapter"),
		Venue:               "braza",
		Env:                 pkgconfig.GetEnv("ENV", "dev"),
		DatabaseURL:         pkgconfig.GetEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        pkgconfig.GetEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		RedisURL:            pkgconfig.GetEnv("REDIS_URL", "redis://localhost:6379"),
		AWSRegion:           pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:            pkgconfig.GetEnv("LOG_LEVEL", "info"),
		Port:                pkgconfig.GetEnvInt("BRAZA_PORT", 9010),
		CacheTTL:            pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      pkgconfig.GetEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.BRAZA"),
		OutboundSubject:     pkgconfig.GetEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.BRAZA"),
		BrazaBaseURL:      pkgconfig.GetEnv("BRAZA_BASE_URL", "http://braza.local"),
		ClientBalancesIDs:   pkgconfig.GetEnv("CLIENT_BALANCES_IDS", ""),
		ClientInstrumentID:  pkgconfig.GetEnv("CLIENT_INSTRUMENT_ID", ""),
		SettlementCutOff:    pkgconfig.GetEnvTime("SETTLEMENT_CUT_OFF", "17:00"),
	}

	secretPath := fmt.Sprintf("%s/%s", cfg.Env, cfg.ServiceName)
	sm, err := pkgconfig.FetchServiceSecret(ctx, cfg.AWSRegion, secretPath)
	if err != nil {
		log.Printf("[config] service secret unavailable (%s): %v", secretPath, err)
	} else {
		cfg.applyServiceSecret(sm)
	}

	return cfg
}

// applyServiceSecret overlays non-empty values from the AWS Secrets Manager
// service secret onto the config, overriding env var defaults.
func (c *Config) applyServiceSecret(m map[string]string) {
	if v := m["database_url"]; v != "" {
		c.DatabaseURL = v
	}
	if v := m["nats_url"]; v != "" {
		c.NATSURL = v
	}
	if v := m["redis_url"]; v != "" {
		c.RedisURL = v
	}
	if v := m["log_level"]; v != "" {
		c.LogLevel = v
	}
}

