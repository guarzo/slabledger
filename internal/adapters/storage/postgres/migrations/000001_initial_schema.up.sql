-- Initial Postgres schema for SlabLedger.
--
-- This is the CONSOLIDATED end-state equivalent of applying all 75 SQLite
-- migrations (internal/adapters/storage/sqlite/migrations/000001..000075)
-- to an empty database. It is NOT a replay of the incremental migrations.
--
-- 36 tables, 7 views, 1 trigger, 53 indexes.
--
-- ================================================================
-- Type-drift fixes (SQLite column → Postgres column, Go field reference)
-- ================================================================
-- SQLite was permissively typed and many columns stored values that did not
-- match their declared affinity. When the declared SQLite type disagreed
-- with the Go struct field that reads/writes the column, the Postgres type
-- below matches the struct field (per Rule 1 of the migration plan).
--
--   1. campaigns.cl_confidence
--        SQLite: REAL NOT NULL DEFAULT 0
--        Postgres: TEXT NOT NULL DEFAULT ''
--        Go: Campaign.CLConfidence string (e.g. "2.5-4")  [types_core.go:121]
--
--   2. campaigns.exclusion_mode
--        SQLite: INTEGER NOT NULL DEFAULT 0
--        Postgres: BOOLEAN NOT NULL DEFAULT FALSE
--        Go: Campaign.ExclusionMode bool  [types_core.go:125]
--
--   3. campaign_purchases.was_refunded
--        SQLite: INTEGER NOT NULL DEFAULT 0
--        Postgres: BOOLEAN NOT NULL DEFAULT FALSE
--        Go: Purchase.WasRefunded bool  [types_core.go:184]
--
--   4. campaign_purchases.received_at
--        SQLite: DATETIME (nullable)
--        Postgres: TEXT (nullable)
--        Go: Purchase.ReceivedAt *string  [types_core.go:181]
--        Rationale: stored as ISO8601 text; struct field is *string, not time.Time.
--
--   5. campaign_sales.sold_at_asking_price / was_cracked
--        SQLite: INTEGER NOT NULL DEFAULT 0
--        Postgres: BOOLEAN NOT NULL DEFAULT FALSE
--        Go: Sale.SoldAtAskingPrice bool, Sale.WasCracked bool  [types_core.go:345,348]
--
--   6. allowed_emails.email
--        SQLite: TEXT PRIMARY KEY COLLATE NOCASE
--        Postgres: CITEXT PRIMARY KEY (via citext extension)
--
--   7. dh_suggestions.is_manual
--        SQLite: BOOLEAN NOT NULL  (SQLite stores this as integer)
--        Postgres: BOOLEAN NOT NULL
--        Go: Suggestion.IsManual bool  [intelligence/types.go]
--
--   8. users.is_admin
--        SQLite: BOOLEAN NOT NULL DEFAULT 0  (integer under the hood)
--        Postgres: BOOLEAN NOT NULL DEFAULT FALSE
--        Go: auth.User.IsAdmin bool
--
-- Advisor trigger: SQLite's `trg_advisor_cache_updated_at` trigger becomes a
-- Postgres `CREATE FUNCTION` + `CREATE TRIGGER` pair at the bottom of this file.
-- ================================================================

CREATE EXTENSION IF NOT EXISTS citext;

-- ================================================================
-- Auth tables (users must come before FK referrers)
-- ================================================================

-- users: auth.User
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,           -- User.ID int64
    google_id     TEXT UNIQUE NOT NULL,            -- User.GoogleID string
    username      TEXT,                            -- User.Username string
    email         TEXT,                            -- User.Email string
    avatar_url    TEXT,                            -- User.AvatarURL string
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,  -- User.IsAdmin bool
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP                        -- User.LastLoginAt *time.Time
);
CREATE UNIQUE INDEX idx_users_google_id ON users(google_id);

