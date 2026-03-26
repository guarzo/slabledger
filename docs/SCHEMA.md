# Database Schema Reference

SlabLedger uses SQLite in WAL mode. Migrations are embedded in the binary and run automatically on startup. Migration files live in `internal/adapters/storage/sqlite/migrations/` (19 pairs, `000001`–`000019`).

All monetary values are stored in **cents** (integer). Timestamps use `DATETIME`/`TIMESTAMP` as SQLite text in UTC. Boolean columns use `INTEGER` (`0`/`1`).

---

## Tables

Tables are listed in dependency order: tables with no foreign keys first, then tables that reference them.

---

### `users`
Registered users authenticated via Google OAuth.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `google_id` | TEXT | UNIQUE NOT NULL | Google OAuth subject |
| `username` | TEXT | | Display name |
| `email` | TEXT | | |
| `avatar_url` | TEXT | | Profile picture |
| `is_admin` | BOOLEAN | NOT NULL DEFAULT 0 | Grants admin privileges |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `last_login_at` | TIMESTAMP | | |

**Indexes:** `idx_users_google_id` UNIQUE on `(google_id)`

**Foreign Keys:** none

---

### `oauth_states`
Short-lived CSRF tokens used during the OAuth authorization flow.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `state` | TEXT | PK | Random nonce |
| `expires_at` | DATETIME | NOT NULL | |
| `created_at` | DATETIME | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** `idx_oauth_states_expires` on `(expires_at)`

**Foreign Keys:** none

---

### `api_rate_limits`
Per-provider rate limit state and 429-block tracking.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `provider` | TEXT | PK, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |
| `calls_last_minute` | INTEGER | DEFAULT 0 | |
| `calls_last_hour` | INTEGER | DEFAULT 0 | |
| `calls_last_day` | INTEGER | DEFAULT 0 | |
| `last_429_at` | TIMESTAMP | | When the last 429 was received |
| `blocked_until` | TIMESTAMP | | Request gate: block until this time |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

---

### `api_calls`
Log of every outbound pricing API call for observability and rate analysis.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `provider` | TEXT | NOT NULL, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |
| `endpoint` | TEXT | | URL path or method name |
| `status_code` | INTEGER | | HTTP response code |
| `error` | TEXT | | Error string if failed |
| `latency_ms` | INTEGER | | Round-trip time |
| `timestamp` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_api_calls_provider` on `(provider, timestamp DESC)`
- `idx_api_calls_timestamp` on `(timestamp DESC)`
- `idx_api_calls_errors` on `(provider, status_code)` WHERE `status_code >= 400` (partial)

**Foreign Keys:** none

---

### `ai_calls`
Log of every AI (Azure OpenAI) call including token usage and estimated cost.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `operation` | TEXT | NOT NULL, CHECK IN ('digest','campaign_analysis','liquidation','purchase_assessment','social_caption','social_suggestion') | |
| `status` | TEXT | NOT NULL, CHECK IN ('success','error','rate_limited') | |
| `error_message` | TEXT | DEFAULT '' | |
| `latency_ms` | INTEGER | NOT NULL DEFAULT 0 | |
| `tool_rounds` | INTEGER | NOT NULL DEFAULT 0 | Number of tool-use iterations |
| `input_tokens` | INTEGER | NOT NULL DEFAULT 0 | |
| `output_tokens` | INTEGER | NOT NULL DEFAULT 0 | |
| `total_tokens` | INTEGER | NOT NULL DEFAULT 0 | |
| `cost_estimate_cents` | INTEGER | NOT NULL DEFAULT 0 | Added in migration 000017 |
| `timestamp` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_ai_calls_timestamp` on `(timestamp DESC)`
- `idx_ai_calls_operation` on `(operation, timestamp DESC)`

**Foreign Keys:** none

---

### `sync_state`
Generic key-value store for background scheduler checkpoints and sync cursors.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `key` | TEXT | PK | Logical name for the state entry |
| `value` | TEXT | NOT NULL | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

---

### `cashflow_config`
Singleton row holding global cashflow parameters.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, CHECK(id = 1) | Enforces singleton |
| `credit_limit_cents` | INTEGER | NOT NULL DEFAULT 5000000 | $50,000 |
| `cash_buffer_cents` | INTEGER | NOT NULL DEFAULT 1000000 | $10,000 |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none

