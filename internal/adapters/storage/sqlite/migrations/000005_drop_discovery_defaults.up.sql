-- Remove misleading DEFAULT clauses from provider and failure_reason.
-- SQLite does not support ALTER COLUMN, so we recreate the table.
CREATE TABLE discovery_failures_new (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    failure_reason TEXT NOT NULL,
    query_attempted TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 1,
    last_attempted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, card_number, provider)
);

INSERT INTO discovery_failures_new SELECT * FROM discovery_failures;
DROP TABLE discovery_failures;
ALTER TABLE discovery_failures_new RENAME TO discovery_failures;

CREATE INDEX IF NOT EXISTS idx_discovery_failures_provider
    ON discovery_failures(provider, last_attempted_at DESC);
