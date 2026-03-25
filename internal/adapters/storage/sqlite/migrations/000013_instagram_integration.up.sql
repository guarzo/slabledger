-- Instagram account connection (singleton row)
CREATE TABLE IF NOT EXISTS instagram_config (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    access_token TEXT NOT NULL DEFAULT '',
    ig_user_id TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    token_expires_at TEXT NOT NULL DEFAULT '',
    connected_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Add instagram_post_id to social_posts for tracking published posts
ALTER TABLE social_posts ADD COLUMN instagram_post_id TEXT NOT NULL DEFAULT '';
