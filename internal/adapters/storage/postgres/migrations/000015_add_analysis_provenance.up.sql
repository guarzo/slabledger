-- CL value snapshot at purchase/first-enrichment time. 0 = no snapshot yet.
-- Never overwritten once set (guard in UpdatePurchaseCLValue).
ALTER TABLE campaign_purchases ADD COLUMN cl_value_at_purchase_cents BIGINT NOT NULL DEFAULT 0;

-- TRUE = sale attributed to invoice-driven forced liquidation (heuristic:
-- forced channel within 6 days before an invoice due date; operator-overridable).
ALTER TABLE campaign_sales ADD COLUMN forced_liquidation BOOLEAN NOT NULL DEFAULT FALSE;
