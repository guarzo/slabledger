-- CardLadder value coverage by purchase month and intake cohort.
--
-- Run:  psql "$SUPABASE_DB_URL" -f scripts/cl-coverage.sql
--
-- Mirrors GetCLCoverageByMonth in
-- internal/adapters/storage/postgres/cl_coverage_store.go. The bucket CASE
-- expression below and the one in that file MUST stay identical; if you change
-- one, change both.
--
-- The `TIMESTAMP '2026-04-13 04:00:13'` literal in the 'pre_cl' arm below is a
-- hardcoded copy of the CLCoverageEraStart constant in that file. It is NOT
-- read from that constant -- this script must run with zero application
-- dependencies during an incident -- so if CLCoverageEraStart ever changes,
-- update this literal by hand in the same change.
--
-- Coverage is measured on cl_value_updated_at, NOT cl_value_cents. The latter
-- has a second writer (the Shopify external import) that never calls
-- CardLadder, so 339 production rows carry a positive cl_value_cents that
-- CardLadder never produced. cl_value_updated_at has exactly one writer.
WITH classified AS (
    SELECT
        left(p.purchase_date, 7) AS month,
        CASE WHEN p.purchase_source <> '' THEN 'campaign' ELSE 'external' END AS cohort,
        CASE
            -- CardLadder returned a positive value at least once.
            WHEN p.cl_value_updated_at <> ''                     THEN 'resolved'
            -- Asked and failed for a real reason (today: always 'no_value').
            WHEN p.cl_last_error NOT IN ('', 'quota_exhausted')  THEN 'unresolved'
            -- Still reachable by the sweep: no linked sale, campaign open.
            -- Matches ListAllUnsoldPurchases (purchase_store.go:177-183).
            -- Tested BEFORE both the quota arm and the era cutoff, because
            -- eligibility is what actually decides whether a row will ever be
            -- answered. A quota-marked row that is still eligible is pending;
            -- one that has since been sold or closed is NOT -- the sweep can
            -- no longer see it, and nothing clears cl_last_error on sale
            -- (sale_store.go writes no CL column; purchase_store.go:284 is the
            -- sole writer). Ordering quota first would park such a row in
            -- 'pending' forever.
            WHEN c.phase <> 'closed'
                 AND NOT EXISTS (
                     SELECT 1 FROM campaign_sales s
                     WHERE s.purchase_id = p.id
                 )                                               THEN 'pending'
            -- Created before CardLadder ever ran, and never touched by it.
            -- The cl_last_error = '' guard matters: a quota-marked row proves
            -- the sweep DID reach it, so it can never honestly be pre_cl no
            -- matter how old it is.
            WHEN p.cl_last_error = ''
                 AND p.created_at < TIMESTAMP '2026-04-13 04:00:13'    THEN 'pre_cl'
            -- Never answered and no longer reachable by the sweep.
            ELSE 'stranded'
        END AS bucket,
        CASE
            WHEN p.purchase_source <> '' AND p.campaign_id = 'external'
            THEN 1 ELSE 0
        END AS reassigned
    FROM campaign_purchases p
    INNER JOIN campaigns c ON c.id = p.campaign_id
    -- NOTE: both filters from GetCLPriceStats (s.id IS NULL, phase != 'closed')
    -- are deliberately ABSENT. This query covers ALL purchases, including sold
    -- ones -- 1,473 of 1,542 priced rows are already sold, which is precisely
    -- why the freshness panel could not have caught the original incident. The
    -- join and the NOT EXISTS above are used to LABEL rows, not to exclude
    -- them. Do not "fix" this by adding a WHERE clause.
)
SELECT
    month,
    cohort,
    count(*)                                        AS rows,
    count(*) FILTER (WHERE bucket = 'resolved')     AS resolved,
    count(*) FILTER (WHERE bucket = 'unresolved')   AS unresolved,
    count(*) FILTER (WHERE bucket = 'pending')      AS pending,
    count(*) FILTER (WHERE bucket = 'stranded')     AS stranded,
    count(*) FILTER (WHERE bucket = 'pre_cl')       AS pre_cl,
    sum(reassigned)                                 AS reassigned,
    -- resolved / (resolved + unresolved). pending, stranded and pre_cl are
    -- excluded from the denominator: CardLadder never answered for those rows,
    -- so they are evidence neither for nor against coverage. NULL, not 0, on an
    -- empty denominator.
    round(
        100.0 * count(*) FILTER (WHERE bucket = 'resolved')
        / nullif(count(*) FILTER (WHERE bucket IN ('resolved', 'unresolved')), 0)
    , 1)                                            AS pct
FROM classified
GROUP BY month, cohort
ORDER BY month DESC, cohort;
