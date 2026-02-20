-- ==========================================================
-- DROP EXISTING OBJECTS (safe to re-run during dev)
-- ==========================================================
DROP TABLE IF EXISTS ledger.balance_event CASCADE;
DROP TABLE IF EXISTS ledger.balance_snapshot CASCADE;
DROP FUNCTION IF EXISTS ledger.update_balance_snapshot();
-- ==========================================================
-- IMMUTABLE EVENT LOG
-- ==========================================================
CREATE TABLE ledger.balance_event (
       id               BIGSERIAL PRIMARY KEY,
       client_id        VARCHAR(255) NOT NULL,
       venue            VARCHAR(255) NOT NULL,
       instrument       VARCHAR(255) NOT NULL,
       available        NUMERIC(20,8) DEFAULT 0,
       held             NUMERIC(20,8) DEFAULT 0,
       can_buy          BOOLEAN DEFAULT TRUE,
       can_sell         BOOLEAN DEFAULT TRUE,
       recorded_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_balance_event_tenant_client_instrument
    ON ledger.balance_event (client_id, venue, instrument);

COMMENT ON TABLE ledger.balance_event IS 'Immutable ledger of all adapter-reported balances from Braza and other venues.';
COMMENT ON COLUMN ledger.balance_event.available IS 'Current available balance at event time.';
COMMENT ON COLUMN ledger.balance_event.held IS 'Portion of funds locked or unavailable for trading.';
COMMENT ON COLUMN ledger.balance_event.recorded_at IS 'Timestamp when balance event was recorded by the adapter.';


-- ==========================================================
-- MATERIALIZED SNAPSHOT (projection / latest state)
-- ==========================================================
CREATE TABLE ledger.balance_snapshot (
          client_id   VARCHAR(255) NOT NULL,
          venue       VARCHAR(255) NOT NULL,
          instrument  VARCHAR(255) NOT NULL,
          available   NUMERIC(20,8) DEFAULT 0,
          held        NUMERIC(20,8) DEFAULT 0,
          can_buy     BOOLEAN DEFAULT TRUE,
          can_sell    BOOLEAN DEFAULT TRUE,
          as_of       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
          PRIMARY KEY (client_id, venue, instrument)
);

CREATE INDEX idx_balance_snapshot_asof
    ON ledger.balance_snapshot (as_of DESC);

COMMENT ON TABLE ledger.balance_snapshot IS 'Latest balance state per tenant/client/venue/instrument, updated from balance_event.';


-- ==========================================================
-- OPTIONAL: AUTO-MAINTAIN SNAPSHOT VIA TRIGGER
-- ==========================================================
CREATE OR REPLACE FUNCTION ledger.update_balance_snapshot()
RETURNS TRIGGER AS $$
BEGIN
INSERT INTO ledger.balance_snapshot (
    client_id, venue, instrument,
    available, held, can_buy, can_sell, as_of
)
VALUES (
           NEW.client_id, NEW.venue, NEW.instrument,
           NEW.available, NEW.held, NEW.can_buy, NEW.can_sell, NOW()
       )
    ON CONFLICT (client_id, venue, instrument)
    DO UPDATE
               SET available = EXCLUDED.available,
               held = EXCLUDED.held,
               can_buy = EXCLUDED.can_buy,
               can_sell = EXCLUDED.can_sell,
               as_of = EXCLUDED.as_of;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_balance_event_update_snapshot ON ledger.balance_event;
CREATE TRIGGER trg_balance_event_update_snapshot
    AFTER INSERT ON ledger.balance_event
    FOR EACH ROW
    EXECUTE FUNCTION ledger.update_balance_snapshot();