-- Sale outcome enrichment: capture HOW cards sold, not just that they sold.
ALTER TABLE campaign_sales ADD COLUMN original_list_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_sales ADD COLUMN price_reductions INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_sales ADD COLUMN days_listed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_sales ADD COLUMN sold_at_asking_price INTEGER NOT NULL DEFAULT 0;
