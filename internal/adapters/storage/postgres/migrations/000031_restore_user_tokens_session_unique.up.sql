-- Restore the one-token-per-session guarantee that StoreTokens' upsert depends on.
--
-- 000001 created idx_user_tokens_session_unique, a bare UNIQUE index on
-- user_tokens(session_id). 000003 dropped it as part of a sweep of "never
-- scanned" indexes -- which is exactly what a UNIQUE index enforcing a
-- constraint looks like to an index advisor, since enforcement is not a scan.
--
-- AuthRepository.StoreTokens upserts with ON CONFLICT(session_id). Postgres
-- resolves that inference target against a unique index or constraint and
-- rejects the whole statement with SQLSTATE 42P10 when neither exists, before
-- touching a row. So every call has failed since 000003 shipped, and the sole
-- caller (handlers/auth.go) logs a warning and continues -- leaving the table
-- silently unwritten for the entire life of the Postgres schema (SLA-57).
--
-- Restored as a named constraint rather than a bare index so the intent is
-- visible in \d and an advisor cannot recommend dropping it a second time.

-- Any surviving row necessarily predates 000003, because StoreTokens has been
-- unable to insert since. Its tokens expired long ago, so collapsing duplicates
-- loses nothing of value -- and skipping this would abort the ALTER below on
-- precisely the deployed databases this migration exists to repair.

-- NULL session_id rows can never satisfy the upsert's conflict target and would
-- otherwise block the NOT NULL below.
DELETE FROM user_tokens WHERE session_id IS NULL;

-- Keep the newest row per session. COALESCE guards a NULL updated_at (the
-- column is nullable, and a NULL comparison would leave both rows in place and
-- fail the ALTER); the id tiebreak makes the ordering total, so exactly one row
-- per session_id survives.
DELETE FROM user_tokens a
USING user_tokens b
WHERE a.session_id = b.session_id
  AND (COALESCE(a.updated_at, 'epoch'::timestamp), a.id)
    < (COALESCE(b.updated_at, 'epoch'::timestamp), b.id);

-- NOT NULL is load-bearing, not tidiness: a UNIQUE constraint permits unlimited
-- NULLs, so NULL-session rows would bypass the conflict target and accumulate
-- silently -- the same invisible-failure shape as the bug being fixed.
ALTER TABLE user_tokens ALTER COLUMN session_id SET NOT NULL;

ALTER TABLE user_tokens ADD CONSTRAINT user_tokens_session_id_key UNIQUE (session_id);
