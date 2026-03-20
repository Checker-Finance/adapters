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

	"github.com/Checker-Finance/adapters/braza-adapter/internal/api"
	"github.com/Checker-Finance/adapters/internal/jobs"
	"github.com/Checker-Finance/adapters/internal/legacy"
	"github.com/Checker-Finance/adapters/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	intsecrets "github.com/Checker-Finance/adapters/braza-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// --- Load configuration ---
	cfg := config.Load(ctx)

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	slog.Info("starting [braza-adapter]...")
	slog.Info("connection to DSN", "dsn", utils.MaskDSN(cfg.DatabaseURL))
	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck

	// --- Create secrets cache ---
	cache := pkgsecrets.NewCache[pkgsecrets.Credentials](30 * time.Minute)
	cacheAdapter := auth.NewCacheAdapter(cache)

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		slog.Error("failed to init AWS provider", "error", err)
		os.Exit(1)
	}

	// --- Internal secrets resolver (multi-tenant) ---
	resolver := intsecrets.NewAWSResolver(
		cfg.Env,
		awsProvider,
		cache,
	)

	// --- Auth Manager (handles Braza JWTs) ---
	authMgr := auth.NewManager(
		awsProvider, cacheAdapter, cfg.BrazaBaseURL,
	)

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.braza", "BRAZA_EVENTS")
	if err != nil {
		slog.Error("failed to init publisher", "error", err)
		os.Exit(1)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 5,
		Burst:             10,
		Cooldown:          1 * time.Second,
	})

	// --- Store (Redis + Postgres hybrid) ---
	st, err := store.NewHybrid(cfg.RedisURL, cfg.DatabaseURL, store.PGPoolConfig{})
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}
	defer st.Close() //nolint:errcheck

	productResolver := braza.NewProductResolver(cfg.BrazaBaseURL)

	rfqSweeper := legacy.NewRFQSweeper(
		st.(*store.HybridStore).PG,
		5*time.Minute,  // sweep interval
		15*time.Minute, // RFQ TTL
	)
	go rfqSweeper.Start(ctx)

	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, "braza-adapter")
	// --- Braza service (core adapter logic) ---
	brazaSvc := braza.NewService(
		ctx,
		*cfg,
		nc,
		cfg.BrazaBaseURL,
		authMgr,
		resolver,
		rateMgr,
		pub,
		st,
		productResolver,
		tradeSyncWriter,
	)

	app := fiber.New()
	h := &api.Handler{
		Service: brazaSvc,
		Store:   st,
	}

	ph := api.NewProductsHandler(brazaSvc, cfg)

	oh := &api.OrderResolveHandler{
		Service:   brazaSvc,
		Store:     st,
		TradeSync: tradeSyncWriter,
	}

	api.RegisterRoutes(app, nc, st, h, ph, oh)

	go func() {
		slog.Info("HTTP API listening", "port", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			slog.Error("fiber.listen_failed", "error", err)
			os.Exit(1)
		}
	}()

	// --- Create Poller (handles balances + trade tracking) ---
	poller := braza.NewPoller(
		*cfg,
		brazaSvc,
		pub,
		st,
		resolver,
		authMgr,
		cacheAdapter,
		cfg.PollInterval,
		tradeSyncWriter,
	)

	brazaSvc.SetPoller(poller)

	refresher := jobs.NewSummaryRefresher(
		nc,
		st.(*store.HybridStore).PG, // expose DB handle
		pub,
		24*time.Hour, // every midnight UTC
	)

	go refresher.Start(ctx)

	// --- Define tenants/clients/desks ---
	clientsBalances := parseClientIDs(cfg.ClientBalancesIDs)
	if len(clientsBalances) == 0 {
		slog.Warn("no client IDs configured; skipping balance poller startup")
	} else {
		// --- Start periodic poller ---
		go poller.Start(ctx, clientsBalances)
	}

	// --- Main process stays alive until interrupted ---
	slog.Info("[braza-adapter] running",
		"nats", cfg.NATSURL,
		"poll_interval", cfg.PollInterval)

	<-ctx.Done()
	stop()
	slog.Info("shutting down [braza-adapter]...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app.ShutdownWithContext(shutdownCtx) //nolint:errcheck
}

// parseClientIDs safely splits and trims a comma-separated list of client IDs.
func parseClientIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}
