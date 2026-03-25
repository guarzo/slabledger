-- Consolidated initial schema
-- Represents the final state of all tables, indexes, and views.

-- ============================================================================
-- Price Data
-- ============================================================================

CREATE TABLE price_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    confidence REAL DEFAULT 1.0,
    source TEXT NOT NULL CHECK(source IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion'
    )),
    fusion_source_count INTEGER,
    fusion_outliers_removed INTEGER,
    fusion_method TEXT,
    price_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, card_number, grade, source, price_date)
);

CREATE INDEX idx_price_history_card ON price_history(card_name, set_name, grade);
CREATE INDEX idx_price_history_staleness ON price_history(source, updated_at DESC);
CREATE INDEX idx_price_history_date ON price_history(price_date DESC);
CREATE INDEX idx_price_history_lookup ON price_history(card_name, set_name, card_number, grade, source, price_date DESC);

CREATE TABLE api_calls (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL CHECK(provider IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion'
    )),
    endpoint TEXT,
    status_code INTEGER,
    error TEXT,
    latency_ms INTEGER,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_calls_provider ON api_calls(provider, timestamp DESC);
CREATE INDEX idx_api_calls_timestamp ON api_calls(timestamp DESC);
CREATE INDEX idx_api_calls_errors ON api_calls(provider, status_code) WHERE status_code >= 400;

CREATE TABLE api_rate_limits (
    provider TEXT PRIMARY KEY CHECK(provider IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion'
    )),
    calls_last_minute INTEGER DEFAULT 0,
    calls_last_hour INTEGER DEFAULT 0,
    calls_last_day INTEGER DEFAULT 0,
    last_429_at TIMESTAMP,
    blocked_until TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE price_refresh_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    grade TEXT NOT NULL CHECK(grade IN (
        'PSA 10', 'PSA 9', 'PSA 8', 'PSA 7', 'PSA 6',
        'PSA 5', 'PSA 4', 'PSA 3', 'PSA 2', 'PSA 1',
        'BGS 10', 'BGS 9.5', 'BGS 9', 'BGS 8.5', 'BGS 8',
        'CGC 10', 'CGC 9.5', 'CGC 9', 'CGC 8.5', 'CGC 8',
        'Raw', 'Ungraded'
    )),
    source TEXT NOT NULL CHECK(source IN (
        'pricecharting', 'pokemonprice', 'cardmarket', 'cardhedger', 'fusion'
    )),
    priority INTEGER DEFAULT 2 CHECK(priority IN (1, 2, 3)),
    scheduled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_attempted_at TIMESTAMP,
    attempts INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'in_progress', 'completed', 'failed')),
    error TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(card_name, set_name, grade, source),
    FOREIGN KEY (source) REFERENCES api_rate_limits(provider) ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE INDEX idx_refresh_queue_priority ON price_refresh_queue(priority ASC, scheduled_at ASC)
    WHERE status = 'pending';
CREATE INDEX idx_refresh_queue_status ON price_refresh_queue(status, last_attempted_at);

CREATE TABLE card_access_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    access_type TEXT CHECK(access_type IS NULL OR access_type IN (
        'analysis', 'search', 'watchlist', 'collection'
    )),
    accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_access_log_card ON card_access_log(card_name, set_name, card_number, accessed_at DESC);
CREATE INDEX idx_access_log_covering ON card_access_log(card_name, set_name, card_number, accessed_at);
CREATE INDEX idx_card_access_log_recent ON card_access_log(accessed_at DESC, card_name, set_name, card_number);

-- ============================================================================
-- Auth & Sessions
-- ============================================================================

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    google_id TEXT UNIQUE NOT NULL,
    username TEXT,
    email TEXT,
    avatar_url TEXT,
    is_admin BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_users_google_id ON users(google_id);

CREATE TABLE user_sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_agent TEXT,
    ip_address TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at);

CREATE TABLE user_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_type TEXT DEFAULT 'Bearer',
    expires_at TIMESTAMP NOT NULL,
    scope TEXT,
    session_id TEXT REFERENCES user_sessions(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_user_tokens_user_id ON user_tokens(user_id);
CREATE INDEX idx_user_tokens_session_id ON user_tokens(session_id);
CREATE UNIQUE INDEX idx_user_tokens_session_unique ON user_tokens(session_id);
CREATE INDEX idx_user_tokens_expires_at ON user_tokens(expires_at);

CREATE TABLE oauth_states (
    state TEXT PRIMARY KEY,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_oauth_states_expires ON oauth_states(expires_at);

-- ============================================================================
-- User Features
-- ============================================================================

CREATE TABLE favorites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    image_url TEXT,
    notes TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, card_name, set_name, card_number)
);

CREATE INDEX idx_favorites_user_created ON favorites(user_id, created_at DESC);

CREATE TABLE allowed_emails (
    email TEXT PRIMARY KEY COLLATE NOCASE,
    added_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notes TEXT
);

