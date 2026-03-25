CREATE TABLE IF NOT EXISTS discovery_failures (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT 'cardhedger',
    failure_reason TEXT NOT NULL DEFAULT '',
    query_attempted TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 1,
    last_attempted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, card_number, provider)
);

CREATE INDEX IF NOT EXISTS idx_discovery_failures_provider
    ON discovery_failures(provider, last_attempted_at DESC);
