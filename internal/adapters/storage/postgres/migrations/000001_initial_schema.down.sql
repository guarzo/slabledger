-- Drop the entire initial schema in reverse dependency order.
-- Idempotent: uses IF EXISTS / CASCADE so a partial state drops cleanly.

-- Trigger + function (drop before the table they reference is gone — but we
-- DROP TABLE with CASCADE below, which drops triggers anyway. Do function
-- explicitly since it is not table-owned.)
DROP TRIGGER IF EXISTS trg_advisor_cache_updated_at ON advisor_cache;
DROP FUNCTION IF EXISTS trg_advisor_cache_updated_at_fn();

-- Views (drop before the tables they depend on)
DROP VIEW IF EXISTS api_daily_summary CASCADE;
DROP VIEW IF EXISTS api_hourly_distribution CASCADE;
DROP VIEW IF EXISTS api_usage_summary CASCADE;
DROP VIEW IF EXISTS ai_usage_by_operation CASCADE;
DROP VIEW IF EXISTS ai_usage_summary CASCADE;
DROP VIEW IF EXISTS expired_sessions CASCADE;
DROP VIEW IF EXISTS active_sessions CASCADE;

-- Tables — dropped in reverse dependency order.
-- CASCADE ensures FKs, indexes, and dependent views are removed.
DROP TABLE IF EXISTS scheduler_run_stats CASCADE;
DROP TABLE IF EXISTS psa_pending_items CASCADE;
DROP TABLE IF EXISTS card_price_trajectory CASCADE;
DROP TABLE IF EXISTS dh_state_events CASCADE;
DROP TABLE IF EXISTS dh_character_cache CASCADE;
DROP TABLE IF EXISTS dh_card_cache CASCADE;
DROP TABLE IF EXISTS dh_push_config CASCADE;
DROP TABLE IF EXISTS sell_sheet_items CASCADE;
DROP TABLE IF EXISTS scoring_data_gaps CASCADE;
DROP TABLE IF EXISTS dh_suggestions CASCADE;
DROP TABLE IF EXISTS market_intelligence CASCADE;
DROP TABLE IF EXISTS mm_card_mappings CASCADE;
DROP TABLE IF EXISTS marketmovers_config CASCADE;
DROP TABLE IF EXISTS cl_sales_comps CASCADE;
DROP TABLE IF EXISTS cl_card_mappings CASCADE;
DROP TABLE IF EXISTS cardladder_config CASCADE;
DROP TABLE IF EXISTS api_rate_limits CASCADE;
DROP TABLE IF EXISTS api_calls CASCADE;
DROP TABLE IF EXISTS ai_calls CASCADE;
DROP TABLE IF EXISTS advisor_cache CASCADE;
DROP TABLE IF EXISTS sync_state CASCADE;
DROP TABLE IF EXISTS card_id_mappings CASCADE;
DROP TABLE IF EXISTS card_access_log CASCADE;
DROP TABLE IF EXISTS price_flags CASCADE;
DROP TABLE IF EXISTS revocation_flags CASCADE;
DROP TABLE IF EXISTS cashflow_config CASCADE;
DROP TABLE IF EXISTS invoices CASCADE;
DROP TABLE IF EXISTS campaign_sales CASCADE;
DROP TABLE IF EXISTS campaign_purchases CASCADE;
DROP TABLE IF EXISTS campaigns CASCADE;
DROP TABLE IF EXISTS allowed_emails CASCADE;
DROP TABLE IF EXISTS oauth_states CASCADE;
DROP TABLE IF EXISTS user_tokens CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Note: we do NOT drop the citext extension — it's shared with other schemas
-- and Supabase convention is to leave extensions installed.
