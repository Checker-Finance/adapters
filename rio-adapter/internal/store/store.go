package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/Checker-Finance/adapters/rio-adapter/pkg/model"
)

// Store defines the contract for caching and persisting balance data.
type Store interface {
	RecordBalanceEvent(ctx context.Context, balance model.Balance) error
	UpdateBalanceSnapshot(ctx context.Context, balance model.Balance) error
	GetBalance(ctx context.Context, tenantID, clientID, venue, instrument string) (*model.Balance, error)
	GetClientBalances(ctx context.Context, clientID string) ([]model.Balance, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	GetJSON(ctx context.Context, key string, dest any) error
	StoreProduct(ctx context.Context, p model.Product) error
	ListProducts(ctx context.Context, venue string) ([]model.Product, error)
	GetQuoteByQuoteID(ctx context.Context, quoteID string) (*model.QuoteRecord, error)
	GetOrderIDByRFQ(ctx context.Context, rfqID string) (string, error)
	HealthCheck(ctx context.Context) error
	Close() error
}

type HybridStore struct {
	redis  *redis.Client
	PG     *pgxpool.Pool
	logger *zap.Logger
}

type PGPoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// NewHybrid creates a Redis-first, Postgres-backed store.
func NewHybrid(redisAddr string, redisDB int, pgURL string, pgPoolConfig PGPoolConfig, logger *zap.Logger) (Store, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   redisDB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	var pgPool *pgxpool.Pool
	if pgURL != "" {
		cfg, err := pgxpool.ParseConfig(pgURL)
		if err != nil {
			return nil, fmt.Errorf("invalid pg config: %w", err)
		}
		if pgPoolConfig.MaxConns > 0 {
			cfg.MaxConns = pgPoolConfig.MaxConns
		}
		if pgPoolConfig.MinConns > 0 {
			cfg.MinConns = pgPoolConfig.MinConns
		}
		if pgPoolConfig.MaxConnLifetime > 0 {
			cfg.MaxConnLifetime = pgPoolConfig.MaxConnLifetime
		}
		if pgPoolConfig.MaxConnIdleTime > 0 {
			cfg.MaxConnIdleTime = pgPoolConfig.MaxConnIdleTime
		}
		if pgPoolConfig.HealthCheckPeriod > 0 {
			cfg.HealthCheckPeriod = pgPoolConfig.HealthCheckPeriod
		}
		pgPool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to postgres: %w", err)
		}
	}

	return &HybridStore{redis: rdb, PG: pgPool, logger: logger}, nil
}

