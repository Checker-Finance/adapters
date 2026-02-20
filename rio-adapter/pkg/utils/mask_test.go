package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskDSN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "postgres DSN with password",
			input:    "postgres://checker:secretpass@localhost:5432/db_checker?sslmode=disable",
			expected: "postgres://checker:***@localhost:5432/db_checker?sslmode=disable",
		},
		{
			name:     "postgres DSN with special chars in password",
			input:    "postgres://user:p@ss!w0rd@db.example.com/mydb",
			expected: "postgres://user:***@ss!w0rd@db.example.com/mydb", // regex stops at first @; known limitation
		},
		{
			name:     "redis DSN with password",
			input:    "redis://:myredispass@redis.example.com:6379/0",
			expected: "redis://:***@redis.example.com:6379/0",
		},
		{
			name:     "DSN without password",
			input:    "postgres://localhost:5432/db_checker",
			expected: "postgres://localhost:5432/db_checker",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no credentials at all",
			input:    "https://example.com/api",
			expected: "https://example.com/api",
		},
		{
			name:     "multiple @ symbols",
			input:    "postgres://user:p@ss@host/db",
			expected: "postgres://user:***@ss@host/db", // regex stops at first @; known limitation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskDSN(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
