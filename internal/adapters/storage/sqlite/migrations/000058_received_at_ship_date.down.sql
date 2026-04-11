ALTER TABLE campaign_purchases ADD COLUMN vault_status TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases DROP COLUMN psa_ship_date;
ALTER TABLE campaign_purchases DROP COLUMN received_at;
