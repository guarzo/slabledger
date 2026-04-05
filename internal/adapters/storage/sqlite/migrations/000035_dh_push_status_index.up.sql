CREATE INDEX idx_campaign_purchases_dh_push_status
    ON campaign_purchases(dh_push_status)
    WHERE dh_push_status != '';
