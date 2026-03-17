package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/config"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/instruments"
	kiinats "github.com/Checker-Finance/adapters/kiiex-adapter/internal/nats"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/order"
	kiisecrets "github.com/Checker-Finance/adapters/kiiex-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/security"
	"github.com/Checker-Finance/adapters/kiiex-adapter/internal/tracking"
	"github.com/Checker-Finance/adapters/kiiex-adapter/pkg/eventbus"
	pkglogger "github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// Version is set at build time
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load(ctx)

	pkglogger.Init("kiiex-adapter", cfg.Profile, cfg.LogLevel)
	defer pkglogger.Sync()
	logg := pkglogger.L()

	logg.Info("Starting kiiex-adapter", zap.String("version", Version))
	logg.Info("Configuration loaded",
		zap.Int("serverPort", cfg.ServerPort),
		zap.String("provider", cfg.Provider),
		zap.String("websocketURL", cfg.WebSocketURL),
		zap.String("env", cfg.Env),
	)

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatal("Failed to create AWS Secrets Manager provider", zap.Error(err))
	}

	// --- Per-client config resolver (secrets cached in-memory) ---
	cache := pkgsecrets.NewCache[security.Auth](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go cache.StartCleaner(10*time.Minute, stopCleaner)
	defer close(stopCleaner)

	resolver := kiisecrets.NewAWSResolver(logg, cfg.Env, awsProvider, cache)

	// --- Discover configured clients ---
	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		logg.Warn("Failed to discover clients from AWS Secrets Manager", zap.Error(err))
	} else {
		logg.Info("Discovered Kiiex clients", zap.Int("count", len(clients)), zap.Strings("clients", clients))
	}

	// --- Create event bus ---
	eventBus := eventbus.New()

	// --- Create instrument master ---
	instrumentMaster := instruments.NewMaster(logg)
	if err := instrumentMaster.LoadFromFile(cfg.SymbolMappingPath); err != nil {
		logg.Fatal("Failed to load symbol mappings", zap.Error(err))
	}

	// --- Create order service (sessions are created on demand, one per client) ---
	orderService := order.NewService(resolver, instrumentMaster, eventBus, cfg.WebSocketURL, logg)

	// --- Create trade status service ---
	tradeStatusService := tracking.NewTradeStatusService(orderService, eventBus, logg)
	go tradeStatusService.Start(ctx)

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatal("Failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.kiiex", "KIIEX_EVENTS")
	if err != nil {
		logg.Fatal("Failed to init NATS publisher", zap.Error(err))
	}

	// --- NATS publisher (subscribes to eventbus, forwards to NATS) ---
	_ = kiinats.NewNATSPublisher(pub, eventBus, logg)

	// --- NATS command consumer ---
	consumer := kiinats.NewCommandConsumer(nc, orderService, logg)
	if err := consumer.Subscribe(ctx, cfg.InboundSubject, cfg.CancelSubject); err != nil {
		logg.Fatal("Failed to subscribe NATS command consumer", zap.Error(err))
	}
	defer consumer.Drain()

	// --- Minimal health endpoint ---
	healthServer := startHealthServer(cfg.ServerPort, logg)
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = healthServer.Shutdown(shutCtx)
	}()

	logg.Info("kiiex-adapter started successfully",
		zap.String("natsURL", cfg.NATSURL),
		zap.Int("healthPort", cfg.ServerPort),
	)

	<-ctx.Done()

	logg.Info("Shutdown signal received, starting graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	tradeStatusService.Stop()
	if err := orderService.Close(); err != nil {
		logg.Error("failed to close order service sessions", zap.Error(err))
	}

	select {
	case <-shutdownCtx.Done():
		logg.Warn("Shutdown timeout exceeded")
	default:
		logg.Info("Graceful shutdown completed")
	}
}

func startHealthServer(port int, logg *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logg.Error("health server error", zap.Error(err))
		}
	}()

	return srv
}
