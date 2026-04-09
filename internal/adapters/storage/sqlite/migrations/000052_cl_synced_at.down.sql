-- SQLite does not support DROP COLUMN prior to 3.35.0; recreate without cl_synced_at.
-- Use explicit schema to preserve constraints, types, and defaults.

PRAGMA foreign_keys = OFF;

CREATE TABLE campaign_purchases_new (
    id                        TEXT PRIMARY KEY,
    campaign_id               TEXT NOT NULL,
    card_name                 TEXT NOT NULL,
    card_number               TEXT NOT NULL DEFAULT '',
    set_name                  TEXT NOT NULL DEFAULT '',
    cert_number               TEXT NOT NULL,
    population                INTEGER NOT NULL DEFAULT 0,
    cl_value_cents            INTEGER NOT NULL DEFAULT 0,
    buy_cost_cents            INTEGER NOT NULL DEFAULT 0,
    psa_sourcing_fee_cents    INTEGER NOT NULL DEFAULT 0,
    purchase_date             TEXT NOT NULL,
    last_sold_cents           INTEGER DEFAULT 0,
    lowest_list_cents         INTEGER DEFAULT 0,
    conservative_cents        INTEGER DEFAULT 0,
    median_cents              INTEGER DEFAULT 0,
    active_listings           INTEGER DEFAULT 0,
    sales_last_30d            INTEGER DEFAULT 0,
    trend_30d                 REAL DEFAULT 0,
    snapshot_date             TEXT DEFAULT '',
    created_at                DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    vault_status              TEXT NOT NULL DEFAULT '',
    invoice_date              TEXT NOT NULL DEFAULT '',
    was_refunded              INTEGER NOT NULL DEFAULT 0,
    front_image_url           TEXT NOT NULL DEFAULT '',
    back_image_url            TEXT NOT NULL DEFAULT '',
    purchase_source           TEXT NOT NULL DEFAULT '',
    grader                    TEXT NOT NULL DEFAULT 'PSA' CHECK(grader IN ('PSA', 'CGC', 'BGS', 'SGC')),
    grade_value               REAL NOT NULL DEFAULT 0,
    snapshot_json             TEXT NOT NULL DEFAULT '',
    snapshot_status           TEXT NOT NULL DEFAULT '' CHECK(snapshot_status IN ('', 'pending', 'failed', 'exhausted')),
    snapshot_retry_count      INTEGER NOT NULL DEFAULT 0,
    psa_listing_title         TEXT NOT NULL DEFAULT '',
    override_price_cents      INTEGER NOT NULL DEFAULT 0 CHECK(override_price_cents >= 0),
    override_source           TEXT NOT NULL DEFAULT '',
    override_set_at           TEXT NOT NULL DEFAULT '',
    ai_suggested_price_cents  INTEGER NOT NULL DEFAULT 0 CHECK(ai_suggested_price_cents >= 0),
    ai_suggested_at           TEXT NOT NULL DEFAULT '',
    card_year                 TEXT NOT NULL DEFAULT '',
    ebay_export_flagged_at    TIMESTAMP NULL,
    reviewed_price_cents      INTEGER NOT NULL DEFAULT 0,
    reviewed_at               TEXT NOT NULL DEFAULT '',
    review_source             TEXT NOT NULL DEFAULT '',
    dh_card_id                INTEGER NOT NULL DEFAULT 0,
    dh_inventory_id           INTEGER NOT NULL DEFAULT 0,
    dh_cert_status            TEXT NOT NULL DEFAULT '',
    dh_listing_price_cents    INTEGER NOT NULL DEFAULT 0,
    dh_channels_json          TEXT NOT NULL DEFAULT '',
    dh_status                 TEXT NOT NULL DEFAULT '',
    dh_push_status            TEXT NOT NULL DEFAULT '',
    dh_candidates             TEXT NOT NULL DEFAULT '',
    gem_rate_id               TEXT NOT NULL DEFAULT '',
    psa_spec_id               INTEGER NOT NULL DEFAULT 0,
    dh_hold_reason            TEXT NOT NULL DEFAULT '',
    mm_value_cents            INTEGER NOT NULL DEFAULT 0,
    card_player               TEXT NOT NULL DEFAULT '',
    card_variation            TEXT NOT NULL DEFAULT '',
    card_category             TEXT NOT NULL DEFAULT '',
    mm_trend_pct              REAL NOT NULL DEFAULT 0,
    mm_sales_30d              INTEGER NOT NULL DEFAULT 0,
    mm_active_low_cents       INTEGER NOT NULL DEFAULT 0,
    mm_value_updated_at       TEXT NOT NULL DEFAULT '',

    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE(grader, cert_number)
);