-- ============================================================================
-- Campaigns
-- ============================================================================

CREATE TABLE campaigns (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sport TEXT NOT NULL DEFAULT '',
    year_range TEXT NOT NULL DEFAULT '',
    grade_range TEXT NOT NULL DEFAULT '',
    price_range TEXT NOT NULL DEFAULT '',
    cl_confidence REAL NOT NULL DEFAULT 0,
    buy_terms_cl_pct REAL NOT NULL DEFAULT 0,
    daily_spend_cap_cents INTEGER NOT NULL DEFAULT 0,
    inclusion_list TEXT NOT NULL DEFAULT '',
    exclusion_mode INTEGER NOT NULL DEFAULT 0,
    phase TEXT NOT NULL DEFAULT 'pending',
    psa_sourcing_fee_cents INTEGER NOT NULL DEFAULT 300,
    ebay_fee_pct REAL NOT NULL DEFAULT 0.1235,
    expected_fill_rate REAL NOT NULL DEFAULT 0.0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE campaign_purchases (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL,
    card_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    set_name TEXT NOT NULL DEFAULT '',
    cert_number TEXT NOT NULL,
    population INTEGER NOT NULL DEFAULT 0,
    cl_value_cents INTEGER NOT NULL DEFAULT 0,
    buy_cost_cents INTEGER NOT NULL DEFAULT 0,
    psa_sourcing_fee_cents INTEGER NOT NULL DEFAULT 0,
    purchase_date TEXT NOT NULL,
    last_sold_cents INTEGER DEFAULT 0,
    lowest_list_cents INTEGER DEFAULT 0,
    conservative_cents INTEGER DEFAULT 0,
    median_cents INTEGER DEFAULT 0,
    active_listings INTEGER DEFAULT 0,
    sales_last_30d INTEGER DEFAULT 0,
    trend_30d REAL DEFAULT 0,
    snapshot_date TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    vault_status TEXT NOT NULL DEFAULT '',
    invoice_date TEXT NOT NULL DEFAULT '',
    was_refunded INTEGER NOT NULL DEFAULT 0,
    front_image_url TEXT NOT NULL DEFAULT '',
    back_image_url TEXT NOT NULL DEFAULT '',
    purchase_source TEXT NOT NULL DEFAULT '',
    grader TEXT NOT NULL DEFAULT 'PSA' CHECK(grader IN ('PSA', 'CGC', 'BGS', 'SGC')),
    grade_value REAL NOT NULL DEFAULT 0,
    snapshot_json TEXT NOT NULL DEFAULT '',
    snapshot_status TEXT NOT NULL DEFAULT '' CHECK(snapshot_status IN ('', 'pending', 'failed', 'exhausted')),
    snapshot_retry_count INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE(grader, cert_number)
);

CREATE INDEX idx_purchases_campaign ON campaign_purchases(campaign_id);
CREATE INDEX idx_purchases_date ON campaign_purchases(purchase_date);
CREATE INDEX idx_purchases_campaign_date ON campaign_purchases(campaign_id, purchase_date DESC);
CREATE INDEX idx_purchases_snapshot_pending ON campaign_purchases(snapshot_status) WHERE snapshot_status != '';

CREATE TABLE campaign_sales (
    id TEXT PRIMARY KEY,
    purchase_id TEXT NOT NULL,
    sale_channel TEXT NOT NULL,
    sale_price_cents INTEGER NOT NULL DEFAULT 0,
    sale_fee_cents INTEGER NOT NULL DEFAULT 0,
    sale_date TEXT NOT NULL,
    days_to_sell INTEGER NOT NULL DEFAULT 0,
    net_profit_cents INTEGER NOT NULL DEFAULT 0,
    last_sold_cents INTEGER DEFAULT 0,
    lowest_list_cents INTEGER DEFAULT 0,
    conservative_cents INTEGER DEFAULT 0,
    median_cents INTEGER DEFAULT 0,
    active_listings INTEGER DEFAULT 0,
    sales_last_30d INTEGER DEFAULT 0,
    trend_30d REAL DEFAULT 0,
    snapshot_date TEXT DEFAULT '',
    snapshot_json TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (purchase_id) REFERENCES campaign_purchases(id) ON DELETE CASCADE,
    UNIQUE(purchase_id)
);

CREATE INDEX idx_sales_channel ON campaign_sales(sale_channel);
CREATE INDEX idx_sales_date ON campaign_sales(sale_date);

-- ============================================================================
-- Invoices & Cashflow
-- ============================================================================

CREATE TABLE invoices (
    id TEXT PRIMARY KEY,
    invoice_date TEXT NOT NULL,
    total_cents INTEGER NOT NULL DEFAULT 0,
    paid_cents INTEGER NOT NULL DEFAULT 0,
    due_date TEXT NOT NULL DEFAULT '',
    paid_date TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unpaid' CHECK(status IN ('unpaid', 'partial', 'paid')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_invoices_date ON invoices(invoice_date);
CREATE INDEX idx_invoices_status ON invoices(status);

CREATE TABLE cashflow_config (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    credit_limit_cents INTEGER NOT NULL DEFAULT 5000000,
    cash_buffer_cents INTEGER NOT NULL DEFAULT 1000000,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO cashflow_config (id, credit_limit_cents, cash_buffer_cents) VALUES (1, 5000000, 1000000);

-- ============================================================================
-- Card ID Mappings & Sync State
-- ============================================================================

CREATE TABLE card_id_mappings (
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    collector_number TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    hint_source TEXT NOT NULL DEFAULT 'auto',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, collector_number, provider)
);

CREATE INDEX idx_card_id_mappings_provider_external_id
    ON card_id_mappings (provider, external_id);

CREATE TABLE sync_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ============================================================================
-- Revocation Flags
-- ============================================================================

CREATE TABLE revocation_flags (
    id TEXT PRIMARY KEY,
    segment_label TEXT NOT NULL,
    segment_dimension TEXT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'sent')),
    email_text TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    sent_at DATETIME
);

