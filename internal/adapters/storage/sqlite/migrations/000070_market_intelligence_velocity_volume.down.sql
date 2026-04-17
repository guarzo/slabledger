-- Requires SQLite 3.35.0+ for DROP COLUMN support.
ALTER TABLE market_intelligence DROP COLUMN volume_7d;
ALTER TABLE market_intelligence DROP COLUMN volume_30d;
ALTER TABLE market_intelligence DROP COLUMN volume_90d;
ALTER TABLE market_intelligence DROP COLUMN sell_through_30d_pct;
ALTER TABLE market_intelligence DROP COLUMN sell_through_60d_pct;
ALTER TABLE market_intelligence DROP COLUMN sell_through_90d_pct;
ALTER TABLE market_intelligence DROP COLUMN velocity_sample_size;
ALTER TABLE market_intelligence DROP COLUMN velocity_last_fetch;
