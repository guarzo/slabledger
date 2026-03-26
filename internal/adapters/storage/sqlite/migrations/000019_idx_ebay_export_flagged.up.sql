CREATE INDEX IF NOT EXISTS idx_campaign_purchases_ebay_export_flagged_at
    ON campaign_purchases (ebay_export_flagged_at)
    WHERE ebay_export_flagged_at IS NOT NULL;
