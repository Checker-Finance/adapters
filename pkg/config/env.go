package config

import (
	"os"
	"strconv"
	"time"
)

// GetEnv returns the environment variable value for key, or def if unset or empty.
func GetEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

// GetEnvInt returns the environment variable value for key parsed as int, or def if unset or invalid.
func GetEnvInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return def
}

// GetEnvDuration returns the environment variable value for key parsed as time.Duration, or def if unset or invalid.
func GetEnvDuration(key string, def time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return def
}

// GetEnvTime returns the environment variable value for key parsed as HH:MM time, or defaultTime if unset or invalid.
// The returned time is on Jan 1, year 0000 — only the time-of-day portion is meaningful.
func GetEnvTime(key, defaultTime string) time.Time {
	value := os.Getenv(key)
	if value == "" {
		value = defaultTime
	}

	// Parse as time only (HH:MM format)
	t, err := time.Parse("15:04", value)
	if err != nil {
		t, _ = time.Parse("15:04", defaultTime)
	}

	return t
}
