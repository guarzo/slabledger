# Phase 6b — CI/CD Gating Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent broken code from reaching `main` (and therefore Fly auto-deploy) by requiring green CI, actually running Postgres-backed tests in CI, and giving the operator a short rollback runbook.

**Architecture:** Three independent, revertible changes. (1) GitHub branch protection on `main` blocks direct pushes and requires the two existing status checks (`ci`, `build`) to pass before merge. (2) CI job gets a Postgres 17 service attached + a new migration up/down/up roundtrip test so the store tests that previously skipped now actually run. (3) A short `docs/OPERATIONS.md` runbook documents how to roll back a bad deploy via `flyctl releases` + `flyctl deploy --image`.

**Tech Stack:** GitHub Actions + Postgres 17 service container, `golang-migrate/migrate/v4`, `jackc/pgx/v5/stdlib`, `gh api` (GitHub REST API for branch protection), markdown.

**Spec:** `docs/specs/2026-04-20-phase6b-cicd-gating-design.md`

---

## File Structure

### New files
- `.github/workflows/test.yml` — modified, not new, but documented here because it's the central CI definition.
- `internal/adapters/storage/postgres/migrations_test.go` — single test: `TestMigrations_UpDownUpRoundtrip`.
- `docs/OPERATIONS.md` — rollback runbook.

### Modified files
- `.github/workflows/test.yml` — add `services.postgres` block and `POSTGRES_TEST_URL` env on the test step.

### External configuration (no file change in the repo)
- GitHub branch protection on `main` — applied via `gh api --method PUT`. Documented in Task 1.

---

## Section A — Migration roundtrip test and CI wiring

The roundtrip test is written and passes locally first. Once the test file exists and the Postgres service is attached to the CI job, CI will both run it and run the previously-skipped Postgres store tests (`campaign_store_test.go`, `cl_sales_store_test.go`, `metrics_test.go` already runs since it's pure Go).

### Task 1: Add the migrations up/down/up roundtrip test

**Files:**
- Create: `internal/adapters/storage/postgres/migrations_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/storage/postgres/migrations_test.go`:

```go
package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrations_UpDownUpRoundtrip exercises the full migration cycle against a
// real Postgres instance: fresh schema → Up → Down → Up. Each step must succeed
// and leave the DB in the expected shape. Catches broken SQL in either
// direction, missing DROPs in down migrations, and type mismatches between up
// and down.
//
// This test explicitly drops and recreates the public schema at the start to
// guarantee a known baseline. It does NOT use t.Parallel — its schema-drop
// would interfere with any parallel test that expects state.
func TestMigrations_UpDownUpRoundtrip(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		url = "postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable"
	}

	logger := mocks.NewMockLogger()
	db, err := Open(url, logger)
	if err != nil {
		t.Skipf("Postgres not reachable at %q: %v (set POSTGRES_TEST_URL to override)", url, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	// Guaranteed-empty baseline.
	_, err = db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	require.NoError(t, err, "reset public schema")

	// UP #1
	require.NoError(t, RunMigrations(db, ""), "first up migration")
	maxVersion := countEmbeddedMigrations(t)
	version := currentMigrationVersion(t, db)
	assert.Equal(t, maxVersion, version, "after first Up, version should equal max embedded migration")
	assert.Positive(t, countPublicTables(t, db), "after Up, public schema should have at least one user table")

	// DOWN
	require.NoError(t, runMigrationsDown(db), "down migration")

	// After Down, only schema_migrations (golang-migrate's tracking table) should
	// remain in the public schema. All application tables must be dropped.
	remaining := listPublicTables(t, db)
	for _, table := range remaining {
		assert.Equal(t, "schema_migrations", table,
			"unexpected table remaining after Down: %s", table)
	}

	// UP #2 — proves Down didn't leave the DB in a state that blocks a re-up
	require.NoError(t, RunMigrations(db, ""), "second up migration")
	version = currentMigrationVersion(t, db)
	assert.Equal(t, maxVersion, version, "after second Up, version should equal max embedded migration")
	assert.Positive(t, countPublicTables(t, db), "after second Up, public schema should have user tables again")
}

// runMigrationsDown applies all down migrations until the DB is at version 0.
// Mirrors RunMigrations' setup but calls Down() instead of Up().
func runMigrationsDown(db *DB) error {
	driver, err := migratepgx.WithInstance(db.DB, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	iofsDriver, err := iofs.New(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", iofsDriver, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create migration instance: %w", err)
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("down: %w", err)
	}
	return nil
}

// countEmbeddedMigrations returns the highest migration number in MigrationsFS,
// by counting *.up.sql files.
func countEmbeddedMigrations(t *testing.T) uint {
	t.Helper()
	entries, err := MigrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	var count uint
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			count++
		}
	}
	return count
}

// currentMigrationVersion returns the migration version recorded by golang-migrate.
func currentMigrationVersion(t *testing.T, db *DB) uint {
	t.Helper()
	var version uint
	var dirty bool
	err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty)
	require.NoError(t, err, "read schema_migrations")
	require.False(t, dirty, "schema_migrations.dirty should be false")
	return version
}

// countPublicTables returns the number of BASE TABLE rows in the public schema.
func countPublicTables(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&n)
	require.NoError(t, err)
	return n
}

// listPublicTables returns BASE TABLE names in the public schema.
func listPublicTables(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.Query(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		 ORDER BY table_name`,
	)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
