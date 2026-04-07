-- Revert to full (non-partial) index
DROP INDEX IF EXISTS idx_purchases_gem_rate_id;
CREATE INDEX idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id);
