package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func requireLegacyOutcomeConstraint(t *testing.T, err error) {
	t.Helper()
	var constraint *pgconn.PgError
	require.ErrorAs(t, err, &constraint)
	require.Equal(t, "23514", constraint.Code)
	require.Equal(t, "dh_mutation_attempts_outcome_check", constraint.ConstraintName)
}

// Removing the new outcome or coercing history on down must fail these real-SQL tests.
func TestDHLegacyPriceAbandonmentMigration(t *testing.T) {
	for _, abandoned := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty history permits down", true: "abandoned history forbids down"}[abandoned], func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "legacy-migration-cert", 364577, "")
			ctx := context.Background()
			a := preparePriceAttempt(t, store, id, livePriceRequest())
			if abandoned {
				_, err := db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp(),receipt='administrative evidence' WHERE id=$1`, a.ID)
				require.NoError(t, err, "migration must admit abandoned without changing journal identity")
			}
			down, err := MigrationsFS.ReadFile("migrations/000051_dh_legacy_price_abandonment.down.sql")
			require.NoError(t, err)
			if abandoned {
				// No surrounding transaction: the down statement itself must fail
				// atomically without dropping the current constraint or history.
				_, err = db.ExecContext(ctx, string(down))
				requireLegacyOutcomeConstraint(t, err)
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET outcome='invalid' WHERE id=$1`, a.ID)
				requireLegacyOutcomeConstraint(t, err)
			} else {
				tx, err := db.BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = tx.Rollback() }()
				_, err = tx.ExecContext(ctx, string(down))
				require.NoError(t, err)
				_, err = tx.ExecContext(ctx, `UPDATE dh_mutation_attempts SET outcome='abandoned',settled_at=clock_timestamp() WHERE id=$1`, a.ID)
				requireLegacyOutcomeConstraint(t, err)
				require.NoError(t, tx.Rollback())
			}
			persisted, err := scanAttempt(db.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM dh_mutation_attempts WHERE id=$1`, a.ID))
			require.NoError(t, err)
			require.NotNil(t, persisted)
			require.Equal(t, a.PayloadIdentity, persisted.PayloadIdentity)
			require.Equal(t, a.Key, persisted.Key)
			if abandoned {
				require.Equal(t, "abandoned", persisted.Outcome)
				var receipt string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT receipt FROM dh_mutation_attempts WHERE id=$1`, a.ID).Scan(&receipt))
				require.Equal(t, "administrative evidence", receipt)
			} else {
				require.Equal(t, "open", persisted.Outcome)
			}
		})
	}
}
