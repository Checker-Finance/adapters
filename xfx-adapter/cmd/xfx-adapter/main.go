package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strings"

	"github.com/Checker-Finance/adapters/xfx-adapter/internal/api"
	"github.com/Checker-Finance/adapters/internal/jobs"
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
	st, err := store.NewHybrid(cfg.RedisURL, cfg.DatabaseURL, store.PGPoolConfig{
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

	// --- RFQ sweeper: expires stale open RFQs and quotes in the legacy DB ---
	rfqSweeper := legacy.NewRFQSweeper(
		st.(*store.HybridStore).PG,
		logger.L(),
		cfg.RFQSweepInterval,
		cfg.RFQSweepTTL,
	)
	go rfqSweeper.Start(ctx)

	// --- Summary refresher: nightly balance materialized view refresh ---
	refresher := jobs.NewSummaryRefresher(
		logg.Desugar(),
		nc,
		st.(*store.HybridStore).PG,
		pub,
		cfg.SummaryRefreshInterval,
	)
	go refresher.Start(ctx)

	// --- Fetch service-level config (Auth0 endpoint + audience) ---
	svcCfg, err := internalsecrets.FetchServiceConfig(ctx, awsProvider, cfg.Env)
	if err != nil {
		logg.Fatalw("failed to fetch XFX service config", "error", err)
	}

	// --- XFX Auth token manager ---
	tokenMgr := xfx.NewTokenManager(logg.Desugar(), svcCfg.Auth0Endpoint, svcCfg.Auth0Audience)

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

	// --- NATS command consumer: quote requests and trade execute commands ---
	cmdConsumer := xfx.NewCommandConsumer(nc, xfxSvc, logg.Desugar())
	if err := cmdConsumer.Subscribe(ctx, cfg.InboundSubject, cfg.TradeExecuteSubject); err != nil {
		logg.Fatalw("failed to subscribe to NATS command subjects", "error", err)
	}

	// --- Balance poller: periodic account balance sync ---
	balanceClients := parseClientIDs(cfg.ClientBalanceIDs)
	if len(balanceClients) > 0 {
		go poller.StartBalancePolling(ctx, balanceClients)
	} else {
		logg.Warn("no CLIENT_BALANCE_IDS configured; skipping balance polling")
	}

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	clientValidator := api.NewResolverValidator(resolver)
	xfxHandler := api.NewXFXHandler(logg.Desugar(), xfxSvc, clientValidator)
	resolveHandler := api.NewOrderResolveHandler(logg.Desugar(), xfxSvc, st, tradeSyncWriter)
	balanceHandler := api.NewBalanceHandler(logg.Desugar(), st)
	productsHandler := api.NewProductsHandler(xfxSvc)

	api.RegisterRoutes(app, nc, st, xfxHandler, resolveHandler, balanceHandler, productsHandler)

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
	cmdConsumer.Drain()
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

// parseClientIDs splits and trims a comma-separated list of client IDs.
func parseClientIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
