package main

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

// Independent opt-in prevents concurrent `go test ./...` storage schema resets.
// Only the task-owned loopback disposable database may be rebuilt by this file.
func transitionDB(t *testing.T) *postgres.DB {
	t.Helper()
	raw := os.Getenv("SHOW_PREP_TRANSITION_TEST_URL")
	if raw == "" {
		t.Skip("SHOW_PREP_TRANSITION_TEST_URL unset; disposable transition DB tests not run")
	}
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.True(t, (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Host == "127.0.0.1:5434" && u.Path == "/slabledger_show_scan_test" && u.RawQuery == "sslmode=disable" && u.Fragment == "" && u.User != nil && u.User.Username() == "slabledger", "refusing non-dedicated database")
	require.Empty(t, os.Getenv("POSTGRES_TEST_URL"), "do not run alongside destructive adapter tests")
	ctx := context.Background()
	db, err := postgres.Open(ctx, raw, observability.NewNoopLogger())
	require.NoError(t, err)
	var name, owner string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT current_database(),pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Scan(&name, &owner))
	require.Equal(t, "slabledger_show_scan_test", name)
	require.Equal(t, "slabledger", owner)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, resetTransitionDB(db))
	return db
}
func resetTransitionDB(db *postgres.DB) error {
	if _, err := db.ExecContext(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		return err
	}
	return postgres.RunMigrations(db, "") // Fixture only; never called by the tool.
}

func seedTransition(t *testing.T, db *postgres.DB, empty bool) sp.ListDetail {
	t.Helper()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO campaigns(id,name,phase) VALUES($1,'Fixture campaign','active');`, commandID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,received_at,reviewed_price_cents,reviewed_at,dh_listing_price_cents,gem_rate_id) VALUES($1,$2,'Card','12345678','PSA',10,'2026-09-01','2026-09-02',12345,'2026-09-03',23456,'profile-1')`, purchaseID, commandID)
	require.NoError(t, err)
	store := postgres.NewShowPrepStore(db.DB)
	_, err = store.CreateList(ctx, listID, "Fixture show")
	require.NoError(t, err)
	if !empty {
		_, err = db.ExecContext(ctx, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,packed_at,version,acknowledged_price_cents,acknowledged_status) VALUES($1,$2,$3,'Card','12345678','PSA',10,'2026-09-04','2026-09-05',1,9999,'supported')`, itemID, listID, purchaseID)
		require.NoError(t, err)
	}
	l, err := store.GetList(ctx, listID)
	require.NoError(t, err)
	items, err := store.GetItems(ctx, listID)
	require.NoError(t, err)
	return sp.ListDetail{List: l, Items: items, StateFingerprint: sp.ListStateFingerprint(l, items)}
}

// Full row JSON for every base table detects writes outside intended show state,
// including money, receipt, DH authorization, evidence/holds and migrations.
func databaseState(t *testing.T, db *postgres.DB, excludeShow bool) map[string]string {
	t.Helper()
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	require.NoError(t, err)
	names := []string{}
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		if !excludeShow || !strings.HasPrefix(n, "showprep_") {
			names = append(names, n)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	out := map[string]string{}
	for _, n := range names {
		var s string
		// Identifiers originate from pg_tables, not a CLI input.
		quoted := `"` + strings.ReplaceAll(n, `"`, `""`) + `"`
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(x ORDER BY x::text),'[]'::jsonb)::text FROM (SELECT to_jsonb(t) AS x FROM `+quoted+` t) rows`).Scan(&s))
		out[n] = s
	}
	return out
}

func TestPostgresTransition(t *testing.T) {
	db := transitionDB(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name, operation string
		empty, apply    bool
		change          string
		wantErr         bool
	}{
		{"read-only dry-run", "confirm-existing", false, false, "", false},
		{"read-only collision does not observe", "confirm-existing", false, false, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,grade_value,purchase_date,gem_rate_id) SELECT '22222222-2222-4222-8222-222222222222',campaign_id,card_name,cert_number,'BGS',grade_value,purchase_date,gem_rate_id FROM campaign_purchases WHERE id='11111111-1111-4111-8111-111111111111'`, false},
		{"changed membership dry-run", "confirm-existing", false, false, "DELETE FROM showprep_items", true},
		{"changed membership apply", "confirm-existing", false, true, "DELETE FROM showprep_items", true},
		{"invalid member dry-run", "confirm-existing", false, false, "UPDATE campaign_purchases SET received_at=NULL", true},
		{"invalid member apply", "confirm-existing", false, true, "UPDATE campaign_purchases SET received_at=NULL", true},
		{"delete gained member", "delete-empty", true, true, `INSERT INTO showprep_items(id,list_id,purchase_id,card_name,cert_number,grader,grade,added_at,version,acknowledged_price_cents,acknowledged_status) VALUES('44444444-4444-4444-8444-444444444444','33333333-3333-4333-8333-333333333333','11111111-1111-4111-8111-111111111111','Card','12345678','PSA',10,'2026-09-06',1,0,'')`, true},
		{"empty delete dry-run", "delete-empty", true, false, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, resetTransitionDB(db))
			d := seedTransition(t, db, tc.empty)
			if tc.change != "" {
				_, err := db.ExecContext(ctx, tc.change)
				require.NoError(t, err)
			}
			before := databaseState(t, db, false)
			readDB := db
			if !tc.apply {
				var err error
				readDB, err = postgres.Open(ctx, os.Getenv("SHOW_PREP_TRANSITION_TEST_URL")+"&options=-c%20default_transaction_read_only%3Don", observability.NewNoopLogger())
				require.NoError(t, err)
				defer func() { _ = readDB.Close() }()
			}
			store := postgres.NewShowPrepStore(readDB.DB)
			var out bytes.Buffer
			err := execute(ctx, options{operation: tc.operation, commandID: commandID, apply: tc.apply}, d, store, sp.NewService(store, nil, nil), &out)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, before, databaseState(t, db, false), "dry-run/rejection changed persisted rows")
		})
	}
}

