# Phase 6c — Local Dev Ergonomics Cleanup

## Context

Phases 6a and 6b shipped. The SQLite-to-Supabase-Postgres cutover completed on 2026-04-20. The Postgres adapter is in production, branch protection is enabled, and CI runs the Postgres-backed store tests against a service container.

Residual SQLite-era references remain across the dev-environment layer. None are *broken* — the app reads `DATABASE_URL`, ignores `DATABASE_PATH`, and runs correctly — but a fresh-checkout contributor reading these files would form a wrong mental model. This phase removes the stale references so the dev surface matches the Postgres-only reality.

Explicitly not in scope: seed data, first-run flows, test helper changes, `make db-pull` end-to-end verification. Those were discussed during brainstorming and deemed low value given the solo-developer context.

## Goals

1. Every SQLite and `DATABASE_PATH` reference in live configuration files is gone or correctly reflects the Postgres-only setup.
2. `grep -rn 'slabledger\.db\|DATABASE_PATH' --include=Makefile --include='*.yml' --include='*.toml' --include='*.sh' --include='*.md' .` returns hits only under `docs/private/`, historical commit messages, or the `data/` directory (retained local SQLite backup file — intentionally kept, see below).

## Non-goals

- Deleting the `/workspace/data/slabledger.db` SQLite file. Kept as a local-only archive of the wanderer dump until Phase 7 decommissions wanderer. Gitignored.
- Deleting `docker-compose.prod.yml`. Marked legacy in Phase 6; Phase 7 can remove it when wanderer goes away.
- Seed data for fresh contributors.
- A `make db-pull` end-to-end dry run against Supabase.
- Any changes to `setupTestDB` / test isolation.
- Changes to `docs/SCHEMA.md` or the Postgres migration file (those were generated fresh during Phase 1 and already match reality).

## Design

Mechanical pass across six files. Each change is a small, obvious edit; no behavior change; verifiable via a single `grep`.

### 1. `docker-compose.yml`

Remove the `DATABASE_PATH: /app/data/slabledger.db` environment line (line 40 as of commit `cb21f439`). The service already picks up `DATABASE_URL` from `.env`, so this removal is zero-behavior-change.

### 2. `docker-compose.prod.yml`

Remove the `DATABASE_PATH: /app/data/slabledger.db` line (line 18). The `# LEGACY` banner at the top is kept — this file remains an emergency wanderer-rollback reference until Phase 7. The env var was meaningless after the Postgres cutover anyway.

### 3. `.devcontainer/docker-compose.yml`

Remove the `DATABASE_PATH: /workspace/data/slabledger.db` line (line 47). The `DATABASE_URL` line immediately below it is what the app actually reads.

### 4. `.devcontainer/post-start.sh` (lines 114–116)

Current code:

```bash
if [ -f "data/slabledger.db" ]; then
    DB_SIZE=$(du -h data/slabledger.db | awk '{print $1}')
    echo "💾 Database: data/slabledger.db ($DB_SIZE)"
fi
```

Replace with a Postgres readiness check:

```bash
if command -v pg_isready >/dev/null 2>&1; then
    if pg_isready -h postgres -U slabledger -q; then
        echo "💾 Postgres: ready (postgres:5432)"
    else
        echo "⚠️  Postgres not yet ready at postgres:5432 — check docker compose logs postgres"
    fi
fi
```

Reports signal the dev actually wants — "can I run the app?" rather than "is there a dead SQLite file on disk?".

### 5. `.devcontainer/README.md` (lines 276–292, "Database Access" section)

Current section shows `sqlite3 data/slabledger.db`, `.tables`, `.schema`, `cp data/slabledger.db data/backup_$(date +%Y%m%d).db`, `rm data/slabledger.db`. Replace with:

```markdown
### Database Access

The database is Postgres (Supabase in production, local Postgres container in the
devcontainer). The app reads `DATABASE_URL` from `.env` and falls back to the
devcontainer service on `postgres:5432`.

```bash
# Interactive psql shell against the devcontainer Postgres
psql "$DATABASE_URL"

# One-liners
psql "$DATABASE_URL" -c '\dt'              # list tables
psql "$DATABASE_URL" -c '\d campaigns'     # describe a table
psql "$DATABASE_URL" -c 'SELECT COUNT(*) FROM campaign_purchases;'

# Dump / restore
pg_dump --format=custom --file=local-$(date +%Y%m%d).dump "$DATABASE_URL"
pg_restore --no-owner --no-privileges --dbname="$DATABASE_URL" local-$(date +%Y%m%d).dump

# Reset local schema (destructive — destroys all local data; app will re-run
# migrations on next startup)
psql "$DATABASE_URL" -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
```

For syncing with prod Supabase, see `make db-pull` / `make db-push` in the top-level Makefile.
```

### 6. `.claude/skills/ui-screenshot-improve/SKILL.md` (line 45)

Current text: "…starts the server with local data (`data/slabledger.db`)…"
Replace with: "…starts the server with the devcontainer Postgres via `DATABASE_URL`…"

The `make screenshots` target itself was already updated in Phase 6 to pass `DATABASE_URL`; this is doc-only.

## Files to modify

- `docker-compose.yml` — drop one env line.
- `docker-compose.prod.yml` — drop one env line.
- `.devcontainer/docker-compose.yml` — drop one env line.
- `.devcontainer/post-start.sh` — replace SQLite-size banner with Postgres readiness check.
- `.devcontainer/README.md` — rewrite "Database Access" section.
- `.claude/skills/ui-screenshot-improve/SKILL.md` — one sentence.

## Verification

1. `grep -rn 'slabledger\.db' --include=Makefile --include='*.yml' --include='*.toml' --include='*.sh' --include='*.md' .` — hits only under `docs/private/` or unrelated historical mentions (verify no live config remains).
2. `grep -rn 'DATABASE_PATH' --include=Makefile --include='*.yml' --include='*.toml' --include='*.sh' --include='*.md' --include='*.go' .` — zero hits. The env var is extinct.
3. Devcontainer rebuild works: the app still starts after a clean rebuild.
4. `docker compose` command still starts the app (for local non-devcontainer users who use `docker-compose.yml`).

## Rollback

Each of the six file changes is independently revertible. No schema migration, no dependency change, no data movement. Full rollback = revert the PR.

## Out of scope — tracked for future phases

- **Seed data / first-run flow** — if/when a second contributor joins or the fresh-checkout friction becomes real, revisit as a standalone item. Deferred indefinitely.
- **`make db-pull` live verification** — worth doing once, just not in 6c. Can be a 10-minute follow-up task next time you're touching Supabase.
- **Phase 7 (wanderer decommission)** — removes `docker-compose.prod.yml` entirely, tears down wanderer's compose stack, deletes unused GitHub secrets (`DEPLOY_HOST`, etc., plus now `FLY_API_TOKEN` which is unused after the dashboard Auto-Deploy switch).