**Foreign Keys:** none

---

### `allowed_emails`
Allowlist of emails permitted to log in (access control gate).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `email` | TEXT | PK COLLATE NOCASE | Case-insensitive match |
| `added_by` | INTEGER | REFERENCES users(id) ON DELETE SET NULL | Admin who granted access |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `notes` | TEXT | | Optional reason/label |

**Indexes:** none (PK lookup only)

**Foreign Keys:** `added_by → users(id)` ON DELETE SET NULL

---

### `revocation_flags`
Records of access revocation notices to be emailed to affected users.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | |
| `segment_label` | TEXT | NOT NULL | Human-readable segment name |
| `segment_dimension` | TEXT | NOT NULL | Dimension key (e.g. channel) |
| `reason` | TEXT | NOT NULL | |
| `status` | TEXT | NOT NULL DEFAULT 'pending', CHECK IN ('pending','sent') | |
| `email_text` | TEXT | NOT NULL DEFAULT '' | Pre-rendered email body |
| `created_at` | DATETIME | NOT NULL | |
| `sent_at` | DATETIME | | |

**Indexes:**
- `idx_revocation_flags_status` on `(status)`
- `idx_revocation_flags_segment` on `(segment_label, segment_dimension)`

**Foreign Keys:** none

---

### `card_id_mappings`
Cached provider-specific external IDs for card name/set/number triples.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `card_name` | TEXT | NOT NULL, PK part | |
| `set_name` | TEXT | NOT NULL, PK part | |
| `collector_number` | TEXT | NOT NULL DEFAULT '', PK part | |
| `provider` | TEXT | NOT NULL, PK part | |
| `external_id` | TEXT | NOT NULL | Provider's card ID |
| `hint_source` | TEXT | NOT NULL DEFAULT 'auto' | How the mapping was found |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Primary Key:** `(card_name, set_name, collector_number, provider)`

**Indexes:** `idx_card_id_mappings_provider_external_id` on `(provider, external_id)`

**Foreign Keys:** none

---

### `price_history`
Time series of card prices from all pricing sources (one row per card+grade+source+date).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | Collector number |
| `grade` | TEXT | NOT NULL | e.g. "PSA 10" |
| `price_cents` | INTEGER | NOT NULL | |
| `confidence` | REAL | DEFAULT 1.0 | Source confidence weight |
| `source` | TEXT | NOT NULL, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |
| `fusion_source_count` | INTEGER | | Number of sources fused (fusion rows only) |
| `fusion_outliers_removed` | INTEGER | | Outlier count removed during fusion |
| `fusion_method` | TEXT | | Algorithm used (e.g. "weighted_avg") |
| `price_date` | DATE | NOT NULL | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(card_name, set_name, card_number, grade, source, price_date)`

**Indexes:**
- `idx_price_history_card` on `(card_name, set_name, grade)`
- `idx_price_history_staleness` on `(source, updated_at DESC)`
- `idx_price_history_date` on `(price_date DESC)`
- `idx_price_history_lookup` on `(card_name, set_name, card_number, grade, source, price_date DESC)`

**Foreign Keys:** none

---

### `price_refresh_queue`
Work queue for the background price-refresh scheduler.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `grade` | TEXT | NOT NULL, CHECK IN (PSA/BGS/CGC grades + Raw/Ungraded) | |
| `source` | TEXT | NOT NULL, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |
| `priority` | INTEGER | DEFAULT 2, CHECK IN (1,2,3) | 1=high, 3=low |
| `scheduled_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | When to run next |
| `last_attempted_at` | TIMESTAMP | | |
| `attempts` | INTEGER | DEFAULT 0 | |
| `status` | TEXT | DEFAULT 'pending', CHECK IN ('pending','in_progress','completed','failed') | |
| `error` | TEXT | | Last error message |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(card_name, set_name, grade, source)`

**Indexes:**
- `idx_refresh_queue_priority` on `(priority ASC, scheduled_at ASC)` WHERE `status = 'pending'` (partial)
- `idx_refresh_queue_status` on `(status, last_attempted_at)`

**Foreign Keys:** `source → api_rate_limits(provider)` ON UPDATE CASCADE ON DELETE RESTRICT

---

### `card_access_log`
Access log used to prioritize price staleness detection (recently viewed cards get fresher data).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `access_type` | TEXT | CHECK IN ('analysis','search','watchlist','collection') or NULL | |
| `accessed_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_access_log_card` on `(card_name, set_name, card_number, accessed_at DESC)`
- `idx_access_log_covering` on `(card_name, set_name, card_number, accessed_at)`
- `idx_card_access_log_recent` on `(accessed_at DESC, card_name, set_name, card_number)`

**Foreign Keys:** none

---

### `discovery_failures`
Records of failed card discovery attempts to avoid hammering providers for cards they don't know about.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `card_name` | TEXT | NOT NULL, PK part | |
| `set_name` | TEXT | NOT NULL, PK part | |
| `card_number` | TEXT | NOT NULL DEFAULT '', PK part | |
| `provider` | TEXT | NOT NULL, PK part | e.g. 'cardhedger' |
| `failure_reason` | TEXT | NOT NULL | |
| `query_attempted` | TEXT | NOT NULL DEFAULT '' | The query string that failed |
| `attempts` | INTEGER | NOT NULL DEFAULT 1 | |
| `last_attempted_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Primary Key:** `(card_name, set_name, card_number, provider)`

