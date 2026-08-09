# Database Schema Reference

SlabLedger uses Postgres (Supabase in prod, local Postgres in the devcontainer). Migrations are embedded in the binary and run automatically on startup. Migration files live in `internal/adapters/storage/postgres/migrations/`.

All monetary values are stored in **cents** (integer).

## Migration numbering

Every `migration NNNNN` citation in this document refers to a file in
`internal/adapters/storage/postgres/migrations/`. There is no other numbering in play.

`000001_initial_schema` is the **final-state schema after the cutover from SQLite** — it
creates every table and column that survived the cutover in one step. Anything it creates
is cited here as `000001`, regardless of which pre-cutover SQLite migration originally
introduced it; those SQLite migration numbers (which ran as high as 000067) no longer
correspond to any file in this repo and are not cited. Every later migration is
incremental.

To confirm a citation:

```bash
ls internal/adapters/storage/postgres/migrations/ | grep '^NNNNN'
```

Objects that were removed before the cutover never appear in the Postgres history at all.
They are kept below as struck-through stubs, marked "dropped pre-cutover", because code
and older docs still reference the names.

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

**Indexes:** none — `idx_users_google_id` was dropped in migration 000003 (see "Dropped indexes" below). The `google_id` UNIQUE constraint still enforces uniqueness.

**Foreign Keys:** none

---

### `oauth_states`
Short-lived CSRF tokens used during the OAuth authorization flow.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `state` | TEXT | PK | Random nonce |
| `expires_at` | DATETIME | NOT NULL | |
| `created_at` | DATETIME | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** `idx_oauth_states_expires` on `(expires_at)` — created by 000001, dropped by 000003 as unused, restored by migration 000026

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

**Foreign Keys:** none

---

### ~~`ai_calls`~~ — DROPPED (migration 000035)

Dropped in migration 000035 with the removal of the Azure AI advisor. Logged every LLM call — operation, status, latency, tool rounds, token counts, and estimated cost in cents. Its two indexes were already gone (dropped in migration 000003, listed under "Dropped indexes" below).

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

**Indexes:**
- `idx_allowed_emails_added_by` on `(added_by)`; added migration 000003

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

**Indexes:** none — both were dropped in migration 000003 (see "Dropped indexes" below).

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

**Indexes:**
- `idx_card_id_mappings_provider_external_id` on `(provider, external_id)`
- `idx_card_id_mappings_card_name` on `(card_name)`; added migration 000003
- `idx_card_id_mappings_collector_number` on `(collector_number)`; added migration 000002

**Foreign Keys:** none

---

### ~~`price_history`~~ — DROPPED (pre-cutover)

Dropped before the Postgres cutover; never created by any migration in this repo. DH computes prices in-memory; no production code wrote to this table.

---

### ~~`price_refresh_queue`~~ — DROPPED (pre-cutover)

Dropped before the Postgres cutover; never created by any migration in this repo. Was always empty; replaced by purchase-driven refresh via `campaign_purchases`.

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

### ~~`discovery_failures`~~ — DROPPED (pre-cutover)

Dropped before the Postgres cutover; never created by any migration in this repo. Was used for external pricing source discovery; source removed.

---

### ~~`advisor_cache`~~ — DROPPED (migration 000013)

