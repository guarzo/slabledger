-- Card Ladder API config (singleton row)
CREATE TABLE IF NOT EXISTS cardladder_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    encrypted_refresh_token TEXT NOT NULL,
    collection_id TEXT NOT NULL,
    firebase_api_key TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Maps purchase cert numbers to CL card IDs for sync
CREATE TABLE IF NOT EXISTS cl_card_mappings (
    slab_serial TEXT PRIMARY KEY,
    cl_collection_card_id TEXT NOT NULL,
    cl_gem_rate_id TEXT NOT NULL DEFAULT '',
    cl_condition TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
