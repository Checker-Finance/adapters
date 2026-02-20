-- Rollback for 0004_balance_summary_view.sql
BEGIN;
DROP FUNCTION IF EXISTS ledger.fn_refresh_balance_summary();
DROP INDEX IF EXISTS ledger.idx_vw_balance_summary_total_balance;
DROP INDEX IF EXISTS ledger.idx_vw_balance_summary_pk;
DROP MATERIALIZED VIEW IF EXISTS ledger.vw_balance_summary;
COMMIT;
