CREATE TABLE psa_pending_items (
    id                   TEXT PRIMARY KEY,
    cert_number          TEXT NOT NULL,
    card_name            TEXT NOT NULL DEFAULT '',
    set_name             TEXT NOT NULL DEFAULT '',
    card_number          TEXT NOT NULL DEFAULT '',
    grade                REAL NOT NULL DEFAULT 0,
    buy_cost_cents       INTEGER NOT NULL DEFAULT 0,
    purchase_date        TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL CHECK (status IN ('ambiguous', 'unmatched')),
    candidates           TEXT NOT NULL DEFAULT '[]',
    source               TEXT NOT NULL CHECK (source IN ('scheduler', 'manual')),
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at          DATETIME,
    resolved_campaign_id TEXT,
    UNIQUE(cert_number)
);