Dropped in migration 000013, along with its `idx_advisor_cache_type` unique index and `trg_advisor_cache_updated_at` trigger. Cached results from the AI advisor scheduler, one row per analysis type.

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
| `inclusion_list` | TEXT | NOT NULL DEFAULT '' | Legacy substring filter. Kept as a derived, write-only mirror of `subjects`/`subject_filter_mode` for one release (nothing reads it) — see migration 000024 |
| `exclusion_mode` | INTEGER | NOT NULL DEFAULT 0 | Legacy polarity flag mirroring `subject_filter_mode == 'Exclude'`. Same write-only status as `inclusion_list` |
| `phase` | TEXT | NOT NULL DEFAULT 'pending' | e.g. 'pending','active','paused','closed' |
| `psa_sourcing_fee_cents` | INTEGER | NOT NULL DEFAULT 300 | Per-card fee ($3.00) |
| `ebay_fee_pct` | REAL | NOT NULL DEFAULT 0.1235 | eBay/TCGPlayer fee percentage |
| `expected_fill_rate` | REAL | NOT NULL DEFAULT 0.0 | Expected % of offers accepted |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `psa_campaign_request_id` | TEXT | | Linked PSA portal campaign request ID; added migration 000018 |
| `target_languages` | JSONB | NOT NULL DEFAULT '[]' | `[]string` — curated PSA spec-list language tokens this campaign buys: any of `'english'`, `'japanese'`. An **empty array is an open net** (buys any language); a non-empty array requires the card's classified language to be a member. Unordered set — order is not meaningful and must not be compared. Added migration 000024 |
| `subject_filter_mode` | TEXT | NOT NULL DEFAULT 'Target', CHECK IN ('Target','Exclude','') | `'Target'` (buy only `subjects`) or `'Exclude'` (buy everything except `subjects`); added migration 000024. `''` is permitted because the domain treats it as valid and both read and write paths normalize it to `'Target'` |
| `subjects` | JSONB | NOT NULL DEFAULT '[]' | `[]TargetSubject` (`{id, name}`) — character subjects this campaign targets or excludes, ids copied verbatim from the portal. Migration 000024's legacy backfill writes id `-1` as a sentinel meaning "legacy name, never reconciled against the portal"; push translation refuses a campaign containing sentinel entries until a baseline pull replaces them. Id `0` is distinct and means "operator-typed name awaiting name-based resolution". Added migration 000024 |
| `denied_specs` | JSONB | NOT NULL DEFAULT '[]' | `[]TargetSubject` — individual cards excluded regardless of `subjects`; added migration 000024 |

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
| `session_id` | TEXT | NOT NULL, UNIQUE, REFERENCES user_sessions(id) ON DELETE CASCADE | `StoreTokens` writes the SHA-256 hash of the session id, matching `user_sessions.id`. NOT NULL + UNIQUE added migration 000031 |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |
| `updated_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** `user_tokens_session_id_key` UNIQUE on `session_id` — the implicit index behind the constraint migration 000031 added.

000001's four indexes were all dropped in migration 000003 (see "Dropped indexes" below). One of them, `idx_user_tokens_session_unique`, was UNIQUE, and dropping it removed the one-token-per-session guarantee at the database level. `AuthRepository.StoreTokens` upserts with `ON CONFLICT(session_id)`, which Postgres resolves against a unique index or constraint and otherwise rejects outright with SQLSTATE 42P10 — so from 000003 until 000031 every token write failed, silently, because the sole caller only logs a warning (SLA-57). 000031 restores the guarantee as a **named constraint** rather than a bare index, so the intent is visible in `\d` and an index advisor cannot recommend dropping it a second time. The `NOT NULL` is part of the fix: a UNIQUE constraint permits unlimited NULLs, which would let NULL-session rows bypass the conflict target and accumulate.

**Foreign Keys:**
- `user_id → users(id)` ON DELETE CASCADE
- `session_id → user_sessions(id)` ON DELETE CASCADE

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
| `psa_listing_title` | TEXT | NOT NULL DEFAULT '' | Raw PSA title used for DH card matching; added migration 000001 |
| `override_price_cents` | INTEGER | NOT NULL DEFAULT 0, CHECK >= 0 | User-set price override; added migration 000001 |
| `override_source` | TEXT | NOT NULL DEFAULT '' | Source label for override; added migration 000001 |
| `override_set_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime of override; added migration 000001 |
| `ai_suggested_price_cents` | INTEGER | NOT NULL DEFAULT 0, CHECK >= 0 | AI suggestion (pending user accept); added migration 000001 |
| `ai_suggested_at` | TEXT | NOT NULL DEFAULT '' | Added migration 000001 |
| `card_year` | TEXT | NOT NULL DEFAULT '' | Added migration 000001 |
| `ebay_export_flagged_at` | TIMESTAMP | NULL | When flagged for eBay export; added migration 000001 |
| `dh_card_id` | INTEGER | NOT NULL DEFAULT 0 | DH card identity from cert resolution; added migration 000001 |
| `dh_inventory_id` | INTEGER | NOT NULL DEFAULT 0 | DH inventory item ID; added migration 000001 |
| `dh_cert_status` | TEXT | NOT NULL DEFAULT '' | Resolution state: matched, ambiguous, not_found; added migration 000001 |
| `dh_listing_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Current DH listing price; added migration 000001 |
| `dh_channels_json` | TEXT | NOT NULL DEFAULT '' | Per-channel sync status JSON; added migration 000001 |
| `reviewed_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Human-reviewed price; added migration 000001 |
| `reviewed_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime of review; added migration 000001 |
| `review_source` | TEXT | NOT NULL DEFAULT '' | Source label for review; added migration 000001 |
| `dh_status` | TEXT | NOT NULL DEFAULT '' | DH inventory status; added migration 000001 |
| `dh_push_status` | TEXT | NOT NULL DEFAULT '' | Pipeline status: "", "pending", "matched", "unmatched", "manual"; added migration 000001 |
| `dh_candidates` | TEXT | NOT NULL DEFAULT '' | Ambiguous cert resolution candidates JSON; added migration 000001 |
| `gem_rate_id` | TEXT | NOT NULL DEFAULT '' | CardLadder gem rate identifier; added migration 000001 |
| `psa_spec_id` | INTEGER | NOT NULL DEFAULT 0 | PSA spec identifier; added migration 000001 |
| `dh_hold_reason` | TEXT | NOT NULL DEFAULT '' | Safety hold reason blocking DH push; added migration 000001 |
| `card_player` | TEXT | NOT NULL DEFAULT '' | Player/character name from CL metadata; added migration 000001 |
| `card_variation` | TEXT | NOT NULL DEFAULT '' | Card variation from CL metadata; added migration 000001 |
| `card_category` | TEXT | NOT NULL DEFAULT '' | Card category from CL metadata; added migration 000001 |
| `cl_synced_at` | TEXT | DEFAULT '' | When card was last synced to Card Ladder; added migration 000001 |
| `received_at` | DATETIME | DEFAULT NULL | ISO datetime when PSA returned the card; added migration 000001 |
| `psa_ship_date` | TEXT | NOT NULL DEFAULT '' | Date PSA shipped the card to user; added migration 000001 |
| `dh_last_synced_at` | TEXT | NOT NULL DEFAULT '' | Last time DH push pipeline ran for this card; added migration 000001 |
| `cl_last_error` | TEXT | NOT NULL DEFAULT '' | Last Card Ladder integration error; added migration 000001 |
| `cl_last_error_at` | TEXT | NOT NULL DEFAULT '' | ISO datetime of last CL error; added migration 000001 |
| `cl_value_updated_at` | TEXT | NOT NULL DEFAULT '' | When CL value was last refreshed; added migration 000001 |
| `mid_price_cents` | INTEGER | NOT NULL DEFAULT 0 | Mid-market price from DH snapshot; added migration 000001 |
| `last_sold_date` | TEXT | NOT NULL DEFAULT '' | ISO date of last DH sale; added migration 000001 |
| `cl_confidence_at_purchase` | SMALLINT | NULL | Card Ladder confidence at time of purchase; NULL=not captured; added migration 000022 |
| `population_at_purchase` | BIGINT | NULL | PSA population snapshot at time of purchase; NULL=not captured; added migration 000022 |
| `dh_confidence_at_purchase` | DOUBLE PRECISION | NULL | DH price confidence at time of purchase; NULL=not captured; added migration 000022 |
| `source_count_at_purchase` | BIGINT | NULL | Number of pricing sources observed at purchase; NULL=not captured; added migration 000022 |
| `active_listings_at_purchase` | BIGINT | NULL | Active listing count at time of purchase; NULL=not captured; added migration 000022 |
| `sales_last_30d_at_purchase` | BIGINT | NULL | 30-day sale count at time of purchase; NULL=not captured; added migration 000022 |

**Unique:** `(grader, cert_number)`

**Indexes:**
- `idx_purchases_campaign` on `(campaign_id)`
- `idx_purchases_date` on `(purchase_date)`
- `idx_purchases_campaign_date` on `(campaign_id, purchase_date DESC)`
- `idx_purchases_snapshot_pending` on `(snapshot_status)` WHERE `snapshot_status != ''` (partial)
- `idx_purchases_invoice_date` on `(invoice_date)` WHERE `invoice_date != ''` (partial)
- `idx_campaign_purchases_cert_number` on `(cert_number)`; added migration 000003
- `idx_campaign_purchases_attribution_source` on `(attribution_source)`; added migration 000023
- `idx_campaign_purchases_dh_inventory_id` on `(dh_inventory_id)`; added migration 000002
- `idx_campaign_purchases_dh_push_status` on `(dh_push_status)` WHERE `dh_push_status != ''` (partial)
- `idx_purchases_cl_last_error` on `(cl_last_error)` WHERE `cl_last_error != ''` (partial)

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
| `original_list_price_cents` | INTEGER | NOT NULL DEFAULT 0 | List price at first posting; added migration 000001 |
| `price_reductions` | INTEGER | NOT NULL DEFAULT 0 | Count of price drops; added migration 000001 |
| `days_listed` | INTEGER | NOT NULL DEFAULT 0 | Added migration 000001 |
| `sold_at_asking_price` | INTEGER | NOT NULL DEFAULT 0 | Boolean; added migration 000001 |
| `was_cracked` | INTEGER | NOT NULL DEFAULT 0 | 1 if slab was cracked out; added migration 000001 |
| `order_id` | TEXT | NOT NULL DEFAULT '' | DH order ID for poll idempotency; added migration 000001 |
| `sale_reason` | TEXT | NOT NULL DEFAULT '', CHECK IN ('', 'discretionary', 'invoice_pressure', 'aging_policy', 'bulk_lot', 'show_clearout') | Why the sale happened; empty is backfilled by the `campaign_sales_derive_reason` trigger (see below); added migration 000022 |
| `cl_value_at_sale_cents` | BIGINT | NOT NULL DEFAULT 0 | Card Ladder value at time of sale; added migration 000022 |
| `channel_fee_pct_at_sale` | DOUBLE PRECISION | NULL | Effective channel fee % at time of sale; NULL=not captured; added migration 000022 |

**Unique:** `(purchase_id)` — one sale record per purchase

**Indexes:**
- `idx_sales_date` on `(sale_date)`
- `idx_sales_order_id` UNIQUE on `(order_id)` WHERE `order_id != ''` (partial)

**Foreign Keys:** `purchase_id → campaign_purchases(id)` ON DELETE CASCADE

**Trigger:** `campaign_sales_derive_reason_trg` (BEFORE INSERT OR UPDATE, added migration 000022) — if
`sale_reason` is left empty on a write, derives it from `forced_liquidation`
(`TRUE` → `'invoice_pressure'`, `FALSE` → `'discretionary'`). `forced_liquidation`
remains a plain, app-maintained boolean column (not a generated column); the app
keeps it in sync with `sale_reason` (`forced_liquidation = (sale_reason =
'invoice_pressure')`). The trigger exists primarily so a legacy-shaped INSERT
(no `sale_reason`, e.g. from a rolled-back previous image) still gets a
non-empty reason rather than silently falling out of reason-based analysis.

---

### `psa_pending_items`
PSA card items awaiting cert resolution or campaign matching. Tracks ambiguous or unmatched certs from PSA partner feeds that need manual or algorithmic resolution.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `cert_number` | TEXT | NOT NULL UNIQUE | PSA cert ID |
| `card_name` | TEXT | NOT NULL DEFAULT '' | Card name |
| `set_name` | TEXT | NOT NULL DEFAULT '' | Set name |
| `card_number` | TEXT | NOT NULL DEFAULT '' | Card number |
| `grade` | REAL | NOT NULL DEFAULT 0 | PSA grade |
| `buy_cost_cents` | INTEGER | NOT NULL DEFAULT 0 | Purchase price in cents |
| `purchase_date` | TEXT | NOT NULL DEFAULT '' | ISO date of purchase |
| `status` | TEXT | NOT NULL CHECK IN ('ambiguous', 'unmatched') | Resolution state |
| `candidates` | TEXT | NOT NULL DEFAULT '[]' | JSON array of candidate campaigns (for ambiguous) |
| `source` | TEXT | NOT NULL CHECK IN ('scheduler', 'manual') | How the item entered pending state |
| `created_at` | DATETIME | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `resolved_at` | DATETIME | DEFAULT NULL | When resolution occurred (NULL if unresolved) |
| `resolved_campaign_id` | TEXT | DEFAULT NULL | Campaign ID after resolution; added migration 000001 |

**Unique:** `(cert_number)` — enforced only for unresolved rows, via the partial unique index below

**Indexes:**
- `idx_pending_items_unresolved_cert` UNIQUE on `(cert_number)` WHERE `resolved_at IS NULL` (partial)
- `idx_psa_pending_items_resolved_at` on `(resolved_at)`; added migration 000004

**Foreign Keys:** none (external resolution may link to campaigns)

---

### `psa_campaign_snapshot`
Singleton table holding the most recent PSA portal campaign-list fetch. Written by the
`cmd/psa-harvest` job (see [docs/psa-harvester.md](../docs/psa-harvester.md#campaign-sync));
the main app only reads it (`GET /api/psa-campaigns`), never fetches from PSA itself.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, CHECK(id = 1) | Enforces singleton |
| `raw_json` | JSONB | NOT NULL | Serialized `[]PortalCampaign` |
| `fetched_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | When the harvester fetched the snapshot |
| `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | Row last-write time |

**Indexes:** none

**Foreign Keys:** none

**Added:** migration 000018

---

### `psa_campaign_push_queue`
Human-approval queue for proposed PSA campaign edits. The app enqueues `pending` rows
(`POST /api/campaigns/{id}/psa-propose`) and flips them to `approved`
(`POST /api/campaigns/{id}/psa-publish`); only the harvester (`DrainPushQueue`) moves a
row further, first atomically claiming it (`approved` → `pushing`) and then recording
`pushed`/`failed` after actually calling PSA's `updateCampaign`.

Both transitions the harvester makes are conditional on the row's current status, so a
drain run holding a stale snapshot cannot overwrite a row another run already claimed and
pushed. RLS is enabled with a `TO service_role` policy and the `anon`/`authenticated`
grants revoked (migration 000029, the same pattern as 000027), so a PostgREST caller
holding either of those roles cannot flip a row to `approved`. That is not a control
against a `service_role` or direct-database writer — the status column is a claim the
database cannot verify — which is what the claim guard above, and SLA-44, cover.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | TEXT | PK | UUID |
| `psa_campaign_id` | TEXT | NOT NULL | PSA portal `campaignRequestId` |
| `internal_campaign_id` | TEXT | | Linked internal campaign ID |
| `proposed_diff` | JSONB | NOT NULL | Serialized `ProposedDiff` (field changes) |
| `status` | TEXT | NOT NULL DEFAULT 'pending' | `pending`, `approved`, `pushing`, `pushed`, `failed` |
| `requested_by` | TEXT | | Username that proposed the change |
| `approved_by` | TEXT | | Username that approved the change |
| `result_json` | JSONB | | Result payload from the push attempt |
| `error` | TEXT | | Error message if `status = 'failed'` |
| `approved_at` | TIMESTAMPTZ | | When the approval was signed; part of the signed envelope |
| `payload_digest` | TEXT | | Hex SHA-256 of the canonical `proposed_diff` at approval time |
| `approval_signature` | TEXT | | Hex HMAC-SHA256 over the approval envelope; key never stored here |
| `signature_key_id` | TEXT | | Identifier of the signing key, so rotation does not strand in-flight approvals |
| `created_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |
| `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

The four signature columns (migration 000034, SLA-44) are nullable: rows approved
before that migration carry no signature, and the drain refuses them rather than
treating an unsigned row as authorized. The signature is what closes the gap the
paragraph above describes — `status = 'approved'` is a claim the database cannot
verify, but the HMAC is minted with `PSA_PUSH_SIGNING_KEY`, which lives only in the
application environment. See [docs/psa-harvester.md](psa-harvester.md).

**Indexes:**
- `idx_psa_push_queue_status` on `(status)`
- `uq_psa_push_queue_create_unresolved` UNIQUE on `(internal_campaign_id)` WHERE `operation = 'create' AND status IN ('pending','approved','pushing')` (partial); added migration 000020

**Foreign Keys:** none

**Added:** migration 000018 (RLS enabled in 000029; approval signature columns in 000034)

---

### `psa_portal_catalog`
Persisted PSA portal reference data (curated spec lists and character subjects) harvested
by `cmd/psa-harvest`. The main app has no portal session, so it reads this table to build a
pure `psacampaign.Resolver` at translation time instead of calling the portal — see
[docs/psa-harvester.md](../docs/psa-harvester.md#baseline-pull-one-time-targeting-migration).

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `kind` | TEXT | PK (composite) | `'spec_lists'` or `'subjects'` |
| `key` | TEXT | PK (composite) | `''` for spec lists; the category id as text (e.g. `'16'` for Pokemon) for subjects |
| `payload` | JSONB | NOT NULL | Serialized `[]SpecListRef` or `[]SubjectRef` |
| `fetched_at` | TIMESTAMPTZ | NOT NULL | When the harvester last wrote this row; `psacampaign.NewCatalogResolver` refuses to build a resolver from a row older than `psacampaign.CatalogMaxAge` (7 days) |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000025

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
- `idx_price_flags_resolved_at` on `(resolved_at)`; added migration 000004
- `idx_price_flags_flagged_by` on `(flagged_by)`; added migration 000003
- `idx_price_flags_resolved_by` on `(resolved_by)`; added migration 000003

**Foreign Keys:**
- `purchase_id → campaign_purchases(id)` ON DELETE CASCADE
- `flagged_by → users(id)`
- `resolved_by → users(id)`

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
| `firebase_uid` | TEXT | NOT NULL DEFAULT '' | Firebase user ID; added migration 000001 |
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
| `condition` | TEXT | NOT NULL DEFAULT '' | Grade-specific condition label; added migration 000001 |
| `created_at` | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | |

**Unique:** `(gem_rate_id, condition, item_id)`

**Indexes:**
- `idx_cl_sales_comps_gem_cond_date` on `(gem_rate_id, condition, sale_date DESC)`
- `idx_cl_sales_comps_item` UNIQUE on `(gem_rate_id, condition, item_id)`

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
- `idx_market_intelligence_velocity_last_fetch` on `(velocity_last_fetch)`; added migration 000003

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
- `idx_dh_suggestions_fetched_at` on `(fetched_at)`; added migration 000003

**Foreign Keys:** none

---

### ~~`scoring_data_gaps`~~ — DROPPED (migration 000035)

Dropped in migration 000035 with the removal of the scoring domain. Recorded missing data encountered during scoring — factor name, reason, entity type and id, card and set name. Its surviving index `idx_scoring_gaps_recorded` went with the table; `idx_scoring_gaps_factor` had already been dropped in migration 000003.

---

### ~~`sell_sheet_items`~~ — DROPPED (migration 000007)

Dropped in migration 000007. Held global sell sheet item selections (`purchase_id` PK, `added_at`), persisted across sessions and not scoped to a user.

---

### `dh_push_config`
Singleton row holding safety thresholds for the DH price push pipeline. Added in migration 000001.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK, CHECK(id = 1) | Enforces singleton |
| `swing_pct_threshold` | INTEGER | NOT NULL DEFAULT 20 | Max allowed price swing % |
| `swing_min_cents` | INTEGER | NOT NULL DEFAULT 5000 | Min absolute swing to trigger hold ($50) |
| `disagreement_pct_threshold` | INTEGER | NOT NULL DEFAULT 25 | Max CL/DH price disagreement % |
| `unreviewed_change_pct_threshold` | INTEGER | NOT NULL DEFAULT 15 | Max unreviewed price change % |
| `unreviewed_change_min_cents` | INTEGER | NOT NULL DEFAULT 3000 | Min absolute unreviewed change ($30) |
| `updated_at` | TIMESTAMP | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none

**Foreign Keys:** none

---

### ~~`marketmovers_config`~~ — DROPPED (migration 000030)

Held Market Movers API credentials (singleton row: `username`, `encrypted_refresh_token`). Market Movers was retired in migration 000021, which dropped its siblings `mm_sales_comps` and `mm_card_mappings` by name but missed this one; 000030 finished the job. The single production row held a credential for the retired vendor and was discarded deliberately — `000030_drop_marketmovers_config.down.sql` restores the structure only.

---

### ~~`mm_card_mappings`~~ — DROPPED (migration 000021)

Dropped in migration 000021 alongside `mm_sales_comps` and the seven `mm_*` columns on `campaign_purchases`. Mapped purchase cert numbers to Market Movers collectible IDs for value sync. Market Movers was retired in favor of Card Ladder.

---

### `dh_card_cache`
Per-card DH enterprise analytics + demand cache. Populated by the daily DH analytics refresh scheduler (`DH_ANALYTICS_REFRESH_ENABLED`). Keyed by `(card_id, window)`. Added in migration 000001.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `card_id` | TEXT | PK part | DH card ID (stringified) |
| `window` | TEXT | PK part | `'7d'` or `'30d'` |
| `demand_score` | REAL | nullable | From `/market/demand_signals`; NULL when card lacked demand data |
| `demand_data_quality` | TEXT | nullable | `'proxy'` \| `'full'` \| NULL |
| `character_name` | TEXT | nullable | DH character this card belongs to (migration 000033, SLA-63); NULL until the scheduler backfills it |
| `analytics_computed_at` | TIMESTAMP | nullable | DH's `computed_at` for analytics; NULL = not computed (404) |
| `demand_computed_at` | TIMESTAMP | nullable | DH's `computed_at` for demand |
| `fetched_at` | TIMESTAMP | NOT NULL | When we last upserted the row |

`character_name` is sticky on upsert: the analytics refresh rewrites every row
each run without knowing the character, so the `ON CONFLICT` clause uses
`COALESCE(excluded.character_name, dh_card_cache.character_name)` — a new value
overwrites, but a NULL never clears attribution we paid a per-card API call for.

**Indexes:** `idx_card_cache_character_name` on `character_name`, partial
(`WHERE character_name IS NOT NULL`) since unattributed rows are never selected
— added in migration 000033. `idx_card_cache_demand_score` was dropped in
migration 000003 (see "Dropped indexes" below).

**Dropped columns:** `demand_json`, `velocity_json`, `trend_json`,
`saturation_json` and `price_distribution_json` (all nullable TEXT, from 000001)
were dropped in migration 000036. SLA-41 narrowed this table's read/write path to
seven columns and stopped touching all five; 000036 is the contract half of that
expand/contract pair, deliberately held back a release so an image rollback past
SLA-41 still found the schema it expected. `dh_character_cache`'s similarly-named
`*_json` columns are unrelated and still live.

**Foreign Keys:** none (DH card IDs aren't FK'd to our tables)

---

### `dh_character_cache`
Per-character DH analytics + demand cache. Populated by the same scheduler as `dh_card_cache`. Keyed by `(character, window)`. Added in migration 000001.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `character` | TEXT | PK part | DH-normalized Pokemon character name |
| `window` | TEXT | PK part | `'7d'` or `'30d'` |
| `demand_json` | TEXT | nullable | `/character_demand` response (includes `by_era` when scheduler requested it) |
| `velocity_json` | TEXT | nullable | From `/characters/velocity` |
| `saturation_json` | TEXT | nullable | From `/characters/saturation` |
| `demand_computed_at` | TIMESTAMP | nullable | |
| `analytics_computed_at` | TIMESTAMP | nullable | |
| `fetched_at` | TIMESTAMP | NOT NULL | |

**Indexes:** none (PK lookup + full scan for leaderboard)

**Note:** the three `*_json` columns are still read and written, but since SLA-41
they decode into named domain structs (`demand.CharacterDemand`,
`CharacterVelocity`, `CharacterSaturation`) inside the postgres adapter rather
than being handed to the domain as opaque strings. A column that fails to decode
is recorded per-column and does not fail the row scan.

**Foreign Keys:** none

---

### `dh_card_tombstones`
DH card IDs that repeatedly fail to resolve. Suppresses retry storms against the DH API for cards that will never match.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `dh_card_id` | BIGINT | PK | |
| `first_seen_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | First failure |
| `last_seen_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | Most recent failure |
| `attempts` | INT | NOT NULL DEFAULT 1 | Cumulative failure count |
| `last_error` | TEXT | NOT NULL DEFAULT '' | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000011

---

### `dh_comp_cache`
Pre-aggregated sales analytics from DH's graded-sales-analytics endpoint, keyed by card and grade. Read by liquidation comp pricing.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `dh_card_id` | INT | NOT NULL, PK part | |
| `grade` | TEXT | NOT NULL, PK part | |
| `total_sales` | INT | NOT NULL DEFAULT 0 | All-time sale count |
| `recent_count_90d` | INT | NOT NULL DEFAULT 0 | Sales in trailing 90 days |
| `median_cents` | INT | NOT NULL DEFAULT 0 | |
| `avg_cents` | INT | NOT NULL DEFAULT 0 | |
| `min_cents` | INT | NOT NULL DEFAULT 0 | |
| `max_cents` | INT | NOT NULL DEFAULT 0 | |
| `price_change_30d_pct` | REAL | nullable | NULL when not computable |
| `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

