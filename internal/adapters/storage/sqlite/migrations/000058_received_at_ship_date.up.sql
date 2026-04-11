ALTER TABLE campaign_purchases ADD COLUMN received_at DATETIME;
ALTER TABLE campaign_purchases ADD COLUMN psa_ship_date TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases DROP COLUMN vault_status;