-- user_sessions: auth.Session
CREATE TABLE user_sessions (
    id               TEXT PRIMARY KEY,             -- Session.ID string
    user_id          BIGINT NOT NULL,              -- Session.UserID int64
    expires_at       TIMESTAMP NOT NULL,           -- Session.ExpiresAt time.Time
    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    user_agent       TEXT,                         -- Session.UserAgent string
    ip_address       TEXT,                         -- Session.IPAddress string
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions(expires_at);

-- user_tokens: auth.UserTokens
CREATE TABLE user_tokens (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    access_token  TEXT NOT NULL,                   -- UserTokens.AccessToken string
    refresh_token TEXT NOT NULL,                   -- UserTokens.RefreshToken string
    token_type    TEXT DEFAULT 'Bearer',           -- UserTokens.TokenType string
    expires_at    TIMESTAMP NOT NULL,              -- UserTokens.ExpiresAt time.Time
    scope         TEXT,                            -- UserTokens.Scope string
    session_id    TEXT REFERENCES user_sessions(id) ON DELETE CASCADE,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_user_tokens_user_id ON user_tokens(user_id);
CREATE INDEX idx_user_tokens_session_id ON user_tokens(session_id);
CREATE UNIQUE INDEX idx_user_tokens_session_unique ON user_tokens(session_id);
CREATE INDEX idx_user_tokens_expires_at ON user_tokens(expires_at);

-- oauth_states: no direct Go struct; state cache table
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_oauth_states_expires ON oauth_states(expires_at);

-- allowed_emails: auth.AllowedEmail (email is CITEXT — see drift fix #6)
CREATE TABLE allowed_emails (
    email      CITEXT PRIMARY KEY,                 -- AllowedEmail.Email string (case-insensitive)
    added_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,  -- AllowedEmail.AddedBy *int64
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notes      TEXT                                -- AllowedEmail.Notes string
);

-- ================================================================
-- Core inventory tables
-- ================================================================

-- campaigns: inventory.Campaign  [types_core.go:114]
CREATE TABLE campaigns (
    id                       TEXT PRIMARY KEY,                        -- Campaign.ID string
    name                     TEXT NOT NULL,                           -- Campaign.Name string
    sport                    TEXT NOT NULL DEFAULT '',                -- Campaign.Sport string
    year_range               TEXT NOT NULL DEFAULT '',                -- Campaign.YearRange string
    grade_range              TEXT NOT NULL DEFAULT '',                -- Campaign.GradeRange string
    price_range              TEXT NOT NULL DEFAULT '',                -- Campaign.PriceRange string
    cl_confidence            TEXT NOT NULL DEFAULT '',                -- drift fix #1: string "2.5-4"
    buy_terms_cl_pct         DOUBLE PRECISION NOT NULL DEFAULT 0,     -- Campaign.BuyTermsCLPct float64
    daily_spend_cap_cents    BIGINT NOT NULL DEFAULT 0,               -- Campaign.DailySpendCapCents int
    inclusion_list           TEXT NOT NULL DEFAULT '',                -- Campaign.InclusionList string
    exclusion_mode           BOOLEAN NOT NULL DEFAULT FALSE,          -- drift fix #2: bool
    phase                    TEXT NOT NULL DEFAULT 'pending',         -- Campaign.Phase Phase (named string)
    psa_sourcing_fee_cents   BIGINT NOT NULL DEFAULT 300,             -- Campaign.PSASourcingFeeCents int
    ebay_fee_pct             DOUBLE PRECISION NOT NULL DEFAULT 0.1235,-- Campaign.EbayFeePct float64
    expected_fill_rate       DOUBLE PRECISION NOT NULL DEFAULT 0.0,   -- Campaign.ExpectedFillRate float64
    created_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- campaign_purchases: inventory.Purchase  [types_core.go:157]
CREATE TABLE campaign_purchases (
    id                         TEXT PRIMARY KEY,                     -- Purchase.ID string
    campaign_id                TEXT NOT NULL,                        -- Purchase.CampaignID string
    card_name                  TEXT NOT NULL,                        -- Purchase.CardName string
    card_number                TEXT NOT NULL DEFAULT '',             -- Purchase.CardNumber string
    set_name                   TEXT NOT NULL DEFAULT '',             -- Purchase.SetName string
    cert_number                TEXT NOT NULL,                        -- Purchase.CertNumber string
    population                 BIGINT NOT NULL DEFAULT 0,            -- Purchase.Population int
    cl_value_cents             BIGINT NOT NULL DEFAULT 0,            -- Purchase.CLValueCents int
    buy_cost_cents             BIGINT NOT NULL DEFAULT 0,            -- Purchase.BuyCostCents int
    psa_sourcing_fee_cents     BIGINT NOT NULL DEFAULT 0,            -- Purchase.PSASourcingFeeCents int
    purchase_date              TEXT NOT NULL,                        -- Purchase.PurchaseDate string (YYYY-MM-DD)
    last_sold_cents            BIGINT DEFAULT 0,                     -- MarketSnapshotData.LastSoldCents int
    lowest_list_cents          BIGINT DEFAULT 0,                     -- MarketSnapshotData.LowestListCents int
    conservative_cents         BIGINT DEFAULT 0,                     -- MarketSnapshotData.ConservativeCents int
    median_cents               BIGINT DEFAULT 0,                     -- MarketSnapshotData.MedianCents int
    active_listings            BIGINT DEFAULT 0,                     -- MarketSnapshotData.ActiveListings int
    sales_last_30d             BIGINT DEFAULT 0,                     -- MarketSnapshotData.SalesLast30d int
    trend_30d                  DOUBLE PRECISION DEFAULT 0,           -- MarketSnapshotData.Trend30d float64
    snapshot_date              TEXT DEFAULT '',                      -- MarketSnapshotData.SnapshotDate string
    created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    invoice_date               TEXT NOT NULL DEFAULT '',             -- Purchase.InvoiceDate string
    was_refunded               BOOLEAN NOT NULL DEFAULT FALSE,       -- drift fix #3: bool
    front_image_url            TEXT NOT NULL DEFAULT '',             -- Purchase.FrontImageURL string
    back_image_url             TEXT NOT NULL DEFAULT '',             -- Purchase.BackImageURL string
    purchase_source            TEXT NOT NULL DEFAULT '',             -- Purchase.PurchaseSource string
    grader                     TEXT NOT NULL DEFAULT 'PSA' CHECK(grader IN ('PSA', 'CGC', 'BGS', 'SGC')),  -- Purchase.Grader string
    grade_value                DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Purchase.GradeValue float64
    snapshot_json              TEXT NOT NULL DEFAULT '',             -- MarketSnapshotData.SnapshotJSON string
    snapshot_status            TEXT NOT NULL DEFAULT '' CHECK(snapshot_status IN ('', 'pending', 'failed', 'exhausted')),  -- SnapshotStatus (named string)
    snapshot_retry_count       BIGINT NOT NULL DEFAULT 0,            -- Purchase.SnapshotRetryCount int
    psa_listing_title          TEXT NOT NULL DEFAULT '',             -- Purchase.PSAListingTitle string
    override_price_cents       BIGINT NOT NULL DEFAULT 0 CHECK (override_price_cents >= 0),  -- Purchase.OverridePriceCents int
    override_source            TEXT NOT NULL DEFAULT '',             -- Purchase.OverrideSource (named string)
    override_set_at            TEXT NOT NULL DEFAULT '',             -- Purchase.OverrideSetAt string
    ai_suggested_price_cents   BIGINT NOT NULL DEFAULT 0 CHECK (ai_suggested_price_cents >= 0),  -- Purchase.AISuggestedPriceCents int
    ai_suggested_at            TEXT NOT NULL DEFAULT '',             -- Purchase.AISuggestedAt string
    card_year                  TEXT NOT NULL DEFAULT '',             -- Purchase.CardYear string
    ebay_export_flagged_at     TIMESTAMP NULL,                       -- Purchase.EbayExportFlaggedAt *time.Time
    reviewed_price_cents       BIGINT NOT NULL DEFAULT 0,            -- Purchase.ReviewedPriceCents int
    reviewed_at                TEXT NOT NULL DEFAULT '',             -- Purchase.ReviewedAt string
    review_source              TEXT NOT NULL DEFAULT '',             -- Purchase.ReviewSource (named string)
    dh_card_id                 BIGINT NOT NULL DEFAULT 0,            -- Purchase.DHCardID int
    dh_inventory_id            BIGINT NOT NULL DEFAULT 0,            -- Purchase.DHInventoryID int
    dh_cert_status             TEXT NOT NULL DEFAULT '',             -- Purchase.DHCertStatus string
    dh_listing_price_cents     BIGINT NOT NULL DEFAULT 0,            -- Purchase.DHListingPriceCents int
    dh_channels_json           TEXT NOT NULL DEFAULT '',             -- Purchase.DHChannelsJSON string
    dh_status                  TEXT NOT NULL DEFAULT '',             -- Purchase.DHStatus (named string)
    dh_push_status             TEXT NOT NULL DEFAULT '',             -- Purchase.DHPushStatus (named string)
    dh_candidates              TEXT NOT NULL DEFAULT '',             -- Purchase.DHCandidatesJSON string
    gem_rate_id                TEXT NOT NULL DEFAULT '',             -- Purchase.GemRateID string
    psa_spec_id                BIGINT NOT NULL DEFAULT 0,            -- Purchase.PSASpecID int
    dh_hold_reason             TEXT NOT NULL DEFAULT '',             -- Purchase.DHHoldReason string
    mm_value_cents             BIGINT NOT NULL DEFAULT 0,            -- Purchase.MMValueCents int
    card_player                TEXT NOT NULL DEFAULT '',             -- Purchase.CardPlayer string
    card_variation             TEXT NOT NULL DEFAULT '',             -- Purchase.CardVariation string
    card_category              TEXT NOT NULL DEFAULT '',             -- Purchase.CardCategory string
    mm_trend_pct               DOUBLE PRECISION NOT NULL DEFAULT 0,  -- Purchase.MMTrendPct float64
    mm_sales_30d               BIGINT NOT NULL DEFAULT 0,            -- Purchase.MMSales30d int
    mm_active_low_cents        BIGINT NOT NULL DEFAULT 0,            -- Purchase.MMActiveLowCents int
    cl_synced_at               TEXT DEFAULT '',                      -- Purchase.CLSyncedAt string
    mm_value_updated_at        TEXT NOT NULL DEFAULT '',             -- Purchase.MMValueUpdatedAt string
    received_at                TEXT NULL,                            -- drift fix #4: *string, NOT timestamp
    psa_ship_date              TEXT NOT NULL DEFAULT '',             -- Purchase.PSAShipDate string
    dh_last_synced_at          TEXT NOT NULL DEFAULT '',             -- Purchase.DHLastSyncedAt string
    mm_last_error              TEXT NOT NULL DEFAULT '',             -- tracked at repo layer
    mm_last_error_at           TEXT NOT NULL DEFAULT '',
    cl_last_error              TEXT NOT NULL DEFAULT '',             -- Purchase.CLLastError string
    cl_last_error_at           TEXT NOT NULL DEFAULT '',
    cl_value_updated_at        TEXT NOT NULL DEFAULT '',
    mid_price_cents            BIGINT NOT NULL DEFAULT 0,            -- MarketSnapshotData.MidPriceCents int
    last_sold_date             TEXT NOT NULL DEFAULT '',             -- MarketSnapshotData.LastSoldDate string
    dh_unlisted_detected_at    TIMESTAMP NULL,                       -- Purchase.DHUnlistedDetectedAt *time.Time
    dh_push_attempts           BIGINT NOT NULL DEFAULT 0,            -- Purchase.DHPushAttempts int
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE(grader, cert_number)
);
CREATE INDEX idx_purchases_campaign ON campaign_purchases(campaign_id);
CREATE INDEX idx_purchases_date ON campaign_purchases(purchase_date);
CREATE INDEX idx_purchases_campaign_date ON campaign_purchases(campaign_id, purchase_date DESC);
CREATE INDEX idx_purchases_snapshot_pending ON campaign_purchases(snapshot_status) WHERE snapshot_status != '';
CREATE INDEX idx_campaign_purchases_ebay_export_flagged_at
    ON campaign_purchases (ebay_export_flagged_at)
    WHERE ebay_export_flagged_at IS NOT NULL;
CREATE INDEX idx_purchases_invoice_date ON campaign_purchases(invoice_date)
    WHERE invoice_date != '';
CREATE INDEX idx_purchases_dh_cert_status ON campaign_purchases(dh_cert_status)
    WHERE dh_cert_status != '';
CREATE INDEX idx_campaign_purchases_dh_push_status
    ON campaign_purchases(dh_push_status)
    WHERE dh_push_status != '';
CREATE INDEX idx_purchases_gem_rate_id ON campaign_purchases(gem_rate_id) WHERE gem_rate_id != '';
CREATE INDEX idx_purchases_mm_last_error
    ON campaign_purchases(mm_last_error)
    WHERE mm_last_error != '';
CREATE INDEX idx_purchases_cl_last_error
    ON campaign_purchases(cl_last_error)
    WHERE cl_last_error != '';

-- campaign_sales: inventory.Sale  [types_core.go:327]
CREATE TABLE campaign_sales (
    id                         TEXT PRIMARY KEY,                     -- Sale.ID string
    purchase_id                TEXT NOT NULL,                        -- Sale.PurchaseID string
    sale_channel               TEXT NOT NULL,                        -- Sale.SaleChannel (named string)
    sale_price_cents           BIGINT NOT NULL DEFAULT 0,            -- Sale.SalePriceCents int
    sale_fee_cents             BIGINT NOT NULL DEFAULT 0,            -- Sale.SaleFeeCents int
    sale_date                  TEXT NOT NULL,                        -- Sale.SaleDate string
    days_to_sell               BIGINT NOT NULL DEFAULT 0,            -- Sale.DaysToSell int
    net_profit_cents           BIGINT NOT NULL DEFAULT 0,            -- Sale.NetProfitCents int
    last_sold_cents            BIGINT DEFAULT 0,                     -- MarketSnapshotData.LastSoldCents int
    lowest_list_cents          BIGINT DEFAULT 0,                     -- MarketSnapshotData.LowestListCents int
    conservative_cents         BIGINT DEFAULT 0,                     -- MarketSnapshotData.ConservativeCents int
    median_cents               BIGINT DEFAULT 0,                     -- MarketSnapshotData.MedianCents int
    active_listings            BIGINT DEFAULT 0,                     -- MarketSnapshotData.ActiveListings int
    sales_last_30d             BIGINT DEFAULT 0,                     -- MarketSnapshotData.SalesLast30d int
    trend_30d                  DOUBLE PRECISION DEFAULT 0,           -- MarketSnapshotData.Trend30d float64
    snapshot_date              TEXT DEFAULT '',                      -- MarketSnapshotData.SnapshotDate string
    snapshot_json              TEXT NOT NULL DEFAULT '',             -- MarketSnapshotData.SnapshotJSON string
    created_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    original_list_price_cents  BIGINT NOT NULL DEFAULT 0,            -- Sale.OriginalListPriceCents int
    price_reductions           BIGINT NOT NULL DEFAULT 0,            -- Sale.PriceReductions int
    days_listed                BIGINT NOT NULL DEFAULT 0,            -- Sale.DaysListed int
    sold_at_asking_price       BOOLEAN NOT NULL DEFAULT FALSE,       -- drift fix #5: bool
    was_cracked                BOOLEAN NOT NULL DEFAULT FALSE,       -- drift fix #5: bool
    order_id                   TEXT NOT NULL DEFAULT '',             -- Sale.OrderID string
    FOREIGN KEY (purchase_id) REFERENCES campaign_purchases(id) ON DELETE CASCADE,
    UNIQUE(purchase_id)
);
CREATE INDEX idx_sales_channel ON campaign_sales(sale_channel);
CREATE INDEX idx_sales_date ON campaign_sales(sale_date);
CREATE UNIQUE INDEX idx_sales_order_id ON campaign_sales(order_id) WHERE order_id != '';

-- ================================================================
-- Finance / invoice tables
-- ================================================================

-- invoices: inventory.Invoice  [types_core.go:306]
CREATE TABLE invoices (
    id           TEXT PRIMARY KEY,                 -- Invoice.ID string
    invoice_date TEXT NOT NULL,                    -- Invoice.InvoiceDate string
    total_cents  BIGINT NOT NULL DEFAULT 0,        -- Invoice.TotalCents int
    paid_cents   BIGINT NOT NULL DEFAULT 0,        -- Invoice.PaidCents int
    due_date     TEXT NOT NULL DEFAULT '',         -- Invoice.DueDate string
    paid_date    TEXT NOT NULL DEFAULT '',         -- Invoice.PaidDate string
    status       TEXT NOT NULL DEFAULT 'unpaid' CHECK(status IN ('unpaid', 'partial', 'paid')),  -- Invoice.Status string
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_invoices_date ON invoices(invoice_date);
CREATE INDEX idx_invoices_status ON invoices(status);

-- cashflow_config: inventory.CashflowConfig  [types_core.go:320]
CREATE TABLE cashflow_config (
    id                   BIGINT PRIMARY KEY CHECK(id = 1),
    credit_limit_cents   BIGINT NOT NULL DEFAULT 5000000,    -- TODO: no direct Go struct field; legacy column kept for compatibility
    cash_buffer_cents    BIGINT NOT NULL DEFAULT 1000000,    -- CashflowConfig.CashBufferCents int
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- revocation_flags: inventory.RevocationFlag  [types_core.go:390]
CREATE TABLE revocation_flags (
    id                TEXT PRIMARY KEY,                      -- RevocationFlag.ID string
    segment_label     TEXT NOT NULL,                         -- RevocationFlag.SegmentLabel string
    segment_dimension TEXT NOT NULL,                         -- RevocationFlag.SegmentDimension string
    reason            TEXT NOT NULL,                         -- RevocationFlag.Reason string
    status            TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'sent')),  -- RevocationFlag.Status string
    email_text        TEXT NOT NULL DEFAULT '',              -- RevocationFlag.EmailText string
    created_at        TIMESTAMP NOT NULL,                    -- RevocationFlag.CreatedAt time.Time
    sent_at           TIMESTAMP                              -- RevocationFlag.SentAt *time.Time
);
CREATE INDEX idx_revocation_flags_status ON revocation_flags(status);
CREATE INDEX idx_revocation_flags_segment ON revocation_flags(segment_label, segment_dimension);

-- ================================================================
-- Price flags (FKs: campaign_purchases + users)
-- ================================================================

-- price_flags: inventory.PriceFlag  [price_flags.go:43]
CREATE TABLE price_flags (
    id           BIGSERIAL PRIMARY KEY,                      -- PriceFlag.ID int64
    purchase_id  TEXT NOT NULL,                              -- PriceFlag.PurchaseID string
    flagged_by   BIGINT NOT NULL,                            -- PriceFlag.FlaggedBy int64
    flagged_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,  -- PriceFlag.FlaggedAt time.Time
    reason       TEXT NOT NULL CHECK(reason IN ('wrong_match', 'stale_data', 'wrong_grade', 'source_disagreement', 'other')),  -- PriceFlag.Reason (named string)
    resolved_at  TIMESTAMP,                                  -- PriceFlag.ResolvedAt *time.Time
    resolved_by  BIGINT,                                     -- PriceFlag.ResolvedBy *int64
    FOREIGN KEY (purchase_id) REFERENCES campaign_purchases(id) ON DELETE CASCADE,
    FOREIGN KEY (flagged_by) REFERENCES users(id),
    FOREIGN KEY (resolved_by) REFERENCES users(id)
);
CREATE INDEX idx_price_flags_open ON price_flags(resolved_at) WHERE resolved_at IS NULL;
CREATE INDEX idx_price_flags_purchase ON price_flags(purchase_id);

-- ================================================================
-- Card ID mappings, card access log, sync state
-- ================================================================

-- card_access_log: pricing.AccessTracker storage
CREATE TABLE card_access_log (
    id          BIGSERIAL PRIMARY KEY,
    card_name   TEXT NOT NULL,
    set_name    TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    access_type TEXT CHECK(access_type IS NULL OR access_type IN (
        'analysis', 'search', 'watchlist', 'collection'
    )),
    accessed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_access_log_card ON card_access_log(card_name, set_name, card_number, accessed_at DESC);
CREATE INDEX idx_access_log_covering ON card_access_log(card_name, set_name, card_number, accessed_at);
CREATE INDEX idx_card_access_log_recent ON card_access_log(accessed_at DESC, card_name, set_name, card_number);

-- card_id_mappings: CardIDMapping (adapter-defined)
CREATE TABLE card_id_mappings (
    card_name        TEXT NOT NULL,                         -- CardIDMapping.CardName string
    set_name         TEXT NOT NULL,                         -- CardIDMapping.SetName string
    collector_number TEXT NOT NULL DEFAULT '',              -- CardIDMapping.CollectorNumber string
    provider         TEXT NOT NULL,
    external_id      TEXT NOT NULL,                         -- CardIDMapping.ExternalID string
    hint_source      TEXT NOT NULL DEFAULT 'auto',
    created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (card_name, set_name, collector_number, provider)
);
CREATE INDEX idx_card_id_mappings_provider_external_id
    ON card_id_mappings (provider, external_id);

-- sync_state: generic KV store used by sync schedulers
CREATE TABLE sync_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ================================================================
-- Advisor cache
-- ================================================================

-- advisor_cache: advisor.CachedAnalysis  [advisor/cache.go:27]
CREATE TABLE advisor_cache (
    id            BIGSERIAL PRIMARY KEY,
    analysis_type TEXT NOT NULL,                             -- CachedAnalysis.AnalysisType (named string)
    status        TEXT NOT NULL DEFAULT 'pending',           -- CachedAnalysis.Status (named string)
    content       TEXT NOT NULL DEFAULT '',                  -- CachedAnalysis.Content string
    error_message TEXT NOT NULL DEFAULT '',                  -- CachedAnalysis.ErrorMessage string
    started_at    TEXT DEFAULT NULL,                         -- CachedAnalysis.StartedAt time.Time (stored as ISO8601 text in SQLite)
    completed_at  TEXT DEFAULT NULL,                         -- CachedAnalysis.CompletedAt time.Time (stored as ISO8601 text in SQLite)
    created_at    TEXT NOT NULL DEFAULT (NOW()::text),       -- stored as text per SQLite history
    updated_at    TEXT NOT NULL DEFAULT (NOW()::text)
);
CREATE UNIQUE INDEX idx_advisor_cache_type ON advisor_cache(analysis_type);

-- ================================================================
-- AI usage tracking
-- ================================================================

-- ai_calls: ai.AICallRecord  [ai/tracking.go:31]
CREATE TABLE ai_calls (
    id                  BIGSERIAL PRIMARY KEY,
    operation           TEXT NOT NULL CHECK(operation IN (
        'digest', 'campaign_analysis', 'liquidation',
        'purchase_assessment', 'social_caption', 'social_suggestion'
    )),                                                                  -- AICallRecord.Operation (named string)
    status              TEXT NOT NULL CHECK(status IN ('success', 'error', 'rate_limited')),
    error_message       TEXT DEFAULT '',                                 -- AICallRecord.ErrorMessage string
    latency_ms          BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.LatencyMS int64
    tool_rounds         BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.ToolRounds int
    input_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.InputTokens int
    output_tokens       BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.OutputTokens int
    total_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.TotalTokens int
    timestamp           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,             -- AICallRecord.Timestamp time.Time
    cost_estimate_cents BIGINT NOT NULL DEFAULT 0                        -- AICallRecord.CostEstimateCents int
);
CREATE INDEX idx_ai_calls_timestamp ON ai_calls(timestamp DESC);
CREATE INDEX idx_ai_calls_operation ON ai_calls(operation, timestamp DESC);

-- ================================================================
-- API tracking
-- ================================================================

-- api_calls: pricing.APICallRecord  [pricing/repository.go:30]
CREATE TABLE api_calls (
    id          BIGSERIAL PRIMARY KEY,
    provider    TEXT NOT NULL,                                  -- APICallRecord.Provider string
    endpoint    TEXT,                                           -- APICallRecord.Endpoint string
    status_code BIGINT,                                         -- APICallRecord.StatusCode int
    error       TEXT,                                           -- APICallRecord.Error string
    latency_ms  BIGINT,                                         -- APICallRecord.LatencyMS int64
    timestamp   TIMESTAMP DEFAULT CURRENT_TIMESTAMP             -- APICallRecord.Timestamp time.Time
);
CREATE INDEX idx_api_calls_provider ON api_calls(provider, timestamp DESC);
CREATE INDEX idx_api_calls_timestamp ON api_calls(timestamp DESC);
CREATE INDEX idx_api_calls_errors ON api_calls(provider, status_code) WHERE status_code >= 400;

-- api_rate_limits: pricing.APITracker rate-limit state
CREATE TABLE api_rate_limits (
    provider           TEXT PRIMARY KEY,
    calls_last_minute  BIGINT DEFAULT 0,
    calls_last_hour    BIGINT DEFAULT 0,
    calls_last_day     BIGINT DEFAULT 0,
    last_429_at        TIMESTAMP,
    blocked_until      TIMESTAMP,
    updated_at         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ================================================================
-- Card Ladder integration
-- ================================================================

-- cardladder_config: sqlite.CardLadderConfig  [cardladder_store.go:17]
CREATE TABLE cardladder_config (
    id                       BIGINT PRIMARY KEY CHECK (id = 1),
    email                    TEXT NOT NULL,                              -- CardLadderConfig.Email string
    encrypted_refresh_token  TEXT NOT NULL,                              -- encrypted CardLadderConfig.RefreshToken
    collection_id            TEXT NOT NULL,                              -- CardLadderConfig.CollectionID string
    firebase_api_key         TEXT NOT NULL,                              -- CardLadderConfig.FirebaseAPIKey string
    updated_at               TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    firebase_uid             TEXT NOT NULL DEFAULT ''                    -- CardLadderConfig.FirebaseUID string
);

-- cl_card_mappings: sqlite.CLCardMapping  [cardladder_store.go:26]
CREATE TABLE cl_card_mappings (
    slab_serial            TEXT PRIMARY KEY,                            -- CLCardMapping.SlabSerial string
    cl_collection_card_id  TEXT NOT NULL,                               -- CLCardMapping.CLCollectionCardID string
    cl_gem_rate_id         TEXT NOT NULL DEFAULT '',                    -- CLCardMapping.CLGemRateID string
    cl_condition           TEXT NOT NULL DEFAULT '',                    -- CLCardMapping.CLCondition string
    updated_at             TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- cl_sales_comps: CL sales comparables
-- TODO: no direct single Go struct; used by cl_sales_store.go for comp lookups.
CREATE TABLE cl_sales_comps (
    id            BIGSERIAL PRIMARY KEY,
    gem_rate_id   TEXT NOT NULL,
    item_id       TEXT NOT NULL,
    sale_date     TEXT NOT NULL,                                    -- stored as YYYY-MM-DD text (SQLite DATE affinity)
    price_cents   BIGINT NOT NULL,
    platform      TEXT NOT NULL,
    listing_type  TEXT NOT NULL DEFAULT '',
    seller        TEXT NOT NULL DEFAULT '',
    item_url      TEXT NOT NULL DEFAULT '',
    slab_serial   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    condition     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_cl_sales_comps_gem_rate
    ON cl_sales_comps(gem_rate_id, sale_date DESC);
CREATE UNIQUE INDEX idx_cl_sales_comps_item ON cl_sales_comps(gem_rate_id, condition, item_id);
CREATE INDEX idx_cl_sales_comps_gem_cond_date
    ON cl_sales_comps(gem_rate_id, condition, sale_date DESC);

-- ================================================================
-- Market Movers integration
-- ================================================================

-- marketmovers_config: sqlite.MarketMoversConfig  [marketmovers_store.go:17]
CREATE TABLE marketmovers_config (
    id                       BIGINT PRIMARY KEY CHECK (id = 1),
    username                 TEXT NOT NULL DEFAULT '',                  -- MarketMoversConfig.Username string
    encrypted_refresh_token  TEXT NOT NULL DEFAULT '',                  -- encrypted MarketMoversConfig.RefreshToken
    updated_at               TEXT NOT NULL DEFAULT ''                   -- stored as text per SQLite history
);

-- mm_card_mappings: sqlite.MMCardMapping  [marketmovers_store.go:23]
CREATE TABLE mm_card_mappings (
    slab_serial           TEXT PRIMARY KEY,                            -- MMCardMapping.SlabSerial string
    mm_collectible_id     BIGINT NOT NULL,                             -- MMCardMapping.MMCollectibleID int64
    updated_at            TEXT NOT NULL DEFAULT '',
    mm_master_id          BIGINT NOT NULL DEFAULT 0,                   -- MMCardMapping.MasterID int64
    mm_search_title       TEXT NOT NULL DEFAULT '',                    -- MMCardMapping.SearchTitle string
    mm_collection_item_id BIGINT NOT NULL DEFAULT 0                    -- MMCardMapping.CollectionItemID int64
);

-- ================================================================
-- Market intelligence (DH Tier 3)
-- ================================================================

-- market_intelligence: intelligence.MarketIntelligence + Velocity + Trend  [intelligence/types.go]
CREATE TABLE market_intelligence (
    card_name             TEXT NOT NULL,                               -- MarketIntelligence.CardName string
    set_name              TEXT NOT NULL,                               -- MarketIntelligence.SetName string
    card_number           TEXT NOT NULL DEFAULT '',                    -- MarketIntelligence.CardNumber string
    dh_card_id            TEXT NOT NULL,                               -- MarketIntelligence.DHCardID string
    sentiment_score       DOUBLE PRECISION,                            -- Sentiment.Score float64
    sentiment_mentions    BIGINT,                                      -- Sentiment.MentionCount int
    sentiment_trend       TEXT,                                        -- Sentiment.Trend string
    forecast_price_cents  BIGINT,                                      -- Forecast.PredictedPriceCents int64
    forecast_confidence   DOUBLE PRECISION,                            -- Forecast.Confidence float64
    forecast_date         TEXT,                                        -- Forecast.ForecastDate (stored as text)
    grading_roi           TEXT,                                        -- JSON blob (GradeROI[])
    recent_sales          TEXT,                                        -- JSON blob (Sale[])
    population            TEXT,                                        -- JSON blob (PopulationEntry[])
    insights_headline     TEXT,                                        -- Insights.Headline string
    insights_detail       TEXT,                                        -- Insights.Detail string
    fetched_at            TIMESTAMP NOT NULL,                          -- MarketIntelligence.FetchedAt time.Time
    created_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    volume_7d             BIGINT,                                      -- Trend.Volume7d int
    volume_30d            BIGINT,                                      -- Trend.Volume30d int
    volume_90d            BIGINT,                                      -- Trend.Volume90d int
    sell_through_30d_pct  DOUBLE PRECISION,                            -- Velocity.SellThrough30dPct float64
    sell_through_60d_pct  DOUBLE PRECISION,                            -- Velocity.SellThrough60dPct float64
    sell_through_90d_pct  DOUBLE PRECISION,                            -- Velocity.SellThrough90dPct float64
    velocity_sample_size  BIGINT,                                      -- Velocity.SampleSize int
    velocity_last_fetch   TIMESTAMP,                                   -- Velocity.LastFetch time.Time
    PRIMARY KEY (card_name, set_name, card_number)
);
CREATE INDEX idx_market_intelligence_dh_card_id ON market_intelligence(dh_card_id);
CREATE INDEX idx_market_intelligence_fetched_at ON market_intelligence(fetched_at);

-- dh_suggestions: intelligence.Suggestion  [intelligence/types.go:99]
CREATE TABLE dh_suggestions (
    suggestion_date      TEXT NOT NULL,                                -- Suggestion.SuggestionDate string
    type                 TEXT NOT NULL,                                -- Suggestion.Type string
    category             TEXT NOT NULL,                                -- Suggestion.Category string
    rank                 BIGINT NOT NULL,                              -- Suggestion.Rank int
    is_manual            BOOLEAN NOT NULL,                             -- drift fix #7: bool
    dh_card_id           TEXT NOT NULL,                                -- Suggestion.DHCardID string
    card_name            TEXT NOT NULL,                                -- Suggestion.CardName string
    set_name             TEXT NOT NULL,                                -- Suggestion.SetName string
    card_number          TEXT NOT NULL DEFAULT '',                     -- Suggestion.CardNumber string
    image_url            TEXT,                                         -- Suggestion.ImageURL string
    current_price_cents  BIGINT,                                       -- Suggestion.CurrentPriceCents int64
    confidence_score     DOUBLE PRECISION,                             -- Suggestion.ConfidenceScore float64
    reasoning            TEXT,                                         -- Suggestion.Reasoning string
    structured_reasoning TEXT,                                         -- Suggestion.StructuredReasoning string (JSON)
    metrics              TEXT,                                         -- Suggestion.Metrics string (JSON)
    sentiment_score      DOUBLE PRECISION,                             -- Suggestion.SentimentScore float64
    sentiment_trend      DOUBLE PRECISION,                             -- Suggestion.SentimentTrend float64
    sentiment_mentions   BIGINT,                                       -- Suggestion.SentimentMentions int
    fetched_at           TIMESTAMP NOT NULL,                           -- Suggestion.FetchedAt time.Time
    PRIMARY KEY (suggestion_date, type, category, rank)
);
CREATE INDEX idx_dh_suggestions_date ON dh_suggestions(suggestion_date);
CREATE INDEX idx_dh_suggestions_card ON dh_suggestions(card_name, set_name);

-- ================================================================
-- Scoring gaps (factor coverage)
-- ================================================================

-- scoring_data_gaps: scoring.GapRecord  [scoring/gap.go:8]
CREATE TABLE scoring_data_gaps (
    id           BIGSERIAL PRIMARY KEY,
    factor_name  TEXT NOT NULL,                                        -- GapRecord.FactorName string
    reason       TEXT NOT NULL,                                        -- GapRecord.Reason string
    entity_type  TEXT NOT NULL,                                        -- GapRecord.EntityType string
    entity_id    TEXT NOT NULL,                                        -- GapRecord.EntityID string
    card_name    TEXT NOT NULL DEFAULT '',                             -- GapRecord.CardName string
    set_name     TEXT NOT NULL DEFAULT '',                             -- GapRecord.SetName string
    recorded_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP          -- GapRecord.RecordedAt time.Time
);
CREATE INDEX idx_scoring_gaps_recorded ON scoring_data_gaps(recorded_at);
CREATE INDEX idx_scoring_gaps_factor ON scoring_data_gaps(factor_name, recorded_at);

-- ================================================================
-- Sell sheet (export)
-- ================================================================

-- sell_sheet_items: export.Service sell-sheet set
CREATE TABLE sell_sheet_items (
    purchase_id TEXT NOT NULL PRIMARY KEY,
    added_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ================================================================
-- DH push config + DH caches
-- ================================================================

-- dh_push_config: inventory.DHPushConfig  [types_dh.go]
CREATE TABLE dh_push_config (
    id                               BIGINT PRIMARY KEY CHECK (id = 1),
    swing_pct_threshold              BIGINT NOT NULL DEFAULT 20,        -- DHPushConfig.SwingPctThreshold int
    swing_min_cents                  BIGINT NOT NULL DEFAULT 5000,      -- DHPushConfig.SwingMinCents int
    disagreement_pct_threshold       BIGINT NOT NULL DEFAULT 25,        -- DHPushConfig.DisagreementPctThreshold int
    unreviewed_change_pct_threshold  BIGINT NOT NULL DEFAULT 15,        -- DHPushConfig.UnreviewedChangePctThreshold int
    unreviewed_change_min_cents      BIGINT NOT NULL DEFAULT 3000,      -- DHPushConfig.UnreviewedChangeMinCents int
    updated_at                       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- dh_card_cache: DH demand/velocity/trend/saturation analytics cache (adapter-defined)
-- Note: "window" is a reserved word in Postgres; it must be double-quoted.
CREATE TABLE dh_card_cache (
    card_id                 TEXT NOT NULL,
    "window"                TEXT NOT NULL,                             -- '7d' or '30d'
    demand_score            DOUBLE PRECISION,
    demand_data_quality     TEXT,
    demand_json             TEXT,
    velocity_json           TEXT,
    trend_json              TEXT,
    saturation_json         TEXT,
    price_distribution_json TEXT,
    analytics_computed_at   TIMESTAMP,
    demand_computed_at      TIMESTAMP,
    fetched_at              TIMESTAMP NOT NULL,
    PRIMARY KEY (card_id, "window")
);
CREATE INDEX idx_card_cache_demand_score ON dh_card_cache(demand_score DESC);

-- dh_character_cache: DH character analytics cache (adapter-defined)
-- Note: "window" is a reserved word in Postgres; it must be double-quoted.
CREATE TABLE dh_character_cache (
    character             TEXT NOT NULL,
    "window"              TEXT NOT NULL,
    demand_json           TEXT,
    velocity_json         TEXT,
    saturation_json       TEXT,
    demand_computed_at    TIMESTAMP,
    analytics_computed_at TIMESTAMP,
    fetched_at            TIMESTAMP NOT NULL,
    PRIMARY KEY (character, "window")
);

-- dh_state_events: dhevents.Event  [dhevents/events.go:50]
CREATE TABLE dh_state_events (
    id                BIGSERIAL PRIMARY KEY,
    purchase_id       TEXT,                                            -- Event.PurchaseID string (empty for orphan)
    cert_number       TEXT,                                            -- Event.CertNumber string
    event_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_type        TEXT NOT NULL,                                   -- Event.Type (named string)
    prev_push_status  TEXT,                                            -- Event.PrevPushStatus string
    new_push_status   TEXT,                                            -- Event.NewPushStatus string
    prev_dh_status    TEXT,                                            -- Event.PrevDHStatus string
    new_dh_status     TEXT,                                            -- Event.NewDHStatus string
    dh_inventory_id   BIGINT,                                          -- Event.DHInventoryID int
    dh_card_id        BIGINT,                                          -- Event.DHCardID int
    dh_order_id       TEXT,                                            -- Event.DHOrderID string
    sale_price_cents  BIGINT,                                          -- Event.SalePriceCents int
    source            TEXT NOT NULL,                                   -- Event.Source (named string)
    notes             TEXT                                             -- Event.Notes string
);
CREATE INDEX idx_dh_state_events_purchase ON dh_state_events(purchase_id, event_at);
CREATE INDEX idx_dh_state_events_cert ON dh_state_events(cert_number, event_at);
CREATE INDEX idx_dh_state_events_type_time ON dh_state_events(event_type, event_at DESC);

-- ================================================================
-- Price trajectory (weekly aggregates)
-- ================================================================

-- card_price_trajectory: intelligence.WeeklyBucket  [intelligence/trajectory.go:10]
CREATE TABLE card_price_trajectory (
    dh_card_id         TEXT NOT NULL,
    week_start         TEXT NOT NULL,                                  -- WeeklyBucket.WeekStart time.Time (stored as ISO text)
    sale_count         BIGINT NOT NULL,                                -- WeeklyBucket.SaleCount int
    avg_price_cents    BIGINT NOT NULL,                                -- WeeklyBucket.AvgPriceCents int64
    median_price_cents BIGINT NOT NULL,                                -- WeeklyBucket.MedianPriceCents int64
    refreshed_at       TIMESTAMP NOT NULL,
    PRIMARY KEY (dh_card_id, week_start)
);
CREATE INDEX idx_card_price_trajectory_card ON card_price_trajectory(dh_card_id, week_start DESC);

-- ================================================================
-- PSA pending items
-- ================================================================

-- psa_pending_items: inventory.PendingItem  [pending_items.go:10]
CREATE TABLE psa_pending_items (
    id                   TEXT PRIMARY KEY,                            -- PendingItem.ID string
    cert_number          TEXT NOT NULL,                               -- PendingItem.CertNumber string
    card_name            TEXT NOT NULL DEFAULT '',                    -- PendingItem.CardName string
    set_name             TEXT NOT NULL DEFAULT '',                    -- PendingItem.SetName string
    card_number          TEXT NOT NULL DEFAULT '',                    -- PendingItem.CardNumber string
    grade                DOUBLE PRECISION NOT NULL DEFAULT 0,         -- PendingItem.Grade float64
    buy_cost_cents       BIGINT NOT NULL DEFAULT 0,                   -- PendingItem.BuyCostCents int
    purchase_date        TEXT NOT NULL DEFAULT '',                    -- PendingItem.PurchaseDate string
    status               TEXT NOT NULL CHECK (status IN ('ambiguous', 'unmatched')),  -- PendingItem.Status string
    candidates           TEXT NOT NULL DEFAULT '[]',                  -- PendingItem.Candidates []string (stored as JSON)
    source               TEXT NOT NULL CHECK (source IN ('scheduler', 'manual')),
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at          TIMESTAMP,                                   -- PendingItem.ResolvedAt *time.Time
    resolved_campaign_id TEXT                                         -- PendingItem.ResolvedCampaignID string
);
CREATE UNIQUE INDEX idx_pending_items_unresolved_cert
    ON psa_pending_items(cert_number) WHERE resolved_at IS NULL;

-- ================================================================
-- Scheduler stats
-- ================================================================

-- scheduler_run_stats: sqlite.SchedulerRunStats  [scheduler_stats_store.go:27]
CREATE TABLE scheduler_run_stats (
    name        TEXT PRIMARY KEY,                                     -- SchedulerRunStats.Name string
    last_run_at TEXT NOT NULL,                                        -- SchedulerRunStats.LastRunAt time.Time (stored as RFC3339 text)
    duration_ms BIGINT NOT NULL,                                      -- SchedulerRunStats.DurationMs int64
    stats_json  TEXT NOT NULL,                                        -- SchedulerRunStats.StatsJSON string
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ================================================================
-- Views (depend on tables above)
-- ================================================================

-- active_sessions: displays currently-valid sessions joined with users
-- hours_until_expiry uses Postgres date arithmetic (EXTRACT(EPOCH ...) / 3600).
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
    ROUND(CAST(EXTRACT(EPOCH FROM (s.expires_at - NOW())) / 3600 AS NUMERIC), 1) AS hours_until_expiry
FROM user_sessions s
JOIN users u ON s.user_id = u.id
WHERE s.expires_at > NOW()
ORDER BY s.last_accessed_at DESC;

-- expired_sessions
CREATE VIEW expired_sessions AS
SELECT id
FROM user_sessions
WHERE expires_at <= NOW();

-- ai_usage_summary
CREATE VIEW ai_usage_summary AS
SELECT
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status = 'success' THEN 1 END) AS success_calls,
    COUNT(CASE WHEN status = 'error' THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status = 'rate_limited' THEN 1 END) AS rate_limit_hits,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(input_tokens), 0) AS total_input_tokens,
    COALESCE(SUM(output_tokens), 0) AS total_output_tokens,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents,
    TO_CHAR(MAX(timestamp), 'YYYY-MM-DD HH24:MI:SS') AS last_call_at,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '24 hours' THEN 1 END) AS calls_last_24h
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days';

-- ai_usage_by_operation
CREATE VIEW ai_usage_by_operation AS
SELECT
    operation,
    COUNT(*) AS calls,
    COUNT(CASE WHEN status = 'error' OR status = 'rate_limited' THEN 1 END) AS errors,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY operation;

-- api_usage_summary
CREATE VIEW api_usage_summary AS
SELECT
    provider,
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits,
    AVG(latency_ms) AS avg_latency_ms,
    MAX(timestamp) AS last_call_at,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '1 hour' THEN 1 END) AS calls_last_hour,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '5 minutes' THEN 1 END) AS calls_last_5min
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY provider;

-- api_hourly_distribution
CREATE VIEW api_hourly_distribution AS
SELECT
    provider,
    TO_CHAR(timestamp, 'YYYY-MM-DD HH24:00') AS hour,
    COUNT(*) AS call_count,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY provider, TO_CHAR(timestamp, 'YYYY-MM-DD HH24:00')
ORDER BY hour DESC;

-- api_daily_summary
CREATE VIEW api_daily_summary AS
SELECT
    provider,
    CAST(timestamp AS DATE) AS date,
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status_code < 400 THEN 1 END) AS successful_calls,
    COUNT(CASE WHEN status_code >= 400 THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status_code = 429 THEN 1 END) AS rate_limit_hits,
    ROUND(100.0 * COUNT(CASE WHEN status_code < 400 THEN 1 END) / COUNT(*), 1) AS success_rate_pct,
    ROUND(CAST(AVG(latency_ms) AS NUMERIC)) AS avg_latency_ms,
    MIN(timestamp) AS first_call,
    MAX(timestamp) AS last_call
FROM api_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY provider, CAST(timestamp AS DATE)
ORDER BY date DESC, provider;

-- ================================================================
-- Triggers (convert from SQLite trigger syntax)
-- ================================================================

-- advisor_cache updated_at trigger: Postgres FUNCTION + TRIGGER pair
CREATE FUNCTION trg_advisor_cache_updated_at_fn() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := (NOW())::text;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_advisor_cache_updated_at
BEFORE UPDATE ON advisor_cache
FOR EACH ROW
EXECUTE FUNCTION trg_advisor_cache_updated_at_fn();