func TestPostgresUncertainResponseReplay(t *testing.T) {
	db := transitionDB(t)
	ctx := context.Background()
	for _, tc := range []struct{ operation, purchase string }{
		{"confirm-existing", purchaseID}, {"confirm-existing", "legacy-card-1"}, {"delete-empty", purchaseID},
	} {
		t.Run(tc.operation+"/"+tc.purchase, func(t *testing.T) {
			operation := tc.operation
			require.NoError(t, resetTransitionDB(db))
			d := seedTransition(t, db, operation == "delete-empty")
			if tc.purchase != purchaseID {
				_, err := db.ExecContext(ctx, `DELETE FROM showprep_items`)
				require.NoError(t, err)
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET id=$1 WHERE id=$2`, tc.purchase, purchaseID)
				require.NoError(t, err)
				d.Items[0].PurchaseID = tc.purchase
				store := postgres.NewShowPrepStore(db.DB)
				require.NoError(t, store.Within(ctx, func(tx sp.Session) error { return tx.InsertItem(ctx, listID, d.Items[0]) }))
			}
			if operation == "confirm-existing" {
				_, err := db.ExecContext(ctx, `UPDATE showprep_items SET last_command=$1 WHERE id=$2`, sp.Fingerprint("legacy update"), itemID)
				require.NoError(t, err)
			}
			path := filepath.Join(t.TempDir(), "exported.json")
			beforeExport := databaseState(t, db, false)
			t.Setenv("DATABASE_URL", os.Getenv("SHOW_PREP_TRANSITION_TEST_URL")+"&options=-c%20default_transaction_read_only%3Don")
			var exported bytes.Buffer
			require.NoError(t, run(ctx, []string{"-operation", "export", "-list-id", listID, "-snapshot", path}, &exported))
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
			captured, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, beforeExport, databaseState(t, db, false), "export changed database rows")
			loaded, err := loadSnapshot(path, operation)
			require.NoError(t, err)
			if operation == "confirm-existing" {
				require.Equal(t, tc.purchase, loaded.Items[0].PurchaseID)
				require.Equal(t, sp.Fingerprint("legacy update"), loaded.Items[0].LastCommand)
			}
			args := []string{"-operation", operation, "-snapshot", path, "-command-id", commandID}
			var dry bytes.Buffer
			require.NoError(t, run(ctx, args, &dry))
			require.Contains(t, dry.String(), `"mode":"dry_run"`)
			require.Equal(t, beforeExport, databaseState(t, db, false), "dry-run changed database rows")
			t.Setenv("DATABASE_URL", os.Getenv("SHOW_PREP_TRANSITION_TEST_URL"))
			before := databaseState(t, db, true)
			// /dev/full fails the output AFTER the domain transaction commits.
			full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
			require.NoError(t, err)
			defer func() { _ = full.Close() }()
			args = append(args, "-apply")
			err = run(ctx, args, full)
			require.ErrorContains(t, err, "may have committed")
			require.Equal(t, before, databaseState(t, db, true), "transition changed financial/migration rows")
			store := postgres.NewShowPrepStore(db.DB)
			if operation == "confirm-existing" {
				items, err := store.GetItems(ctx, listID)
				require.NoError(t, err)
				require.Len(t, items, 1)
				require.Equal(t, itemID, items[0].ID)
				require.Equal(t, tc.purchase, items[0].PurchaseID)
				require.Equal(t, d.Items[0].AddedAt, items[0].AddedAt)
				require.Equal(t, d.Items[0].PackedAt, items[0].PackedAt)
				require.Equal(t, 12345, items[0].AcknowledgedPriceCents)
				require.Equal(t, sp.NeedsReview, items[0].AcknowledgedStatus)
				require.Equal(t, int64(2), items[0].Version)
				l, err := store.GetList(ctx, listID)
				require.NoError(t, err)
				require.Equal(t, d.List.CreatedAt, l.CreatedAt)
				require.Equal(t, d.List.WithdrawalGeneration, l.WithdrawalGeneration)
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET reviewed_price_cents=45678,reviewed_at=now()`)
				require.NoError(t, err)
			} else {
				_, err := store.GetList(ctx, listID)
				require.ErrorIs(t, err, sp.ErrNotFound)
			}
			completed := databaseState(t, db, false)
			var out bytes.Buffer
			require.NoError(t, run(ctx, args, &out))
			require.Contains(t, out.String(), `"replay":true`)
			require.Equal(t, completed, databaseState(t, db, false), "replay mutated current baseline/lifetime")
			afterReplay, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, captured, afterReplay, "captured snapshot changed during apply/replay")
			args[5] = "66666666-6666-4666-8666-666666666666"
			require.Error(t, run(ctx, args, &out))
			require.Equal(t, completed, databaseState(t, db, false))
		})
	}
}

