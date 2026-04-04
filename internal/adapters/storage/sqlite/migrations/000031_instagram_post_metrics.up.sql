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