func (s *HybridStore) GetClientBalances(ctx context.Context, clientID string) ([]model.Balance, error) {
	if s.PG == nil {
		return nil, fmt.Errorf("postgres unavailable")
	}
	rows, err := s.PG.Query(ctx, `
		SELECT client_id, venue, instrument, available, held, can_buy, can_sell, as_of
		FROM ledger.balance_snapshot
		WHERE client_id = $1
		ORDER BY as_of DESC;
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.Balance
	for rows.Next() {
		var b model.Balance
		if err := rows.Scan(&b.ClientID, &b.Venue, &b.Instrument,
			&b.Available, &b.Held, &b.CanBuy, &b.CanSell, &b.LastUpdated); err != nil {
			return nil, err
		}
		results = append(results, b)
	}
	return results, nil
}

// RecordBalanceEvent inserts an immutable event into ledger.balance_event.
func (s *HybridStore) RecordBalanceEvent(ctx context.Context, balance model.Balance) error {
	if s.PG == nil {
		return nil
	}
	_, err := s.PG.Exec(ctx, `
		INSERT INTO ledger.balance_event (
			client_id, venue, instrument,
			available, held, can_buy, can_sell, recorded_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, balance.ClientID, balance.Venue, balance.Instrument,
		balance.Available, balance.Held, balance.CanBuy, balance.CanSell)
	if err != nil {
		s.logger.Error("store.pg.insert_event_failed", zap.Error(err))
	}
	return err
}

// UpdateBalanceSnapshot updates the projection table directly (optional).
func (s *HybridStore) UpdateBalanceSnapshot(ctx context.Context, balance model.Balance) error {
	if s.PG == nil {
		return nil
	}
	_, err := s.PG.Exec(ctx, `
		INSERT INTO ledger.balance_snapshot (
			client_id, venue, instrument,
			available, held, can_buy, can_sell, as_of
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (client_id, venue, instrument)
		DO UPDATE SET
			available = EXCLUDED.available,
			held = EXCLUDED.held,
			can_buy = EXCLUDED.can_buy,
			can_sell = EXCLUDED.can_sell,
			as_of = EXCLUDED.as_of;
	`, balance.ClientID, balance.Venue, balance.Instrument,
		balance.Available, balance.Held, balance.CanBuy, balance.CanSell)
	if err != nil {
		s.logger.Error("store.pg.snapshot_update_failed", zap.Error(err))
	}
	return err
}

func (s *HybridStore) GetBalance(ctx context.Context, tenantID, clientID, venue, instrument string) (*model.Balance, error) {
	key := fmt.Sprintf("balance:%s:%s:%s:%s", tenantID, clientID, venue, instrument)
	data, err := s.redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var bal model.Balance
	if err := json.Unmarshal(data, &bal); err != nil {
		return nil, err
	}
	return &bal, nil
}

func (s *HybridStore) ListProducts(ctx context.Context, venue string) ([]model.Product, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT venue_code, instrument_symbol, product_id, product_name, product_secondary_id, is_blocked, as_of
		FROM reference.venue_products
		WHERE ($1 = '' OR LOWER(venue_code) = LOWER($1))
		ORDER BY instrument_symbol;
	`, venue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(
			&p.VenueCode,
			&p.InstrumentSymbol,
			&p.ProductID,
			&p.ProductName,
			&p.SecondaryID,
			&p.IsBlocked,
			&p.AsOf,
		); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

func (s *HybridStore) StoreProduct(ctx context.Context, p model.Product) error {
	_, err := s.PG.Exec(ctx, `
        INSERT INTO reference.venue_products
            (venue_code, instrument_symbol, product_id, product_name, product_secondary_id, is_blocked, as_of)
        VALUES ($1, $2, $3, $4, $5, $6, now())
        ON CONFLICT (venue_code, product_id, product_secondary_id, instrument_symbol)
        DO UPDATE SET
            product_id = EXCLUDED.product_id,
            product_name = EXCLUDED.product_name,
            product_secondary_id = EXCLUDED.product_secondary_id,
            is_blocked = EXCLUDED.is_blocked,
            as_of = now();
    `,
		p.VenueCode, p.InstrumentSymbol, p.ProductID, p.ProductName, p.SecondaryID, p.IsBlocked,
	)

	if err != nil {
		s.logger.Error("store.pg.insert_product_failed", zap.Error(err))
		return err
	}

	return nil
}

func (h *HybridStore) GetQuoteByQuoteID(ctx context.Context, quoteID string) (*model.QuoteRecord, error) {
	const q = `
        SELECT
            q.s_id_quote,
            q.s_id_request_for_quote,
            q.s_id_quote_external, 
            r.s_id_issuer as s_id_client
        FROM activity.t_quote q
        INNER JOIN activity.t_request_for_quote r ON 
            q.s_id_request_for_quote = r.s_id_request_for_quote
        WHERE q.s_id_quote = $1
        LIMIT 1;
    `

	row := h.PG.QueryRow(ctx, q, quoteID)

	var rec model.QuoteRecord
	if err := row.Scan(&rec.QuoteID, &rec.RFQID, &rec.ProviderOrderID, &rec.ClientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("GetQuoteByQuoteID no rows: %w", err)
		}
		return nil, fmt.Errorf("GetQuoteByQuoteID scan failed: %w", err)
	}

	return &rec, nil
}

func (h *HybridStore) GetOrderIDByRFQ(ctx context.Context, rfqID string) (string, error) {
	const q = `
        SELECT s_id_order
        FROM activity.t_order
        WHERE s_id_rfq = $1
        LIMIT 1;
    `

	var orderID sql.NullString
	if err := h.PG.QueryRow(ctx, q, rfqID).Scan(&orderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("GetOrderIDByRFQ no rows for RFQID [%s]: %w", rfqID, err)
		}
		return "", fmt.Errorf("GetOrderIDByRFQ scan failed: %w", err)
	}

	if !orderID.Valid {
		return "", nil // No provider order yet
	}

	return orderID.String, nil
}

func (s *HybridStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, key, data, ttl).Err()
}

func (s *HybridStore) GetJSON(ctx context.Context, key string, dest any) error {
	data, err := s.redis.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (s *HybridStore) HealthCheck(ctx context.Context) error {
	if s.redis == nil {
		return fmt.Errorf("redis not initialized")
	}
	if err := s.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	if s.PG != nil {
		if err := s.PG.Ping(ctx); err != nil {
			return fmt.Errorf("postgres ping failed: %w", err)
		}
	}
	return nil
}

func (s *HybridStore) Close() error {
	if s.PG != nil {
		s.PG.Close()
	}
	if s.redis != nil {
		return s.redis.Close()
	}
	return nil
}
