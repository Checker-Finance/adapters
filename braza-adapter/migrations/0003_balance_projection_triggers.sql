CREATE OR REPLACE FUNCTION ledger.fn_rebuild_balance_snapshot()
RETURNS VOID AS $$
BEGIN
TRUNCATE TABLE ledger.balance_snapshot;

INSERT INTO ledger.balance_snapshot (
    client_id, venue, instrument,
    available, held, can_buy, can_sell, as_of
)
SELECT DISTINCT ON (client_id, venue, instrument)
    client_id, venue, instrument,
    available, held, can_buy, can_sell, recorded_at AS as_of
FROM ledger.balance_event
ORDER BY client_id, venue, instrument, recorded_at DESC;

RAISE NOTICE 'Balance snapshot successfully rebuilt from balance_event log.';
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

COMMENT ON FUNCTION ledger.fn_rebuild_balance_snapshot() IS
'Rebuilds the current snapshot state from all historical balance_event records.';