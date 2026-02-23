package config

import (
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the core runtime configuration for a service instance.
// It supports environment-based initialization, with sensible defaults.
type Config struct {
	ServiceName  string // e.g. "rio-adapter"
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
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration
	HTTPBodyLimit    int

	CacheTTL         time.Duration // TTL for secret cache
	CleanupFreq      time.Duration // frequency for cache cleanup goroutine
	SettlementCutOff time.Time

	// Optional per-service paths or topics
	InboundSubject  string // NATS subject for commands
	OutboundSubject string // NATS subject for events

	ProductSyncInterval time.Duration
	ProductTable        string

	PGMaxConns          int
	PGMinConns          int
	PGMaxConnLifetime   time.Duration
	PGMaxConnIdleTime   time.Duration
	PGHealthCheckPeriod time.Duration

	// Rio-specific configuration
	// Per-client config (api_key, base_url, country) is resolved from AWS Secrets Manager
	// at runtime. See internal/secrets/resolver.go for the naming convention.
	RioPollInterval           time.Duration // Polling interval for Rio order status (fallback for webhooks)
	RioWebhookURL             string        // Callback URL for Rio webhooks
	RioWebhookSecret          string        // Webhook secret for signature validation
	RioWebhookSignatureHeader string        // Signature header name for webhook validation
}

// Load loads configuration from environment variables and .env file if present.
func Load() *Config {
	// load .env silently (no error if missing)
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:         pkgconfig.GetEnv("SERVICE_NAME", "rio-adapter"),
		Venue:               "rio",
		Env:                 pkgconfig.GetEnv("ENV", "dev"),
		DatabaseURL:         pkgconfig.GetEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        pkgconfig.GetEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		RedisAddr:           pkgconfig.GetEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:             pkgconfig.GetEnvInt("REDIS_DB", 0),
		RedisPass:           pkgconfig.GetEnv("REDIS_PASS", ""),
		AWSRegion:           pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:            pkgconfig.GetEnv("LOG_LEVEL", "info"),
		Port:                pkgconfig.GetEnvInt("RIO_PORT", 9010),
		HTTPReadTimeout:     pkgconfig.GetEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:    pkgconfig.GetEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:     pkgconfig.GetEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPBodyLimit:       pkgconfig.GetEnvInt("HTTP_BODY_LIMIT", 1*1024*1024),
		CacheTTL:            pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      pkgconfig.GetEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.RIO"),
		OutboundSubject:     pkgconfig.GetEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.RIO"),
		ProductSyncInterval: pkgconfig.GetEnvDuration("PRODUCT_SYNC_INTERVAL", 1*time.Hour),
		ProductTable:        pkgconfig.GetEnv("PRODUCT_TABLE", "reference.venue_products"),
		SettlementCutOff:    pkgconfig.GetEnvTime("SETTLEMENT_CUT_OFF", "17:00"),
		PGMaxConns:          pkgconfig.GetEnvInt("PG_MAX_CONNS", 10),
		PGMinConns:          pkgconfig.GetEnvInt("PG_MIN_CONNS", 2),
		PGMaxConnLifetime:   pkgconfig.GetEnvDuration("PG_MAX_CONN_LIFETIME", 30*time.Minute),
		PGMaxConnIdleTime:   pkgconfig.GetEnvDuration("PG_MAX_CONN_IDLE_TIME", 5*time.Minute),
		PGHealthCheckPeriod: pkgconfig.GetEnvDuration("PG_HEALTH_CHECK_PERIOD", 1*time.Minute),

		// Rio-specific configuration (per-client config resolved from AWS Secrets Manager)
		RioPollInterval:           pkgconfig.GetEnvDuration("RIO_POLL_INTERVAL", 30*time.Second),
		RioWebhookURL:             pkgconfig.GetEnv("RIO_WEBHOOK_URL", ""),
		RioWebhookSecret:          pkgconfig.GetEnv("RIO_WEBHOOK_SECRET", ""),
		RioWebhookSignatureHeader: pkgconfig.GetEnv("RIO_WEBHOOK_SIGNATURE_HEADER", "X-Rio-Signature"),
	}

	return cfg
}

