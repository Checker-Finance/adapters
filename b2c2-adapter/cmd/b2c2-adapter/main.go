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

	"github.com/Checker-Finance/adapters/b2c2-adapter/internal/b2c2"
	b2c2nats "github.com/Checker-Finance/adapters/b2c2-adapter/internal/nats"
	internalsecrets "github.com/Checker-Finance/adapters/b2c2-adapter/internal/secrets"
	"github.com/Checker-Finance/adapters/b2c2-adapter/pkg/config"
	"github.com/Checker-Finance/adapters/internal/publisher"
	"github.com/Checker-Finance/adapters/internal/rate"
	pkglogger "github.com/Checker-Finance/adapters/pkg/logger"
	pkgsecrets "github.com/Checker-Finance/adapters/pkg/secrets"
)

// Version is set at build time.
var Version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load(ctx)

	pkglogger.Init(cfg.ServiceName, cfg.Env, cfg.LogLevel)
	defer pkglogger.Sync()
	logg := pkglogger.L()

	logg.Info("starting b2c2-adapter", zap.String("version", Version))

	// --- AWS Secrets Manager provider ---
	awsProvider, err := pkgsecrets.NewAWSProvider(cfg.AWSRegion)
	if err != nil {
		logg.Fatal("failed to create AWS Secrets Manager provider", zap.Error(err))
	}

	// --- Per-client config cache + resolver ---
	configCache := pkgsecrets.NewCache[b2c2.B2C2ClientConfig](cfg.CacheTTL)
	stopCleaner := make(chan struct{})
	go configCache.StartCleaner(cfg.CleanupFreq, stopCleaner)
	defer close(stopCleaner)

	resolver := internalsecrets.NewAWSResolver(logg, cfg, awsProvider, configCache)

	clients, err := resolver.DiscoverClients(ctx)
	if err != nil {
		logg.Warn("failed to discover clients from AWS Secrets Manager", zap.Error(err))
	} else {
		logg.Info("discovered B2C2 clients", zap.Int("count", len(clients)), zap.Strings("clients", clients))
	}

	// --- Rate limiter ---
	rateMgr := rate.NewManager(rate.Config{
		RequestsPerSecond: 10,
		Burst:             20,
		Cooldown:          1 * time.Second,
	})

	// --- B2C2 HTTP client ---
	client := b2c2.NewClient(logg, rateMgr)

	// --- Connect to NATS ---
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logg.Fatal("failed to connect to NATS", zap.Error(err))
	}
	defer nc.Close()

	// --- Publisher ---
	pub, err := publisher.New(nc, "evt.b2c2", "B2C2_EVENTS")
	if err != nil {
		logg.Fatal("failed to init NATS publisher", zap.Error(err))
	}

	natsPublisher := b2c2nats.NewPublisher(pub, logg)

	// --- B2C2 service ---
	service := b2c2.NewService(logg, client, resolver, natsPublisher)

	// --- NATS command consumer ---
	consumer := b2c2nats.NewCommandConsumer(nc, service, logg)
	if err := consumer.Subscribe(ctx, cfg.InboundRFQSubject, cfg.InboundOrderSubject, cfg.InboundCancelSubject); err != nil {
		logg.Fatal("failed to subscribe NATS command consumer", zap.Error(err))
	}
	defer consumer.Drain()

	// --- Minimal health endpoint ---
	healthServer := startHealthServer(cfg.HealthPort, logg)
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = healthServer.Shutdown(shutCtx)
	}()

	logg.Info("b2c2-adapter started successfully",
		zap.String("natsURL", cfg.NATSURL),
		zap.Int("healthPort", cfg.HealthPort),
	)

	<-ctx.Done()
	logg.Info("shutdown signal received, stopping b2c2-adapter...")
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
