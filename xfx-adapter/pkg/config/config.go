package config

import (
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the runtime configuration for the xfx-adapter.
type Config struct {
	ServiceName  string
	Env          string
	Venue        string
	DatabaseURL  string
	PollInterval time.Duration
	NATSURL      string
	RedisAddr    string
	RedisDB      int
	RedisPass    string
	AWSRegion    string
	LogLevel     string
	Port         int
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration
	HTTPBodyLimit    int

	CacheTTL    time.Duration
	CleanupFreq time.Duration

	InboundSubject  string
	OutboundSubject string

	PGMaxConns          int
	PGMinConns          int
	PGMaxConnLifetime   time.Duration
	PGMaxConnIdleTime   time.Duration
	PGHealthCheckPeriod time.Duration

	// XFX-specific configuration
	// Per-client config (client_id, client_secret, base_url) is resolved from
	// AWS Secrets Manager at runtime. See internal/secrets/resolver.go.
	XFXPollInterval time.Duration // Polling interval for XFX transaction status
}

// Load loads configuration from environment variables and optional .env file.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		ServiceName:         pkgconfig.GetEnv("SERVICE_NAME", "xfx-adapter"),
		Venue:               "xfx",
		Env:                 pkgconfig.GetEnv("ENV", "dev"),
		DatabaseURL:         pkgconfig.GetEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:        pkgconfig.GetEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:             pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		RedisAddr:           pkgconfig.GetEnv("REDIS_ADDR", "localhost:6379"),
		RedisDB:             pkgconfig.GetEnvInt("REDIS_DB", 0),
		RedisPass:           pkgconfig.GetEnv("REDIS_PASS", ""),
		AWSRegion:           pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:            pkgconfig.GetEnv("LOG_LEVEL", "info"),
		Port:                pkgconfig.GetEnvInt("XFX_PORT", 9030),
		HTTPReadTimeout:     pkgconfig.GetEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:    pkgconfig.GetEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:     pkgconfig.GetEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPBodyLimit:       pkgconfig.GetEnvInt("HTTP_BODY_LIMIT", 1*1024*1024),
		CacheTTL:            pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:         pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:      pkgconfig.GetEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.XFX"),
		OutboundSubject:     pkgconfig.GetEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.XFX"),
		PGMaxConns:          pkgconfig.GetEnvInt("PG_MAX_CONNS", 10),
		PGMinConns:          pkgconfig.GetEnvInt("PG_MIN_CONNS", 2),
		PGMaxConnLifetime:   pkgconfig.GetEnvDuration("PG_MAX_CONN_LIFETIME", 30*time.Minute),
		PGMaxConnIdleTime:   pkgconfig.GetEnvDuration("PG_MAX_CONN_IDLE_TIME", 5*time.Minute),
		PGHealthCheckPeriod: pkgconfig.GetEnvDuration("PG_HEALTH_CHECK_PERIOD", 1*time.Minute),
		XFXPollInterval:     pkgconfig.GetEnvDuration("XFX_POLL_INTERVAL", 15*time.Second),
	}
}
