-- Re-drop the dh_state_events lookup indexes, returning to the 000003 state.
--
-- Rolling back past this point also removes the /api/dh/events read path, so
-- the indexes go back to being unused.
DROP INDEX IF EXISTS idx_dh_state_events_purchase;
DROP INDEX IF EXISTS idx_dh_state_events_cert;
