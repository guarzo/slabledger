-- Replace table-wide UNIQUE(cert_number) with a partial unique index
-- that only enforces uniqueness for unresolved rows. This allows a
-- cert_number to reappear as pending after its previous entry was resolved.

-- SQLite does not support DROP CONSTRAINT, so we recreate the table.
CREATE TABLE psa_pending_items_new (
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
    resolved_campaign_id TEXT
);

INSERT INTO psa_pending_items_new SELECT * FROM psa_pending_items;

DROP TABLE psa_pending_items;

ALTER TABLE psa_pending_items_new RENAME TO psa_pending_items;

-- Partial unique index: only one unresolved row per cert_number.
CREATE UNIQUE INDEX idx_pending_items_unresolved_cert
    ON psa_pending_items(cert_number) WHERE resolved_at IS NULL;