CREATE INDEX idx_revocation_flags_status ON revocation_flags(status);
CREATE INDEX idx_revocation_flags_segment ON revocation_flags(segment_label, segment_dimension);

-- ============================================================================
-- Views
-- ============================================================================

CREATE VIEW stale_prices AS
WITH recent_access AS (
    SELECT DISTINCT card_name, set_name, card_number
    FROM card_access_log
    WHERE accessed_at > DATETIME('now', '-24 hours')
)
SELECT
    ph.card_name,
    ph.card_number,
    ph.set_name,
    ph.grade,
    ph.source,
    ph.price_cents,
    ph.price_date,
    ph.updated_at,
    ROUND((JULIANDAY('now') - JULIANDAY(ph.updated_at)) * 24, 1) as hours_old,
    CASE
        WHEN ph.price_cents > 10000 THEN 1
        WHEN ph.price_cents > 5000 THEN 2
        ELSE 3
    END as priority,
    CASE WHEN ra.card_name IS NOT NULL THEN 1 ELSE 0 END as recently_accessed
FROM price_history ph
LEFT JOIN recent_access ra ON ra.card_name = ph.card_name AND ra.set_name = ph.set_name AND ra.card_number = ph.card_number
WHERE
    (ph.price_cents > 10000 AND ph.updated_at < DATETIME('now', '-12 hours'))
    OR (ph.price_cents > 5000 AND ph.price_cents <= 10000 AND ph.updated_at < DATETIME('now', '-24 hours'))
    OR (ph.price_cents <= 5000 AND ph.updated_at < DATETIME('now', '-48 hours'));

CREATE VIEW api_usage_summary AS
SELECT
    provider,
    COUNT(*) as total_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits,
    AVG(latency_ms) as avg_latency_ms,
    MAX(timestamp) as last_call_at,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-1 hour') THEN 1 END) as calls_last_hour,
    COUNT(CASE WHEN timestamp > DATETIME('now', '-5 minutes') THEN 1 END) as calls_last_5min
FROM api_calls
WHERE timestamp > DATETIME('now', '-24 hours')
GROUP BY provider;

CREATE VIEW api_hourly_distribution AS
SELECT
    provider,
    STRFTIME('%Y-%m-%d %H:00', timestamp) as hour,
    COUNT(*) as call_count,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits
FROM api_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY provider, hour
ORDER BY hour DESC;

CREATE VIEW api_daily_summary AS
SELECT
    provider,
    DATE(timestamp) as date,
    COUNT(*) as total_calls,
    COUNT(CASE WHEN status_code < 400 THEN 1 END) as successful_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) as error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) as rate_limit_hits,
    ROUND(100.0 * COUNT(CASE WHEN status_code < 400 THEN 1 END) / COUNT(*), 1) as success_rate_pct,
    ROUND(AVG(latency_ms)) as avg_latency_ms,
    MIN(timestamp) as first_call,
    MAX(timestamp) as last_call
FROM api_calls
WHERE timestamp > DATETIME('now', '-7 days')
GROUP BY provider, DATE(timestamp)
ORDER BY date DESC, provider;

CREATE VIEW active_sessions AS
SELECT
    s.id,
    s.user_id,
    u.username,
    u.google_id,
    s.expires_at,
    s.created_at,
    s.last_accessed_at,
    s.user_agent,
    s.ip_address,
    ROUND((JULIANDAY(s.expires_at) - JULIANDAY('now')) * 24, 1) as hours_until_expiry
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.expires_at > DATETIME('now')
ORDER BY s.last_accessed_at DESC;

CREATE VIEW expired_sessions AS
SELECT id
FROM user_sessions
WHERE expires_at <= DATETIME('now');
