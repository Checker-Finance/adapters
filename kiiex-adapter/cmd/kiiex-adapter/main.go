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

	"github.com/Checker-Finance/adapters/internal/publisher"
	kiiexapi "github.com/Checker-Finance/adapters/kiiex-adapter/internal/api"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/config"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	kiinats "github.com/Checker-Finance/adapters/kiiex-adapter/internal/nats"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load(ctx)

	pkglogger.Init("kiiex-adapter", cfg.Profile, cfg.LogLevel)
	defer pkglogger.Sync()

	slog.Info("Starting kiiex-adapter", "version", Version)
	slog.Info("Configuration loaded",
		"serverPort", cfg.ServerPort,
		"provider", cfg.Provider,
		"websocketURL", cfg.WebSocketURL,
		"env", cfg.Env,
	)

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("Failed to create AWS Secrets Manager provider", "error", err)
		os.Exit(1)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	cache := pkgsecrets.NewCache[security.Auth](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go cache.StartCleaner(10*time.Minute, stopCleaner)
	defer close(stopCleaner)

	resolver := kiisecrets.NewAWSResolver(cfg.Env, awsProvider, cache)

	// --- Discover configured clients ---
	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		slog.Warn("Failed to discover clients from AWS Secrets Manager", "error", err)
	} else {
		slog.Info("Discovered Kiiex clients", "count", len(clients), "clients", clients)
	}

	// --- Create event bus ---
	eventBus := eventbus.New()

	// --- Create instrument master ---
	instrumentMaster := instruments.NewMaster()
	if err := instrumentMaster.LoadFromFile(cfg.SymbolMappingPath); err != nil {
		slog.Error("Failed to load symbol mappings", "error", err)
		os.Exit(1)
	}

	// --- Create order service (sessions are created on demand, one per client) ---
	orderService := order.NewService(resolver, instrumentMaster, eventBus, cfg.WebSocketURL)

	// --- Create trade status service ---
	tradeStatusService := tracking.NewTradeStatusService(orderService, eventBus)
	go tradeStatusService.Start(ctx)

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.kiiex", "KIIEX_EVENTS")
	if err != nil {
		slog.Error("Failed to init NATS publisher", "error", err)
		os.Exit(1)
	}

	// --- NATS publisher (subscribes to eventbus, forwards to NATS) ---
	_ = kiinats.NewNATSPublisher(pub, eventBus)

	// --- NATS command consumer ---
	consumer := kiinats.NewCommandConsumer(nc, orderService)
	if err := consumer.Subscribe(ctx, cfg.InboundSubject, cfg.CancelSubject); err != nil {
		slog.Error("Failed to subscribe NATS command consumer", "error", err)
		os.Exit(1)
	}
	defer consumer.Drain()

	// --- Fiber HTTP server ---
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	handler := kiiexapi.NewKiiexHandler(orderService)
	kiiexapi.RegisterRoutes(app, handler, nc)

	go func() {
		if err := app.Listen(fmt.Sprintf(":%d", cfg.ServerPort)); err != nil {
			slog.Error("fiber server error", "error", err)
		}
	}()

	slog.Info("kiiex-adapter started successfully",
		"natsURL", cfg.NATSURL,
		"serverPort", cfg.ServerPort,
	)

	<-ctx.Done()

	slog.Info("Shutdown signal received, starting graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Error("fiber shutdown error", "error", err)
	}

	tradeStatusService.Stop()
	if err := orderService.Close(); err != nil {
		slog.Error("failed to close order service sessions", "error", err)
	}

	select {
	case <-shutdownCtx.Done():
		slog.Warn("Shutdown timeout exceeded")
	default:
		slog.Info("Graceful shutdown completed")
	}
}
