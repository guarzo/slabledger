-- Re-create social_posts (parent table, includes instagram_post_id added by 000013)
CREATE TABLE IF NOT EXISTS social_posts (
    id TEXT PRIMARY KEY,
    post_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    caption TEXT NOT NULL DEFAULT '',
    hashtags TEXT NOT NULL DEFAULT '',
    cover_title TEXT NOT NULL DEFAULT '',
    card_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    instagram_post_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_social_posts_status ON social_posts(status);
CREATE INDEX idx_social_posts_type ON social_posts(post_type);

-- Re-create social_post_cards (FK to social_posts)
CREATE TABLE IF NOT EXISTS social_post_cards (
    post_id TEXT NOT NULL REFERENCES social_posts(id) ON DELETE CASCADE,
    purchase_id TEXT NOT NULL,
    slide_order INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (post_id, purchase_id)
);
CREATE INDEX idx_social_post_cards_purchase ON social_post_cards(purchase_id);

-- Re-create instagram_config (standalone singleton)
CREATE TABLE IF NOT EXISTS instagram_config (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    access_token TEXT NOT NULL DEFAULT '',
    ig_user_id TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    token_expires_at TEXT NOT NULL DEFAULT '',
    connected_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Re-create instagram_post_metrics (FK to social_posts)
CREATE TABLE IF NOT EXISTS instagram_post_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    post_id TEXT NOT NULL,
    impressions INTEGER NOT NULL DEFAULT 0,
    reach INTEGER NOT NULL DEFAULT 0,
    likes INTEGER NOT NULL DEFAULT 0,
    comments INTEGER NOT NULL DEFAULT 0,
    saves INTEGER NOT NULL DEFAULT 0,
    shares INTEGER NOT NULL DEFAULT 0,
    polled_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (post_id) REFERENCES social_posts(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_post_metrics_post_id ON instagram_post_metrics(post_id);
CREATE INDEX IF NOT EXISTS idx_post_metrics_polled_at ON instagram_post_metrics(polled_at);
CREATE INDEX IF NOT EXISTS idx_post_metrics_post_id_polled_at ON instagram_post_metrics(post_id, polled_at DESC);
