-- Market intelligence from DoubleHolo Tier 3
CREATE TABLE market_intelligence (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    dh_card_id TEXT NOT NULL,
    sentiment_score REAL,
    sentiment_mentions INTEGER,
    sentiment_trend TEXT,
    forecast_price_cents INTEGER,
    forecast_confidence REAL,
    forecast_date TEXT,
    grading_roi TEXT,
    recent_sales TEXT,
    population TEXT,
    insights_headline TEXT,
    insights_detail TEXT,
    fetched_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, card_number)
);

CREATE INDEX idx_market_intelligence_dh_card_id ON market_intelligence(dh_card_id);
CREATE INDEX idx_market_intelligence_fetched_at ON market_intelligence(fetched_at);

-- Daily buy/sell suggestions from DoubleHolo
CREATE TABLE dh_suggestions (
    suggestion_date TEXT NOT NULL,
    type TEXT NOT NULL,
    category TEXT NOT NULL,
    rank INTEGER NOT NULL,
    is_manual BOOLEAN NOT NULL,
    dh_card_id TEXT NOT NULL,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    image_url TEXT,
    current_price_cents INTEGER,
    confidence_score REAL,
    reasoning TEXT,
    structured_reasoning TEXT,
    metrics TEXT,
    sentiment_score REAL,
    sentiment_trend REAL,
    sentiment_mentions INTEGER,
    fetched_at TIMESTAMP NOT NULL,
    PRIMARY KEY (suggestion_date, type, category, rank)
);

CREATE INDEX idx_dh_suggestions_date ON dh_suggestions(suggestion_date);
CREATE INDEX idx_dh_suggestions_card ON dh_suggestions(card_name, set_name);

-- Seed rate limit entry for doubleholo provider
INSERT OR IGNORE INTO api_rate_limits (provider, calls_per_minute, calls_per_hour, calls_per_day, last_429_at, blocked_until)
VALUES ('doubleholo', 60, 2000, 2000, NULL, NULL);
