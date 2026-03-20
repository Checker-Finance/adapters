package logger

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// Init initializes the global slog default logger.
// env: "dev" → TextHandler (stdout), anything else → JSONHandler (stdout).
func Init(service, env, level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339Nano))
			}
			return a
		},
	}

	var handler slog.Handler
	if env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler).With("service", service, "env", env))
	slog.Info("logger initialized", "level", level)
}

// Sync is a no-op retained for call-site compatibility (slog does not buffer).
func Sync() {}