**Primary Key:** `(dh_card_id, grade)`

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000005

---

### `card_price_trajectory`
Weekly rolled-up DH sale statistics per card, used for trend/velocity signals.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `dh_card_id` | TEXT | NOT NULL, PK part | Stored as TEXT here, unlike the BIGINT/INT `dh_card_id` on `dh_state_events` and `dh_comp_cache` |
| `week_start` | TEXT | NOT NULL, PK part | ISO date of the week's Monday |
| `sale_count` | BIGINT | NOT NULL | Sales observed that week |
| `avg_price_cents` | BIGINT | NOT NULL | |
| `median_price_cents` | BIGINT | NOT NULL | |
| `refreshed_at` | TIMESTAMP | NOT NULL | |

**Primary Key:** `(dh_card_id, week_start)`

**Indexes:** none — `idx_card_price_trajectory_card` was created by 000001 and dropped by 000003. The PK `(dh_card_id, week_start)` still serves prefix lookups by card.

**Foreign Keys:** none

---

### `dh_state_events`
Append-only audit log of DH push/inventory state transitions. Maps to `dhevents.Event` (`internal/domain/dhevents/events.go`). Most columns are nullable because a given event type only fills the fields relevant to it.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | BIGSERIAL | PK | |
| `purchase_id` | TEXT | nullable | Not an FK — events outlive the purchases they describe |
| `cert_number` | TEXT | nullable | |
| `event_at` | TIMESTAMP | NOT NULL DEFAULT CURRENT_TIMESTAMP | |
| `event_type` | TEXT | NOT NULL | |
| `prev_push_status` | TEXT | nullable | |
| `new_push_status` | TEXT | nullable | |
| `prev_dh_status` | TEXT | nullable | |
| `new_dh_status` | TEXT | nullable | |
| `dh_inventory_id` | BIGINT | nullable | |
| `dh_card_id` | BIGINT | nullable | |
| `dh_order_id` | TEXT | nullable | |
| `sale_price_cents` | BIGINT | nullable | |
| `source` | TEXT | NOT NULL | What produced the event (scheduler, handler, …) |
| `notes` | TEXT | nullable | |

