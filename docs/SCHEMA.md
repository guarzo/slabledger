# Database Schema Reference

SlabLedger uses SQLite in WAL mode. Migrations are embedded in the binary and run automatically on startup. Migration files live in `internal/adapters/storage/sqlite/migrations/` (43 pairs, `000001`–`000043`).

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
| `provider` | TEXT | PK | |
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
| `provider` | TEXT | NOT NULL | |
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

### ~~`price_history`~~ — DROPPED (migration 000038)

Dropped in migration 000038. DH computes prices in-memory; no production code wrote to this table.

---

### ~~`price_refresh_queue`~~ — DROPPED (migration 000038)

Dropped in migration 000038. Was always empty; replaced by purchase-driven refresh via `campaign_purchases`.

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

### ~~`discovery_failures`~~ — DROPPED (migration 000038)

Dropped in migration 000038. Was used for external pricing source discovery; source removed.

---

### `card_request_submissions`
Tracks card IDs submitted to external pricing services.

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
| `cardhedger_request_id` | TEXT | NOT NULL DEFAULT '' | Response ID from external service (legacy column name) |
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
| `dh_card_id` | INTEGER | NOT NULL DEFAULT 0 | DH card identity from cert resolution; added migration 000030 |
| `dh_inventory_id` | INTEGER | NOT NULL DEFAULT 0 | DH inventory item ID; added migration 000030 |
| `dh_cert_status` | TEXT | NOT NULL DEFAULT '' | Resolution state: matched, ambiguous, not_found; added migration 000030 |
| `dh_listing_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Current DH listing price; added migration 000030 |
| `dh_channels_json` | TEXT | NOT NULL DEFAULT '' | Per-channel sync status JSON; added migration 000030 |
| `reviewed_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Human-reviewed price; added migration 000020 |
| `reviewed_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime of review; added migration 000020 |
| `review_source` | TEXT | NOT NULL DEFAULT '' | Source label for review; added migration 000020 |
| `dh_status` | TEXT | NOT NULL DEFAULT '' | DH inventory status; added migration 000032 |
| `dh_push_status` | TEXT | NOT NULL DEFAULT '' | Pipeline status: "", "pending", "matched", "unmatched", "manual"; added migration 000034 |
| `dh_candidates` | TEXT | NOT NULL DEFAULT '' | Ambiguous cert resolution candidates JSON; added migration 000039 |
| `gem_rate_id` | TEXT | NOT NULL DEFAULT '' | CardLadder gem rate identifier; added migration 000040 |
| `psa_spec_id` | INTEGER | NOT NULL DEFAULT 0 | PSA spec identifier; added migration 000040 |

**Unique:** `(grader, cert_number)`

**Indexes:**
- `idx_purchases_campaign` on `(campaign_id)`
- `idx_purchases_date` on `(purchase_date)`
- `idx_purchases_campaign_date` on `(campaign_id, purchase_date DESC)`
- `idx_purchases_snapshot_pending` on `(snapshot_status)` WHERE `snapshot_status != ''` (partial)
- `idx_campaign_purchases_ebay_export_flagged_at` on `(ebay_export_flagged_at)` WHERE `ebay_export_flagged_at IS NOT NULL` (partial); added migration 000019
- `idx_purchases_invoice_date` on `(invoice_date)` WHERE `invoice_date != ''` (partial); added migration 000027
- `idx_purchases_dh_cert_status` on `(dh_cert_status)` WHERE `dh_cert_status != ''` (partial); added migration 000030
- `idx_campaign_purchases_dh_push_status` on `(dh_push_status)` WHERE `dh_push_status != ''` (partial); added migration 000035
- `idx_purchases_gem_rate_id` on `(gem_rate_id)` WHERE `gem_rate_id != ''` (partial); added migration 000040, converted to partial in 000043

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
| `order_id` | TEXT | NOT NULL DEFAULT '' | DH order ID for poll idempotency; added migration 000030 |

**Unique:** `(purchase_id)` — one sale record per purchase

**Indexes:**
- `idx_sales_channel` on `(sale_channel)`
- `idx_sales_date` on `(sale_date)`
- `idx_sales_order_id` on `(order_id)` WHERE `order_id != ''` (partial unique); added migration 000030

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
| `background_urls` | TEXT | DEFAULT NULL | Background image URLs; added migration 000021 |
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

### `price_flags`
Price data quality flags raised by users for review.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `purchase_id` | TEXT | NOT NULL | |
| `flagged_by` | INTEGER | NOT NULL | User who flagged |
| `flagged_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `reason` | TEXT | NOT NULL, CHECK IN ('wrong_match','stale_data','wrong_grade','source_disagreement','other') | |
| `resolved_at` | DATETIME | | NULL until resolved |
| `resolved_by` | INTEGER | | User who resolved |

**Indexes:**
- `idx_price_flags_open` on `(resolved_at)` WHERE `resolved_at IS NULL` (partial)
- `idx_price_flags_purchase` on `(purchase_id)`

