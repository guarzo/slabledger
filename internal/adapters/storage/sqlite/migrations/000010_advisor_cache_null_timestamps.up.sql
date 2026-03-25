-- Allow started_at and completed_at to be NULL instead of empty strings.
-- SQLite doesn't support ALTER COLUMN, so we recreate the table.

CREATE TABLE IF NOT EXISTS advisor_cache_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    analysis_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    content TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT DEFAULT NULL,
    completed_at TEXT DEFAULT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO advisor_cache_new (id, analysis_type, status, content, error_message, started_at, completed_at, created_at, updated_at)
SELECT id, analysis_type, status, content, error_message,
    CASE WHEN started_at = '' THEN NULL ELSE started_at END,
    CASE WHEN completed_at = '' THEN NULL ELSE completed_at END,
    created_at, updated_at
FROM advisor_cache;

DROP TABLE advisor_cache;
ALTER TABLE advisor_cache_new RENAME TO advisor_cache;

CREATE UNIQUE INDEX IF NOT EXISTS idx_advisor_cache_type ON advisor_cache(analysis_type);

-- Auto-update updated_at on row changes.
CREATE TRIGGER IF NOT EXISTS trg_advisor_cache_updated_at
AFTER UPDATE ON advisor_cache
FOR EACH ROW
BEGIN
    UPDATE advisor_cache SET updated_at = datetime('now') WHERE id = OLD.id;
END;
