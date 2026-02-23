package config

import (
	pkgconfig "github.com/Checker-Finance/adapters/pkg/config"
)

// Config holds all configuration for the application
type Config struct {
	ServerPort        int
	WebSocketURL      string
	RabbitMQURL       string
	Provider          string
	AWSRegion         string
	AWSSecretName     string
	Profile           string
	LogLevel          string
	CheckerIssuer     string
	SymbolMappingPath string
}

// Load creates a Config from environment variables with defaults
func Load() *Config {
	return &Config{
		ServerPort:        pkgconfig.GetEnvInt("SERVER_PORT", 8082),
		WebSocketURL:      pkgconfig.GetEnv("KIIEX_WEBSOCKET_URL", "wss://api.kiire.alphaprod.net/WSGateway"),
		RabbitMQURL:       pkgconfig.GetEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		Provider:          pkgconfig.GetEnv("CHECKER_OTC_ADAPTER_PROVIDER", "kiiex"),
		AWSRegion:         pkgconfig.GetEnv("AWS_REGION", "us-east-2"),
		AWSSecretName:     pkgconfig.GetEnv("AWS_SECRET_NAME", ""),
		Profile:           pkgconfig.GetEnv("SPRING_PROFILES_ACTIVE", ""),
		LogLevel:          pkgconfig.GetEnv("LOG_LEVEL", "info"),
		CheckerIssuer:     pkgconfig.GetEnv("CHECKER_ISSUER", ""),
		SymbolMappingPath: pkgconfig.GetEnv("SYMBOL_MAPPING_PATH", "configs/symbol_mapping.json"),
	}
}
