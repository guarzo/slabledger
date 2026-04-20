# Phase 6b — CI/CD Gating

## Context

Phase 6a shipped. Fly.io's dashboard "Auto-Deploy on push" is enabled — every push to `main` triggers a production deploy, regardless of CI state. CI (`test.yml`: `go test -race`, lint, build) runs on PRs and on pushes to `main`, but in parallel with the deploy, not as a prerequisite. A PR that breaks tests can still be merged and shipped.

Three related gaps drive this phase:
1. **No deploy gate.** CI passing is not required for merge. Branch protection on `main` is not configured (`gh api .../branches/main/protection` → 404).
2. **Migrations aren't actually tested in CI.** The Postgres-backed store tests (including the initial-schema migration) silently skip in CI because no Postgres service is attached to the runner. `testhelper_test.go:27` skips when Postgres isn't reachable.
3. **No documented rollback procedure.** If a deploy breaks prod, the operator has to figure out `fly releases` + `fly deploy --image ...` from scratch under pressure.

This phase addresses all three with minimum-viable mechanisms. Preview environments per PR, blue-green deploys, and automatic rollback on health-check failure are explicitly out of scope.

## Goals

1. Direct pushes to `main` blocked; merges require green CI.
2. CI actually runs the Postgres-backed tests and verifies the migration up/down/up roundtrip.
3. A rollback runbook exists and is short enough to read under pressure.

## Non-goals

- Preview environments per PR.
- Blue-green or canary deploys.
- Automatic rollback on health-check failure.
- Required human reviewers on PRs (solo-developer repo).
- Signed commits.
- Automating the rollback command itself into a script.
- Dashboarding / alerting on deploy events.
- Backfilling Postgres-backed tests for store files that currently have no tests (scope creep from Phase 6a).

## Design

### 1. Deploy gate via GitHub branch protection

Add branch protection on `main` with these settings:

- **`required_pull_request_reviews`**: `{ required_approving_review_count: 0, dismiss_stale_reviews: false }` — require a PR, but no human approver required.
- **`required_status_checks`**: `{ strict: true, contexts: ["ci", "build"] }` — both must pass; `strict: true` forces the branch to be up-to-date with `main` so checks reflect the actual merge state.
- **`enforce_admins`**: `false` — the repo owner can still admin-push if there's an emergency. Branch protection without an admin escape hatch is brittle for a solo-developer repo.
- **`restrictions`**: `null` — no push restrictions beyond the PR requirement.
- **`allow_force_pushes`**: `false`.
- **`allow_deletions`**: `false`.