INSERT INTO campaign_purchases_new (
    id, campaign_id, card_name, card_number, set_name, cert_number, population,
    cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents, purchase_date,
    last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
    active_listings, sales_last_30d, trend_30d, snapshot_date,
    created_at, updated_at, vault_status, invoice_date, was_refunded,
    front_image_url, back_image_url, purchase_source, grader, grade_value,
    snapshot_json, snapshot_status, snapshot_retry_count,
    psa_listing_title,
    override_price_cents, override_source, override_set_at,
    ai_suggested_price_cents, ai_suggested_at,
    card_year, ebay_export_flagged_at,
    reviewed_price_cents, reviewed_at, review_source,
    dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json,
    dh_status, dh_push_status, dh_candidates,
    gem_rate_id, psa_spec_id, dh_hold_reason,
    mm_value_cents,
    card_player, card_variation, card_category,
    mm_trend_pct, mm_sales_30d, mm_active_low_cents,
    mm_value_updated_at
) SELECT
    id, campaign_id, card_name, card_number, set_name, cert_number, population,
    cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents, purchase_date,
    last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
    active_listings, sales_last_30d, trend_30d, snapshot_date,
    created_at, updated_at, vault_status, invoice_date, was_refunded,
    front_image_url, back_image_url, purchase_source, grader, grade_value,
    snapshot_json, snapshot_status, snapshot_retry_count,
    psa_listing_title,
    override_price_cents, override_source, override_set_at,
    ai_suggested_price_cents, ai_suggested_at,
    card_year, ebay_export_flagged_at,
    reviewed_price_cents, reviewed_at, review_source,
    dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json,
    dh_status, dh_push_status, dh_candidates,
    gem_rate_id, psa_spec_id, dh_hold_reason,
    mm_value_cents,
    card_player, card_variation, card_category,
    mm_trend_pct, mm_sales_30d, mm_active_low_cents,
    mm_value_updated_at
FROM campaign_purchases;

DROP TABLE campaign_purchases;

ALTER TABLE campaign_purchases_new RENAME TO campaign_purchases;

-- Recreate all indexes.
CREATE INDEX idx_purchases_campaign ON campaign_purchases(campaign_id);
CREATE INDEX idx_purchases_date ON campaign_purchases(purchase_date);
CREATE INDEX idx_purchases_campaign_date ON campaign_purchases(campaign_id, purchase_date DESC);
CREATE INDEX idx_purchases_snapshot_pending ON campaign_purchases(snapshot_status) WHERE snapshot_status != '';
CREATE INDEX idx_campaign_purchases_ebay_export_flagged_at ON campaign_purchases(ebay_export_flagged_at) WHERE ebay_export_flagged_at IS NOT NULL;
CREATE INDEX idx_purchases_invoice_date ON campaign_purchases(invoice_date) WHERE invoice_date != '';
CREATE INDEX idx_purchases_dh_cert_status ON campaign_purchases(dh_cert_status) WHERE dh_cert_status != '';
CREATE INDEX idx_campaign_purchases_dh_push_status ON campaign_purchases(dh_push_status) WHERE dh_push_status != '';
CREATE INDEX idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id) WHERE gem_rate_id != '';

PRAGMA foreign_keys = ON;
