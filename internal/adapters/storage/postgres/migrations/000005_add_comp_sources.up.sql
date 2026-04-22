-- mm_sales_comps: individual completed sales from MarketMovers
CREATE TABLE IF NOT EXISTS mm_sales_comps (
    mm_collectible_id BIGINT NOT NULL,
    sale_id BIGINT NOT NULL,
    sale_date TEXT NOT NULL,
    price_cents INT NOT NULL,
    platform TEXT NOT NULL DEFAULT '',
    listing_type TEXT NOT NULL DEFAULT '',
    seller TEXT NOT NULL DEFAULT '',
    sale_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (mm_collectible_id, sale_id)
);
CREATE INDEX IF NOT EXISTS idx_mm_sales_comps_lookup ON mm_sales_comps (mm_collectible_id, sale_date DESC);

-- dh_comp_cache: pre-aggregated sales analytics from DH graded-sales-analytics
CREATE TABLE IF NOT EXISTS dh_comp_cache (
    dh_card_id INT NOT NULL,
    grade TEXT NOT NULL,
    total_sales INT NOT NULL DEFAULT 0,
    recent_count_90d INT NOT NULL DEFAULT 0,
    median_cents INT NOT NULL DEFAULT 0,
    avg_cents INT NOT NULL DEFAULT 0,
    min_cents INT NOT NULL DEFAULT 0,
    max_cents INT NOT NULL DEFAULT 0,
    price_change_30d_pct REAL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dh_card_id, grade)
);
