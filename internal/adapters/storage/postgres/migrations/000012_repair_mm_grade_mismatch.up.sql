-- Repair MM mappings poisoned by grade mismatch (see 2026-06-02 fix).
-- A row is "wrong" when the stored mm_search_title clearly does NOT mention
-- the purchase's "<grader> <grade>" token. Rows with no search_title are left
-- alone — they predate the search-title column being populated.

WITH bad AS (
    SELECT m.slab_serial
    FROM mm_card_mappings m
    JOIN campaign_purchases p ON p.cert_number = m.slab_serial
    WHERE COALESCE(m.mm_search_title, '') <> ''
      AND p.grade_value > 0
      AND UPPER(m.mm_search_title) !~
          ('(^|[^A-Z])' ||
           UPPER(COALESCE(NULLIF(p.grader, ''), 'PSA')) ||
           ' ' ||
           -- FormatGrade: integer grades have no decimal, half-grades like 9.5 do.
           CASE
             WHEN p.grade_value = TRUNC(p.grade_value)
               THEN TRUNC(p.grade_value)::TEXT
             ELSE p.grade_value::TEXT
           END ||
           '([^0-9.]|$)')
),
deleted_mappings AS (
    DELETE FROM mm_card_mappings
    WHERE slab_serial IN (SELECT slab_serial FROM bad)
    RETURNING slab_serial
)
-- Clear MM price fields ONLY on purchases whose mapping was just deleted by
-- this migration. The next MM refresh / on-demand price will refill them.
-- Untouched: purchases that never had a mapping (those weren't poisoned).
UPDATE campaign_purchases
SET mm_value_cents = 0,
    mm_trend_pct = 0,
    mm_sales_30d = 0,
    mm_active_low_cents = 0,
    mm_value_updated_at = NULL
WHERE cert_number IN (SELECT slab_serial FROM deleted_mappings);
