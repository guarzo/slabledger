CREATE TABLE IF NOT EXISTS social_posts (
    id TEXT PRIMARY KEY,
    post_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    caption TEXT NOT NULL DEFAULT '',
    hashtags TEXT NOT NULL DEFAULT '',
    cover_title TEXT NOT NULL DEFAULT '',
    card_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_social_posts_status ON social_posts(status);
CREATE INDEX idx_social_posts_type ON social_posts(post_type);

CREATE TABLE IF NOT EXISTS social_post_cards (
    post_id TEXT NOT NULL REFERENCES social_posts(id) ON DELETE CASCADE,
    purchase_id TEXT NOT NULL,
    slide_order INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (post_id, purchase_id)
);
CREATE INDEX idx_social_post_cards_purchase ON social_post_cards(purchase_id);
