-- Rollback for 0003_balance_projection_triggers.sql
BEGIN;
DROP FUNCTION IF EXISTS ledger.fn_rebuild_balance_snapshot();
COMMIT;
