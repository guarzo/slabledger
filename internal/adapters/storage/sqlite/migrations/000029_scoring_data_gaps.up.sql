CREATE TABLE scoring_data_gaps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    factor_name TEXT NOT NULL,
    reason TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    card_name TEXT NOT NULL DEFAULT '',
    set_name TEXT NOT NULL DEFAULT '',
    recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_scoring_gaps_recorded ON scoring_data_gaps(recorded_at);
CREATE INDEX idx_scoring_gaps_factor ON scoring_data_gaps(factor_name, recorded_at);