The check names `ci` and `build` match the job names that appear in `gh pr checks` today (see PR #220, PR #222 — both report `ci` and `build` as the two required checks; CodeRabbit is informational and not required).

Configuration is via `gh api --method PUT repos/guarzo/slabledger/branches/main/protection` with a JSON body. The exact JSON lives in the implementation plan. The call is idempotent — re-running with the same settings is a no-op.

**Why branch protection instead of reintroducing a Fly workflow that `needs: ci`:**

- The `fly-deploy.yml` workflow was deleted in PR #220 specifically to reduce surface area. Reintroducing it couples deploys to GitHub Actions again and means CI runs twice per change (once on the PR, once on the push to main).
- Branch protection solves the gate at the merge point — one CI run per PR, one deploy per merge.
- Fly dashboard Auto-Deploy stays on. The only way anything reaches `main` (and therefore the deploy trigger) is through a merged PR that has passed CI.

**Consequence:** Pushing docs or fixups directly to `main` stops working, including from this development workflow. All future changes go through PRs. Acceptable tradeoff.

### 2. Migration preview in CI

**Current CI behavior:** `test.yml:ci` runs `go test -race -timeout 10m -coverprofile=coverage.out -covermode=atomic ./...`. The Postgres-backed store tests (`campaign_store_test.go`, `cl_sales_store_test.go`, migrations-touching code) read `POSTGRES_TEST_URL` from the env and skip via `t.Skipf` when Postgres is unreachable. Because the GitHub Actions runner has no Postgres service today, they all skip silently. Migrations are verified only locally.

**Changes:**

#### 2a. Attach a Postgres service to the `ci` job

Add a `services:` block to the `ci` job in `.github/workflows/test.yml`:

```yaml
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
```

Set an env var on the test step so `setupTestDB` finds it:

```yaml
env:
  POSTGRES_TEST_URL: postgresql://slabledger:slabledger@localhost:5432/slabledger?sslmode=disable
```

After this change, the existing Postgres-backed tests will run for real in CI rather than skipping.

#### 2b. Migration up/down/up roundtrip test

Create `internal/adapters/storage/postgres/migrations_test.go` with one function:

```go
func TestMigrations_UpDownUpRoundtrip(t *testing.T)
```

Contract:
1. Open a fresh `*DB` against `POSTGRES_TEST_URL` (reuse `setupTestDB`, but call `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` at the start to get a guaranteed-empty baseline).
2. Call `RunMigrations(db, "")` — assert no error. Assert the current version matches the highest migration number embedded in `MigrationsFS`.
3. Invoke `migrate.Down()` (via a helper that mirrors the `RunMigrations` setup but calls `Down()` instead of `Up()`). Assert no error. Assert that the `public` schema has no user tables (query `information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'` — count must be 0 except for the `schema_migrations` tracking table, which `migrate` retains).
4. Call `RunMigrations(db, "")` a second time. Assert no error, assert version matches the max again.

This catches: missing `DROP` in a down migration, SQL syntax errors in either direction, columns whose up-types don't match their down-types, and anything that would leave the DB dirty.

**Test isolation:** because this test drops the whole public schema, it must run in its own process or be ordered to run last. `go test` within a single package runs tests sequentially by default, which is enough — but this test **must not** use `t.Parallel()`, and any other test that also drops the schema must be ordered to cooperate. The existing `setupTestDB` uses `TRUNCATE` for isolation between tests, not `DROP`, so there's no conflict.

#### 2c. Test isolation after DROP SCHEMA

`setupTestDB` already calls `RunMigrations` on every invocation and reads `POSTGRES_TEST_URL` with a devcontainer fallback — no change needed for existing tests to run in CI once the Postgres service is attached. The only wrinkle is the new roundtrip test: its explicit `DROP SCHEMA public CASCADE` runs between `setupTestDB` calls in the same process. Since `golang-migrate` tracks state in `schema_migrations` (also inside `public`), dropping the schema wipes that too, and the next `setupTestDB` call re-runs migrations from scratch — which is the intended behavior. No `sync.Once` or helper reorganization required.

### 3. Rollback runbook

Create `docs/OPERATIONS.md` (~60-80 lines, short enough to read under pressure).

**Outline (exact content specified in the plan):**

1. **Find a previous release.** `flyctl releases --app slabledger` — shows version, image ref, date.
2. **Roll back by image.** `flyctl deploy --app slabledger --image <registry.fly.io/slabledger:deployment-NNNN>`. No rebuild; pulls the previous image and deploys.
3. **When to roll back vs. fix-forward.** Short decision table:
   - App down / broken / corrupting data → roll back immediately.
   - Minor regression, fix < 15 min to write + review → fix-forward.
   - Regression that's a subset of a larger broken feature → roll back, then fix-forward with proper testing.
4. **Post-rollback cleanup.** Revert the offending commit on GitHub (`git revert <sha>`, open PR, pass CI, merge). Branch protection from Section 1 means the revert goes through the same gate as anything else.
5. **Database migration caveat.** Rolling back the image does NOT undo any migration the bad deploy ran. If the broken deploy added / changed schema:
   - `migrate.Down()` is possible but risky (drops columns, possibly data).
   - The safer path is writing a new "fix-forward" migration that undoes the damage, since down-migrations aren't always symmetric with up-migrations.
   - Either way, manual judgment required. This is called out explicitly so the operator isn't surprised.

**What the runbook does NOT cover** (flagged as separate work):
- Recovering from a corrupt Supabase backup.
- Restoring from the SQLite backup on wanderer (decommissioned after Phase 7).
- Secret rotation mid-incident.

## Files to create / modify

- `docs/OPERATIONS.md` *(new)* — rollback runbook.
- `.github/workflows/test.yml` — add `services:` block + `POSTGRES_TEST_URL` env.
- `internal/adapters/storage/postgres/migrations_test.go` *(new)* — roundtrip test.
- Branch protection is applied via `gh api` — no file change, but the implementation plan will include the exact command for reproducibility.

## Verification

- `gh api repos/guarzo/slabledger/branches/main/protection` returns a 200 body with the expected settings (not 404).
- A dummy PR with a deliberately-broken test cannot be merged; the Merge button is disabled until the failing check is resolved.
- A real PR that passes CI can be merged normally and auto-deploys via Fly dashboard.
- CI runs now include the Postgres-backed tests (check the run logs for `TestCLSalesStore_GetCompSummariesByKeys` reporting PASS, not SKIP).
- `TestMigrations_UpDownUpRoundtrip` passes in CI.
- `docs/OPERATIONS.md` exists and follows the outline above.

## Rollback plan for this phase

- Branch protection: `gh api --method DELETE repos/guarzo/slabledger/branches/main/protection` removes it.
- CI changes: revert the workflow PR.
- Runbook: delete the file.

Each change is independently revertible.

## Out of scope — tracked for 6c

- **6c (local dev ergonomics):** seed data for new contributors, schema-reset workflow, Postgres test isolation beyond `sync.Once`, `make db-pull` end-to-end verification, devcontainer improvements.
- **Preview environments per PR** — deferred indefinitely. Revisit when there's a second developer or a change flow that needs it.