**Foreign Keys:**
- `purchase_id → campaign_purchases(id)` ON DELETE CASCADE
- `flagged_by → users(id)`
- `resolved_by → users(id)`

---

### `ai_picks`
AI-generated acquisition picks (daily card recommendations).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `pick_date` | DATE | NOT NULL | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `grade` | TEXT | NOT NULL | |
| `direction` | TEXT | NOT NULL, CHECK IN ('buy','watch','avoid') | |
| `confidence` | TEXT | NOT NULL, CHECK IN ('high','medium','low') | |
| `buy_thesis` | TEXT | NOT NULL | |
| `target_buy_price` | INTEGER | | In cents |
| `expected_sell_price` | INTEGER | | In cents |
| `signals_json` | TEXT | NOT NULL DEFAULT '[]' | |
| `rank` | INTEGER | NOT NULL | |
| `source` | TEXT | NOT NULL, CHECK IN ('ai','watchlist_reassessment') | |
| `created_at` | DATETIME | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(pick_date, card_name, set_name, grade)`

**Indexes:** `idx_ai_picks_date` on `(pick_date)`

**Foreign Keys:** none

---

### `acquisition_watchlist`
User/AI acquisition watchlist for cards to track.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `grade` | TEXT | NOT NULL | |
| `source` | TEXT | NOT NULL, CHECK IN ('manual','auto_from_pick') | |
| `active` | INTEGER | NOT NULL DEFAULT 1 | Boolean |
| `latest_pick_id` | INTEGER | REFERENCES ai_picks(id) ON DELETE SET NULL | |
| `added_at` | DATETIME | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(card_name, set_name, grade)` WHERE `active = 1` (partial)

**Indexes:** `idx_acq_watchlist_active` on `(active)`

**Foreign Keys:** `latest_pick_id → ai_picks(id)` ON DELETE SET NULL

---

### `cardladder_config`
Singleton row holding Card Ladder API credentials.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, CHECK(id = 1) | Enforces singleton |
| `email` | TEXT | NOT NULL | CL account email |
| `encrypted_refresh_token` | TEXT | NOT NULL | AES-encrypted |
| `collection_id` | TEXT | NOT NULL | CL collection ID |
| `firebase_api_key` | TEXT | NOT NULL | Firebase auth key |
| `firebase_uid` | TEXT | NOT NULL DEFAULT '' | Firebase user ID; added migration 000025 |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none

**Foreign Keys:** none

---

### `cl_card_mappings`
Maps purchase cert numbers to Card Ladder card IDs for sync.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `slab_serial` | TEXT | PK | Cert number |
| `cl_collection_card_id` | TEXT | NOT NULL | CL card identifier |
| `cl_gem_rate_id` | TEXT | NOT NULL DEFAULT '' | CL gem rate identifier |
| `cl_condition` | TEXT | NOT NULL DEFAULT '' | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

---

