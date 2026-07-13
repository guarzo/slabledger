ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS psa_campaign_request_id TEXT;

CREATE TABLE IF NOT EXISTS psa_campaign_snapshot (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    raw_json   JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT psa_campaign_snapshot_singleton CHECK (id = 1)
);

CREATE TABLE IF NOT EXISTS psa_campaign_push_queue (
    id                   TEXT PRIMARY KEY,
    psa_campaign_id      TEXT NOT NULL,
    internal_campaign_id TEXT,
    proposed_diff        JSONB NOT NULL,
    status               TEXT NOT NULL DEFAULT 'pending',
    requested_by         TEXT,
    approved_by          TEXT,
    result_json          JSONB,
    error                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_psa_push_queue_status ON psa_campaign_push_queue (status);
