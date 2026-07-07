-- =============================================================================
-- Backfill invoice due dates — 2026-07-07
-- =============================================================================
--
-- Context:
--   Historical invoices were created without a due_date (pre-cutover code, and
--   autoDetectInvoices' update branch never populated it). The forced-liquidation
--   heuristic (IsForcedLiquidation) keys on Invoice.DueDate. This one-off script
--   backfills the empty rows. All affected rows are paid & historical, so this is
--   a finance-reporting cleanup; it does not change detection on new sales.
--
--   Era-aware terms (what PSA actually applied when each invoice was issued):
--     invoice_date <  2026-05-15            -> +14 calendar days
--     2026-05-15 .. 2026-06-30 (inclusive)  -> +1 business day
--     invoice_date >= 2026-07-01            -> +7 calendar days
--
-- Target DB: Postgres (Supabase in prod, local Postgres in dev). Dates are TEXT
--   in YYYY-MM-DD; cast to ::date for arithmetic, format back with to_char.
--
-- Usage:
--   # DRY RUN — preview only, no writes (Step 0 runs outside any transaction):
--   psql "$DATABASE_URL" -f scripts/backfill-invoice-due-dates-2026-07-07.sql
--   # ...review Step 0 output with the operator, THEN uncomment Step 1's
--   #    BEGIN/UPDATE/COMMIT block and re-run to apply.
--
-- Rollback:
--   UPDATE invoices SET due_date = '' WHERE id IN (<ids listed in the preview>);
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Step 0: Preview — rows that WILL be changed and their computed due dates
-- ---------------------------------------------------------------------------
SELECT
    id,
    invoice_date,
    status,
    CASE
        WHEN invoice_date::date < DATE '2026-05-15' THEN 'pre-05-15 (+14d)'
        WHEN invoice_date::date <= DATE '2026-06-30' THEN 'mid (+1 business day)'
        ELSE 'post-07-01 (+7d)'
    END AS era,
    to_char(
        CASE
            WHEN invoice_date::date < DATE '2026-05-15'
                THEN invoice_date::date + 14
            WHEN invoice_date::date <= DATE '2026-06-30'
                THEN invoice_date::date + CASE EXTRACT(DOW FROM invoice_date::date)
                                              WHEN 5 THEN 3  -- Fri -> Mon
                                              WHEN 6 THEN 2  -- Sat -> Mon
                                              ELSE 1          -- Sun/Mon-Thu -> next day
                                          END
            ELSE invoice_date::date + 7
        END, 'YYYY-MM-DD') AS computed_due_date
FROM invoices
WHERE due_date = ''
ORDER BY invoice_date;

-- ---------------------------------------------------------------------------
-- Step 1: Apply — UNCOMMENT after the Step 0 preview is reviewed
-- ---------------------------------------------------------------------------
-- BEGIN;
--
-- UPDATE invoices
-- SET due_date = to_char(
--         CASE
--             WHEN invoice_date::date < DATE '2026-05-15'
--                 THEN invoice_date::date + 14
--             WHEN invoice_date::date <= DATE '2026-06-30'
--                 THEN invoice_date::date + CASE EXTRACT(DOW FROM invoice_date::date)
--                                               WHEN 5 THEN 3
--                                               WHEN 6 THEN 2
--                                               ELSE 1
--                                           END
--             ELSE invoice_date::date + 7
--         END, 'YYYY-MM-DD'),
--     updated_at = now()
-- WHERE due_date = '';
--
-- COMMIT;

-- ---------------------------------------------------------------------------
-- Step 2: Verify — after applying, this should return zero rows
-- ---------------------------------------------------------------------------
SELECT count(*) AS remaining_empty_due_dates FROM invoices WHERE due_date = '';
