CREATE TABLE IF NOT EXISTS psa_portal_snapshot (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    rows       JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT psa_portal_snapshot_singleton CHECK (id = 1)
);