**Indexes:** `idx_discovery_failures_provider` on `(provider, last_attempted_at DESC)`

**Foreign Keys:** none

---

### `card_request_submissions`
Tracks card IDs submitted to CardHedger for inclusion in their database.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `cert_number` | TEXT | NOT NULL | |
| `grader` | TEXT | NOT NULL DEFAULT 'PSA' | |
| `card_name` | TEXT | NOT NULL DEFAULT '' | |
| `set_name` | TEXT | NOT NULL DEFAULT '' | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `grade` | TEXT | NOT NULL DEFAULT '' | |
| `front_image_url` | TEXT | NOT NULL DEFAULT '' | |
| `variant` | TEXT | NOT NULL DEFAULT '' | |
| `status` | TEXT | NOT NULL DEFAULT 'pending' | e.g. 'pending','submitted' |
| `cardhedger_request_id` | TEXT | NOT NULL DEFAULT '' | Response ID from CardHedger |
| `submitted_at` | DATETIME | | When submitted to provider |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(grader, cert_number)`

**Indexes:** none (unique constraint only)

**Foreign Keys:** none

---

### `market_snapshot_history`
Daily archive of market data snapshots for unsold inventory — enables price trajectory analysis.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `grade_value` | REAL | NOT NULL | Numeric grade (e.g. 10.0) |
| `median_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `conservative_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `optimistic_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `last_sold_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `lowest_list_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `estimated_value_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `active_listings` | INTEGER | NOT NULL DEFAULT 0 | |
| `sales_last_30d` | INTEGER | NOT NULL DEFAULT 0 | |
| `sales_last_90d` | INTEGER | NOT NULL DEFAULT 0 | |
| `daily_velocity` | REAL | NOT NULL DEFAULT 0 | Cards sold per day |
| `weekly_velocity` | REAL | NOT NULL DEFAULT 0 | |
| `trend_30d` | REAL | NOT NULL DEFAULT 0 | Price trend over 30 days |
| `trend_90d` | REAL | NOT NULL DEFAULT 0 | |
| `volatility` | REAL | NOT NULL DEFAULT 0 | Price volatility metric |
| `source_count` | INTEGER | NOT NULL DEFAULT 0 | Number of pricing sources |
| `fusion_confidence` | REAL | NOT NULL DEFAULT 0 | |
| `snapshot_json` | TEXT | NOT NULL DEFAULT '' | Full snapshot blob |
| `snapshot_date` | DATE | NOT NULL | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(card_name, set_name, card_number, grade_value, snapshot_date)`

**Indexes:**
- `idx_msh_card_grade_date` UNIQUE on `(card_name, set_name, card_number, grade_value, snapshot_date)`
- `idx_msh_date` on `(snapshot_date DESC)`

**Foreign Keys:** none

---

