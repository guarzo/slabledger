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
-- SEQUENCING (important):
--   Run this backfill BEFORE the next full-history PSA re-import. The invoice
--   auto-detect heal path fills an empty due_date with a uniform invoice_date + 7
--   days (not era-aware). If a full-history re-import runs first, pre-July empty
--   rows get +7 and this script's `WHERE due_date = ''` will then skip them,
--   leaving a wrong-era value. Impact is limited to reporting on paid historical
--   rows (persisted forced_liquidation flags on past sales are not recomputed),
--   but running this script first keeps the era-aware terms intact.
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
--   After applying, revert exactly the rows this script changed:
--     UPDATE invoices i SET due_date = ''
--     FROM backfill_due_date_targets t
--     WHERE i.id = t.id;
--   (backfill_due_date_targets is the temp table built in Step 0; it lives for
--    the psql session, so run the rollback in the SAME session as the apply.)
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Step 0: Build the target set once — the ids to change and their computed due
--   dates. The era CASE lives here only; Step 1 and the rollback reference this
--   table so the preview, the update, and the revert can never drift.
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE backfill_due_date_targets ON COMMIT PRESERVE ROWS AS
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
WHERE due_date = '';

-- Preview: rows that WILL be changed and their computed due dates.
SELECT id, invoice_date, status, era, computed_due_date
FROM backfill_due_date_targets
ORDER BY invoice_date;

-- ---------------------------------------------------------------------------
-- Step 1: Apply — UNCOMMENT after the Step 0 preview is reviewed. Reads the
--   computed due date straight from the target table (no recomputation here).
-- ---------------------------------------------------------------------------
-- BEGIN;
--
-- UPDATE invoices i
-- SET due_date = t.computed_due_date,
--     updated_at = now()
-- FROM backfill_due_date_targets t
-- WHERE i.id = t.id;
--
-- COMMIT;

-- ---------------------------------------------------------------------------
-- Step 2: Verify — AFTER the Step 1 apply block is uncommented and run, this
--   returns zero. On a dry run (apply block still commented) a NONZERO count is
--   expected — it is the set of rows the apply block WILL change.
-- ---------------------------------------------------------------------------
SELECT count(*) AS remaining_empty_due_dates FROM invoices WHERE due_date = '';
