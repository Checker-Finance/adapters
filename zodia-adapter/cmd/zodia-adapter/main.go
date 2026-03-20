package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/api"
	internalsecrets "github.com/Checker-Finance/adapters/zodia-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Load configuration ---
	cfg := config.Load(ctx)
	cfg.ServiceName = "zodia-adapter"
	cfg.Venue = "zodia"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	slog.Info("starting [zodia-adapter]...")
	slog.Info("connection to DSN", "dsn", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("failed to create AWS Secrets Manager provider", "error", err)
		os.Exit(1)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[zodia.ZodiaClientConfig](cfg.CacheTTL)
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
		slog.Info("discovered Zodia clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.zodia", "ZODIA_EVENTS")
	if err != nil {
		slog.Error("failed to init publisher", "error", err)
		os.Exit(1)
	}

	// --- Rate limiter (30 req/s for Zodia REST API) ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 30,
		Burst:             30,
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
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, "zodia-adapter")

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

	// --- Zodia REST + WS Auth components ---
	signer := zodia.NewHMACSigner()
	restClient := zodia.NewRESTClient(rateMgr, signer)
	wsTokenMgr := zodia.NewWSTokenManager(restClient)
	wsClient := zodia.NewWSClient()
	sessionMgr := zodia.NewSessionManager(wsClient, wsTokenMgr, cfg.WSMaxRetries)

	// --- Zodia Service ---
	zodiaSvc := zodia.NewService(
		ctx,
		*cfg,
		nc,
		restClient,
		sessionMgr,
		resolver,
		pub,
		st,
		tradeSyncWriter,
	)

	// --- Zodia Poller ---
	poller := zodia.NewPoller(
		*cfg,
		zodiaSvc,
		pub,
		st,
		cfg.ZodiaPollInterval,
		tradeSyncWriter,
	)
	zodiaSvc.SetPoller(poller)

	// --- NATS command consumer: quote requests and trade execute commands ---
	cmdConsumer := zodia.NewCommandConsumer(nc, zodiaSvc)
	if err := cmdConsumer.Subscribe(ctx, cfg.InboundSubject, cfg.TradeExecuteSubject); err != nil {
		slog.Error("failed to subscribe to NATS command subjects", "error", err)
		os.Exit(1)
	}

	// --- Balance poller: periodic account balance sync ---
	balanceClients := parseClientIDs(cfg.ClientBalanceIDs)
	if len(balanceClients) > 0 {
		go poller.StartBalancePolling(ctx, balanceClients)
	} else {
		slog.Warn("no CLIENT_BALANCE_IDS configured; skipping balance polling")
	}

	// --- Fiber HTTP Server ---
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
		BodyLimit:    cfg.HTTPBodyLimit,
	})

	clientValidator := api.NewResolverValidator(resolver)
	zodiaHandler := api.NewZodiaHandler(zodiaSvc, clientValidator)
	resolveHandler := api.NewOrderResolveHandler(zodiaSvc, st, tradeSyncWriter)
	balanceHandler := api.NewBalanceHandler(st)
	productsHandler := api.NewProductsHandler(zodiaSvc)
	mapper := zodia.NewMapper()
	webhookHandler := api.NewWebhookHandler(st, mapper, tradeSyncWriter, pub)

	api.RegisterRoutes(app, nc, st, zodiaHandler, resolveHandler, balanceHandler, productsHandler, webhookHandler)

	// Start HTTP server
	go func() {
		slog.Info("HTTP API listening", "port", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			slog.Error("fiber.listen_failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("[zodia-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.ZodiaPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	slog.Info("shutting down [zodia-adapter]...")

	close(stopCleaner)
	cmdConsumer.Drain()
	sessionMgr.CloseAll()
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
