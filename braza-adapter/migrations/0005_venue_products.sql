CREATE SCHEMA IF NOT EXISTS reference;

-- Base instrument table (already in your security master)
-- reference.instruments(id, symbol, base_ccy, quote_ccy, description, ...)
-- reference.venues(id, name, code, ...)
DROP TABLE IF EXISTS reference.venue_products;

CREATE TABLE IF NOT EXISTS reference.venue_products (
    id                      BIGSERIAL PRIMARY KEY,
    venue_code              VARCHAR(255) NOT NULL,          -- e.g. "BRAZA"
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