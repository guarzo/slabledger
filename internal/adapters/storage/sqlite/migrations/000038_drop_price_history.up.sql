-- Drop price_history table and all dependent/related objects that are no longer
-- used now that DH (DoubleHolo) is the sole price source and computes prices
-- in-memory without persisting to price_history.
--
-- Objects removed:
--   stale_prices VIEW          — depends on price_history; no longer queried
--   price_history TABLE        — ~374K legacy rows, no production code writes here
--   price_refresh_queue TABLE  — empty; was part of the old multi-source workflow
--   discovery_failures TABLE   — CardHedger-only table; source was removed 2026-04-06
--
-- Orphan rows in api_calls, api_rate_limits, and card_id_mappings for removed
-- providers are also cleaned up.
--
-- NOTE: price_refresh_queue has a FOREIGN KEY referencing api_rate_limits(provider),
-- so it must be dropped BEFORE we delete from api_rate_limits.

-- ── 0. stale_prices VIEW (depends on price_history — drop first) ──────────────

DROP VIEW IF EXISTS stale_prices;

-- ── 1. price_history TABLE ────────────────────────────────────────────────────

DROP TABLE IF EXISTS price_history;

-- ── 2. price_refresh_queue TABLE (FK to api_rate_limits — drop before cleanup) ─

DROP TABLE IF EXISTS price_refresh_queue;

-- ── 3. discovery_failures TABLE ───────────────────────────────────────────────

DROP TABLE IF EXISTS discovery_failures;

-- ── 4. Clean orphan rows for removed providers ────────────────────────────────
-- Providers removed: cardhedger, pricecharting, justtcg, pokemonprice, cardmarket

DELETE FROM api_calls
WHERE provider IN ('cardhedger', 'pricecharting', 'justtcg', 'pokemonprice', 'cardmarket');

DELETE FROM api_rate_limits
WHERE provider IN ('cardhedger', 'pricecharting', 'justtcg', 'pokemonprice', 'cardmarket');

DELETE FROM card_id_mappings
WHERE provider IN ('cardhedger', 'pricecharting', 'justtcg', 'pokemonprice', 'cardmarket');
