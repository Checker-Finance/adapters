package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for the application
type Config struct {
	ServerPort      int
	WebSocketURL    string
	RabbitMQURL     string
	Provider        string
	AWSRegion       string
	AWSSecretName   string
	Profile         string
	CheckerIssuer   string
	SymbolMappingPath string
}

// Load creates a Config from environment variables with defaults
func Load() *Config {
	return &Config{
		ServerPort:        getEnvInt("SERVER_PORT", 8082),
		WebSocketURL:      getEnv("KIIEX_WEBSOCKET_URL", "wss://api.kiire.alphaprod.net/WSGateway"),
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		Provider:          getEnv("CHECKER_OTC_ADAPTER_PROVIDER", "kiiex"),
		AWSRegion:         getEnv("AWS_REGION", "us-east-2"),
		AWSSecretName:     getEnv("AWS_SECRET_NAME", ""),
		Profile:           getEnv("SPRING_PROFILES_ACTIVE", ""),
		CheckerIssuer:     getEnv("CHECKER_ISSUER", ""),
		SymbolMappingPath: getEnv("SYMBOL_MAPPING_PATH", "configs/symbol_mapping.json"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
