package config

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds all configuration for the application
type Config struct {
	ServiceName       string
	ServerPort        int
	WebSocketURL      string
	NATSURL           string
	InboundSubject    string
	CancelSubject     string
	Provider          string
	AWSRegion         string
	Env               string
	CacheTTL          time.Duration
	Profile           string
	LogLevel          string
	CheckerIssuer     string
	SymbolMappingPath string
}

// Load creates a Config from environment variables with defaults
func Load(ctx context.Context) *Config {
	_ = godotenv.Load()

	cfg := &Config{
		ServiceName:       pkgconfig.GetEnv("SERVICE_NAME", "kiiex-adapter"),
		ServerPort:        pkgconfig.GetEnvInt("SERVER_PORT", 9070),
		WebSocketURL:      pkgconfig.GetEnv("KIIEX_WEBSOCKET_URL", "wss://api.kiire.alphaprod.net/WSGateway"),
		NATSURL:           pkgconfig.GetEnv("NATS_URL", "nats://localhost:4222"),
		InboundSubject:    pkgconfig.GetEnv("KIIEX_INBOUND_SUBJECT", "cmd.lp.trade_execute.v1.KIIEX"),
		CancelSubject:     pkgconfig.GetEnv("KIIEX_CANCEL_SUBJECT", "cmd.lp.trade_cancel.v1.KIIEX"),
		Provider:          pkgconfig.GetEnv("CHECKER_OTC_ADAPTER_PROVIDER", "kiiex"),
		AWSRegion:         pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		Env:               pkgconfig.GetEnv("ENV", "dev"),
		CacheTTL:          pkgconfig.GetEnvDuration("CACHE_TTL", 24*time.Hour),
		Profile:           pkgconfig.GetEnv("SPRING_PROFILES_ACTIVE", ""),
		LogLevel:          pkgconfig.GetEnv("LOG_LEVEL", "info"),
		CheckerIssuer:     pkgconfig.GetEnv("CHECKER_ISSUER", ""),
		SymbolMappingPath: pkgconfig.GetEnv("SYMBOL_MAPPING_PATH", "configs/symbol_mapping.json"),
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
