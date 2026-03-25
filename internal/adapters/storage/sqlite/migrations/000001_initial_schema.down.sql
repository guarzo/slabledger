-- Drop all views
DROP VIEW IF EXISTS expired_sessions;
DROP VIEW IF EXISTS active_sessions;
DROP VIEW IF EXISTS api_daily_summary;
DROP VIEW IF EXISTS api_hourly_distribution;
DROP VIEW IF EXISTS api_usage_summary;
DROP VIEW IF EXISTS stale_prices;

-- Drop all tables (reverse dependency order)
DROP TABLE IF EXISTS revocation_flags;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS card_id_mappings;
DROP TABLE IF EXISTS cashflow_config;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS campaign_sales;
DROP TABLE IF EXISTS campaign_purchases;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS allowed_emails;
DROP TABLE IF EXISTS favorites;
DROP TABLE IF EXISTS oauth_states;
DROP TABLE IF EXISTS user_tokens;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS card_access_log;
DROP TABLE IF EXISTS price_refresh_queue;
DROP TABLE IF EXISTS api_rate_limits;
DROP TABLE IF EXISTS api_calls;
DROP TABLE IF EXISTS price_history;
