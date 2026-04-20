-- Revert to the full (non-partial) index from 000002.

DROP INDEX IF EXISTS idx_campaign_purchases_received_at;

CREATE INDEX IF NOT EXISTS idx_campaign_purchases_received_at
    ON campaign_purchases(received_at);
