-- Rollback for 0001_init_schema.sql
-- WARNING: This will drop all data in the ledger schema.
BEGIN;
DROP SCHEMA IF EXISTS ledger CASCADE;
COMMIT;
