-- Drop the compatibility trigger first (it references sale_reason).
DROP TRIGGER IF EXISTS campaign_sales_derive_reason_trg ON campaign_sales;
DROP FUNCTION IF EXISTS campaign_sales_derive_reason();

-- forced_liquidation is already a plain boolean and is left in place (it existed
-- pre-000022). Only the columns 000022 added are dropped.
ALTER TABLE campaign_sales
    DROP COLUMN channel_fee_pct_at_sale,
    DROP COLUMN cl_value_at_sale_cents,
    DROP COLUMN sale_reason;

ALTER TABLE campaign_purchases
    DROP COLUMN sales_last_30d_at_purchase,
    DROP COLUMN active_listings_at_purchase,
    DROP COLUMN source_count_at_purchase,
    DROP COLUMN dh_confidence_at_purchase,
    DROP COLUMN population_at_purchase,
    DROP COLUMN cl_confidence_at_purchase;
