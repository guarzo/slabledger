-- One-shot data repair; no schema change to undo. The previously-sold state
-- cannot be re-derived once campaign_sales rows are gone.
SELECT 1;