### `population_history`
Tracks PSA population counts over time for population-based analytics.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `grade_value` | REAL | NOT NULL | |
| `grader` | TEXT | NOT NULL DEFAULT 'PSA' | |
| `population` | INTEGER | NOT NULL | Total pop at this grade |
| `pop_higher` | INTEGER | NOT NULL DEFAULT 0 | Pop at grades above this |
| `observation_date` | DATE | NOT NULL | |
| `source` | TEXT | NOT NULL DEFAULT 'csv_import' | How the data was ingested |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(card_name, set_name, card_number, grade_value, grader, observation_date)`

**Indexes:** `idx_pop_history_card_date` UNIQUE on `(card_name, set_name, card_number, grade_value, grader, observation_date)`

**Foreign Keys:** none

---

### `cl_value_history`
Tracks Card Ladder (CL) value changes per cert over time.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `cert_number` | TEXT | NOT NULL | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `grade_value` | REAL | NOT NULL | |
| `cl_value_cents` | INTEGER | NOT NULL | Card Ladder valuation |
| `observation_date` | DATE | NOT NULL | |
| `source` | TEXT | NOT NULL DEFAULT 'csv_import' | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(cert_number, observation_date)`

**Indexes:** `idx_cl_history_cert_date` UNIQUE on `(cert_number, observation_date)`

**Foreign Keys:** none

---

### `advisor_cache`
Cached results from the AI advisor scheduler (one row per analysis type).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `analysis_type` | TEXT | NOT NULL | Unique key for the analysis (e.g. 'digest') |
| `status` | TEXT | NOT NULL DEFAULT 'pending' | e.g. 'pending','running','done','error' |
| `content` | TEXT | NOT NULL DEFAULT '' | Rendered analysis output |
| `error_message` | TEXT | NOT NULL DEFAULT '' | |
| `started_at` | TEXT | DEFAULT NULL | ISO datetime or NULL |
| `completed_at` | TEXT | DEFAULT NULL | ISO datetime or NULL |
| `created_at` | TEXT | NOT NULL DEFAULT (datetime('now')) | |
| `updated_at` | TEXT | NOT NULL DEFAULT (datetime('now')) | Auto-updated via trigger |

**Unique:** `idx_advisor_cache_type` on `(analysis_type)`

**Triggers:** `trg_advisor_cache_updated_at` — sets `updated_at = datetime('now')` on every UPDATE.

**Foreign Keys:** none

---

### `instagram_config`
Singleton row holding the connected Instagram account credentials.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, CHECK(id = 1) | Enforces singleton |
| `access_token` | TEXT | NOT NULL DEFAULT '' | Long-lived token |
| `ig_user_id` | TEXT | NOT NULL DEFAULT '' | Instagram user ID |
| `username` | TEXT | NOT NULL DEFAULT '' | |
| `token_expires_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime |
| `connected_at` | TEXT | NOT NULL DEFAULT '' | |
| `updated_at` | TEXT | NOT NULL DEFAULT (datetime('now')) | |

**Indexes:** none

**Foreign Keys:** none

---

### `invoices`
Purchase invoices from PSA Partner Offers for cashflow tracking.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | |
| `invoice_date` | TEXT | NOT NULL | ISO date |
| `total_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `paid_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `due_date` | TEXT | NOT NULL DEFAULT '' | |
| `paid_date` | TEXT | NOT NULL DEFAULT '' | |
| `status` | TEXT | NOT NULL DEFAULT 'unpaid', CHECK IN ('unpaid','partial','paid') | |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_invoices_date` on `(invoice_date)`
- `idx_invoices_status` on `(status)`

**Foreign Keys:** none

---

### `campaigns`
Top-level acquisition campaigns defining buying parameters and strategy.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `name` | TEXT | NOT NULL | |
| `sport` | TEXT | NOT NULL DEFAULT '' | e.g. 'pokemon' |
| `year_range` | TEXT | NOT NULL DEFAULT '' | e.g. '2000-2005' |
| `grade_range` | TEXT | NOT NULL DEFAULT '' | e.g. 'PSA 8-10' |
| `price_range` | TEXT | NOT NULL DEFAULT '' | e.g. '$50-$500' |
| `cl_confidence` | REAL | NOT NULL DEFAULT 0 | Min CL confidence threshold |
| `buy_terms_cl_pct` | REAL | NOT NULL DEFAULT 0 | Target buy price as % of CL value |
| `daily_spend_cap_cents` | INTEGER | NOT NULL DEFAULT 0 | Max daily spend |
| `inclusion_list` | TEXT | NOT NULL DEFAULT '' | Newline-separated card list filter |
| `exclusion_mode` | INTEGER | NOT NULL DEFAULT 0 | 1 = treat inclusion_list as exclusions |
| `phase` | TEXT | NOT NULL DEFAULT 'pending' | e.g. 'pending','active','paused','closed' |
| `psa_sourcing_fee_cents` | INTEGER | NOT NULL DEFAULT 300 | Per-card fee ($3.00) |
| `ebay_fee_pct` | REAL | NOT NULL DEFAULT 0.1235 | eBay/TCGPlayer fee percentage |
| `expected_fill_rate` | REAL | NOT NULL DEFAULT 0.0 | Expected % of offers accepted |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

