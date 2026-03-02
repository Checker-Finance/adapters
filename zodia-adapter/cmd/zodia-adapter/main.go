package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Checker-Finance/adapters/zodia-adapter/internal/api"
	"github.com/Checker-Finance/adapters/internal/jobs"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/zodia-adapter/internal/zodia"
	internalsecrets "github.com/Checker-Finance/adapters/zodia-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/zodia-adapter/pkg/config"
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
	cfg.ServiceName = "zodia-adapter"
	cfg.Venue = "zodia"

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	logg := logger.S()
	logg.Info("starting [zodia-adapter]...")
	logg.Info("connection to DSN: ", utils.MaskDSN(cfg.DatabaseURL))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := secrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatalw("failed to create AWS Secrets Manager provider", "error", err)
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	configCache := secrets.NewCache[zodia.ZodiaClientConfig](cfg.CacheTTL)
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
		logg.Infow("discovered Zodia clients", "count", len(clients), "clients", clients)
	}

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatalw("failed to connect to NATS", "error", err)
	}

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.zodia", "ZODIA_EVENTS")
	if err != nil {
		logg.Fatalw("failed to init publisher", "error", err)
	}

	// --- Rate limiter (30 req/s for Zodia REST API) ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 30,
		Burst:             30,
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
	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, logger.L(), "zodia-adapter")

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

	// --- Zodia REST + WS Auth components ---
	signer := zodia.NewHMACSigner()
	restClient := zodia.NewRESTClient(logg.Desugar(), rateMgr, signer)
	wsTokenMgr := zodia.NewWSTokenManager(logg.Desugar(), restClient)
	wsClient := zodia.NewWSClient(logg.Desugar())
	sessionMgr := zodia.NewSessionManager(logg.Desugar(), wsClient, wsTokenMgr, cfg.WSMaxRetries)

	// --- Zodia Service ---
	zodiaSvc := zodia.NewService(
		ctx,
		*cfg,
		logg.Desugar(),
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
		logg.Desugar(),
		*cfg,
		zodiaSvc,
		pub,
		st,
		cfg.ZodiaPollInterval,
		tradeSyncWriter,
	)
	zodiaSvc.SetPoller(poller)

	// --- NATS command consumer: quote requests and trade execute commands ---
	cmdConsumer := zodia.NewCommandConsumer(nc, zodiaSvc, logg.Desugar())
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
	zodiaHandler := api.NewZodiaHandler(logg.Desugar(), zodiaSvc, clientValidator)
	resolveHandler := api.NewOrderResolveHandler(logg.Desugar(), zodiaSvc, st, tradeSyncWriter)
	balanceHandler := api.NewBalanceHandler(logg.Desugar(), st)
	productsHandler := api.NewProductsHandler(zodiaSvc)
	mapper := zodia.NewMapper()
	webhookHandler := api.NewWebhookHandler(logg.Desugar(), st, mapper, tradeSyncWriter, pub)

	api.RegisterRoutes(app, nc, st, zodiaHandler, resolveHandler, balanceHandler, productsHandler, webhookHandler)

	// Start HTTP server
	go func() {
		logg.Infof("HTTP API listening on :%d", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logg.Fatalw("fiber.listen_failed", "error", err)
		}
	}()

	logg.Infow("[zodia-adapter] running",
		"nats", cfg.NATSURL,
		"env", cfg.Env,
		"poll_interval", cfg.ZodiaPollInterval,
		"discovered_clients", len(clients))

	<-ctx.Done()
	logg.Info("shutting down [zodia-adapter]...")

	close(stopCleaner)
	cmdConsumer.Drain()
	sessionMgr.CloseAll()
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
