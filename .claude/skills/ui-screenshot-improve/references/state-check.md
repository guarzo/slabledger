# DB state precondition

The skill captures screenshots from `$SCREENSHOT_DB_URL` (default: the devcontainer Postgres, same as `$LOCAL_DB_URL`). If that DB is unseeded or has rolled over, every page renders its empty state and the audit surfaces polish-class findings instead of real product friction.

## Threshold

A DB is "realistic enough to audit" when **all** of these are true:

- `campaigns ≥ 2`
- `campaign_purchases ≥ 20`
- `campaign_sales ≥ 1`
- `invoices ≥ 1`

Rationale: two campaigns exercises list and empty-state-free rendering; 20 purchases fills the inventory table and exercises virtualization; one sale exercises revenue metrics and sold-item rendering; one invoice exercises the "Invoice Readiness" dashboard surface. Below any of these, the audit is dominated by empty states.

## Recovery

If the state check fails and `$PROD_DB_URL` (or `$SUPABASE_URL`) is set, the skill automatically runs:

```bash
YES=1 make db-pull
```

…which dumps prod into the local DB. After the pull, the skill re-runs the state check. If the counts are still below threshold (prod itself is empty), the skill halts with guidance — it does not attempt to audit an empty product.

If `$PROD_DB_URL` is not set, the skill halts immediately with:

> "The local DB looks unseeded (N campaigns, N purchases, N sales, N invoices). Run `make db-pull` manually with `PROD_DB_URL` set, then re-run this skill."

## Override

Set `UI_ALLOW_EMPTY=1` in the environment to intentionally audit empty-state copy (e.g. when the goal is to polish first-run messaging). The skill still runs the state check, but the "below threshold" halt becomes a warning instead of a halt.

## Why we don't seed a synthetic fixture

`make db-pull` gives us real prod data — real campaign names, real price flags, real invoice states — which means the audit sees the actual product a user will see. A synthetic fixture would drift from reality as the schema and seed scripts diverge, and would give us a false sense of coverage. Using the live pull path keeps the audit honest.