---

### `user_sessions`
Active browser sessions for authenticated users.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | Session token (opaque) |
| `user_id` | INTEGER | NOT NULL | |
| `expires_at` | TIMESTAMP | NOT NULL | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `last_accessed_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `user_agent` | TEXT | | Browser user-agent |
| `ip_address` | TEXT | | |

**Indexes:**
- `idx_user_sessions_user_id` on `(user_id)`
- `idx_user_sessions_expires_at` on `(expires_at)`

**Foreign Keys:** `user_id → users(id)` ON DELETE CASCADE

---

### `user_tokens`
OAuth access/refresh tokens, scoped to a session.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `user_id` | INTEGER | NOT NULL | |
| `access_token` | TEXT | NOT NULL | AES-encrypted |
| `refresh_token` | TEXT | NOT NULL | AES-encrypted |
| `token_type` | TEXT | DEFAULT 'Bearer' | |
| `expires_at` | TIMESTAMP | NOT NULL | |
| `scope` | TEXT | | OAuth scopes |
| `session_id` | TEXT | REFERENCES user_sessions(id) ON DELETE CASCADE | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_user_tokens_user_id` on `(user_id)`
- `idx_user_tokens_session_id` on `(session_id)`
- `idx_user_tokens_session_unique` UNIQUE on `(session_id)`
- `idx_user_tokens_expires_at` on `(expires_at)`

**Foreign Keys:**
- `user_id → users(id)` ON DELETE CASCADE
- `session_id → user_sessions(id)` ON DELETE CASCADE

---

### `favorites`
User-saved favorite cards for quick access.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `user_id` | INTEGER | NOT NULL | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `image_url` | TEXT | | |
| `notes` | TEXT | | |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(user_id, card_name, set_name, card_number)`

**Indexes:** `idx_favorites_user_created` on `(user_id, created_at DESC)`

**Foreign Keys:** `user_id → users(id)` ON DELETE CASCADE

---

### `campaign_purchases`
Individual graded cards bought under a campaign.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `campaign_id` | TEXT | NOT NULL | |
| `card_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `set_name` | TEXT | NOT NULL DEFAULT '' | |
| `cert_number` | TEXT | NOT NULL | Grading company cert |
| `population` | INTEGER | NOT NULL DEFAULT 0 | PSA pop at time of purchase |
| `cl_value_cents` | INTEGER | NOT NULL DEFAULT 0 | Card Ladder valuation |
| `buy_cost_cents` | INTEGER | NOT NULL DEFAULT 0 | Purchase price paid |
| `psa_sourcing_fee_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `purchase_date` | TEXT | NOT NULL | ISO date |
| `last_sold_cents` | INTEGER | DEFAULT 0 | Market snapshot |
| `lowest_list_cents` | INTEGER | DEFAULT 0 | |
| `conservative_cents` | INTEGER | DEFAULT 0 | |
| `median_cents` | INTEGER | DEFAULT 0 | |
| `active_listings` | INTEGER | DEFAULT 0 | |
| `sales_last_30d` | INTEGER | DEFAULT 0 | |
| `trend_30d` | REAL | DEFAULT 0 | |
| `snapshot_date` | TEXT | DEFAULT '' | ISO date of last snapshot |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `vault_status` | TEXT | NOT NULL DEFAULT '' | PSA vault status |
| `invoice_date` | TEXT | NOT NULL DEFAULT '' | |
| `was_refunded` | INTEGER | NOT NULL DEFAULT 0 | Boolean |
| `front_image_url` | TEXT | NOT NULL DEFAULT '' | |
| `back_image_url` | TEXT | NOT NULL DEFAULT '' | |
| `purchase_source` | TEXT | NOT NULL DEFAULT '' | e.g. 'psa_partner_offers' |
| `grader` | TEXT | NOT NULL DEFAULT 'PSA', CHECK IN ('PSA','CGC','BGS','SGC') | |
| `grade_value` | REAL | NOT NULL DEFAULT 0 | Numeric grade |
| `snapshot_json` | TEXT | NOT NULL DEFAULT '' | Full market snapshot blob |
| `snapshot_status` | TEXT | NOT NULL DEFAULT '', CHECK IN ('','pending','failed','exhausted') | |
| `snapshot_retry_count` | INTEGER | NOT NULL DEFAULT 0 | |
| `psa_listing_title` | TEXT | NOT NULL DEFAULT '' | Raw PSA title for LLM fallback; added migration 000003 |
| `override_price_cents` | INTEGER | NOT NULL DEFAULT 0, CHECK >= 0 | User-set price override; added migration 000008 |
| `override_source` | TEXT | NOT NULL DEFAULT '' | Source label for override; added migration 000008 |
| `override_set_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime of override; added migration 000008 |
| `ai_suggested_price_cents` | INTEGER | NOT NULL DEFAULT 0, CHECK >= 0 | AI suggestion (pending user accept); added migration 000008 |
| `ai_suggested_at` | TEXT | NOT NULL DEFAULT '' | Added migration 000008 |
| `card_year` | TEXT | NOT NULL DEFAULT '' | Added migration 000018 |
| `ebay_export_flagged_at` | TIMESTAMP | NULL | When flagged for eBay export; added migration 000018 |

