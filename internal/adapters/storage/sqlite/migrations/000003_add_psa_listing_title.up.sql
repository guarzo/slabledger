-- Add PSA listing title column to campaign_purchases.
-- Stores the raw PSA listing title for LLM-based fallback matching
-- in the pricing pipeline (CardHedger tier-3 query).
ALTER TABLE campaign_purchases ADD COLUMN psa_listing_title TEXT NOT NULL DEFAULT '';

-- Recreate stale_prices view to include psa_listing_title from campaign_purchases.
-- The LEFT JOIN picks the best matching purchase (by card_name + set_name + card_number)
-- so the price refresh scheduler can pass the listing title to secondary pricing sources.
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
