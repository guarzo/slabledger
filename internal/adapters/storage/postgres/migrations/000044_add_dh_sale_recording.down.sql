ALTER TABLE campaign_purchases
    DROP COLUMN dh_sale_conflict,
    DROP COLUMN dh_sale_conflict_at;

ALTER TABLE campaign_sales
    DROP COLUMN dh_idempotency_key,
    DROP COLUMN dh_sale_id,
    DROP COLUMN dh_sale_recorded_at;
