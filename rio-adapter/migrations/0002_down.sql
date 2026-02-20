-- Rollback for 0002_create_balance_tables.sql
-- WARNING: This will drop all balance data.
BEGIN;
DROP TRIGGER IF EXISTS trg_balance_event_update_snapshot ON ledger.balance_event;
DROP FUNCTION IF EXISTS ledger.update_balance_snapshot();
DROP TABLE IF EXISTS ledger.balance_snapshot CASCADE;
DROP TABLE IF EXISTS ledger.balance_event CASCADE;
COMMIT;
