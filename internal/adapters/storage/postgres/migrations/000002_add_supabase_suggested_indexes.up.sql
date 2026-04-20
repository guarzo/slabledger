-- Indexes suggested by Supabase index advisor.
-- dh_push_status is intentionally omitted — a partial index already covers non-empty values.

CREATE INDEX IF NOT EXISTS idx_card_access_log_card_number
    ON card_access_log(card_number);

CREATE INDEX IF NOT EXISTS idx_card_id_mappings_collector_number
    ON card_id_mappings(collector_number);

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_received_at
    ON campaign_purchases(received_at);

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_updated_at
    ON campaign_purchases(updated_at);

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_dh_inventory_id
    ON campaign_purchases(dh_inventory_id);