**Unique:** `(grader, cert_number)`

**Indexes:**
- `idx_purchases_campaign` on `(campaign_id)`
- `idx_purchases_date` on `(purchase_date)`
- `idx_purchases_campaign_date` on `(campaign_id, purchase_date DESC)`
- `idx_purchases_snapshot_pending` on `(snapshot_status)` WHERE `snapshot_status != ''` (partial)
- `idx_campaign_purchases_ebay_export_flagged_at` on `(ebay_export_flagged_at)` WHERE `ebay_export_flagged_at IS NOT NULL` (partial); added migration 000019

**Foreign Keys:** `campaign_id → campaigns(id)` ON DELETE CASCADE

---

### `campaign_sales`
Sale records for purchased cards (one per purchase, enforced by UNIQUE).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `purchase_id` | TEXT | NOT NULL | |
| `sale_channel` | TEXT | NOT NULL | e.g. 'ebay','tcgplayer','local','other' |
| `sale_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Gross sale price |
| `sale_fee_cents` | INTEGER | NOT NULL DEFAULT 0 | Platform fees |
| `sale_date` | TEXT | NOT NULL | ISO date |
| `days_to_sell` | INTEGER | NOT NULL DEFAULT 0 | Days from purchase to sale |
| `net_profit_cents` | INTEGER | NOT NULL DEFAULT 0 | |
| `last_sold_cents` | INTEGER | DEFAULT 0 | Market snapshot at time of sale |
| `lowest_list_cents` | INTEGER | DEFAULT 0 | |
| `conservative_cents` | INTEGER | DEFAULT 0 | |
| `median_cents` | INTEGER | DEFAULT 0 | |
| `active_listings` | INTEGER | DEFAULT 0 | |
| `sales_last_30d` | INTEGER | DEFAULT 0 | |
| `trend_30d` | REAL | DEFAULT 0 | |
| `snapshot_date` | TEXT | DEFAULT '' | |
| `snapshot_json` | TEXT | NOT NULL DEFAULT '' | |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `original_list_price_cents` | INTEGER | NOT NULL DEFAULT 0 | List price at first posting; added migration 000007 |
| `price_reductions` | INTEGER | NOT NULL DEFAULT 0 | Count of price drops; added migration 000007 |
| `days_listed` | INTEGER | NOT NULL DEFAULT 0 | Added migration 000007 |
| `sold_at_asking_price` | INTEGER | NOT NULL DEFAULT 0 | Boolean; added migration 000007 |
| `was_cracked` | INTEGER | NOT NULL DEFAULT 0 | 1 if slab was cracked out; added migration 000012 |

**Unique:** `(purchase_id)` — one sale record per purchase

**Indexes:**
- `idx_sales_channel` on `(sale_channel)`
- `idx_sales_date` on `(sale_date)`

**Foreign Keys:** `purchase_id → campaign_purchases(id)` ON DELETE CASCADE

---

### `social_posts`
Social media post drafts (Instagram carousels) generated by the social content scheduler.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `post_type` | TEXT | NOT NULL | e.g. 'recent_acquisitions' |
| `status` | TEXT | NOT NULL DEFAULT 'draft' | 'draft','publishing','published','failed' |
| `caption` | TEXT | NOT NULL DEFAULT '' | Post caption text |
| `hashtags` | TEXT | NOT NULL DEFAULT '' | |
| `cover_title` | TEXT | NOT NULL DEFAULT '' | Slide 0 title |
| `card_count` | INTEGER | NOT NULL DEFAULT 0 | |
| `instagram_post_id` | TEXT | NOT NULL DEFAULT '' | ID returned after publish; added migration 000013 |
| `error_message` | TEXT | NOT NULL DEFAULT '' | Publish error detail; added migration 000014 |
| `slide_urls` | TEXT | DEFAULT NULL | JSON array of slide image URLs; added migration 000016 |
| `created_at` | TEXT | NOT NULL DEFAULT (datetime('now')) | |
| `updated_at` | TEXT | NOT NULL DEFAULT (datetime('now')) | |

**Indexes:**
- `idx_social_posts_status` on `(status)`
- `idx_social_posts_type` on `(post_type)`

**Foreign Keys:** none

---

### `social_post_cards`
Junction table linking social posts to the purchases that appear as slides.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `post_id` | TEXT | NOT NULL, PK part | |
| `purchase_id` | TEXT | NOT NULL, PK part | |
| `slide_order` | INTEGER | NOT NULL DEFAULT 1 | 1-based slide position |

**Primary Key:** `(post_id, purchase_id)`

**Indexes:** `idx_social_post_cards_purchase` on `(purchase_id)`

**Foreign Keys:** `post_id → social_posts(id)` ON DELETE CASCADE

---

## Views

### `stale_prices`
Price rows that have exceeded their staleness threshold, used by the refresh scheduler to select work. Prices above $100 go stale after 12 hours; $50–$100 after 24 hours; under $50 after 48 hours. Includes `psa_listing_title` from the most recent matching purchase for the CardHedger LLM fallback.

### `api_usage_summary`
Aggregated API call statistics (total, errors, 429s, latency, call counts) per provider for the last 24 hours.

### `api_hourly_distribution`
Hourly call counts and rate-limit hits per provider for the last 7 days. Useful for spotting traffic spikes.

### `api_daily_summary`
Daily success rate, error count, and average latency per provider for the last 7 days.

### `active_sessions`
Sessions where `expires_at > now()`, joined to `users` for username/google_id, with hours-until-expiry.

### `expired_sessions`
Session IDs where `expires_at <= now()`, used by the session-cleanup scheduler.

### `ai_usage_summary`
Aggregate AI call statistics for the last 7 days: total calls, success/error/rate-limited counts, token totals, and estimated cost.

### `ai_usage_by_operation`
Per-operation breakdown of AI call counts, error rates, latency, token usage, and cost for the last 7 days.

---

## FK Dependency Graph

```
users
├── user_sessions          (user_id → users.id CASCADE DELETE)
│   └── user_tokens        (session_id → user_sessions.id CASCADE DELETE)
├── user_tokens            (user_id → users.id CASCADE DELETE)
├── favorites              (user_id → users.id CASCADE DELETE)
└── allowed_emails         (added_by → users.id SET NULL)

api_rate_limits
└── price_refresh_queue    (source → api_rate_limits.provider UPDATE CASCADE, DELETE RESTRICT)

campaigns
└── campaign_purchases     (campaign_id → campaigns.id CASCADE DELETE)
    └── campaign_sales     (purchase_id → campaign_purchases.id CASCADE DELETE)

social_posts
└── social_post_cards      (post_id → social_posts.id CASCADE DELETE)

── Standalone tables (no FK dependencies) ──
price_history
api_calls
ai_calls
card_access_log
card_id_mappings
sync_state
cashflow_config
invoices
revocation_flags
discovery_failures
card_request_submissions
market_snapshot_history
population_history
cl_value_history
advisor_cache
instagram_config
oauth_states
```
