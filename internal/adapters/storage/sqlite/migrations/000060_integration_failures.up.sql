-- Integration diagnostics: per-purchase failure reasons for MM and CL sync,
-- plus cl_value_updated_at to enable CL staleness reporting mirroring MM.
--
-- Short failure-reason tags live in mm_last_error / cl_last_error so the
-- admin /failures endpoints can group + display why individual cards failed
-- to map or price. Empty string means "no error / cleared on success".

ALTER TABLE campaign_purchases ADD COLUMN mm_last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN mm_last_error_at TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN cl_last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN cl_last_error_at TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN cl_value_updated_at TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_purchases_mm_last_error
    ON campaign_purchases(mm_last_error)
    WHERE mm_last_error != '';
CREATE INDEX IF NOT EXISTS idx_purchases_cl_last_error
    ON campaign_purchases(cl_last_error)
    WHERE cl_last_error != '';
