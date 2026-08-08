package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userTokensSessionUnique is the constraint 000031 adds. 000001 supplied the
// same guarantee as a bare UNIQUE index (idx_user_tokens_session_unique);
// 000003 dropped it in a "never scanned" sweep, which a UNIQUE index enforcing
// a constraint will always look like. 000031 restores it as a named constraint
// so the intent is visible in \d and an index advisor cannot recommend it away
// a second time (SLA-57).
const userTokensSessionUnique = "user_tokens_session_id_key"

// TestMigration000031_StoreTokensUpsertsBySession is the regression test for
// SLA-57 and the reason this file exists.
//
// StoreTokens upserts with ON CONFLICT(session_id). Postgres resolves that
// inference target against a unique index or constraint; with none present it
// rejects the statement outright with SQLSTATE 42P10, before touching a row.
// Between 000003 and 000031 that was every call, and the sole caller
// (handlers/auth.go) logs and continues — so the table was simply never
// written and nothing surfaced.
//
// The in-memory mock cannot catch this class of defect: it models the upsert in
// Go and has no notion of an inference target. Only real Postgres does.
func TestMigration000031_StoreTokensUpsertsBySession(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	encryptor, err := crypto.NewAESEncryptor("sla57-test-key-with-at-least-32-bytes")
	require.NoError(t, err, "build test encryptor")
	repo := NewAuthRepository(db.DB, encryptor)

	userID := seedUser(ctx, t, db, "google-sla57")
	const sessionID = "plaintext-session-sla57"
	require.NoError(t, repo.CreateSession(ctx, &auth.Session{
		ID:             sessionID,
		UserID:         userID,
		ExpiresAt:      time.Now().Add(time.Hour),
		CreatedAt:      time.Now(),
		LastAccessedAt: time.Now(),
	}), "seed session")

	first := &auth.UserTokens{
		AccessToken:  "access-first",
		RefreshToken: "refresh-first",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		Scope:        "scope-first",
	}
	require.NoError(t, repo.StoreTokens(ctx, userID, sessionID, first),
		"first StoreTokens should insert")

	// The second call is the one that failed for the whole life of the
	// Postgres schema. It must update in place, not error and not duplicate.
	second := &auth.UserTokens{
		AccessToken:  "access-second",
		RefreshToken: "refresh-second",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second),
		Scope:        "scope-second",
	}
	require.NoError(t, repo.StoreTokens(ctx, userID, sessionID, second),
		"second StoreTokens for the same session should upsert, not fail with 42P10")

	var rows int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM user_tokens WHERE session_id = $1`,
		hashSessionID(sessionID)).Scan(&rows), "count token rows")
	assert.Equal(t, 1, rows, "the upsert should leave exactly one row per session")

	var encAccess, encRefresh, scope string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT access_token, refresh_token, scope FROM user_tokens WHERE session_id = $1`,
		hashSessionID(sessionID)).Scan(&encAccess, &encRefresh, &scope), "read stored token row")

	// Decrypt rather than compare ciphertext: AES-GCM uses a fresh nonce per
	// call, so the same plaintext encrypts differently every time.
	gotAccess, err := encryptor.Decrypt(encAccess)
	require.NoError(t, err, "decrypt stored access token")
	gotRefresh, err := encryptor.Decrypt(encRefresh)
	require.NoError(t, err, "decrypt stored refresh token")

	assert.Equal(t, "access-second", gotAccess, "row should hold the second call's access token")
	assert.Equal(t, "refresh-second", gotRefresh, "row should hold the second call's refresh token")
	assert.Equal(t, "scope-second", scope, "row should hold the second call's scope")

	// Tokens must never be readable from the raw column.
	assert.NotEqual(t, "access-second", encAccess, "access token must be stored encrypted")
	assert.NotEqual(t, "refresh-second", encRefresh, "refresh token must be stored encrypted")
}