```

- [ ] **Step 2: Run the test locally, confirm PASS**

Run: `go test ./internal/adapters/storage/postgres/ -run TestMigrations_UpDownUpRoundtrip -v`

Expected: `--- PASS: TestMigrations_UpDownUpRoundtrip` against the devcontainer Postgres at `postgres:5432`. If Postgres isn't reachable, the test skips with a message — that's acceptable but you should verify locally before committing.

If the test fails because of a broken down migration, fix the down SQL, not the test. (As of 2026-04-20 there is a single migration `000001_initial_schema` — if it fails, the down file needs work.)

- [ ] **Step 3: Run the whole postgres test package to confirm no interference**

The new test drops the public schema. Verify it doesn't break adjacent tests that share the process.

Run: `go test -race ./internal/adapters/storage/postgres/... -v`

Expected: all tests pass. If any fail, it's likely ordering-related; the fix is to put `TestMigrations_UpDownUpRoundtrip` logically last (Go runs tests in source order within a package; rename the file or the function alphabetically later if needed). First choice: verify the failure reproducibly before renaming.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/postgres/migrations_test.go
git commit -m "test(postgres): add migrations up/down/up roundtrip

Exercises the full cycle against a real Postgres. Catches broken SQL
in either direction and down migrations that fail to DROP everything
they should."
```

---

### Task 2: Attach Postgres service to the CI job

**Files:**
- Modify: `.github/workflows/test.yml`

- [ ] **Step 1: Modify the `ci` job to add services and test env**

Open `.github/workflows/test.yml`. Inside the `ci:` job, after the `permissions:` block and before `steps:`, add the services block. Also modify the "Run tests..." step to include an env var. The full modified `ci` job (replace the existing one verbatim) is:

```yaml
  ci:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    permissions:
      contents: read

    services:
      postgres:
        image: postgres:17
        env:
          POSTGRES_USER: slabledger
          POSTGRES_PASSWORD: slabledger
          POSTGRES_DB: slabledger
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U slabledger"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
    - name: Checkout code
      uses: actions/checkout@v6

    - name: Set up Go
      uses: actions/setup-go@v6
      with:
        go-version: '1.26'
        cache: true

    - name: Download dependencies
      run: go mod download

    - name: Check architecture and file size invariants
      run: |
        ./scripts/check-imports.sh
        ./scripts/check-file-size.sh

    - name: Run tests with coverage and race detection
      env:
        POSTGRES_TEST_URL: postgresql://slabledger:slabledger@localhost:5432/slabledger?sslmode=disable
      run: go test -race -timeout 10m -coverprofile=coverage.out -covermode=atomic ./...

    - name: Run linter
      uses: golangci/golangci-lint-action@v9
      with:
        version: v2.11.4
        args: --timeout=3m

    - name: Build binary
      run: go build -o slabledger ./cmd/slabledger

    - name: Verify build
      run: ./slabledger --help

    - name: Upload coverage
      uses: codecov/codecov-action@v6
      if: always()
      continue-on-error: true
      with:
        file: ./coverage.out
        fail_ci_if_error: false
```

**Note:** the `integration` job below should NOT get the Postgres service. Integration tests hit live DH/external APIs on a weekly schedule; they don't need the CI Postgres.

**Note on `localhost:5432`:** when a workflow job uses a `services:` block, GitHub exposes the service on the runner's localhost via the `ports` mapping. We intentionally do NOT use the devcontainer hostname `postgres:5432` because GitHub Actions runners don't have that hostname; `localhost` is correct here.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/test.yml
git commit -m "ci: attach postgres 17 service + run postgres-backed tests

