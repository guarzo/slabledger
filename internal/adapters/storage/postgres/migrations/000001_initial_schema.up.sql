-- Spike scope: only what campaign_store.go exercises.
-- The real initial schema (covering all 35 tables, 6 views, 1 trigger) will be
-- generated from the final SQLite state during Phase 1 of the Fly migration.

CREATE TABLE campaigns (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sport TEXT NOT NULL DEFAULT '',
    year_range TEXT NOT NULL DEFAULT '',
    grade_range TEXT NOT NULL DEFAULT '',
    price_range TEXT NOT NULL DEFAULT '',
    cl_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    buy_terms_cl_pct DOUBLE PRECISION NOT NULL DEFAULT 0,
    daily_spend_cap_cents BIGINT NOT NULL DEFAULT 0,
    inclusion_list TEXT NOT NULL DEFAULT '',
    exclusion_mode BOOLEAN NOT NULL DEFAULT FALSE,
    phase TEXT NOT NULL DEFAULT 'pending',
    psa_sourcing_fee_cents BIGINT NOT NULL DEFAULT 300,
    ebay_fee_pct DOUBLE PRECISION NOT NULL DEFAULT 0.1235,
    expected_fill_rate DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
