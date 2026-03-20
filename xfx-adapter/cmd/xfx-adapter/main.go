package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/internal/jobs"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/logger"
	"github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/Checker-Finance/adapters/pkg/utils"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/api"
	internalsecrets "github.com/Checker-Finance/adapters/xfx-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/xfx"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Load configuration ---
	cfg := config.Load(ctx)
	cfg.ServiceName = "xfx-adapter"
	cfg.Venue = "xfx"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	slog.Info("starting [xfx-adapter]...")
	slog.Info("connection to DSN", "dsn", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("failed to create AWS Secrets Manager provider", "error", err)
		os.Exit(1)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[xfx.XFXClientConfig](cfg.CacheTTL)
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
		slog.Info("discovered XFX clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.xfx", "XFX_EVENTS")
	if err != nil {
		slog.Error("failed to init publisher", "error", err)
		os.Exit(1)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
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
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, "xfx-adapter")

	// --- RFQ sweeper: expires stale open RFQs and quotes in the legacy DB ---
	rfqSweeper := legacy.NewRFQSweeper(
		st.(*store.HybridStore).PG,
		cfg.RFQSweepInterval,
		cfg.RFQSweepTTL,
	)
	go rfqSweeper.Start(ctx)

	// --- Summary refresher: nightly balance materialized view refresh ---
	refresher := jobs.NewSummaryRefresher(
		nc,
		st.(*store.HybridStore).PG,
		pub,
		cfg.SummaryRefreshInterval,
	)
	go refresher.Start(ctx)

	// --- XFX Auth token manager ---
	tokenMgr := xfx.NewTokenManager()

	// --- XFX HTTP client ---
	xfxClient := xfx.NewClient(rateMgr, tokenMgr)

	// --- XFX Service ---
	xfxSvc := xfx.NewService(
		ctx,
		*cfg,
		nc,
		xfxClient,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- XFX Poller ---
	poller := xfx.NewPoller(
		*cfg,
		xfxSvc,
		pub,
		st,
		cfg.XFXPollInterval,
		tradeSyncWriter,
	)
	xfxSvc.SetPoller(poller)

	// --- NATS command consumer: quote requests and trade execute commands ---
	cmdConsumer := xfx.NewCommandConsumer(nc, xfxSvc)
	if err := cmdConsumer.Subscribe(ctx, cfg.InboundSubject, cfg.TradeExecuteSubject); err != nil {
		slog.Error("failed to subscribe to NATS command subjects", "error", err)
		os.Exit(1)
	}

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	clientValidator := api.NewResolverValidator(resolver)
	xfxHandler := api.NewXFXHandler(xfxSvc, clientValidator)
	resolveHandler := api.NewOrderResolveHandler(xfxSvc, st, tradeSyncWriter)
	productsHandler := api.NewProductsHandler(xfxSvc)
	balanceHandler := api.NewBalanceHandler(st)

	api.RegisterRoutes(app, nc, st, xfxHandler, resolveHandler, productsHandler, balanceHandler)

	// Start HTTP server
	go func() {
		slog.Info("HTTP API listening", "port", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			slog.Error("fiber.listen_failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("[xfx-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.XFXPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	slog.Info("shutting down [xfx-adapter]...")

	close(stopCleaner)
	cmdConsumer.Drain()
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
