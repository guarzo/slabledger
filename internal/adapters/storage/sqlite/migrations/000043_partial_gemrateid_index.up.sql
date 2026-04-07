-- Replace full index with partial index (consistent with other optional-column indexes)
DROP INDEX IF EXISTS idx_purchases_gem_rate_id;
CREATE INDEX idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id) WHERE gem_rate_id != '';
