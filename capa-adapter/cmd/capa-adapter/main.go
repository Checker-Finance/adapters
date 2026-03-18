package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/capa-adapter/internal/api"
	"github.com/Checker-Finance/adapters/capa-adapter/internal/capa"
	internalsecrets "github.com/Checker-Finance/adapters/capa-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/capa-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/internal/jobs"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/internal/store"
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
	cfg := config.Load(ctx)
	cfg.ServiceName = "capa-adapter"
	cfg.Venue = "capa"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	logg := logger.S()
	logg.Info("starting [capa-adapter]...")
	logg.Info("connection to DSN: ", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatalw("failed to create AWS Secrets Manager provider", "error", err)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[capa.CapaClientConfig](cfg.CacheTTL)
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
		logg.Infow("discovered Capa clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatalw("failed to connect to NATS", "error", err)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.capa", "CAPA_EVENTS")
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
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, logger.L(), "capa-adapter")

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

	// --- Capa HTTP client ---
	capaClient := capa.NewClient(logg.Desugar(), rateMgr)

	// --- Capa Service ---
	capaSvc := capa.NewService(
		ctx,
		*cfg,
		logg.Desugar(),
		nc,
		capaClient,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- Capa Poller ---
	poller := capa.NewPoller(
		logg.Desugar(),
		*cfg,
		capaSvc,
		pub,
		st,
		cfg.CapaPollInterval,
		tradeSyncWriter,
	)
	capaSvc.SetPoller(poller)

	// --- Webhook handler ---
	webhookHandler := capa.NewWebhookHandler(
		logg.Desugar(),
		pub,
		st,
		poller,
		tradeSyncWriter,
		capaSvc,
		resolver,
	)

	// --- NATS command consumer: quote requests and trade execute commands ---
	cmdConsumer := capa.NewCommandConsumer(nc, capaSvc, logg.Desugar())
	if err := cmdConsumer.Subscribe(ctx, cfg.InboundSubject, cfg.TradeExecuteSubject); err != nil {
		logg.Fatalw("failed to subscribe to NATS command subjects", "error", err)
	}

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	clientValidator := api.NewResolverValidator(resolver)
	capaHandler := api.NewCapaHandler(logg.Desugar(), capaSvc, clientValidator)
	resolveHandler := api.NewOrderResolveHandler(logg.Desugar(), capaSvc, st, tradeSyncWriter)
	productsHandler := api.NewProductsHandler(capaSvc)
	balanceHandler := api.NewBalanceHandler(st, logg.Desugar())
	webhookAPIHandler := api.NewWebhookAPIHandler(logg.Desugar(), webhookHandler, st, resolver)

	api.RegisterRoutes(app, nc, st, capaHandler, resolveHandler, productsHandler, balanceHandler, webhookAPIHandler)

	// Start HTTP server
	go func() {
		logg.Infof("HTTP API listening on :%d", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logg.Fatalw("fiber.listen_failed", "error", err)
		}
	}()

	logg.Infow("[capa-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.CapaPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	logg.Info("shutting down [capa-adapter]...")

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