// TestMigration000031_SessionIDConstraints pins the schema shape the upsert
// depends on. Asserted against pg_constraint rather than by attempting a
// duplicate insert, so a failure names the missing guarantee directly.
func TestMigration000031_SessionIDConstraints(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	assert.True(t, uniqueConstraintColumns(ctx, t, db, "user_tokens", userTokensSessionUnique) != nil,
		"%s should exist after migration 000031", userTokensSessionUnique)
	assert.Equal(t, []string{"session_id"},
		uniqueConstraintColumns(ctx, t, db, "user_tokens", userTokensSessionUnique),
		"%s should cover exactly session_id", userTokensSessionUnique)

	// NOT NULL matters because a UNIQUE constraint permits unlimited NULLs:
	// without it, NULL-session rows would bypass the conflict target and
	// accumulate silently — the same invisible-failure shape as the original bug.
	var nullable string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'user_tokens'
		  AND column_name = 'session_id'`).Scan(&nullable), "read session_id nullability")
	assert.Equal(t, "NO", nullable, "user_tokens.session_id should be NOT NULL")
}

// TestMigration000031_DedupesPreExistingRows covers the upgrade path on a
// database that already holds rows.
//
// Any such row predates 000003 — StoreTokens has been unable to insert since —
// so its tokens are long expired and only the newest per session is worth
// keeping. Without the dedupe the ALTER would abort and the migration would
// fail on exactly the deployed databases it is meant to repair.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000031_DedupesPreExistingRows(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 30), "roll back to version 30")

	userID := seedUser(ctx, t, db, "google-sla57-dedupe")
	sessionID := "session-with-duplicates"
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_sessions (id, user_id, expires_at)
		VALUES ($1, $2, $3)`, sessionID, userID, time.Now().Add(time.Hour))
	require.NoError(t, err, "seed session for duplicate rows")

	// Two rows for one session, plus a NOT NULL violator, all legal at v30.
	insertLegacyToken(ctx, t, db, userID, &sessionID, "older", time.Now().Add(-2*time.Hour))
	insertLegacyToken(ctx, t, db, userID, &sessionID, "newer", time.Now().Add(-time.Hour))
	insertLegacyToken(ctx, t, db, userID, nil, "orphan-null", time.Now().Add(-3*time.Hour))

	require.NoError(t, migrateToVersion(db, 31),
		"000031 must survive duplicate and NULL session_id rows rather than aborting")

	var surviving int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM user_tokens WHERE session_id = $1`, sessionID).Scan(&surviving),
		"count surviving rows")
	assert.Equal(t, 1, surviving, "dedupe should keep exactly one row per session_id")

	var kept string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT access_token FROM user_tokens WHERE session_id = $1`, sessionID).Scan(&kept),
		"read surviving row")
	assert.Equal(t, "newer", kept, "dedupe should keep the most recently updated row")

	var nulls int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM user_tokens WHERE session_id IS NULL`).Scan(&nulls),
		"count NULL session rows")
	assert.Zero(t, nulls, "NULL session_id rows should be removed before the NOT NULL is applied")
}

// TestMigration000031_DownRestoresPost000003State proves the rollback returns
// the table to the shape 000003 left it in — no unique constraint, nullable
// session_id — rather than to 000001's.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000031_DownRestoresPost000003State(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 30), "roll back to version 30")

	assert.Nil(t, uniqueConstraintColumns(ctx, t, db, "user_tokens", userTokensSessionUnique),
		"down migration should drop %s", userTokensSessionUnique)

	var nullable string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'user_tokens'
		  AND column_name = 'session_id'`).Scan(&nullable), "read session_id nullability at v30")
	assert.Equal(t, "YES", nullable, "down migration should restore session_id to nullable")

	// Re-applying proves the rollback is not one-way.
	require.NoError(t, migrateToVersion(db, 31), "re-apply version 31")
	assert.Equal(t, []string{"session_id"},
		uniqueConstraintColumns(ctx, t, db, "user_tokens", userTokensSessionUnique),
		"%s should return after re-applying 000031", userTokensSessionUnique)
}

// uniqueConstraintColumns returns the columns covered by a named UNIQUE
// constraint, in ordinal order, or nil when no such constraint exists.
func uniqueConstraintColumns(ctx context.Context, t *testing.T, db *DB, table, constraint string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT a.attname
		FROM pg_constraint c
		JOIN pg_class      r ON r.oid = c.conrelid
		JOIN pg_namespace  n ON n.oid = r.relnamespace
		JOIN unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON TRUE
		JOIN pg_attribute  a ON a.attrelid = r.oid AND a.attnum = k.attnum
		WHERE n.nspname = 'public' AND r.relname = $1
		  AND c.conname = $2 AND c.contype = 'u'
		ORDER BY k.ord`, table, constraint)
	require.NoError(t, err, "query unique constraint %s on %s", constraint, table)
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	require.NoError(t, rows.Err())
	return columns
}

// seedUser inserts a user and returns its id.
func seedUser(ctx context.Context, t *testing.T, db *DB, googleID string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (google_id, username, email)
		VALUES ($1, $1, $1 || '@example.test')
		RETURNING id`, googleID).Scan(&id), "seed user %s", googleID)
	return id
}

// insertLegacyToken writes a user_tokens row directly, bypassing StoreTokens so
// it can create the pre-000031 states (duplicate and NULL session_id) that the
// repository itself can no longer produce.
func insertLegacyToken(ctx context.Context, t *testing.T, db *DB, userID int64, sessionID *string, marker string, updatedAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_tokens (user_id, session_id, access_token, refresh_token, token_type, expires_at, scope, created_at, updated_at)
		VALUES ($1, $2, $3, $3, 'Bearer', $4, 'legacy', $5, $5)`,
		userID, sessionID, marker, time.Now().Add(time.Hour), updatedAt)
	require.NoError(t, err, "insert legacy token row %s", marker)
}
