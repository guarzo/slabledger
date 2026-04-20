-- Reshape idx_campaign_purchases_received_at as a partial index. The column
-- is TEXT NULL with roughly 77% of rows carrying NULL (cards still at PSA,
-- never scanned in). Every query on the column filters by IS NOT NULL or
-- IS NULL, so a btree over the entire column wastes space and write cost
-- on the NULL rows that the non-NULL predicate can never hit.
--
-- Drop the full index created in 000002 and replace it with a partial one.
-- Queries using "AND received_at IS NOT NULL" pick up the new index
-- automatically; the IS NULL paths (finance_store.go:150) fall back to a
-- scan, which is correct — that query is "which cards are still at PSA?",
-- a broad scan rather than a pinpoint lookup.

DROP INDEX IF EXISTS idx_campaign_purchases_received_at;

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_received_at
    ON campaign_purchases(received_at)
    WHERE received_at IS NOT NULL;
