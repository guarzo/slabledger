CREATE TABLE IF NOT EXISTS cl_sales_comps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gem_rate_id TEXT NOT NULL,
    item_id TEXT NOT NULL,
    sale_date DATE NOT NULL,
    price_cents INTEGER NOT NULL,
    platform TEXT NOT NULL,
    listing_type TEXT NOT NULL DEFAULT '',
    seller TEXT NOT NULL DEFAULT '',
    item_url TEXT NOT NULL DEFAULT '',
    slab_serial TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_sales_comps_item
    ON cl_sales_comps(gem_rate_id, item_id);
CREATE INDEX IF NOT EXISTS idx_cl_sales_comps_gem_rate
    ON cl_sales_comps(gem_rate_id, sale_date DESC);