**Indexes:**
- `idx_dh_state_events_type_time` on `(event_type, event_at DESC)`
- `idx_dh_state_events_purchase` on `(purchase_id, event_at)`
- `idx_dh_state_events_cert` on `(cert_number, event_at)`

The last two were created by 000001, dropped by 000003 as unscanned — correct at the
time, since nothing read the table per row — and restored by 000032 when SLA-58 added
the read path (`GET /api/dh/events`).

**Retention:** the table is append-only and no write path ever deletes from it. The
`dh-event-cleanup` scheduler prunes rows older than `DH_EVENT_RETENTION_DAYS`
(default 90) — see [SCHEDULERS.md](SCHEDULERS.md).

**Foreign Keys:** none

---

### `scheduler_run_stats`
One row per named background job recording its last run. Backs the scheduler status endpoints.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `name` | TEXT | PK | Job name |
| `last_run_at` | TEXT | NOT NULL | RFC3339 timestamp |
| `duration_ms` | BIGINT | NOT NULL | |
| `stats_json` | TEXT | NOT NULL | Job-specific counters, shape varies by job |
| `updated_at` | TIMESTAMP | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

---

### `psa_portal_token`
Singleton row caching the PSA portal session access token.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK DEFAULT 1, CHECK(id = 1) | Enforces singleton |
| `access_token` | TEXT | NOT NULL | |
| `expires_at` | TIMESTAMPTZ | NOT NULL | |
| `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000016

---

### `psa_portal_snapshot`
Singleton row holding the most recent raw PSA portal fetch, so the app can serve portal data without re-hitting PSA.

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| `id` | INTEGER | PK DEFAULT 1, CHECK(id = 1) | Enforces singleton |
| `rows` | JSONB | NOT NULL | Raw portal rows as fetched |
| `fetched_at` | TIMESTAMPTZ | NOT NULL | When PSA was called |
| `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

