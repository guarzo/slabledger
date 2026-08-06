-- Persists PSA portal reference data (curated spec lists, subject lists)
-- harvested by cmd/psa-harvest so the main server — which has no portal
-- session — can resolve names to portal ids at translation time.
-- kind is 'spec_lists' or 'subjects'; key is '' for the singleton spec-list
-- catalog and the category id (as text) for a subjects catalog.
CREATE TABLE psa_portal_catalog (
    kind       TEXT        NOT NULL,
    key        TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (kind, key)
);
