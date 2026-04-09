-- SQLite < 3.35.0 does not support DROP COLUMN.
-- The mm_value_updated_at column on campaign_purchases (TEXT NOT NULL DEFAULT '')
-- is inert for older code that does not reference it. Recreating a 63-column table
-- to remove one no-op column would be fragile and error-prone.
-- This migration is intentionally a no-op.
SELECT 1;
