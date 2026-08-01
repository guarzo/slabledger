-- Purchase decision-time provenance (nullable: NULL=not captured, value=observed).
ALTER TABLE campaign_purchases
    ADD COLUMN cl_confidence_at_purchase    SMALLINT,
    ADD COLUMN population_at_purchase        BIGINT,
    ADD COLUMN dh_confidence_at_purchase     DOUBLE PRECISION,
    ADD COLUMN source_count_at_purchase      BIGINT,
    ADD COLUMN active_listings_at_purchase   BIGINT,
    ADD COLUMN sales_last_30d_at_purchase    BIGINT;

-- Sale provenance.
ALTER TABLE campaign_sales
    ADD COLUMN sale_reason             TEXT NOT NULL DEFAULT ''
        CHECK (sale_reason IN ('', 'discretionary', 'invoice_pressure', 'aging_policy', 'bulk_lot', 'show_clearout')),
    ADD COLUMN cl_value_at_sale_cents  BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN channel_fee_pct_at_sale DOUBLE PRECISION;

-- Backfill sale_reason from derivable data.
UPDATE campaign_sales SET sale_reason = 'invoice_pressure'
    WHERE forced_liquidation = TRUE;

UPDATE campaign_sales s SET sale_reason = 'invoice_pressure'
    FROM campaign_purchases p
    WHERE s.purchase_id = p.id
      AND s.sale_reason = ''
      AND s.sale_channel IN ('inperson', 'local', 'cardshow')
      AND p.cl_value_at_purchase_cents > 0
      AND s.sale_price_cents < 0.80 * p.cl_value_at_purchase_cents;

UPDATE campaign_sales SET sale_reason = 'discretionary' WHERE sale_reason = '';

-- forced_liquidation stays a PLAIN boolean (NOT generated), so the previous
-- image — which INSERTs forced_liquidation explicitly — keeps working after an
-- image-only rollback (see docs/OPERATIONS.md; no DB down-migration on rollback).
-- The app maintains it alongside sale_reason (forced == reason='invoice_pressure').
-- Re-sync it here to match the just-backfilled reasons.
UPDATE campaign_sales SET forced_liquidation = (sale_reason = 'invoice_pressure');

-- Rollback-window compatibility: during a rollback to the previous image, that
-- binary INSERTs rows WITHOUT sale_reason (defaults to ''), setting only
-- forced_liquidation. Re-deploying the new image does NOT rerun this migration,
-- so those rows would keep sale_reason='' and analysis would silently skip them.
-- This trigger derives sale_reason from the boolean on any INSERT/UPDATE that
-- leaves sale_reason empty, so legacy-shaped writes are never lost.
CREATE OR REPLACE FUNCTION public.campaign_sales_derive_reason() RETURNS trigger AS $$
BEGIN
    IF NEW.sale_reason IS NULL OR NEW.sale_reason = '' THEN
        NEW.sale_reason := CASE WHEN NEW.forced_liquidation
                                THEN 'invoice_pressure' ELSE 'discretionary' END;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Scope: INSERT (rollback-window: old image inserts with only forced_liquidation)
-- and UPDATE OF forced_liquidation (legacy boolean writes). NOT a blanket UPDATE,
-- so a future code path can still intentionally clear sale_reason to '' without
-- the trigger silently rewriting it.
CREATE TRIGGER campaign_sales_derive_reason_trg
    BEFORE INSERT OR UPDATE OF forced_liquidation ON campaign_sales
    FOR EACH ROW EXECUTE FUNCTION public.campaign_sales_derive_reason();
