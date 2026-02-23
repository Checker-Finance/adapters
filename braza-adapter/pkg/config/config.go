package config

import (
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
	RedisAddr    string // e.g. localhost:6379
	RedisDB      int
	RedisPass    string
	AWSRegion    string // for AWS SDK client
	LogLevel     string // "debug", "info", etc.
	Port         int    // service HTTP or metrics port

	CacheTTL         time.Duration // TTL for secret cache
	CleanupFreq      time.Duration // frequency for cache cleanup goroutine
	SettlementCutOff time.Time

	// Optional per-service paths or topics
	InboundSubject  string // NATS subject for commands
	OutboundSubject string // NATS subject for events

	BrazaBaseURL        string
	ProductSyncInterval time.Duration
	ProductTable        string

	ClientBalancesIDs  string
	ClientInstrumentID string
}

// Load loads configuration from environment variables and .env file if present.
func Load() *Config {
	// load .env silently (no error if missing)
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:         pkgconfig.GetEnv("SERVICE_NAME", "braza-adapter"),
		Venue:               "braza",
		Env:                 pkgconfig.GetEnv("ENV", "dev"),
		DatabaseURL:         pkgconfig.GetEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        pkgconfig.GetEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		RedisAddr:           pkgconfig.GetEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:             pkgconfig.GetEnvInt("REDIS_DB", 0),
		RedisPass:           pkgconfig.GetEnv("REDIS_PASS", ""),
		AWSRegion:           pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:            pkgconfig.GetEnv("LOG_LEVEL", "info"),
		Port:                pkgconfig.GetEnvInt("BRAZA_PORT", 9010),
		CacheTTL:            pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      pkgconfig.GetEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.BRAZA"),
		OutboundSubject:     pkgconfig.GetEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.BRAZA"),
		BrazaBaseURL:        pkgconfig.GetEnv("BRAZA_BASE_URL", "http://braza.local"),
		ProductSyncInterval: pkgconfig.GetEnvDuration("PRODUCT_SYNC_INTERVAL", 1*time.Hour),
		ProductTable:        pkgconfig.GetEnv("PRODUCT_TABLE", "reference.venue_products"),
		ClientBalancesIDs:   pkgconfig.GetEnv("CLIENT_BALANCES_IDS", ""),
		ClientInstrumentID:  pkgconfig.GetEnv("CLIENT_INSTRUMENT_ID", ""),
		SettlementCutOff:    pkgconfig.GetEnvTime("SETTLEMENT_CUT_OFF", "17:00"),
	}

	//log.Printf("[config] Loaded: %+v", cfg)
	return cfg
}