Previously, tests in internal/adapters/storage/postgres/ silently
skipped in CI because no Postgres was reachable. With a service
container and POSTGRES_TEST_URL set, they run for real — including
the new migrations up/down/up roundtrip."
```

- [ ] **Step 3: Push the branch and let CI run once**

Run: `git push -u origin $(git branch --show-current)`

Then: `gh pr create` (see Task 5 for the PR template). Watch the PR's CI job — the `ci` check should run the postgres tests and they should PASS, not SKIP. Look for `TestCLSalesStore_GetCompSummariesByKeys` and `TestMigrations_UpDownUpRoundtrip` in the log with `--- PASS:`.

If the Postgres service fails its health check, the job stalls in `services:` startup. Fix by bumping `--health-retries` or tightening the SQL init.

---

## Section B — Branch protection

### Task 3: Apply branch protection on main

**Files:**
- No file changes in the repo. This task runs a `gh api` command and documents the exact invocation so it's reproducible.

- [ ] **Step 1: Verify current state**

Run: `gh api repos/guarzo/slabledger/branches/main/protection 2>&1`

Expected: either `{"message":"Branch not protected","status":"404"}` (never configured) or an existing protection JSON (already configured, in which case we're amending).

- [ ] **Step 2: Apply protection**

Run the following. The JSON body sets: require PR (no approvers required), require `ci` and `build` status checks, require the branch to be up-to-date (`strict: true`), don't enforce for admins (emergency escape hatch), no force pushes, no deletions.

```bash
gh api --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/guarzo/slabledger/branches/main/protection \
  --input - <<'JSON'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["ci", "build"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
JSON
```

Expected: HTTP 200 with a JSON body echoing the settings. If you see "You cannot require code owner reviews on this branch" or similar, drop the offending field and retry — the minimum we need is `required_status_checks` + `required_pull_request_reviews`.

- [ ] **Step 3: Verify protection is active**

Run: `gh api repos/guarzo/slabledger/branches/main/protection | jq '{required_status_checks, required_pull_request_reviews, enforce_admins}'`

Expected output:
```json
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["ci", "build"]
  },
  "required_pull_request_reviews": { "required_approving_review_count": 0, ... },
  "enforce_admins": { "enabled": false, ... }
}
```

- [ ] **Step 4: Smoke-test by attempting a direct push**

From a scratch commit on main (use a throwaway README edit for this smoke test only, then discard):

```bash
# WARNING: this is a deliberate test of the protection rule.
# Run from the PHASE6B WORKTREE's local main checkout, not from the branch.
# If the rule works, the push fails. If it succeeds, the rule is broken.
git checkout main
echo " " >> README.md
git commit -am "smoke: verify branch protection rejects direct push"
git push 2>&1 | grep -i "protected\|rejected\|denied" || echo "UNEXPECTED: push succeeded"
git reset --hard HEAD~1   # discard the local commit
```

Expected: `remote: error: GH006: Protected branch update failed for refs/heads/main.` — the push is rejected.

If the push succeeds, the protection JSON didn't take effect — inspect Step 3's output and re-run Step 2 with corrections.

- [ ] **Step 5: No commit in this repo**

Branch protection lives in GitHub, not the repo. Move on to Task 4.

---

## Section C — Rollback runbook

### Task 4: Write `docs/OPERATIONS.md`

**Files:**
- Create: `docs/OPERATIONS.md`

- [ ] **Step 1: Create the runbook**

Create `docs/OPERATIONS.md`:

```markdown
# Operations Runbook

Short-form operational guidance. Runbooks are read under stress — prefer
scanability over completeness. If a section needs more than ~20 lines,
it probably belongs somewhere else.

## Rolling back a bad deploy

If production is broken, you can usually roll back to a previous working
release in under two minutes.

### 1. List recent releases

```bash
flyctl releases --app slabledger
```

Output looks like:

```
 VERSION │ STATUS   │ DESCRIPTION │ USER   │ DATE
 v5      │ complete │ Release     │ <you>  │ 4m ago    ← broken
 v4      │ complete │ Release     │ <you>  │ 2h ago    ← last known good
 v3      │ complete │ Release     │ <you>  │ 6h ago
```

Pick the last version that was known-good.

### 2. Find its image reference

```bash
flyctl image show --app slabledger <version>
```

or use the latest known-good image you deployed. The image ref looks like
`registry.fly.io/slabledger:deployment-<timestamp>`.

### 3. Redeploy that image

```bash
flyctl deploy --app slabledger --image registry.fly.io/slabledger:deployment-<timestamp>
```

This pulls the previous image and deploys it in place. No rebuild, ~30s–1m.

### 4. Verify

- `curl https://slabledger.dpao.la/api/health` → `200`
- Load a few pages, check `flyctl logs` for no new errors.
- `flyctl releases --app slabledger` now shows the rolled-back version at
  the top.

### 5. Post-rollback: revert the offending commit

Find the commit that introduced the regression and revert it through a PR:

```bash
git revert <sha>
git push -u origin revert-<sha>
gh pr create --title "revert: <reason>" --body "Rolls back broken deploy."
```

Let CI pass, merge the PR. Fly's auto-deploy will redeploy the reverted
code — this is fine because the `revert` commit is functionally the same
as the rolled-back image.

