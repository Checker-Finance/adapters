BEGIN;

CREATE SCHEMA IF NOT EXISTS reference;

-- Base instrument table (already in your security master)
-- reference.instruments(id, symbol, base_ccy, quote_ccy, description, ...)
-- reference.venues(id, name, code, ...)
CREATE TABLE IF NOT EXISTS reference.venue_products (
    id                      BIGSERIAL PRIMARY KEY,
    venue_code              VARCHAR(255) NOT NULL,          -- e.g. "RIO"
    instrument_symbol       VARCHAR(255) NOT NULL,          -- e.g. "USDC/BRL"
    product_id              VARCHAR(255) NOT NULL,
    product_secondary_id    VARCHAR(255),
    product_name            VARCHAR(255),
    is_blocked              BOOLEAN DEFAULT FALSE,
    as_of                   TIMESTAMPTZ DEFAULT now(),
    canonical_instrument_id BIGINT,
    UNIQUE (venue_code, product_id, product_secondary_id, instrument_symbol)
);

CREATE INDEX IF NOT EXISTS idx_venue_products_venue
    ON reference.venue_products(venue_code);

CREATE INDEX IF NOT EXISTS idx_venue_products_name
    ON reference.venue_products(product_name);

COMMENT ON TABLE reference.venue_products IS 'Product catalog mapping venue-specific product IDs to canonical instruments.';
COMMENT ON COLUMN reference.venue_products.venue_code IS 'Trading venue code (e.g. RIO).';
COMMENT ON COLUMN reference.venue_products.instrument_symbol IS 'Canonical instrument symbol (e.g. USDC/BRL).';
COMMENT ON COLUMN reference.venue_products.product_id IS 'Venue-specific primary product identifier.';
COMMENT ON COLUMN reference.venue_products.product_secondary_id IS 'Venue-specific secondary product identifier.';
COMMENT ON COLUMN reference.venue_products.product_name IS 'Human-readable product name from the venue.';
COMMENT ON COLUMN reference.venue_products.is_blocked IS 'Whether this product is blocked from trading.';
COMMENT ON COLUMN reference.venue_products.as_of IS 'Timestamp of last sync from the venue.';
COMMENT ON COLUMN reference.venue_products.canonical_instrument_id IS 'FK to the canonical instrument reference table.';

COMMIT;