func TestPostgresMissingSchema(t *testing.T) {
	db := transitionDB(t)
	ctx := context.Background()
	d := seedTransition(t, db, false)
	path := snapshotFile(t, d)
	_, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	require.NoError(t, err)
	t.Setenv("DATABASE_URL", os.Getenv("SHOW_PREP_TRANSITION_TEST_URL"))
	before := databaseState(t, db, false)
	for _, apply := range []bool{false, true} {
		t.Run(map[bool]string{false: "dry-run", true: "apply"}[apply], func(t *testing.T) {
			args := []string{"-operation", "confirm-existing", "-snapshot", path, "-command-id", commandID}
			if apply {
				args = append(args, "-apply")
			}
			var out bytes.Buffer
			timed, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			err := run(timed, args, &out)
			require.ErrorContains(t, err, "verify schema/connectivity")
			require.Equal(t, before, databaseState(t, db, false), "tool auto-migrated missing schema")
		})
	}
	// Leave the disposable DB usable for the next independently invoked suite.
	require.NoError(t, resetTransitionDB(db))
}

func TestRunSecureInputs(t *testing.T) {
	d := fixture()
	path := snapshotFile(t, d)
	for _, tc := range []struct{ name, url string }{
		{"missing environment", ""}, {"invalid secret URL", "postgres://secret-token@%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tc.url)
			var out bytes.Buffer
			err := run(context.Background(), []string{"-operation", "confirm-existing", "-snapshot", path, "-command-id", commandID}, &out)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-token")
			require.Empty(t, out.String())
		})
	}
}