### `cl_sales_comps`
Card Ladder sales comparables data (recent auction/sale records).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `gem_rate_id` | TEXT | NOT NULL | CL gem rate identifier |
| `item_id` | TEXT | NOT NULL | CL sale item ID |
| `sale_date` | DATE | NOT NULL | |
| `price_cents` | INTEGER | NOT NULL | |
| `platform` | TEXT | NOT NULL | e.g. 'ebay', 'slab' |
| `listing_type` | TEXT | NOT NULL DEFAULT '' | |
| `seller` | TEXT | NOT NULL DEFAULT '' | |
| `item_url` | TEXT | NOT NULL DEFAULT '' | |
| `slab_serial` | TEXT | NOT NULL DEFAULT '' | |
| `condition` | TEXT | NOT NULL DEFAULT '' | Grade-specific condition label; added migration 000040 |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(gem_rate_id, condition, item_id)`

**Indexes:**
- `idx_cl_sales_comps_gem_rate` on `(gem_rate_id, sale_date DESC)`
- `idx_cl_sales_comps_gem_cond_date` on `(gem_rate_id, condition, sale_date DESC)`; added migration 000041

**Foreign Keys:** none

---

### `market_intelligence`
Market intelligence data from DoubleHolo (sentiment, forecasts, grading ROI).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `card_name` | TEXT | NOT NULL, PK part | |
| `set_name` | TEXT | NOT NULL, PK part | |
| `card_number` | TEXT | NOT NULL DEFAULT '', PK part | |
| `dh_card_id` | TEXT | NOT NULL | DH card identifier |
| `sentiment_score` | REAL | | |
| `sentiment_mentions` | INTEGER | | |
| `sentiment_trend` | TEXT | | |
| `forecast_price_cents` | INTEGER | | |
| `forecast_confidence` | REAL | | |
| `forecast_date` | TEXT | | |
| `grading_roi` | TEXT | | JSON blob |
| `recent_sales` | TEXT | | JSON blob |
| `population` | TEXT | | JSON blob |
| `insights_headline` | TEXT | | |
| `insights_detail` | TEXT | | |
| `fetched_at` | TIMESTAMP | NOT NULL | |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Primary Key:** `(card_name, set_name, card_number)`

**Indexes:**
- `idx_market_intelligence_dh_card_id` on `(dh_card_id)`
- `idx_market_intelligence_fetched_at` on `(fetched_at)`

**Foreign Keys:** none

---

### `dh_suggestions`
Daily buy/sell suggestions from DoubleHolo.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `suggestion_date` | TEXT | NOT NULL, PK part | |
| `type` | TEXT | NOT NULL, PK part | |
| `category` | TEXT | NOT NULL, PK part | |
| `rank` | INTEGER | NOT NULL, PK part | |
| `is_manual` | INTEGER | NOT NULL | Boolean |
| `dh_card_id` | TEXT | NOT NULL | |
| `card_name` | TEXT | NOT NULL | |
| `set_name` | TEXT | NOT NULL | |
| `card_number` | TEXT | NOT NULL DEFAULT '' | |
| `image_url` | TEXT | | |
| `current_price_cents` | INTEGER | | |
| `confidence_score` | REAL | | |
| `reasoning` | TEXT | | |
| `structured_reasoning` | TEXT | | |
| `metrics` | TEXT | | |
| `sentiment_score` | REAL | | |
| `sentiment_trend` | REAL | | |
| `sentiment_mentions` | INTEGER | | |
| `fetched_at` | TIMESTAMP | NOT NULL | |

**Primary Key:** `(suggestion_date, type, category, rank)`

**Indexes:**
- `idx_dh_suggestions_date` on `(suggestion_date)`
- `idx_dh_suggestions_card` on `(card_name, set_name)`

**Foreign Keys:** none

---

### `scoring_data_gaps`
Records of missing data encountered during scoring/analytics.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `factor_name` | TEXT | NOT NULL | Scoring factor that had missing data |
| `reason` | TEXT | NOT NULL | Why data was missing |
| `entity_type` | TEXT | NOT NULL | e.g. 'purchase', 'campaign' |
| `entity_id` | TEXT | NOT NULL | |
| `card_name` | TEXT | NOT NULL DEFAULT '' | |
| `set_name` | TEXT | NOT NULL DEFAULT '' | |
| `recorded_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_scoring_gaps_recorded` on `(recorded_at)`
- `idx_scoring_gaps_factor` on `(factor_name, recorded_at)`

**Foreign Keys:** none

---

### `instagram_post_metrics`
Polled engagement metrics for published Instagram posts.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, AUTOINCREMENT | |
| `post_id` | TEXT | NOT NULL | |
| `impressions` | INTEGER | NOT NULL DEFAULT 0 | |
| `reach` | INTEGER | NOT NULL DEFAULT 0 | |
| `likes` | INTEGER | NOT NULL DEFAULT 0 | |
| `comments` | INTEGER | NOT NULL DEFAULT 0 | |
| `saves` | INTEGER | NOT NULL DEFAULT 0 | |
| `shares` | INTEGER | NOT NULL DEFAULT 0 | |
| `polled_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:**
- `idx_post_metrics_post_id` on `(post_id)`
- `idx_post_metrics_polled_at` on `(polled_at)`
- `idx_post_metrics_post_id_polled_at` on `(post_id, polled_at DESC)`

**Foreign Keys:** `post_id → social_posts(id)` ON DELETE CASCADE

---

### `sell_sheet_items`
Global sell sheet item selections (persisted across sessions, not scoped to a user). Migrated to global in migration 000042.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `purchase_id` | TEXT | PK | |
| `added_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Primary Key:** `purchase_id`

**Indexes:** none

**Foreign Keys:** none

---

## Views

### ~~`stale_prices`~~ — DROPPED (migration 000038)

Dropped with `price_history`. The refresh scheduler now queries `campaign_purchases` directly.

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
├── allowed_emails         (added_by → users.id SET NULL)
└── price_flags            (flagged_by → users.id, resolved_by → users.id)

api_rate_limits                (standalone after price_refresh_queue dropped)

campaigns
└── campaign_purchases     (campaign_id → campaigns.id CASCADE DELETE)
    ├── campaign_sales     (purchase_id → campaign_purchases.id CASCADE DELETE)
    └── price_flags        (purchase_id → campaign_purchases.id CASCADE DELETE)

social_posts
├── social_post_cards      (post_id → social_posts.id CASCADE DELETE)
└── instagram_post_metrics (post_id → social_posts.id CASCADE DELETE)

ai_picks
└── acquisition_watchlist  (latest_pick_id → ai_picks.id SET NULL)

── Standalone tables (no FK dependencies) ──
api_calls
ai_calls
card_access_log
card_id_mappings
sync_state
cashflow_config
invoices
revocation_flags
card_request_submissions
market_snapshot_history
population_history
cl_value_history
advisor_cache
instagram_config
oauth_states
cardladder_config
cl_card_mappings
cl_sales_comps
market_intelligence
dh_suggestions
scoring_data_gaps
sell_sheet_items
```
