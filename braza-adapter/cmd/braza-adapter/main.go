package main

import (
	"context"
	"fmt"
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
	_ "go.uber.org/zap"

	"github.com/Checker-Finance/adapters/braza-adapter/internal/auth"
	"github.com/Checker-Finance/adapters/braza-adapter/internal/braza"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	intsecrets "github.com/Checker-Finance/adapters/braza-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/internal/store"
	"github.com/Checker-Finance/adapters/braza-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// --- Load configuration ---
	cfg := config.Load()

	logger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	logg := logger.S()
	logg.Info("starting [braza-adapter]...")
	logg.Info("connection to DSN: ", utils.MaskDSN(cfg.DatabaseURL))
	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatalw("failed to connect to NATS", "error", err)
	}
	defer nc.Drain() //nolint:errcheck

	// --- Create secrets cache ---
	cache := pkgsecrets.NewCache[pkgsecrets.Credentials](30 * time.Minute)
	cacheAdapter := auth.NewCacheAdapter(cache)

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatalw("failed to init AWS provider", "error", err)
	}

	// --- Internal secrets resolver (multi-tenant) ---
	resolver := intsecrets.NewAWSResolver(
		logg.Desugar(),
		cfg.Env,
		awsProvider,
		cache,
	)

	// --- Auth Manager (handles Braza JWTs) ---
	authMgr := auth.NewManager(
		awsProvider, cacheAdapter, logg.Desugar(), cfg.BrazaBaseURL,
	)

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.braza", "BRAZA_EVENTS")
	if err != nil {
		logg.Fatalw("failed to init publisher", "error", err)
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 5,
		Burst:             10,
		Cooldown:          1 * time.Second,
	})

	// --- Store (Redis + Postgres hybrid) ---
	st, err := store.NewHybrid(cfg.RedisAddr, cfg.RedisDB, cfg.DatabaseURL, store.PGPoolConfig{}, logg.Desugar())
	if err != nil {
		logg.Fatalw("failed to init store", "error", err)
	}
	defer st.Close() //nolint:errcheck

	productResolver := braza.NewProductResolver(cfg.BrazaBaseURL, logger.L())

	rfqSweeper := legacy.NewRFQSweeper(
		st.(*store.HybridStore).PG,
		logger.L(),
		5*time.Minute,  // sweep interval
		15*time.Minute, // RFQ TTL
	)
	go rfqSweeper.Start(ctx)

	tradeSyncWriter := legacy.NewTradeSyncWriter(st.(*store.HybridStore).PG, logger.L(), "braza-adapter")
	// --- Braza service (core adapter logic) ---
	brazaSvc := braza.NewService(
		ctx,
		*cfg,
		logg.Desugar(),
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
		Logger:  logg.Desugar(),
		Service: brazaSvc,
		Store:   st,
	}

	ph := api.NewProductsHandler(brazaSvc, cfg)

	oh := &api.OrderResolveHandler{
		Logger:    logg.Desugar(),
		Service:   brazaSvc,
		Store:     st,
		TradeSync: tradeSyncWriter,
	}

	api.RegisterRoutes(app, h, ph, oh)

	go func() {
		logg.Infof("HTTP API listening on :%d", cfg.Port)
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logg.Fatalw("fiber.listen_failed", "error", err)
		}
	}()

	// --- Create Poller (handles balances + trade tracking) ---
	poller := braza.NewPoller(
		logg.Desugar(),
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
		logg.Desugar(),
		nc,
		st.(*store.HybridStore).PG, // expose DB handle
		pub,
		24*time.Hour, // every midnight UTC
	)

	go refresher.Start(ctx)

	//syncer := braza.NewProductSyncer(
	//	logg.Desugar(),
	//	st,
	//	cfg.BrazaBaseURL,
	//	authMgr,
	//	cfg.ProductSyncInterval,
	//)

	// --- Define tenants/clients/desks ---
	clientsBalances := parseClientIDs(cfg.ClientBalancesIDs)
	if len(clientsBalances) == 0 {
		logg.Warn("no client IDs configured; skipping balance poller startup")
	} else {
		// --- Start periodic poller ---
		go poller.Start(ctx, clientsBalances)
	}

	//clientInstruments := parseClientIDs(cfg.ClientInstrumentsIDs)
	//if len(clientInstruments) == 0 {
	//	logg.Warn("no client IDs configured; skipping product syncer startup")
	//} else {
	//	go syncer.Start(ctx, clientsBalances[0], *cfg)
	//}

	// --- Main process stays alive until interrupted ---
	logg.Infow("[braza-adapter] running",
		"nats", cfg.NATSURL,
		"poll_interval", cfg.PollInterval)

	<-ctx.Done()
	stop()
	logg.Info("shutting down [braza-adapter]...")

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
