-- DH demand + analytics cache tables.
-- Stores per-card and per-character demand/velocity/trend/saturation signals
-- fetched from the DoubleHolo enterprise API. JSON blobs preserve the raw
-- response shape so downstream code can evolve without migrations.
CREATE TABLE dh_card_cache (
    card_id                 TEXT    NOT NULL,
    window                  TEXT    NOT NULL,             -- '7d' or '30d'
    demand_score            REAL,                         -- nullable if demand not present
    demand_data_quality     TEXT,                         -- 'proxy' | 'full' | null
    demand_json             TEXT,                         -- full demand_signals response
    velocity_json           TEXT,                         -- from batch_analytics.velocity
    trend_json              TEXT,                         -- from batch_analytics.trend
    saturation_json         TEXT,                         -- from batch_analytics.saturation
    price_distribution_json TEXT,                         -- from batch_analytics.price_distribution
    analytics_computed_at   TIMESTAMP,                    -- from DH's computed_at (nullable = not computed)
    demand_computed_at      TIMESTAMP,
    fetched_at              TIMESTAMP NOT NULL,
    PRIMARY KEY (card_id, window)
);

CREATE INDEX idx_card_cache_demand_score ON dh_card_cache(demand_score DESC);

CREATE TABLE dh_character_cache (
    character             TEXT    NOT NULL,
    window                TEXT    NOT NULL,
    demand_json           TEXT,                           -- character_demand response (with by_era if requested)
    velocity_json         TEXT,                           -- from /characters/velocity
    saturation_json       TEXT,                           -- from /characters/saturation
    demand_computed_at    TIMESTAMP,
    analytics_computed_at TIMESTAMP,
    fetched_at            TIMESTAMP NOT NULL,
    PRIMARY KEY (character, window)
);
