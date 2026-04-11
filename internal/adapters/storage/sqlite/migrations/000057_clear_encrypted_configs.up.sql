-- Clear tables containing data encrypted with the previous encryption key.
-- After this migration, users must re-authenticate and re-configure:
--   - Card Ladder (POST /api/admin/cardladder/config)
--   - Market Movers (POST /api/admin/marketmovers/config)
--   - Instagram (re-connect via OAuth)
--   - User sessions (re-login via Google OAuth)

DELETE FROM cardladder_config;
DELETE FROM marketmovers_config;
DELETE FROM instagram_config;
DELETE FROM user_tokens;
DELETE FROM user_sessions;
