ALTER TABLE campaign_purchases ADD COLUMN card_year TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN ebay_export_flagged_at TIMESTAMP NULL;
