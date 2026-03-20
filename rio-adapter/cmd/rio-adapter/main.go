package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/logger"
	"github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/Checker-Finance/adapters/pkg/utils"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/api"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	internalsecrets "github.com/Checker-Finance/adapters/rio-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Load configuration ---
	cfg := config.Load(ctx)
	cfg.ServiceName = "rio-adapter"
	cfg.Venue = "rio"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	slog.Info("starting [rio-adapter]...")
	slog.Info("connection to DSN", "dsn", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("failed to create AWS Secrets Manager provider", "error", err)
		os.Exit(1)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[rio.RioClientConfig](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go configCache.StartCleaner(cfg.CleanupFreq, stopCleaner)

	resolver := internalsecrets.NewAWSResolver(
		*cfg,
		awsProvider,
		configCache,
	)

	// --- Discover configured clients ---
	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		slog.Warn("failed to discover clients from AWS Secrets Manager", "error", err)
	} else {
		slog.Info("discovered Rio clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.rio", "RIO_EVENTS")
	if err != nil {
		slog.Error("failed to init publisher", "error", err)
		os.Exit(1)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10, // Rio may have different rate limits
		Burst:             20,
		Cooldown:          1 * time.Second,
	})

	// --- Store (Redis + Postgres hybrid) ---
	st, err := store.NewHybrid(cfg.RedisURL, cfg.DatabaseURL, store.PGPoolConfig{
		MaxConns:          int32(cfg.PGMaxConns),
		MinConns:          int32(cfg.PGMinConns),
		MaxConnLifetime:   cfg.PGMaxConnLifetime,
		MaxConnIdleTime:   cfg.PGMaxConnIdleTime,
		HealthCheckPeriod: cfg.PGHealthCheckPeriod,
	})
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}

	// --- Legacy trade sync writer ---
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, "rio-adapter")

	// --- Rio HTTP Client (config supplied per-request) ---
	rioClient := rio.NewClient(
		rateMgr,
	)

	// --- Rio Service ---
	rioSvc := rio.NewService(
		ctx,
		*cfg,
		nc,
		rioClient,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- Rio Poller (fallback for webhooks) ---
	poller := rio.NewPoller(
		*cfg,
		rioSvc,
		pub,
		st,
		cfg.RioPollInterval,
		tradeSyncWriter,
	)
	rioSvc.SetPoller(poller)

	// --- Rio Webhook Handler ---
	webhookHandler := rio.NewWebhookHandler(
		pub,
		st,
		poller,
		tradeSyncWriter,
		rioSvc,
		resolver,
	)

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	// Rio API Handler (with client validation via config resolver)
	clientValidator := api.NewResolverValidator(resolver)
	rioHandler := api.NewRioHandler(rioSvc, clientValidator)

	// Order resolve handler
	orderResolveHandler := &api.OrderResolveHandler{
		Service:   rioSvc,
		Store:     st,
		TradeSync: tradeSyncWriter,
	}

	productsHandler := api.NewProductsHandler(st, cfg.Venue)
	balanceHandler := api.NewBalanceHandler(st)
	api.RegisterRoutes(app, nc, st, rioHandler, orderResolveHandler, webhookHandler, productsHandler, balanceHandler)

	// Start HTTP server
	serverReady := make(chan struct{})
	go func() {
		slog.Info("HTTP API listening", "port", cfg.Port)
		close(serverReady)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			slog.Error("fiber.listen_failed", "error", err)
			os.Exit(1)
		}
	}()

	// --- Register webhooks with Rio for each configured client ---
	go func() {
		<-serverReady // wait for HTTP server to start
		if err := rioSvc.RegisterOrderWebhook(ctx); err != nil {
			slog.Warn("failed to register webhooks with Rio", "error", err)
		}
	}()

	// --- Main process stays alive until interrupted ---
	slog.Info("[rio-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.RioPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	slog.Info("shutting down [rio-adapter]...")

	close(stopCleaner)
	poller.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Warn("fiber.shutdown_failed", "error", err)
	}
	if err := nc.Drain(); err != nil {
		slog.Warn("nats.drain_failed", "error", err)
	}
	if err := st.Close(); err != nil {
		slog.Warn("store.close_failed", "error", err)
	}
}