**Indexes:** none (PK lookup only)

**Foreign Keys:** none

**Added:** migration 000017

---

## Views

### ~~`stale_prices`~~ — DROPPED (pre-cutover)

Dropped with `price_history`. The refresh scheduler now queries `campaign_purchases` directly.

### `api_usage_summary`
Aggregated API call statistics (total, errors, 429s, latency, call counts) per provider for the last 24 hours.
The only view with a consumer: `DBTracker.GetAPIUsage` selects from it.

### ~~`api_hourly_distribution`~~ — DROPPED (migration 000038)
Dropped in migration 000038 as consumerless. Hourly call counts and rate-limit hits per provider for the last 7 days.

### ~~`api_daily_summary`~~ — DROPPED (migration 000038)
Dropped in migration 000038 as consumerless. Daily success rate, error count, and average latency per provider for the last 7 days.

### ~~`active_sessions`~~ — DROPPED (migration 000038)
Dropped in migration 000038 as consumerless. Sessions where `expires_at > now()`, joined to `users` for username/google_id, with hours-until-expiry.

### ~~`expired_sessions`~~ — DROPPED (migration 000038)
Dropped in migration 000038 as consumerless. Session IDs where `expires_at <= now()`.

This entry previously claimed the view was "used by the session-cleanup scheduler". It was not:
`DeleteExpiredSessions` issues `DELETE FROM user_sessions WHERE expires_at <= $1` against the table
directly and never named the view. The four views above were created by 000001, recreated by 000003
`WITH (security_invoker = true)`, and read by nothing in the repository.

