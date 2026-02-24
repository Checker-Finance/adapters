package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/xfx-adapter/internal/api"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/xfx-adapter/internal/xfx"
	internalsecrets "github.com/Checker-Finance/adapters/xfx-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/xfx-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/logger"
	"github.com/Checker-Finance/adapters/pkg/secrets"
	"github.com/Checker-Finance/adapters/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Load configuration ---
	cfg := config.Load()
	cfg.ServiceName = "xfx-adapter"
	cfg.Venue = "xfx"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	logg := logger.S()
	logg.Info("starting [xfx-adapter]...")
	logg.Info("connection to DSN: ", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatalw("failed to create AWS Secrets Manager provider", "error", err)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[xfx.XFXClientConfig](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go configCache.StartCleaner(cfg.CleanupFreq, stopCleaner)

	resolver := internalsecrets.NewAWSResolver(
		logg.Desugar(),
		*cfg,
		awsProvider,
		configCache,
	)

	// --- Discover configured clients ---
	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		logg.Warnw("failed to discover clients from AWS Secrets Manager", "error", err)
	} else {
		logg.Infow("discovered XFX clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatalw("failed to connect to NATS", "error", err)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.xfx", "XFX_EVENTS")
	if err != nil {
		logg.Fatalw("failed to init publisher", "error", err)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})

	// --- Store (Redis + Postgres hybrid) ---
	st, err := store.NewHybrid(cfg.RedisAddr, cfg.RedisDB, cfg.DatabaseURL, store.PGPoolConfig{
		MaxConns:          int32(cfg.PGMaxConns),
		MinConns:          int32(cfg.PGMinConns),
		MaxConnLifetime:   cfg.PGMaxConnLifetime,
		MaxConnIdleTime:   cfg.PGMaxConnIdleTime,
		HealthCheckPeriod: cfg.PGHealthCheckPeriod,
	}, logg.Desugar())
	if err != nil {
		logg.Fatalw("failed to init store", "error", err)
	}

	// --- Legacy trade sync writer ---
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, logger.L(), "xfx-adapter")

	// --- XFX Auth token manager ---
	tokenMgr := xfx.NewTokenManager(logg.Desugar())

	// --- XFX HTTP client ---
	xfxClient := xfx.NewClient(logg.Desugar(), rateMgr, tokenMgr)

	// --- XFX Service ---
	xfxSvc := xfx.NewService(
		ctx,
		*cfg,
		logg.Desugar(),
		nc,
		xfxClient,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- XFX Poller ---
	poller := xfx.NewPoller(
		logg.Desugar(),
		*cfg,
		xfxSvc,
		pub,
		st,
		cfg.XFXPollInterval,
		tradeSyncWriter,
	)
	xfxSvc.SetPoller(poller)

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	clientValidator := api.NewResolverValidator(resolver)
	xfxHandler := api.NewXFXHandler(logg.Desugar(), xfxSvc, clientValidator)

	api.RegisterRoutes(app, nc, st, xfxHandler)

	// Start HTTP server
	go func() {
		logg.Infof("HTTP API listening on :%d", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logg.Fatalw("fiber.listen_failed", "error", err)
		}
	}()

	logg.Infow("[xfx-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.XFXPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	logg.Info("shutting down [xfx-adapter]...")

	close(stopCleaner)
	poller.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logg.Warnw("fiber.shutdown_failed", "error", err)
	}
	if err := nc.Drain(); err != nil {
		logg.Warnw("nats.drain_failed", "error", err)
	}
	if err := st.Close(); err != nil {
		logg.Warnw("store.close_failed", "error", err)
	}
}
