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

### Migration 000022 (decision provenance) is rollback-safe

Migration `000022_add_decision_provenance` is safe under the image-only
rollback path above (steps 1–4, no `migrate.Down()` needed):

- All new columns are additive and nullable/defaulted (6 nullable columns
  on `campaign_purchases`; `sale_reason`, `cl_value_at_sale_cents`,
  `channel_fee_pct_at_sale` on `campaign_sales`). A rolled-back previous
  image that doesn't know these columns exist simply never reads or
  writes them — no crash.
- `forced_liquidation` is unchanged in shape (still a plain boolean the
  app sets directly); the previous image's INSERTs against it keep
  working unmodified.
- The `campaign_sales_derive_reason` trigger backfills `sale_reason` on
  any row the old image inserts without it (deriving from
  `forced_liquidation`), so sales recorded during the rollback window
  still get a valid, non-empty `sale_reason` once the new image is
  redeployed — no manual backfill needed.

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
