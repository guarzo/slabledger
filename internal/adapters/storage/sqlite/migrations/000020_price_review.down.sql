-- 000020_price_review.down.sql

DROP INDEX IF EXISTS idx_price_flags_purchase;
DROP INDEX IF EXISTS idx_price_flags_open;
DROP TABLE IF EXISTS price_flags;

-- SQLite doesn't support DROP COLUMN before 3.35.0; use the rebuild approach
-- For simplicity and since these columns have defaults, we leave them in place on rollback.
-- The application will simply ignore them if the migration version is rolled back.
