package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/rio-adapter/internal/api"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/rio-adapter/internal/rio"
	internalsecrets "github.com/Checker-Finance/adapters/rio-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/rio-adapter/pkg/config"
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
	cfg.ServiceName = "rio-adapter"
	cfg.Venue = "rio"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	logg := logger.S()
	logg.Info("starting [rio-adapter]...")
	logg.Info("connection to DSN: ", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatalw("failed to create AWS Secrets Manager provider", "error", err)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[rio.RioClientConfig](cfg.CacheTTL)
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
		logg.Infow("discovered Rio clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatalw("failed to connect to NATS", "error", err)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.rio", "RIO_EVENTS")
	if err != nil {
		logg.Fatalw("failed to init publisher", "error", err)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10, // Rio may have different rate limits
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
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, logger.L(), "rio-adapter")

	// --- Rio HTTP Client (config supplied per-request) ---
	rioClient := rio.NewClient(
		logg.Desugar(),
		rateMgr,
	)

	// --- Rio Service ---
	rioSvc := rio.NewService(
		ctx,
		*cfg,
		logg.Desugar(),
		nc,
		rioClient,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- Rio Poller (fallback for webhooks) ---
	poller := rio.NewPoller(
		logg.Desugar(),
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
		logg.Desugar(),
		pub,
		st,
		poller,
		tradeSyncWriter,
		rioSvc,
		cfg.RioWebhookSecret,
		cfg.RioWebhookSignatureHeader,
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
	rioHandler := api.NewRioHandler(logg.Desugar(), rioSvc, clientValidator)

	// Order resolve handler
	orderResolveHandler := &api.OrderResolveHandler{
		Logger:    logg.Desugar(),
		Service:   rioSvc,
		Store:     st,
		TradeSync: tradeSyncWriter,
	}

	api.RegisterRoutes(app, nc, st, rioHandler, orderResolveHandler, webhookHandler)

	// Start HTTP server
	serverReady := make(chan struct{})
	go func() {
		logg.Infof("HTTP API listening on :%d", cfg.Port)
		close(serverReady)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logg.Fatalw("fiber.listen_failed", "error", err)
		}
	}()

	// --- Register webhook with Rio (if URL configured) ---
	if cfg.RioWebhookURL != "" {
		go func() {
			<-serverReady // wait for HTTP server to start
			if err := rioSvc.RegisterOrderWebhook(ctx, cfg.RioWebhookURL); err != nil {
				logg.Warnw("failed to register webhook with Rio",
					"error", err,
					"url", cfg.RioWebhookURL)
			}
		}()
	} else {
		logg.Warn("RIO_WEBHOOK_URL not configured; webhooks disabled, using polling only")
	}

	// --- Main process stays alive until interrupted ---
	logg.Infow("[rio-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.RioPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	logg.Info("shutting down [rio-adapter]...")

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