### ~~`ai_usage_summary`~~ — DROPPED (migration 000035)
Dropped in migration 000035 with `ai_calls`. Aggregated AI call statistics for the last 7 days: total calls, success/error/rate-limited counts, token totals, and estimated cost.

### ~~`ai_usage_by_operation`~~ — DROPPED (migration 000035)
Dropped in migration 000035 with `ai_calls`. Per-operation breakdown of AI call counts, error rates, latency, token usage, and cost for the last 7 days.

---

## Dropped indexes

Migration 000003 dropped 28 indexes that Supabase's advisor reported as never scanned, and
migration 000021 dropped one more along with the Market Movers columns. 25 of 000003's 28
are gone for good and are listed below. Three have since been restored and are live again,
documented under their tables above: `idx_oauth_states_expires` (by 000026), and
`idx_dh_state_events_purchase` / `idx_dh_state_events_cert` (by 000032). They are
catalogued here because older docs, query plans, and commit messages still name them —
none of the rows below exist in the current schema.

| Index | Table | Dropped by |
|-------|-------|-----------|
| `idx_ai_calls_operation` | `ai_calls` | 000003 |
| `idx_ai_calls_timestamp` | `ai_calls` | 000003 |
| `idx_api_calls_errors` | `api_calls` | 000003 |
| `idx_api_calls_timestamp` | `api_calls` | 000003 |
| `idx_campaign_purchases_ebay_export_flagged_at` | `campaign_purchases` | 000003 |
| `idx_campaign_purchases_received_at` | `campaign_purchases` | 000003 |
| `idx_campaign_purchases_updated_at` | `campaign_purchases` | 000003 |
| `idx_purchases_dh_cert_status` | `campaign_purchases` | 000003 |
| `idx_purchases_gem_rate_id` | `campaign_purchases` | 000003 |
| `idx_purchases_mm_last_error` | `campaign_purchases` | 000021 |
| `idx_card_access_log_card_number` | `card_access_log` | 000003 |
| `idx_card_cache_demand_score` | `dh_card_cache` | 000003 |
| `idx_card_price_trajectory_card` | `card_price_trajectory` | 000003 |
| `idx_cl_sales_comps_gem_rate` | `cl_sales_comps` | 000003 |
| `idx_dh_suggestions_card` | `dh_suggestions` | 000003 |
| `idx_invoices_status` | `invoices` | 000003 |
| `idx_revocation_flags_segment` | `revocation_flags` | 000003 |
| `idx_revocation_flags_status` | `revocation_flags` | 000003 |
| `idx_sales_channel` | `campaign_sales` | 000003 |
| `idx_scoring_gaps_factor` | `scoring_data_gaps` | 000003 |
| `idx_user_sessions_user_id` | `user_sessions` | 000003 |
| `idx_user_tokens_expires_at` | `user_tokens` | 000003 |
| `idx_user_tokens_session_id` | `user_tokens` | 000003 |
| `idx_user_tokens_session_unique` (UNIQUE) | `user_tokens` | 000003 |
| `idx_user_tokens_user_id` | `user_tokens` | 000003 |
| `idx_users_google_id` | `users` | 000003 |

