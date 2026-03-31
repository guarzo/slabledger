CREATE INDEX IF NOT EXISTS idx_purchases_invoice_date ON campaign_purchases(invoice_date)
    WHERE invoice_date != '';
