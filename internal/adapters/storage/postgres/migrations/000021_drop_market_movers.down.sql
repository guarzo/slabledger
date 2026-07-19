-- Recreate the Market Movers schema (structure only; historical data is not restored).
-- Mirrors the original definitions from 000001_initial_schema and 000005_add_comp_sources.

ALTER TABLE campaign_purchases
    ADD COLUMN IF NOT EXISTS mm_value_cents      BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mm_trend_pct        DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mm_sales_30d        BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mm_active_low_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS mm_value_updated_at TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mm_last_error       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS mm_last_error_at    TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_purchases_mm_last_error
    ON campaign_purchases(mm_last_error)
    WHERE mm_last_error != '';

CREATE TABLE IF NOT EXISTS mm_card_mappings (
    slab_serial           TEXT PRIMARY KEY,
    mm_collectible_id     BIGINT NOT NULL,
    updated_at            TEXT NOT NULL DEFAULT '',
    mm_master_id          BIGINT NOT NULL DEFAULT 0,
    mm_search_title       TEXT NOT NULL DEFAULT '',
    mm_collection_item_id BIGINT NOT NULL DEFAULT 0
);

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
