DROP TRIGGER IF EXISTS trg_advisor_cache_updated_at;

BEGIN TRANSACTION;

CREATE TABLE IF NOT EXISTS advisor_cache_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    analysis_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    content TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO advisor_cache_old (id, analysis_type, status, content, error_message, started_at, completed_at, created_at, updated_at)
SELECT id, analysis_type, status, content, error_message,
    COALESCE(started_at, ''),
    COALESCE(completed_at, ''),
    created_at, updated_at
FROM advisor_cache;

DROP TABLE advisor_cache;
ALTER TABLE advisor_cache_old RENAME TO advisor_cache;

COMMIT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_advisor_cache_type ON advisor_cache(analysis_type);
