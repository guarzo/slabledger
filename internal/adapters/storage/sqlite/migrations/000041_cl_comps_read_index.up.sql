-- Composite index for comp analytics read queries (GetCompSummary, fetchRecentPricesAndDates, fetchPlatformBreakdown).
-- Covers WHERE gem_rate_id = ? AND condition = ? AND sale_date >= ? predicates.
CREATE INDEX IF NOT EXISTS idx_cl_sales_comps_gem_cond_date
    ON cl_sales_comps(gem_rate_id, condition, sale_date DESC);
