package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the runtime configuration for the xfx-adapter.
type Config struct {
	ServiceName      string
	Env              string
	Venue            string
	DatabaseURL      string
	PollInterval     time.Duration
	NATSURL          string
	RedisURL         string // e.g. redis://localhost:6379 or redis://:pass@host:6379/1
	AWSRegion        string
	LogLevel         string
	Port             int
	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration
	HTTPBodyLimit    int

	CacheTTL    time.Duration
	CleanupFreq time.Duration

	InboundSubject      string // NATS subject for incoming quote request commands
	OutboundSubject     string // NATS subject for outgoing quote response events
	TradeExecuteSubject string // NATS subject for incoming trade execute commands

	PGMaxConns          int
	PGMinConns          int
	PGMaxConnLifetime   time.Duration
	PGMaxConnIdleTime   time.Duration
	PGHealthCheckPeriod time.Duration

	// XFX-specific configuration
	// Per-client config (client_id, client_secret, base_url) is resolved from
	// AWS Secrets Manager at runtime. See internal/secrets/resolver.go.
	XFXPollInterval        time.Duration // Polling interval for XFX transaction status
	RFQSweepInterval       time.Duration // How often to expire stale RFQs/quotes in the legacy DB
	RFQSweepTTL            time.Duration // Age threshold after which an open RFQ/quote is expired
	SummaryRefreshInterval time.Duration // How often to refresh the balance summary materialized view
}

// Load loads configuration from environment variables, then overlays any values
// found in the service-level AWS Secrets Manager secret at {env}/{service-name}.
func Load(ctx context.Context) *Config {
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:            pkgconfig.GetEnv("SERVICE_NAME", "xfx-adapter"),
		Venue:                  "xfx",
		Env:                    pkgconfig.GetEnv("ENV", "dev"),
		DatabaseURL:            pkgconfig.GetEnv("DATABASE_URL", "postgres://checker:checker@localhost/db_checker?sslmode=disable"),
		PollInterval:           pkgconfig.GetEnvDuration("POLL_INTERVAL", 5*time.Minute),
		NATSURL:                pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		RedisURL:               pkgconfig.GetEnv("REDIS_URL", "redis://localhost:6379"),
		AWSRegion:              pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:               pkgconfig.GetEnv("LOG_LEVEL", "info"),
		Port:                   pkgconfig.GetEnvInt("XFX_PORT", 9030),
		HTTPReadTimeout:        pkgconfig.GetEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:       pkgconfig.GetEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:        pkgconfig.GetEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		HTTPBodyLimit:          pkgconfig.GetEnvInt("HTTP_BODY_LIMIT", 1*1024*1024),
		CacheTTL:               pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		CleanupFreq:            pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		InboundSubject:         pkgconfig.GetEnv("INBOUND_SUBJECT", "cmd.lp.quote_request.v1.XFX"),
		OutboundSubject:        pkgconfig.GetEnv("OUTBOUND_SUBJECT", "evt.lp.quote_response.v1.XFX"),
		TradeExecuteSubject:    pkgconfig.GetEnv("TRADE_EXECUTE_SUBJECT", "cmd.lp.trade_execute.v1.XFX"),
		PGMaxConns:             pkgconfig.GetEnvInt("PG_MAX_CONNS", 10),
		PGMinConns:             pkgconfig.GetEnvInt("PG_MIN_CONNS", 2),
		PGMaxConnLifetime:      pkgconfig.GetEnvDuration("PG_MAX_CONN_LIFETIME", 30*time.Minute),
		PGMaxConnIdleTime:      pkgconfig.GetEnvDuration("PG_MAX_CONN_IDLE_TIME", 5*time.Minute),
		PGHealthCheckPeriod:    pkgconfig.GetEnvDuration("PG_HEALTH_CHECK_PERIOD", 1*time.Minute),
		XFXPollInterval:        pkgconfig.GetEnvDuration("XFX_POLL_INTERVAL", 15*time.Second),
		RFQSweepInterval:       pkgconfig.GetEnvDuration("RFQ_SWEEP_INTERVAL", 5*time.Minute),
		RFQSweepTTL:            pkgconfig.GetEnvDuration("RFQ_SWEEP_TTL", 15*time.Minute),
		SummaryRefreshInterval: pkgconfig.GetEnvDuration("SUMMARY_REFRESH_INTERVAL", 24*time.Hour),
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
