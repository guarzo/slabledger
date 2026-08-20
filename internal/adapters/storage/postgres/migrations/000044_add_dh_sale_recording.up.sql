-- DH sale recording (see docs/specs/2026-08-20-dh-record-sale-design.md, §5).
-- Replaces the broken PATCH {"status":"sold"} transition -- DH's inventory
-- vocabulary only ever accepted in_stock/listed, so every sale-side write to
-- dh_status='sold' has 422'd since the feature shipped (see the design doc's
-- Problem section for the 2026-08-19 incident this repairs). DH now offers a
-- purpose-built POST .../inventory/:id/sale endpoint that requires a client
-- idempotency key; these columns are where that key and its outcome live.
--
-- campaign_sales.dh_idempotency_key / dh_sale_id use the same TEXT NOT NULL
-- DEFAULT '' convention as every other provenance column added since
-- migration 000040 (price_source) and 000041 (cl_value_at_*_source), and for
-- the identical reason: every predicate that reads these columns (the §5b
-- recovery query, the mint-on-first-need compare-and-set) tests `= ''` or
-- `<> ''`. A nullable column would make those predicates silently skip every
-- pre-existing row instead of treating it as "not yet recorded" -- the two
-- are operationally different (a NULL needs a NULL-aware predicate everywhere
-- it is read; forgetting one spot is invisible until it is not) even though
-- both mean "no value yet." '' is also the only encoding here that a legacy
-- (pre-migration) binary's INSERT reproduces for free: it omits the column,
-- Postgres applies the DEFAULT, and the row lands exactly where a
-- migration-aware backfill would have put it, so no rollback-window trigger
-- is needed the way 000022's campaign_sales_derive_reason_trg was for
-- sale_reason. dh_sale_recorded_at stays a nullable TIMESTAMP: it answers
-- "when," and unlike the two TEXT columns there is no ambiguity to disambiguate
-- with a sentinel -- NULL already means "never recorded" unambiguously.
--
-- campaign_purchases.dh_sale_conflict follows the same reasoning: it is the
-- §5b recovery pass's terminal-state marker (`dh_sale_conflict = ''` is the
-- predicate that stops a permanently-failed sale from being retried forever),
-- so it must be NOT NULL DEFAULT '' for the same reason as the sale-side
-- columns. dh_sale_conflict_at is nullable TIMESTAMP for the same "when" reason.
--
-- NO BACKFILL of dh_idempotency_key. Every sale row that predates this
-- migration -- including the 25 stranded in the 2026-08-15 incident -- was
-- inserted before this column existed (sale_store.go:27), so there is no
-- correct key to backfill: a key is only correct if it was persisted before
-- the DH call that used it (§5a/§5b), and no historical DH call exists for
-- these rows to have used one. Minting one now, in this migration, would
-- create a key that no DH request has ever carried -- indistinguishable later
-- from a key that really was used, and therefore just as dangerous as the
-- guessed-value backfill migration 000041 rejected for cl_value_at_purchase.
-- Legacy rows are onboarded lazily instead, by the compare-and-set in §5a,
-- the first time a writer needs a key for that row.
ALTER TABLE campaign_sales
    ADD COLUMN dh_idempotency_key   TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_id           TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_recorded_at  TIMESTAMP;

ALTER TABLE campaign_purchases
    ADD COLUMN dh_sale_conflict     TEXT NOT NULL DEFAULT '',
    ADD COLUMN dh_sale_conflict_at  TIMESTAMP;

-- Partial index for the §5b recovery pass (ListSalesNeedingDHRecord), which
-- runs every reconciler cycle filtering on dh_sale_id = ''. That predicate
-- matches the column default, so it is not selective until backfill (the
-- lazy-onboarding compare-and-set in §5a) has worked through the backlog --
-- a full-table scan every tick until then. No CONCURRENTLY: golang-migrate
-- wraps each migration in a transaction (see 000039_drop_redundant_indexes).
CREATE INDEX IF NOT EXISTS idx_campaign_sales_needing_dh_record
    ON campaign_sales (id) WHERE dh_sale_id = '';
