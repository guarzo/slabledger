CREATE TABLE IF NOT EXISTS marketmovers_config (
    id                      INTEGER PRIMARY KEY CHECK (id = 1),
    username                TEXT    NOT NULL DEFAULT '',
    encrypted_refresh_token TEXT    NOT NULL DEFAULT '',
    updated_at              TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mm_card_mappings (
    slab_serial       TEXT    PRIMARY KEY,
    mm_collectible_id INTEGER NOT NULL,
    updated_at        TEXT    NOT NULL DEFAULT ''
);
