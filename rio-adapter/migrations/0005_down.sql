-- Rollback for 0005_venue_products.sql
-- WARNING: This will drop all product data.
BEGIN;
DROP TABLE IF EXISTS reference.venue_products CASCADE;
DROP SCHEMA IF EXISTS reference CASCADE;
COMMIT;
