-- Add DH velocity + volume fields to market_intelligence so the intelligence
-- refresh pipeline can surface sold-comp density (volume_7d/30d/90d) and
-- sell-through momentum (sell_through_*_pct + sample_size) alongside
-- sentiment/forecast/roi.
ALTER TABLE market_intelligence ADD COLUMN volume_7d INTEGER;
ALTER TABLE market_intelligence ADD COLUMN volume_30d INTEGER;
ALTER TABLE market_intelligence ADD COLUMN volume_90d INTEGER;
ALTER TABLE market_intelligence ADD COLUMN sell_through_30d_pct REAL;
ALTER TABLE market_intelligence ADD COLUMN sell_through_60d_pct REAL;
ALTER TABLE market_intelligence ADD COLUMN sell_through_90d_pct REAL;
ALTER TABLE market_intelligence ADD COLUMN velocity_sample_size INTEGER;
ALTER TABLE market_intelligence ADD COLUMN velocity_last_fetch TIMESTAMP;
