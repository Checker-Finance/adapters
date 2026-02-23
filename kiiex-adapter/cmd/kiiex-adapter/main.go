package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/alphapoint"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/config"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/rabbitmq"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/tracking"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
)

// Version is set at build time
var Version = "dev"

func main() {
	// Initialize logger
	logConfig := zap.NewProductionConfig()
	logConfig.EncoderConfig.TimeKey = "timestamp"
	logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := logConfig.Build()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting kiiex-adapter", zap.String("version", Version))

	// Load configuration
	cfg := config.Load()
	logger.Info("Configuration loaded",
		zap.Int("serverPort", cfg.ServerPort),
		zap.String("provider", cfg.Provider),
		zap.String("websocketURL", cfg.WebSocketURL),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load authentication from AWS Secrets Manager or environment
	auth, err := loadAuth(ctx, cfg, logger)
	if err != nil {
		logger.Fatal("Failed to load authentication", zap.Error(err))
	}

	// Create event bus
	eventBus := eventbus.New()

	// Create instrument master
	instrumentMaster := instruments.NewMaster(logger)
	if err := instrumentMaster.LoadFromFile(cfg.SymbolMappingPath); err != nil {
		logger.Fatal("Failed to load symbol mappings", zap.Error(err))
	}

	// Create AlphaPoint client and session
	apClient := alphapoint.NewClient(cfg.WebSocketURL, logger)
	if err := apClient.Connect(ctx); err != nil {
		logger.Fatal("Failed to connect to AlphaPoint", zap.Error(err))
	}

	session := alphapoint.NewSession(apClient, logger)
	session.SetAuth(&alphapoint.AuthenticateUserRequest{
		APIKey:    auth.APIKey,
		Signature: auth.Signature,
		UserID:    auth.UserID,
		Nonce:     auth.Nonce,
	})

	// Create order service
	orderService := order.NewService(session, instrumentMaster, eventBus, auth, logger)

	// Create trade status service
	tradeStatusService := tracking.NewTradeStatusService(orderService, eventBus, logger)
	go tradeStatusService.Start(ctx)

	// Create RabbitMQ consumer
	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL, cfg.Provider, orderService, logger)
	if err != nil {
		logger.Fatal("Failed to create RabbitMQ consumer", zap.Error(err))
	}
	if err := consumer.Start(ctx); err != nil {
		logger.Fatal("Failed to start RabbitMQ consumer", zap.Error(err))
	}

	// Create RabbitMQ publisher
	publisher, err := rabbitmq.NewPublisher(cfg.RabbitMQURL, eventBus, logger)
	if err != nil {
		logger.Fatal("Failed to create RabbitMQ publisher", zap.Error(err))
	}

	logger.Info("Application started successfully")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutdown signal received, starting graceful shutdown...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop services
	tradeStatusService.Stop()
	if err := consumer.Close(); err != nil {
		logger.Error("failed to close consumer", zap.Error(err))
	}
	if err := publisher.Close(); err != nil {
		logger.Error("failed to close publisher", zap.Error(err))
	}
	if err := session.Close(); err != nil {
		logger.Error("failed to close session", zap.Error(err))
	}

	// Cancel main context
	cancel()

	select {
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded")
	default:
		logger.Info("Graceful shutdown completed")
	}
}

func loadAuth(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*security.Auth, error) {
	// Try to load from AWS Secrets Manager if secret name is provided
	if cfg.AWSSecretName != "" {
		logger.Info("Loading auth from AWS Secrets Manager", zap.String("secretName", cfg.AWSSecretName))

		secretsManager, err := security.NewSecretsManager(ctx, cfg.AWSRegion)
		if err != nil {
			return nil, err
		}

		auth, err := secretsManager.GetAuth(ctx, cfg.AWSSecretName)
		if err != nil {
			return nil, err
		}

		// Generate signature if secret is present
		if auth.Secret != "" {
			if err := auth.GenerateSignature(auth.Secret); err != nil {
				return nil, err
			}
		}

		return auth, nil
	}

	// Fall back to environment variables
	logger.Info("Loading auth from environment variables")

	auth := &security.Auth{
		APIKey:    os.Getenv("KIIEX_API_KEY"),
		Nonce:     os.Getenv("KIIEX_API_NONCE"),
		Signature: os.Getenv("KIIEX_API_SIGNATURE"),
		Username:  os.Getenv("KIIEX_USERNAME"),
	}

	// Parse integer values
	if userID := os.Getenv("KIIEX_API_USER_ID"); userID != "" {
		var id int
		if _, err := parseEnvInt(userID, &id); err == nil {
			auth.UserID = id
		}
	}
	if omsID := os.Getenv("KIIEX_OMS_ID"); omsID != "" {
		var id int
		if _, err := parseEnvInt(omsID, &id); err == nil {
			auth.OmsID = id
		}
	}
	if accountID := os.Getenv("KIIEX_ACCOUNT_ID"); accountID != "" {
		var id int
		if _, err := parseEnvInt(accountID, &id); err == nil {
			auth.AccountID = id
		}
	}

	// Generate signature if secret is present
	if secret := os.Getenv("KIIEX_API_SECRET"); secret != "" {
		if err := auth.GenerateSignature(secret); err != nil {
			return nil, err
		}
	}

	return auth, nil
}

func parseEnvInt(s string, v *int) (bool, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, nil
		}
		n = n*10 + int(c-'0')
	}
	*v = n
	return true, nil
}
