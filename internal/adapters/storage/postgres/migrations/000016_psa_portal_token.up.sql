CREATE TABLE IF NOT EXISTS psa_portal_token (
    id           INTEGER PRIMARY KEY DEFAULT 1,
    access_token TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT psa_portal_token_singleton CHECK (id = 1)
);
