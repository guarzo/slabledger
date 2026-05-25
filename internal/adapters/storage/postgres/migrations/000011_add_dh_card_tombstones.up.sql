CREATE TABLE IF NOT EXISTS dh_card_tombstones (
    dh_card_id    BIGINT PRIMARY KEY,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts      INT NOT NULL DEFAULT 1,
    last_error    TEXT NOT NULL DEFAULT ''
);