Four more indexes went away with the object they belonged to rather than by an explicit
`DROP INDEX`: `idx_sell_sheet_items_added_at` (created by 000003, dropped with its table
in 000007), `idx_advisor_cache_type` (dropped with its table in 000013), and
`idx_campaign_purchases_mm_value_cents` and `idx_mm_sales_comps_lookup` (removed by
000021 with their column and table).

One entry above has since been undone. `idx_user_tokens_session_unique` was not a
performance index — it enforced one-token-per-session, which is exactly what a UNIQUE
index looks like to an advisor scanning for unused indexes, since enforcement is not a
scan. Dropping it broke `StoreTokens`' `ON CONFLICT(session_id)` upsert on every call
(SQLSTATE 42P10, SLA-57). Migration 000031 restores the guarantee as the named
constraint `user_tokens_session_id_key`; see the `user_tokens` section above.

---

## FK Dependency Graph

```
users
├── user_sessions          (user_id → users.id CASCADE DELETE)
│   └── user_tokens        (session_id → user_sessions.id CASCADE DELETE)
├── user_tokens            (user_id → users.id CASCADE DELETE)
├── allowed_emails         (added_by → users.id SET NULL)
└── price_flags            (flagged_by → users.id, resolved_by → users.id)

api_rate_limits                (standalone; `price_refresh_queue` never existed in Postgres)

campaigns
└── campaign_purchases     (campaign_id → campaigns.id CASCADE DELETE)
    ├── campaign_sales     (purchase_id → campaign_purchases.id CASCADE DELETE)
    └── price_flags        (purchase_id → campaign_purchases.id CASCADE DELETE)

── Standalone tables (no FK dependencies) ──
api_calls
card_access_log
card_id_mappings
card_price_trajectory
sync_state
cashflow_config
invoices
revocation_flags
oauth_states
cardladder_config
cl_card_mappings
cl_sales_comps
market_intelligence
dh_suggestions
dh_state_events
dh_card_tombstones
dh_comp_cache
scheduler_run_stats
dh_push_config
dh_card_cache
dh_character_cache
psa_pending_items
psa_portal_token
psa_portal_snapshot
psa_portal_catalog
psa_campaign_snapshot
psa_campaign_push_queue
```