Without this step, the next unrelated merge to `main` will re-trigger a
deploy of the original bad code.

## When to roll back vs. fix-forward

| Situation                                                       | Action       |
|-----------------------------------------------------------------|--------------|
| Site is down / returning 5xx                                    | Roll back    |
| Data is being corrupted                                         | Roll back    |
| User-visible regression; fix is < 15 min and well-tested        | Fix forward  |
| Regression in a feature; rolling back means losing other fixes  | Case-by-case |
| Performance degradation only                                    | Fix forward  |

Default to **roll back first, investigate later**. The rollback takes a
minute; diagnosing under pressure costs far more.

## Database migration caveat

**Rolling back the app image does NOT undo a migration.** If the broken
deploy ran a migration (added/renamed/dropped columns), the rolled-back
app may crash because its code doesn't match the schema.

Options when this happens:

- **Fix-forward migration (preferred):** write a new migration that
  brings the schema back to what the rolled-back code expects. New
  migration files go in `internal/adapters/storage/postgres/migrations/`
  as `000NNN_*.up.sql` + `.down.sql` pairs. See `internal/README.md` for
  creation steps.
- **`migrate.Down()` to the previous version (risky):** this drops
  columns, possibly with data. Only safe if you're certain the dropped
  columns hold no new data since the bad deploy. There's no in-repo
  tooling for this today — you'd use `golang-migrate` directly against
  Supabase. If you take this path, take a Supabase backup first.

Either way, this requires manual judgment. This section exists so you're
not surprised mid-incident.

## Where things live

- Prod URL: `https://slabledger.dpao.la`
- Fly app: `slabledger` in `iad`
- Supabase project: `waenohauzqqaeugxilyt` (us-east-2)
- Grafana (metrics): `fly-metrics.net`
- Fly dashboard: `https://fly.io/apps/slabledger`
- Fly logs: `flyctl logs --app slabledger`

## What this runbook does NOT cover

- Recovering from a corrupt Supabase backup.
- Restoring from the SQLite backup on wanderer (decommissioned after
  Phase 7).
- Rotating secrets mid-incident.

Each of those is its own runbook if/when needed.
```

- [ ] **Step 2: Commit**

```bash
git add docs/OPERATIONS.md
git commit -m "docs(ops): rollback runbook

Short-form operational guide: list releases, redeploy a previous
image, revert the offending commit, migration caveat. ~80 lines so
it's readable under pressure."
```

---

## Final verification

### Task 5: Open the PR and merge

- [ ] **Step 1: Verify clean tree and push**

Run:
```bash
git status
git log main..HEAD --oneline
git push
```

Expected commits on this branch (after Tasks 1–4):
```
<sha> docs(ops): rollback runbook
<sha> ci: attach postgres 17 service + run postgres-backed tests
<sha> test(postgres): add migrations up/down/up roundtrip
```

(Task 3 had no commit in the repo — branch protection lives in GitHub.)

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "phase 6b: CI/CD gating" --body "$(cat <<'EOF'
## Summary

- Branch protection on \`main\` now requires \`ci\` + \`build\` to pass before merge (applied out-of-band via \`gh api\`; see Task 3 of the plan).
- CI job gets a Postgres 17 service, so the previously-skipped Postgres-backed tests actually run. Plus a new migration up/down/up roundtrip test.
- \`docs/OPERATIONS.md\` — short rollback runbook.

Spec: \`docs/specs/2026-04-20-phase6b-cicd-gating-design.md\`
Plan: \`docs/plans/2026-04-20-phase6b-cicd-gating.md\`

## Test plan

- [x] \`go test ./internal/adapters/storage/postgres/... -v\` locally includes the new roundtrip test, all PASS
- [ ] This PR's CI run shows the Postgres service starting and tests running (not skipping)
- [ ] Branch protection smoke test: a dummy broken-test PR cannot be merged
- [ ] After merge + Fly auto-deploy: prod still healthy

EOF
)"
```

- [ ] **Step 3: Watch CI, then merge**

Run: `gh pr checks <pr-number> --watch`

When green: `gh pr merge <pr-number> --squash --delete-branch`

- [ ] **Step 4: Confirm branch protection is live (post-merge sanity)**

Run: `gh api repos/guarzo/slabledger/branches/main/protection | jq .required_status_checks.contexts`

Expected: `["ci", "build"]`.

- [ ] **Step 5: Verify prod deploy**

Fly's dashboard Auto-Deploy should fire on the merge commit. Check:
- `flyctl releases --app slabledger` — new version number at top, status `complete`
- `curl https://slabledger.dpao.la/api/health` → 200

If Fly didn't auto-deploy, the Fly-side GitHub integration may still be off — flip the toggle manually and re-push a trivial commit.
