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
--   # DRY RUN — preview only, no invoice writes (Step 1 apply block stays
--   #   commented). Note: Step 0 does create the durable backfill_due_date_targets
--   #   staging table (needed so a later session can roll back); drop it via Step 3
--   #   if you abandon the run without applying.
--   psql "$DATABASE_URL" -f scripts/backfill-invoice-due-dates-2026-07-07.sql
--   # ...review Step 0 output with the operator, THEN uncomment Step 1's
--   #    BEGIN/UPDATE/COMMIT block and re-run to apply.
--
-- Rollback:
--   After applying, revert exactly the rows this script changed:
--     UPDATE invoices i SET due_date = ''
--     FROM backfill_due_date_targets t
--     WHERE i.id = t.id
--       AND i.due_date = t.computed_due_date;
--   The due_date equality guard skips any row a later process has since
--   changed, so rollback never clobbers a value this script didn't set.
--   backfill_due_date_targets is a DURABLE table (Step 0), so the rollback works
--   in a later psql session — not only the one that applied the change. Run the
--   Step 3 cleanup to drop it once the rollback window has closed.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Step 0: Build the target set once — the ids to change and their computed due
--   dates. The era CASE lives here only; Step 1 and the rollback reference this
--   table so the preview, the update, and the revert can never drift. A durable
--   (non-TEMP) table is used deliberately so the ids/values survive past the
--   psql session and remain available for a later rollback; drop it in Step 3.
--
--   The build is GUARDED: it only runs when backfill_due_date_targets does not
--   already exist. This is deliberate — after Step 1 applies, the invoices no
--   longer match `due_date = ''`, so a blind DROP-then-rebuild would silently
--   replace the durable rollback set with an EMPTY table and destroy the ability
--   to revert. Re-running this script while the table exists is therefore a
--   no-op for Step 0 (the existing preview/rollback set is preserved). To start
--   a genuinely fresh preview, run the Step 3 cleanup first, then re-run.
-- ---------------------------------------------------------------------------
DO $$
BEGIN
    IF to_regclass('backfill_due_date_targets') IS NOT NULL THEN
        RAISE NOTICE 'backfill_due_date_targets already exists — preserving the existing preview/rollback set. Run Step 3 cleanup to rebuild from scratch.';
        RETURN;
    END IF;

    CREATE TABLE backfill_due_date_targets AS
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
END $$;

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
-- WHERE i.id = t.id
--   AND i.due_date = '';
--
-- COMMIT;

-- ---------------------------------------------------------------------------
-- Step 2: Verify — AFTER the Step 1 apply block is uncommented and run, this
--   returns zero. On a dry run (apply block still commented) a NONZERO count is
--   expected — it is the set of rows the apply block WILL change.
-- ---------------------------------------------------------------------------
SELECT count(*) AS remaining_empty_due_dates
FROM invoices
WHERE due_date = '';

-- ---------------------------------------------------------------------------
-- Step 3: Cleanup — UNCOMMENT and run once the apply is confirmed and the
--   rollback window has closed. Drops the durable staging table from Step 0.
-- ---------------------------------------------------------------------------
-- DROP TABLE IF EXISTS backfill_due_date_targets;
