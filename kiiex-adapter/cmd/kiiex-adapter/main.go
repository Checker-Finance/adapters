package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/config"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/rabbitmq"
	kiisecrets "github.com/Checker-Finance/adapters/kiiex-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/tracking"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
	pkglogger "github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// Version is set at build time
var Version = "dev"

func main() {
	cfg := config.Load()

	pkglogger.Init("kiiex-adapter", cfg.Profile, cfg.LogLevel)
	defer pkglogger.Sync()
	logg := pkglogger.L()

	logg.Info("Starting kiiex-adapter", zap.String("version", Version))
	logg.Info("Configuration loaded",
		zap.Int("serverPort", cfg.ServerPort),
		zap.String("provider", cfg.Provider),
		zap.String("websocketURL", cfg.WebSocketURL),
		zap.String("env", cfg.Env),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatal("Failed to create AWS Secrets Manager provider", zap.Error(err))
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	cache := pkgsecrets.NewCache[security.Auth](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go cache.StartCleaner(10*time.Minute, stopCleaner)

	resolver := kiisecrets.NewAWSResolver(logg, cfg.Env, awsProvider, cache)

	// --- Discover configured clients ---
	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		logg.Warn("Failed to discover clients from AWS Secrets Manager", zap.Error(err))
	} else {
		logg.Info("Discovered Kiiex clients", zap.Int("count", len(clients)), zap.Strings("clients", clients))
	}

	// --- Create event bus ---
	eventBus := eventbus.New()

	// --- Create instrument master ---
	instrumentMaster := instruments.NewMaster(logg)
	if err := instrumentMaster.LoadFromFile(cfg.SymbolMappingPath); err != nil {
		logg.Fatal("Failed to load symbol mappings", zap.Error(err))
	}

	// --- Create order service (sessions are created on demand, one per client) ---
	orderService := order.NewService(resolver, instrumentMaster, eventBus, cfg.WebSocketURL, logg)

	// --- Create trade status service ---
	tradeStatusService := tracking.NewTradeStatusService(orderService, eventBus, logg)
	go tradeStatusService.Start(ctx)

	// --- Create RabbitMQ consumer ---
	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL, cfg.Provider, orderService, logg)
	if err != nil {
		logg.Fatal("Failed to create RabbitMQ consumer", zap.Error(err))
	}
	if err := consumer.Start(ctx); err != nil {
		logg.Fatal("Failed to start RabbitMQ consumer", zap.Error(err))
	}

	// --- Create RabbitMQ publisher ---
	publisher, err := rabbitmq.NewPublisher(cfg.RabbitMQURL, eventBus, logg)
	if err != nil {
		logg.Fatal("Failed to create RabbitMQ publisher", zap.Error(err))
	}

	logg.Info("Application started successfully")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logg.Info("Shutdown signal received, starting graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	close(stopCleaner)
	tradeStatusService.Stop()
	if err := consumer.Close(); err != nil {
		logg.Error("failed to close consumer", zap.Error(err))
	}
	if err := publisher.Close(); err != nil {
		logg.Error("failed to close publisher", zap.Error(err))
	}
	if err := orderService.Close(); err != nil {
		logg.Error("failed to close order service sessions", zap.Error(err))
	}

	cancel()

	select {
	case <-shutdownCtx.Done():
		logg.Warn("Shutdown timeout exceeded")
	default:
		logg.Info("Graceful shutdown completed")
	}
}
