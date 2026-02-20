BEGIN;

DROP MATERIALIZED VIEW IF EXISTS ledger.vw_balance_summary;

CREATE MATERIALIZED VIEW ledger.vw_balance_summary AS
SELECT
    client_id,
    venue,
    COUNT(DISTINCT instrument) AS instrument_count,
    COALESCE(SUM(available), 0) AS total_available,
    COALESCE(SUM(held), 0) AS total_held,
    COALESCE(SUM(available + held), 0) AS total_balance,
    MIN(as_of) AS oldest_update,
    MAX(as_of) AS newest_update
FROM ledger.balance_snapshot
GROUP BY client_id, venue
    WITH NO DATA;

COMMENT ON MATERIALIZED VIEW ledger.vw_balance_summary IS
'Aggregated snapshot of balances across all instruments, grouped by tenant/client/venue. Updated via REFRESH.';


-- 🧠 Indexes for fast lookups
CREATE UNIQUE INDEX IF NOT EXISTS idx_vw_balance_summary_pk
    ON ledger.vw_balance_summary (client_id, venue);

CREATE INDEX IF NOT EXISTS idx_vw_balance_summary_total_balance
    ON ledger.vw_balance_summary (total_balance DESC);


-- 🛠️ Helper function: refresh the materialized view safely
CREATE OR REPLACE FUNCTION ledger.fn_refresh_balance_summary()
RETURNS VOID AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY ledger.vw_balance_summary;
    RAISE NOTICE 'ledger.vw_balance_summary refreshed successfully.';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

COMMENT ON FUNCTION ledger.fn_refresh_balance_summary() IS
'Safely refreshes the materialized view ledger.vw_balance_summary for analytics and reporting.';

COMMIT;