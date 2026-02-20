-- 0001_init_schema.sql
BEGIN;

CREATE SCHEMA IF NOT EXISTS ledger;

COMMENT ON SCHEMA ledger IS 'Holds account balance and position data across venues and tenants.';

COMMIT;
