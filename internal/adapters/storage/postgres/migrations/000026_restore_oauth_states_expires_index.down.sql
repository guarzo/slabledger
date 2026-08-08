-- Drop the restored expired-OAuth-state cleanup index, returning to the
-- post-000003 state.

DROP INDEX IF EXISTS idx_oauth_states_expires;
