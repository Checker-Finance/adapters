package jobs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/internal/publisher"
)

// SummaryRefresher periodically triggers Postgres materialized view refresh
// and emits a NATS event indicating summary recalculation completion.
type SummaryRefresher struct {
	logger    *zap.Logger
	nc        *nats.Conn
	db        DBExecutor // small interface wrapper over pgxpool.Pool
	publisher *publisher.Publisher
	interval  time.Duration
	stopCh    chan struct{}
}

// DBExecutor defines minimal subset of pgxpool.Pool needed for execution.
type DBExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// NewSummaryRefresher constructs a background job that runs periodically.
func NewSummaryRefresher(logger *zap.Logger, nc *nats.Conn, db DBExecutor, pub *publisher.Publisher, interval time.Duration) *SummaryRefresher {
	return &SummaryRefresher{
		logger:    logger,
		nc:        nc,
		db:        db,
		publisher: pub,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start runs the summary refresh loop (typically every 24 h).
func (r *SummaryRefresher) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("summary_refresher.started", zap.Duration("interval", r.interval))

	for {
		select {
		case <-ticker.C:
			r.runOnce(ctx)
		case <-r.stopCh:
			r.logger.Info("summary_refresher.stopped (manual stop)")
			return
		case <-ctx.Done():
			r.logger.Info("summary_refresher.stopped (context canceled)")
			return
		}
	}
}

// Stop gracefully halts the refresher.
func (r *SummaryRefresher) Stop() {
	close(r.stopCh)
}

// runOnce executes one refresh cycle.
func (r *SummaryRefresher) runOnce(ctx context.Context) {
	start := time.Now()
	r.logger.Info("summary_refresher.running")

	_, err := r.db.Exec(ctx, `SELECT ledger.fn_refresh_balance_summary()`)
	if err != nil {
		r.logger.Error("summary_refresher.refresh_failed", zap.Error(err))
		return
	}

	// Emit event for downstream analytics systems
	event := map[string]any{
		"event":       "evt.balance.summary.refreshed.v1",
		"timestamp":   time.Now().UTC(),
		"duration_ms": time.Since(start).Milliseconds(),
	}
	if err := r.publisher.Publish(ctx, "evt.balance.summary.refreshed.v1", event); err != nil {
		r.logger.Warn("summary_refresher.nats_publish_failed", zap.Error(err))
	}

	r.logger.Info("summary_refresher.success",
		zap.Duration("duration", time.Since(start)))
}
