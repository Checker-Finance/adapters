package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
	b2c2rabbitmq "github.com/Checker-Finance/adapters/b2c2-adapter/internal/rabbitmq"
	internalsecrets "github.com/Checker-Finance/adapters/b2c2-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/b2c2-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/internal/rate"
	pkglogger "github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// Version is set at build time.
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	pkglogger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	defer pkglogger.Sync()
	logg := pkglogger.L()

	logg.Info("starting b2c2-adapter", zap.String("version", Version))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatal("failed to create AWS Secrets Manager provider", zap.Error(err))
	}

	// --- Per-client config cache + resolver ---
	configCache := pkgsecrets.NewCache[b2c2.B2C2ClientConfig](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go configCache.StartCleaner(cfg.CleanupFreq, stopCleaner)
	defer close(stopCleaner)

	resolver := internalsecrets.NewAWSResolver(logg, cfg, awsProvider, configCache)

	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		logg.Warn("failed to discover clients from AWS Secrets Manager", zap.Error(err))
	} else {
		logg.Info("discovered B2C2 clients", zap.Int("count", len(clients)), zap.Strings("clients", clients))
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})

	// --- B2C2 HTTP client ---
	client := b2c2.NewClient(logg, rateMgr)

	// --- RabbitMQ publisher ---
	publisher, err := b2c2rabbitmq.NewPublisher(cfg.RabbitMQURL, logg)
	if err != nil {
		logg.Fatal("failed to create RabbitMQ publisher", zap.Error(err))
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			logg.Error("failed to close publisher", zap.Error(err))
		}
	}()

	// --- B2C2 service ---
	service := b2c2.NewService(logg, client, resolver, publisher)

	// --- RabbitMQ consumer ---
	consumer, err := b2c2rabbitmq.NewConsumer(cfg.RabbitMQURL, service, logg)
	if err != nil {
		logg.Fatal("failed to create RabbitMQ consumer", zap.Error(err))
	}
	if err := consumer.Start(ctx); err != nil {
		logg.Fatal("failed to start RabbitMQ consumer", zap.Error(err))
	}
	defer func() {
		if err := consumer.Close(); err != nil {
			logg.Error("failed to close consumer", zap.Error(err))
		}
	}()

	// --- Minimal health endpoint ---
	healthServer := startHealthServer(cfg.HealthPort, logg)
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = healthServer.Shutdown(shutCtx)
	}()

	logg.Info("b2c2-adapter started successfully",
		zap.String("amqpURL", cfg.RabbitMQURL),
		zap.Int("healthPort", cfg.HealthPort),
	)

	<-ctx.Done()
	logg.Info("shutdown signal received, stopping b2c2-adapter...")
}

func startHealthServer(port int, logg *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logg.Error("health server error", zap.Error(err))
		}
	}()

	return srv
}
