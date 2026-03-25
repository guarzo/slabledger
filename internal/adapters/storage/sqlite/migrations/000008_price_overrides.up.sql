-- User price overrides
ALTER TABLE campaign_purchases ADD COLUMN override_price_cents INTEGER NOT NULL DEFAULT 0 CHECK (override_price_cents >= 0);
ALTER TABLE campaign_purchases ADD COLUMN override_source TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN override_set_at TEXT NOT NULL DEFAULT '';
-- AI price suggestions (separate from overrides — user must accept)
ALTER TABLE campaign_purchases ADD COLUMN ai_suggested_price_cents INTEGER NOT NULL DEFAULT 0 CHECK (ai_suggested_price_cents >= 0);
ALTER TABLE campaign_purchases ADD COLUMN ai_suggested_at TEXT NOT NULL DEFAULT '';
