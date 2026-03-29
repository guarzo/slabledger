-- AI-generated acquisition picks
CREATE TABLE IF NOT EXISTS ai_picks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pick_date DATE NOT NULL,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    grade TEXT NOT NULL,
    direction TEXT NOT NULL CHECK(direction IN ('buy', 'watch', 'avoid')),
    confidence TEXT NOT NULL CHECK(confidence IN ('high', 'medium', 'low')),
    buy_thesis TEXT NOT NULL,
    target_buy_price INTEGER,
    expected_sell_price INTEGER,
    signals_json TEXT NOT NULL DEFAULT '[]',
    rank INTEGER NOT NULL,
    source TEXT NOT NULL CHECK(source IN ('ai', 'watchlist_reassessment')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ai_picks_date ON ai_picks(pick_date);
CREATE UNIQUE INDEX idx_ai_picks_unique ON ai_picks(pick_date, card_name, set_name, grade);

-- Acquisition watchlist (separate from favorites/watchlist)
CREATE TABLE IF NOT EXISTS acquisition_watchlist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    grade TEXT NOT NULL,
    source TEXT NOT NULL CHECK(source IN ('manual', 'auto_from_pick')),
    active INTEGER NOT NULL DEFAULT 1,
    latest_pick_id INTEGER REFERENCES ai_picks(id),
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_acq_watchlist_unique_active ON acquisition_watchlist(card_name, set_name, grade) WHERE active = 1;
CREATE INDEX idx_acq_watchlist_active ON acquisition_watchlist(active);
