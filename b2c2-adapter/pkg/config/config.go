package config

import (
	"time"

	"github.com/joho/godotenv"

	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds the runtime configuration for the b2c2-adapter.
type Config struct {
	ServiceName    string
	Env            string
	AWSRegion      string
	LogLevel       string
	RabbitMQURL    string
	DefaultBaseURL string
	CacheTTL       time.Duration
	CleanupFreq    time.Duration
	HealthPort     int
}

// Load loads configuration from environment variables and optional .env file.
func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		ServiceName:    pkgconfig.GetEnv("SERVICE_NAME", "b2c2-adapter"),
		Env:            pkgconfig.GetEnv("ENV", "dev"),
		AWSRegion:      pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		LogLevel:       pkgconfig.GetEnv("LOG_LEVEL", "info"),
		RabbitMQURL:    pkgconfig.GetEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		DefaultBaseURL: pkgconfig.GetEnv("B2C2_DEFAULT_BASE_URL", "https://api.b2c2.net"),
		CacheTTL:       pkgconfig.GetEnvDuration("CACHE_TTL", 30*time.Minute),
		CleanupFreq:    pkgconfig.GetEnvDuration("CACHE_CLEANUP_FREQ", 10*time.Minute),
		HealthPort:     pkgconfig.GetEnvInt("HEALTH_PORT", 9050),
	}
}
