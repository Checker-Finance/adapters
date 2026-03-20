package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"

	b2c2api "github.com/Checker-Finance/adapters/b2c2-adapter/internal/api"
	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
	b2c2nats "github.com/Checker-Finance/adapters/b2c2-adapter/internal/nats"
	internalsecrets "github.com/Checker-Finance/adapters/b2c2-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/b2c2-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	pkglogger "github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// Version is set at build time.
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load(ctx)

	pkglogger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	defer pkglogger.Sync()

	slog.Info("starting b2c2-adapter", "version", Version)

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("failed to create AWS Secrets Manager provider", "error", err)
		os.Exit(1)
	}

	// --- Per-client config cache + resolver ---
	configCache := pkgsecrets.NewCache[b2c2.B2C2ClientConfig](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go configCache.StartCleaner(cfg.CleanupFreq, stopCleaner)
	defer close(stopCleaner)

	resolver := internalsecrets.NewAWSResolver(cfg, awsProvider, configCache)

	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		slog.Warn("failed to discover clients from AWS Secrets Manager", "error", err)
	} else {
		slog.Info("discovered B2C2 clients", "count", len(clients), "clients", clients)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})

	// --- B2C2 HTTP client ---
	client := b2c2.NewClient(rateMgr)

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.b2c2", "B2C2_EVENTS")
	if err != nil {
		slog.Error("failed to init NATS publisher", "error", err)
		os.Exit(1)
	}

	natsPublisher := b2c2nats.NewPublisher(pub)

	// --- B2C2 service ---
	service := b2c2.NewService(client, resolver, natsPublisher)

	// --- NATS command consumer ---
	consumer := b2c2nats.NewCommandConsumer(nc, service)
	if err := consumer.Subscribe(ctx, cfg.InboundRFQSubject, cfg.InboundOrderSubject, cfg.InboundCancelSubject); err != nil {
		slog.Error("failed to subscribe NATS command consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Drain()

	// --- Fiber HTTP server ---
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	handler := b2c2api.NewB2C2Handler(service)
	b2c2api.RegisterRoutes(app, handler, nc)

	go func() {
		if err := app.Listen(fmt.Sprintf(":%d", cfg.HealthPort)); err != nil {
			slog.Error("fiber server error", "error", err)
		}
	}()

	slog.Info("b2c2-adapter started successfully",
		"natsURL", cfg.NATSURL,
		"healthPort", cfg.HealthPort,
	)

	<-ctx.Done()
	slog.Info("shutdown signal received, stopping b2c2-adapter...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Error("fiber shutdown error", "error", err)
	}
}
