-- 000020_price_review.up.sql

-- Add review columns to campaign_purchases
ALTER TABLE campaign_purchases ADD COLUMN reviewed_price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_purchases ADD COLUMN reviewed_at TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN review_source TEXT NOT NULL DEFAULT '';

-- Price flags table for data quality tracking
CREATE TABLE IF NOT EXISTS price_flags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    purchase_id TEXT NOT NULL,
    flagged_by INTEGER NOT NULL,
    flagged_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason TEXT NOT NULL CHECK(reason IN ('wrong_match', 'stale_data', 'wrong_grade', 'source_disagreement', 'other')),
    resolved_at DATETIME,
    resolved_by INTEGER,
    FOREIGN KEY (purchase_id) REFERENCES campaign_purchases(id) ON DELETE CASCADE,
    FOREIGN KEY (flagged_by) REFERENCES users(id),
    FOREIGN KEY (resolved_by) REFERENCES users(id)
);

CREATE INDEX idx_price_flags_open ON price_flags(resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX idx_price_flags_purchase ON price_flags(purchase_id);
