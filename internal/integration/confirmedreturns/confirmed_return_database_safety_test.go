//go:build integration

package confirmedreturns_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// This suite resets its fixtures; never share the storage-test or application DB.
func openConfirmedReturnsTestDB(t *testing.T, ctx context.Context) *postgres.DB {
	t.Helper()
	dsn := os.Getenv("CONFIRMED_RETURNS_TEST_URL")
	if dsn == "" {
		t.Skip("CONFIRMED_RETURNS_TEST_URL not set")
	}
	db, err := postgres.Open(ctx, dsn, mocks.NewMockLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var name string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name))
	require.NoError(t, validateConfirmedReturnsDatabaseName(name))
	return db
}

func validateConfirmedReturnsDatabaseName(name string) error {
	if name != "slabledger_confirmed_returns_test" {
		return fmt.Errorf("refusing confirmed-return fixtures on database %q: requires slabledger_confirmed_returns_test", name)
	}
	return nil
}

func TestConfirmedReturnsDatabaseNameGuard(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed bool
	}{
		{name: "slabledger_confirmed_returns_test", allowed: true},
		{name: "slabledger_test"},
		{name: "slabledger"},
		{name: "slabledger_confirmed_returns_test_copy"},
		{name: "SLABLEDGER_CONFIRMED_RETURNS_TEST"},
		{name: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfirmedReturnsDatabaseName(tc.name)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestConfirmedReturnsDedicatedOptInUnset(t *testing.T) {
	for _, tc := range []struct {
		name, storageURL, applicationURL string
	}{
		{name: "no URLs"},
		{name: "storage URL is not an opt-in", storageURL: "not-a-database-connection"},
		{name: "application URL is not an opt-in", applicationURL: "not-a-database-connection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CONFIRMED_RETURNS_TEST_URL", "")
			t.Setenv("POSTGRES_TEST_URL", tc.storageURL)
			t.Setenv("DATABASE_URL", tc.applicationURL)
			skipped := false
			ok := t.Run("setup skips before opening a database", func(t *testing.T) {
				defer func() { skipped = t.Skipped() }()
				setupIntegrationReturn(t, "unused-cert")
			})
			require.True(t, ok)
			require.True(t, skipped, "missing dedicated opt-in must skip setup")
		})
	}
}
