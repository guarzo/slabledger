-- Rollback: recreate the objects dropped in 000038_drop_price_history.up.sql.
--
-- IMPORTANT: Data is NOT recoverable. This restores schema only so that the
-- migrate tool can step back to migration 000037 without errors. Any data that
-- existed before the up migration was run is permanently gone.
--
-- Objects recreated (schema only):
--   price_history TABLE + indexes
--   stale_prices VIEW  (from 000037 — current schema version)
--   price_refresh_queue TABLE + indexes
--   discovery_failures TABLE + index
--
-- Orphan rows that were deleted from api_calls, api_rate_limits, and
-- card_id_mappings are NOT restored (data is unrecoverable).

-- ── 0. price_history TABLE ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS price_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    confidence REAL DEFAULT 1.0,
    source TEXT NOT NULL,
    fusion_source_count INTEGER,
    fusion_outliers_removed INTEGER,
    fusion_method TEXT,
    price_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, card_number, grade, source, price_date)
);

CREATE INDEX IF NOT EXISTS idx_price_history_card ON price_history(card_name, set_name, grade);
CREATE INDEX IF NOT EXISTS idx_price_history_staleness ON price_history(source, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_price_history_date ON price_history(price_date DESC);
CREATE INDEX IF NOT EXISTS idx_price_history_lookup ON price_history(card_name, set_name, card_number, grade, source, price_date DESC);

-- ── 1. stale_prices VIEW (000037 schema — current version) ───────────────────

CREATE VIEW IF NOT EXISTS stale_prices AS
WITH recent_access AS (
    SELECT DISTINCT card_name, set_name, card_number
    FROM card_access_log
    WHERE accessed_at > DATETIME('now', '-24 hours')
)
SELECT
    ph.card_name, ph.card_number, ph.set_name, ph.grade, ph.source,
    ph.price_cents, ph.price_date, ph.updated_at,
    ROUND((JULIANDAY('now') - JULIANDAY(ph.updated_at)) * 24, 1) as hours_old,
    CASE WHEN ph.price_cents > 10000 THEN 1 WHEN ph.price_cents > 5000 THEN 2 ELSE 3 END as priority,
    CASE WHEN ra.card_name IS NOT NULL THEN 1 ELSE 0 END as recently_accessed,
    COALESCE(cp.psa_listing_title, '') as psa_listing_title
FROM price_history ph
LEFT JOIN recent_access ra ON ra.card_name = ph.card_name AND ra.set_name = ph.set_name AND ra.card_number = ph.card_number
LEFT JOIN (
    SELECT card_name, card_number, set_name, psa_listing_title,
           ROW_NUMBER() OVER (PARTITION BY card_name, card_number, set_name ORDER BY created_at DESC) as rn
    FROM campaign_purchases WHERE psa_listing_title != ''
) cp ON cp.card_name = ph.card_name AND cp.set_name = ph.set_name AND cp.card_number = ph.card_number AND cp.rn = 1
WHERE
    (ph.price_cents > 10000 AND ph.updated_at < DATETIME('now', '-12 hours'))
    OR (ph.price_cents > 5000 AND ph.price_cents <= 10000 AND ph.updated_at < DATETIME('now', '-24 hours'))
    OR (ph.price_cents <= 5000 AND ph.updated_at < DATETIME('now', '-48 hours'));

-- ── 2. price_refresh_queue TABLE ─────────────────────────────────────────────
-- Recreated with the 000037 schema (no source CHECK constraint).

CREATE TABLE IF NOT EXISTS price_refresh_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    grade TEXT NOT NULL CHECK(grade IN (
        'PSA 10', 'PSA 9', 'PSA 8', 'PSA 7', 'PSA 6',
        'PSA 5', 'PSA 4', 'PSA 3', 'PSA 2', 'PSA 1',
        'BGS 10', 'BGS 9.5', 'BGS 9', 'BGS 8.5', 'BGS 8',
        'CGC 10', 'CGC 9.5', 'CGC 9', 'CGC 8.5', 'CGC 8',
        'Raw', 'Ungraded'
    )),
    source TEXT NOT NULL,
    priority INTEGER DEFAULT 2 CHECK(priority IN (1, 2, 3)),
    scheduled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_attempted_at TIMESTAMP,
    attempts INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'in_progress', 'completed', 'failed')),
    error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, grade, source),
    FOREIGN KEY (source) REFERENCES api_rate_limits(provider) ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_refresh_queue_priority ON price_refresh_queue(priority ASC, scheduled_at ASC)
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_refresh_queue_status ON price_refresh_queue(status, last_attempted_at);

-- ── 3. discovery_failures TABLE ───────────────────────────────────────────────
-- Recreated with the 000005 schema (provider and failure_reason have no DEFAULT).

CREATE TABLE IF NOT EXISTS discovery_failures (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    failure_reason TEXT NOT NULL,
    query_attempted TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 1,
    last_attempted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, card_number, provider)
);

CREATE INDEX IF NOT EXISTS idx_discovery_failures_provider
    ON discovery_failures(provider, last_attempted_at DESC);
