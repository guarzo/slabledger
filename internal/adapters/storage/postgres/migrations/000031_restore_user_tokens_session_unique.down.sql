-- Return user_tokens to the shape 000003 left it in: no unique constraint on
-- session_id, and the column nullable again.
--
-- This restores the schema, not the data. The up migration deletes duplicate
-- and NULL-session rows, and those are not recoverable here -- acceptable
-- because any row it removed predates 000003 and held tokens that expired long
-- ago. Rolling back also re-breaks StoreTokens (SQLSTATE 42P10 on every call),
-- which is the pre-000031 behavior by definition.

ALTER TABLE user_tokens DROP CONSTRAINT IF EXISTS user_tokens_session_id_key;

ALTER TABLE user_tokens ALTER COLUMN session_id DROP NOT NULL;
