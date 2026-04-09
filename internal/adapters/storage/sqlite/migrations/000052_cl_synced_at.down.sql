-- SQLite does not support DROP COLUMN prior to 3.35.0; recreate without cl_synced_at.
-- Save all data except cl_synced_at into a backup table.
CREATE TABLE campaign_purchases_backup AS
    SELECT id, campaign_id, card_name, card_number, set_name, cert_number, population,
           cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents, purchase_date,
           last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
           active_listings, sales_last_30d, trend_30d, snapshot_date,
           created_at, updated_at, vault_status, invoice_date, was_refunded,
           front_image_url, back_image_url, purchase_source, grader, grade_value,
           snapshot_json, snapshot_status, snapshot_retry_count,
           psa_listing_title, override_price_cents, override_source, override_set_at,
           ai_suggested_price_cents, ai_suggested_at, card_year, ebay_export_flagged_at,
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

ALTER TABLE campaign_purchases_backup RENAME TO campaign_purchases;

-- Recreate indexes from the original schema and subsequent migrations.
CREATE INDEX idx_purchases_campaign ON campaign_purchases(campaign_id);
CREATE INDEX idx_purchases_date ON campaign_purchases(purchase_date);
CREATE INDEX idx_purchases_campaign_date ON campaign_purchases(campaign_id, purchase_date DESC);
CREATE INDEX idx_purchases_snapshot_pending ON campaign_purchases(snapshot_status) WHERE snapshot_status != '';
