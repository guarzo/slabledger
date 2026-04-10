-- Revert to table-wide UNIQUE(cert_number).
-- WARNING: This will fail if duplicate cert_numbers exist across resolved rows.

DROP INDEX IF EXISTS idx_pending_items_unresolved_cert;

CREATE TABLE psa_pending_items_old (
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

INSERT INTO psa_pending_items_old SELECT * FROM psa_pending_items;

DROP TABLE psa_pending_items;

ALTER TABLE psa_pending_items_old RENAME TO psa_pending_items;
