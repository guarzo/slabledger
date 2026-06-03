-- Recreate the advisor_cache table (column definitions only).  The RLS
-- policy added in 000003 and the updated_at trigger added in 000001 are
-- the responsibility of those migrations' down steps if they are ever
-- run; we deliberately do not recreate them here.

CREATE TABLE IF NOT EXISTS advisor_cache (
    id            BIGSERIAL PRIMARY KEY,
    analysis_type TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    content       TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TEXT DEFAULT NULL,
    completed_at  TEXT DEFAULT NULL,
    created_at    TEXT NOT NULL DEFAULT (NOW()::text),
    updated_at    TEXT NOT NULL DEFAULT (NOW()::text)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_advisor_cache_type ON advisor_cache(analysis_type);
