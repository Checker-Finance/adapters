package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
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
		ServiceName:         getEnv("SERVICE_NAME", "rio-adapter"),
		Venue:               "rio",
		Env:                 getEnv("ENV", "dev"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        getEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             getEnv("NATS_URL", "nats://localhost:4222"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:             getEnvInt("REDIS_DB", 0),
		RedisPass:           getEnv("REDIS_PASS", ""),
		AWSRegion:           getEnv("AWS_REGION", "us-east-2"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		Port:                getEnvInt("RIO_PORT", 9010),
		HTTPReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPBodyLimit:       getEnvInt("HTTP_BODY_LIMIT", 1*1024*1024),
		CacheTTL:            getEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         getEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      getEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.RIO"),
		OutboundSubject:     getEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.RIO"),
		ProductSyncInterval: getEnvDuration("PRODUCT_SYNC_INTERVAL", 1*time.Hour),
		ProductTable:        getEnv("PRODUCT_TABLE", "reference.venue_products"),
		SettlementCutOff:    getEnvTime("SETTLEMENT_CUT_OFF", "17:00"),
		PGMaxConns:          getEnvInt("PG_MAX_CONNS", 10),
		PGMinConns:          getEnvInt("PG_MIN_CONNS", 2),
		PGMaxConnLifetime:   getEnvDuration("PG_MAX_CONN_LIFETIME", 30*time.Minute),
		PGMaxConnIdleTime:   getEnvDuration("PG_MAX_CONN_IDLE_TIME", 5*time.Minute),
		PGHealthCheckPeriod: getEnvDuration("PG_HEALTH_CHECK_PERIOD", 1*time.Minute),

		// Rio-specific configuration (per-client config resolved from AWS Secrets Manager)
		RioPollInterval:           getEnvDuration("RIO_POLL_INTERVAL", 30*time.Second),
		RioWebhookURL:             getEnv("RIO_WEBHOOK_URL", ""),
		RioWebhookSecret:          getEnv("RIO_WEBHOOK_SECRET", ""),
		RioWebhookSignatureHeader: getEnv("RIO_WEBHOOK_SIGNATURE_HEADER", "X-Rio-Signature"),
	}

	return cfg
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func getEnvInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return def
}

func getEnvTime(key, defaultTime string) time.Time {
	value := os.Getenv(key)
	if value == "" {
		value = defaultTime
	}

	// Parse as time only (HH:MM format)
	t, err := time.Parse("15:04", value)
	if err != nil {
		t, _ = time.Parse("15:04", defaultTime)
	}

	// This gives us a time on Jan 1, 0000, but we only care about the time portion
	return t
}
