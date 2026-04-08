ALTER TABLE campaign_purchases ADD COLUMN dh_hold_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS dh_push_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    swing_pct_threshold INTEGER NOT NULL DEFAULT 20,
    swing_min_cents INTEGER NOT NULL DEFAULT 5000,
    disagreement_pct_threshold INTEGER NOT NULL DEFAULT 25,
    unreviewed_change_pct_threshold INTEGER NOT NULL DEFAULT 15,
    unreviewed_change_min_cents INTEGER NOT NULL DEFAULT 3000,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO dh_push_config (id) VALUES (1);
