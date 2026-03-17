package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the runtime configuration for the b2c2-adapter.
type Config struct {
	ServiceName          string
	Env                  string
	AWSRegion            string
	LogLevel             string
	NATSURL              string
	InboundRFQSubject    string
	InboundOrderSubject  string
	InboundCancelSubject string
	DefaultBaseURL       string
	CacheTTL             time.Duration
	CleanupFreq          time.Duration
	HealthPort           int
}

// Load loads configuration from environment variables, then overlays any values
// found in the service-level AWS Secrets Manager secret at {env}/{service-name}.
func Load(ctx context.Context) *Config {
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:          pkgconfig.GetEnv("SERVICE_NAME", "b2c2-adapter"),
		Env:                  pkgconfig.GetEnv("ENV", "dev"),
		AWSRegion:            pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:             pkgconfig.GetEnv("LOG_LEVEL", "info"),
		NATSURL:              pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		InboundRFQSubject:    pkgconfig.GetEnv("B2C2_INBOUND_RFQ_SUBJECT", "cmd.lp.quote_request.v1.B2C2"),
		InboundOrderSubject:  pkgconfig.GetEnv("B2C2_INBOUND_ORDER_SUBJECT", "cmd.lp.trade_execute.v1.B2C2"),
		InboundCancelSubject: pkgconfig.GetEnv("B2C2_INBOUND_CANCEL_SUBJECT", "cmd.lp.trade_cancel.v1.B2C2"),
		DefaultBaseURL:       pkgconfig.GetEnv("B2C2_DEFAULT_BASE_URL", "https://api.b2c2.net"),
		CacheTTL:             pkgconfig.GetEnvDuration("CACHE_TTL", 30*time.Minute),
		CleanupFreq:          pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		HealthPort:           pkgconfig.GetEnvInt("HEALTH_PORT", 9050),
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
	if v := m["nats_url"]; v != "" {
		c.NATSURL = v
	}
	if v := m["log_level"]; v != "" {
		c.LogLevel = v
	}
}
