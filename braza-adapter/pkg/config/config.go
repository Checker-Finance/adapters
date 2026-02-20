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
		ServiceName:         getEnv("SERVICE_NAME", "braza-adapter"),
		Venue:               "braza",
		Env:                 getEnv("ENV", "dev"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        getEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             getEnv("NATS_URL", "nats://localhost:4222"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:             getEnvInt("REDIS_DB", 0),
		RedisPass:           getEnv("REDIS_PASS", ""),
		AWSRegion:           getEnv("AWS_REGION", "us-east-2"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		Port:                getEnvInt("BRAZA_PORT", 9010),
		CacheTTL:            getEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         getEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      getEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.BRAZA"),
		OutboundSubject:     getEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.BRAZA"),
		BrazaBaseURL:        getEnv("BRAZA_BASE_URL", "http://braza.local"),
		ProductSyncInterval: getEnvDuration("PRODUCT_SYNC_INTERVAL", 1*time.Hour),
		ProductTable:        getEnv("PRODUCT_TABLE", "reference.venue_products"),
		ClientBalancesIDs:   getEnv("CLIENT_BALANCES_IDS", ""),
		ClientInstrumentID:  getEnv("CLIENT_INSTRUMENT_ID", ""),
		SettlementCutOff:    getEnvTime("SETTLEMENT_CUT_OFF", "17:00"),
	}

	//log.Printf("[config] Loaded: %+v", cfg)
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
