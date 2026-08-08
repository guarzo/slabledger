-- Restore the index supporting expired-OAuth-state cleanup.
-- 000001 created idx_oauth_states_expires; 000003 dropped it as an unused index
-- because nothing scanned oauth_states.expires_at at the time. The session
-- cleanup scheduler now sweeps expired states
-- (DELETE FROM oauth_states WHERE expires_at <= $1), so the index is live again.

CREATE INDEX IF NOT EXISTS idx_oauth_states_expires ON oauth_states(expires_at);
