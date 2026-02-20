package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that would override defaults
	envVars := []string{
		"SERVICE_NAME", "ENV", "DATABASE_URL", "POLL_INTERVAL",
		"NATS_URL", "REDIS_ADDR", "REDIS_DB", "AWS_REGION",
		"LOG_LEVEL", "RIO_PORT",
		"RIO_POLL_INTERVAL", "PG_MAX_CONNS",
		"HTTP_READ_TIMEOUT", "HTTP_BODY_LIMIT",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
	}

	cfg := Load()

	if cfg.ServiceName != "rio-adapter" {
		t.Errorf("expected ServiceName=rio-adapter, got %s", cfg.ServiceName)
	}
	if cfg.Env != "dev" {
		t.Errorf("expected Env=dev, got %s", cfg.Env)
	}
	if cfg.NATSURL != "nats://localhost:4222" {
		t.Errorf("expected NATSURL=nats://localhost:4222, got %s", cfg.NATSURL)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("expected RedisAddr=localhost:6379, got %s", cfg.RedisAddr)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("expected RedisDB=0, got %d", cfg.RedisDB)
	}
	if cfg.Port != 9010 {
		t.Errorf("expected Port=9010, got %d", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=info, got %s", cfg.LogLevel)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("expected PollInterval=5m, got %v", cfg.PollInterval)
	}
	if cfg.RioPollInterval != 30*time.Second {
		t.Errorf("expected RioPollInterval=30s, got %v", cfg.RioPollInterval)
	}
	if cfg.PGMaxConns != 10 {
		t.Errorf("expected PGMaxConns=10, got %d", cfg.PGMaxConns)
	}
	if cfg.PGMinConns != 2 {
		t.Errorf("expected PGMinConns=2, got %d", cfg.PGMinConns)
	}
	if cfg.HTTPReadTimeout != 10*time.Second {
		t.Errorf("expected HTTPReadTimeout=10s, got %v", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPBodyLimit != 1*1024*1024 {
		t.Errorf("expected HTTPBodyLimit=1048576, got %d", cfg.HTTPBodyLimit)
	}
	if cfg.RioWebhookSignatureHeader != "X-Rio-Signature" {
		t.Errorf("expected RioWebhookSignatureHeader=X-Rio-Signature, got %s", cfg.RioWebhookSignatureHeader)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("ENV", "prod")
	t.Setenv("NATS_URL", "nats://nats:4222")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_DB", "5")
	t.Setenv("RIO_PORT", "8080")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("POLL_INTERVAL", "10m")
	t.Setenv("RIO_POLL_INTERVAL", "1m")
	t.Setenv("PG_MAX_CONNS", "25")
	t.Setenv("HTTP_READ_TIMEOUT", "30s")
	t.Setenv("HTTP_BODY_LIMIT", "2097152")
	t.Setenv("RIO_WEBHOOK_SECRET", "my-secret")
	t.Setenv("RIO_WEBHOOK_SIGNATURE_HEADER", "X-Custom-Sig")

	cfg := Load()

	if cfg.ServiceName != "test-service" {
		t.Errorf("expected ServiceName=test-service, got %s", cfg.ServiceName)
	}
	if cfg.Env != "prod" {
		t.Errorf("expected Env=prod, got %s", cfg.Env)
	}
	if cfg.NATSURL != "nats://nats:4222" {
		t.Errorf("expected NATSURL=nats://nats:4222, got %s", cfg.NATSURL)
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Errorf("expected RedisAddr=redis:6379, got %s", cfg.RedisAddr)
	}
	if cfg.RedisDB != 5 {
		t.Errorf("expected RedisDB=5, got %d", cfg.RedisDB)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel=debug, got %s", cfg.LogLevel)
	}
	if cfg.PollInterval != 10*time.Minute {
		t.Errorf("expected PollInterval=10m, got %v", cfg.PollInterval)
	}
	if cfg.RioPollInterval != 1*time.Minute {
		t.Errorf("expected RioPollInterval=1m, got %v", cfg.RioPollInterval)
	}
	if cfg.PGMaxConns != 25 {
		t.Errorf("expected PGMaxConns=25, got %d", cfg.PGMaxConns)
	}
	if cfg.HTTPReadTimeout != 30*time.Second {
		t.Errorf("expected HTTPReadTimeout=30s, got %v", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPBodyLimit != 2097152 {
		t.Errorf("expected HTTPBodyLimit=2097152, got %d", cfg.HTTPBodyLimit)
	}
	if cfg.RioWebhookSecret != "my-secret" {
		t.Errorf("expected RioWebhookSecret=my-secret, got %s", cfg.RioWebhookSecret)
	}
	if cfg.RioWebhookSignatureHeader != "X-Custom-Sig" {
		t.Errorf("expected RioWebhookSignatureHeader=X-Custom-Sig, got %s", cfg.RioWebhookSignatureHeader)
	}
}

func TestGetEnv_Fallback(t *testing.T) {
	t.Setenv("NONEXISTENT_KEY_12345", "")
	val := getEnv("NONEXISTENT_KEY_12345", "fallback")
	if val != "fallback" {
		t.Errorf("expected fallback, got %s", val)
	}
}

func TestGetEnv_Set(t *testing.T) {
	t.Setenv("TEST_KEY_ABC", "value123")
	val := getEnv("TEST_KEY_ABC", "fallback")
	if val != "value123" {
		t.Errorf("expected value123, got %s", val)
	}
}

func TestGetEnvInt_InvalidFallsToDefault(t *testing.T) {
	t.Setenv("BAD_INT", "not-a-number")
	val := getEnvInt("BAD_INT", 42)
	if val != 42 {
		t.Errorf("expected default 42 for invalid int, got %d", val)
	}
}

func TestGetEnvDuration_InvalidFallsToDefault(t *testing.T) {
	t.Setenv("BAD_DURATION", "not-a-duration")
	val := getEnvDuration("BAD_DURATION", 5*time.Second)
	if val != 5*time.Second {
		t.Errorf("expected default 5s for invalid duration, got %v", val)
	}
}

func TestGetEnvTime_ValidTime(t *testing.T) {
	t.Setenv("CUT_OFF", "14:30")
	val := getEnvTime("CUT_OFF", "17:00")
	if val.Hour() != 14 || val.Minute() != 30 {
		t.Errorf("expected 14:30, got %02d:%02d", val.Hour(), val.Minute())
	}
}

func TestGetEnvTime_InvalidFallsToDefault(t *testing.T) {
	t.Setenv("CUT_OFF_BAD", "invalid")
	val := getEnvTime("CUT_OFF_BAD", "17:00")
	if val.Hour() != 17 || val.Minute() != 0 {
		t.Errorf("expected 17:00 default, got %02d:%02d", val.Hour(), val.Minute())
	}
}

func TestGetEnvTime_Unset(t *testing.T) {
	t.Setenv("CUT_OFF_UNSET", "")
	val := getEnvTime("CUT_OFF_UNSET", "09:15")
	if val.Hour() != 9 || val.Minute() != 15 {
		t.Errorf("expected 09:15 default, got %02d:%02d", val.Hour(), val.Minute())
	}
}
