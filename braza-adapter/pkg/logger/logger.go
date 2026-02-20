package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.Logger
var sugar *zap.SugaredLogger

// Init initializes the global logger.
// Environment can be "dev", "uat", or "prod".
func Init(service, env, level string) {
	var cfg zap.Config

	if env == "dev" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
	}

	// Level override
	if lvl, err := zapcore.ParseLevel(level); err == nil {
		cfg.Level = zap.NewAtomicLevelAt(lvl)
	}

	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	logger, err := cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	log = logger
	sugar = logger.Sugar()

	sugar.Infow("logger initialized",
		"service", service,
		"env", env,
		"level", level,
	)
}

// L returns the base structured Zap logger (for performance-sensitive paths).
func L() *zap.Logger {
	if log == nil {
		Init("unknown", "dev", "info")
	}
	return log
}

// S returns the Sugared logger (for convenience).
func S() *zap.SugaredLogger {
	if sugar == nil {
		Init("unknown", "dev", "info")
	}
	return sugar
}

// Sync flushes any buffered logs (defer this in main()).
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}
