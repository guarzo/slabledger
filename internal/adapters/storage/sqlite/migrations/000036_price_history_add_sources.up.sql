-- Add 'justtcg' and 'doubleholo' to price_history.source CHECK constraint.
-- SQLite cannot ALTER CHECK constraints, so we recreate the table.
-- This also removes the now-obsolete 'pokemonprice' and 'cardmarket' entries.

-- Step 0: Drop view that depends on price_history (recreated at the end).
DROP VIEW IF EXISTS stale_prices;

-- Step 1: Create new price_history table with updated constraint.
CREATE TABLE price_history_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    confidence REAL DEFAULT 1.0,
    source TEXT NOT NULL CHECK(source IN (
        'pricecharting', 'cardhedger', 'fusion', 'justtcg', 'doubleholo'
    )),
    fusion_source_count INTEGER,
    fusion_outliers_removed INTEGER,
    fusion_method TEXT,
    price_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, card_number, grade, source, price_date)
);

-- Step 2: Copy rows that have valid source values in the new constraint.
-- Rows with 'pokemonprice' or 'cardmarket' are dropped (no longer active sources).
INSERT INTO price_history_new
    SELECT id, card_name, set_name, card_number, grade, price_cents, confidence,
           source, fusion_source_count, fusion_outliers_removed, fusion_method,
           price_date, created_at, updated_at
    FROM price_history
    WHERE source IN ('pricecharting', 'cardhedger', 'fusion', 'justtcg', 'doubleholo');

-- Step 3: Swap tables.
DROP TABLE price_history;
ALTER TABLE price_history_new RENAME TO price_history;

-- Step 4: Recreate indexes.
CREATE INDEX idx_price_history_card ON price_history(card_name, set_name, grade);
CREATE INDEX idx_price_history_staleness ON price_history(source, updated_at DESC);
CREATE INDEX idx_price_history_date ON price_history(price_date DESC);
CREATE INDEX idx_price_history_lookup ON price_history(card_name, set_name, card_number, grade, source, price_date DESC);

-- Step 5: Recreate stale_prices view (same definition as in 000003 migration).
CREATE VIEW stale_prices AS
WITH recent_access AS (
    SELECT DISTINCT card_name, set_name, card_number
    FROM card_access_log
    WHERE accessed_at > DATETIME('now', '-24 hours')
)
SELECT
    ph.card_name,
    ph.card_number,
    ph.set_name,
    ph.grade,
    ph.source,
    ph.price_cents,
    ph.price_date,
    ph.updated_at,
    ROUND((JULIANDAY('now') - JULIANDAY(ph.updated_at)) * 24, 1) as hours_old,
    CASE
        WHEN ph.price_cents > 10000 THEN 1
        WHEN ph.price_cents > 5000 THEN 2
        ELSE 3
    END as priority,
    CASE WHEN ra.card_name IS NOT NULL THEN 1 ELSE 0 END as recently_accessed,
    COALESCE(cp.psa_listing_title, '') as psa_listing_title
FROM price_history ph
LEFT JOIN recent_access ra ON ra.card_name = ph.card_name AND ra.set_name = ph.set_name AND ra.card_number = ph.card_number
LEFT JOIN (
    SELECT card_name, card_number, set_name, psa_listing_title,
           ROW_NUMBER() OVER (PARTITION BY card_name, card_number, set_name ORDER BY created_at DESC) as rn
    FROM campaign_purchases
    WHERE psa_listing_title != ''
) cp ON cp.card_name = ph.card_name AND cp.set_name = ph.set_name AND cp.card_number = ph.card_number AND cp.rn = 1
WHERE
    (ph.price_cents > 10000 AND ph.updated_at < DATETIME('now', '-12 hours'))
    OR (ph.price_cents > 5000 AND ph.price_cents <= 10000 AND ph.updated_at < DATETIME('now', '-24 hours'))
    OR (ph.price_cents <= 5000 AND ph.updated_at < DATETIME('now', '-48 hours'));
