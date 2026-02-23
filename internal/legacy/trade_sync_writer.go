package legacy

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/pkg/model"
)

// TradeSyncWriter writes trades into the legacy activity.t_order table.
type TradeSyncWriter struct {
	db     *pgxpool.Pool
	logger *zap.Logger
	source string
}

// NewTradeSyncWriter constructs a writer to update the legacy activity.t_order table.
// source identifies the adapter writing the record (e.g. "rio-adapter", "braza-adapter").
func NewTradeSyncWriter(db *pgxpool.Pool, logger *zap.Logger, source string) *TradeSyncWriter {
	return &TradeSyncWriter{
		db:     db,
		logger: logger,
		source: source,
	}
}

// SyncTradeUpsert inserts or updates the legacy trade record in activity.t_order.
func (w *TradeSyncWriter) SyncTradeUpsert(ctx context.Context, trade *model.TradeConfirmation) error {
	if trade == nil {
		return nil
	}

	const query = `
		INSERT INTO activity.t_order (
			s_id_order,
			s_instrument_pair,
			dec_price,
			dec_quantity,
			s_side,
			s_status,
			s_type,
			s_id_client,
			dt_order,
			s_id_rfq,
			s_provider,
			s_notes,
			s_source,
			s_source_type,
			s_id_order_external,
			s_id_rfq_external
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16
		)
		ON CONFLICT (s_id_order)
		DO UPDATE SET
			s_status = EXCLUDED.s_status,
			dec_price = EXCLUDED.dec_price,
			dec_quantity = EXCLUDED.dec_quantity,
			dt_order = EXCLUDED.dt_order,
			s_notes = EXCLUDED.s_notes,
			s_provider = EXCLUDED.s_provider,
			s_source = EXCLUDED.s_source,
			s_source_type = EXCLUDED.s_source_type,
			s_id_order_external = EXCLUDED.s_id_order_external,
			s_id_rfq_external = EXCLUDED.s_id_rfq_external;
	`

	_, err := w.db.Exec(ctx, query,
		trade.TradeID,         // s_id_order
		trade.Instrument,      // s_instrument_pair
		trade.Price,           // dec_price
		trade.Quantity,        // dec_quantity
		trade.Side,            // s_side
		trade.Status,          // s_status
		"MARKET",              // s_type
		trade.ClientID,        // s_id_client (UUID of taker)
		trade.ExecutedAt,      // dt_order
		trade.RFQID,           // s_id_rfq
		trade.Venue,           // s_provider
		trade.Notes,           // s_notes (optional)
		w.source,              // s_source
		"automated",           // s_source_type
		trade.ProviderOrderID, // s_id_order_external
		trade.ProviderRFQID,   // s_id_rfq_external
	)
	if err != nil {
		w.logger.Error("legacy.trade_sync_failed",
			zap.String("trade_id", trade.TradeID),
			zap.String("client_id", trade.ClientID),
			zap.Error(err),
		)
		return err
	}

	w.logger.Info("legacy.trade_sync_upsert",
		zap.String("trade_id", trade.TradeID),
		zap.String("status", trade.Status),
		zap.String("client_id", trade.ClientID),
		zap.String("venue", trade.Venue),
		zap.Time("executed_at", trade.ExecutedAt),
	)

	return nil
}
