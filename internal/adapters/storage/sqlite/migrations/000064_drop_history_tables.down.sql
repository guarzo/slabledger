CREATE TABLE IF NOT EXISTS market_snapshot_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade_value REAL NOT NULL,
    median_cents INTEGER NOT NULL DEFAULT 0,
    conservative_cents INTEGER NOT NULL DEFAULT 0,
    optimistic_cents INTEGER NOT NULL DEFAULT 0,
    last_sold_cents INTEGER NOT NULL DEFAULT 0,
    lowest_list_cents INTEGER NOT NULL DEFAULT 0,
    estimated_value_cents INTEGER NOT NULL DEFAULT 0,
    active_listings INTEGER NOT NULL DEFAULT 0,
    sales_last_30d INTEGER NOT NULL DEFAULT 0,
    sales_last_90d INTEGER NOT NULL DEFAULT 0,
    daily_velocity REAL NOT NULL DEFAULT 0,
    weekly_velocity REAL NOT NULL DEFAULT 0,
    trend_30d REAL NOT NULL DEFAULT 0,
    trend_90d REAL NOT NULL DEFAULT 0,
    volatility REAL NOT NULL DEFAULT 0,
    source_count INTEGER NOT NULL DEFAULT 0,
    fusion_confidence REAL NOT NULL DEFAULT 0,
    snapshot_json TEXT NOT NULL DEFAULT '',
    snapshot_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_msh_card_grade_date
    ON market_snapshot_history(card_name, set_name, card_number, grade_value, snapshot_date);
CREATE INDEX IF NOT EXISTS idx_msh_date
    ON market_snapshot_history(snapshot_date DESC);

CREATE TABLE IF NOT EXISTS population_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade_value REAL NOT NULL,
    grader TEXT NOT NULL DEFAULT 'PSA',
    population INTEGER NOT NULL,
    pop_higher INTEGER NOT NULL DEFAULT 0,
    observation_date DATE NOT NULL,
    source TEXT NOT NULL DEFAULT 'csv_import',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pop_history_card_date
    ON population_history(card_name, set_name, card_number, grade_value, grader, observation_date);

CREATE TABLE IF NOT EXISTS cl_value_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cert_number TEXT NOT NULL,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade_value REAL NOT NULL,
    cl_value_cents INTEGER NOT NULL,
    observation_date DATE NOT NULL,
    source TEXT NOT NULL DEFAULT 'csv_import',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_history_cert_date
    ON cl_value_history(cert_number, observation_date);
