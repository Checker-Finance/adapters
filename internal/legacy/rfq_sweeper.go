package legacy

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RFQSweeper periodically expires old RFQs in the legacy table activity.t_request_for_quote.
type RFQSweeper struct {
	db       *pgxpool.Pool
	interval time.Duration
	ttl      time.Duration
}

// NewRFQSweeper creates a new background job for sweeping stale RFQs.
func NewRFQSweeper(db *pgxpool.Pool, interval, ttl time.Duration) *RFQSweeper {
	return &RFQSweeper{
		db:       db,
		interval: interval,
		ttl:      ttl,
	}
}

// Start runs the sweep loop until context cancellation.
func (j *RFQSweeper) Start(ctx context.Context) {
	slog.Info("legacy.rfq_sweeper.start",
		"interval", j.interval,
		"ttl", j.ttl,
	)

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := j.sweep(ctx); err != nil {
				slog.Warn("legacy.rfq_sweeper.sweep_failed", "error", err)
			}
		case <-ctx.Done():
			slog.Info("legacy.rfq_sweeper.stopped")
			return
		}
	}
}

func (j *RFQSweeper) sweep(ctx context.Context) error {
	rfqCount, err := j.sweep_(ctx, rfqQuery)
	if err != nil {
		return err
	}

	slog.Info("legacy.rfq_sweeper.sweep_complete",
		"expired_items", rfqCount,
	)

	quoteCount, err := j.sweep_(ctx, quoteQuery)
	if err != nil {
		return err
	}

	slog.Info("legacy.quote_sweeper.sweep_complete",
		"expired_items", quoteCount,
	)

	return nil
}

func (j *RFQSweeper) sweep_(ctx context.Context, query string) (int, error) {
	rows, err := j.db.Query(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var expiredCount int
	for rows.Next() {
		expiredCount++
	}

	return expiredCount, nil
}

const rfqQuery = `
		UPDATE activity.t_request_for_quote
		SET s_status = 'EXPIRED'
		WHERE s_status = 'OPEN'
		  AND dt_expiry < NOW()
		RETURNING s_id_request_for_quote;
	`

const quoteQuery = `
		UPDATE activity.t_quote
		SET s_status = 'EXPIRED'
		WHERE s_status = 'OPEN'
		  AND dt_expiry < NOW()
		RETURNING s_id_quote;
	`
