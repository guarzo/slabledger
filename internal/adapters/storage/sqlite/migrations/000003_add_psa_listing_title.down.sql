-- Restore original stale_prices view without psa_listing_title.
-- Must happen BEFORE dropping the column, because the UP migration's view
-- references campaign_purchases.psa_listing_title — SQLite refuses DROP COLUMN
-- while any VIEW references the column.
DROP VIEW IF EXISTS stale_prices;
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
    CASE WHEN ra.card_name IS NOT NULL THEN 1 ELSE 0 END as recently_accessed
FROM price_history ph
LEFT JOIN recent_access ra ON ra.card_name = ph.card_name AND ra.set_name = ph.set_name AND ra.card_number = ph.card_number
WHERE
    (ph.price_cents > 10000 AND ph.updated_at < DATETIME('now', '-12 hours'))
    OR (ph.price_cents > 5000 AND ph.price_cents <= 10000 AND ph.updated_at < DATETIME('now', '-24 hours'))
    OR (ph.price_cents <= 5000 AND ph.updated_at < DATETIME('now', '-48 hours'));

-- Now safe to drop the column — no view references it anymore.
ALTER TABLE campaign_purchases DROP COLUMN psa_listing_title;
