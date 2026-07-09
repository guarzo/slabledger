# Runbook: CardLadder profileId backfill

After deploying the profileId fix, existing rows hold OLD hash gemRateIds (or
are empty). Clear them so the next CL refresh re-resolves to profileIds.

## When
Once, immediately after the fix is deployed to production.

## SQL (run against the production Postgres)
```sql
-- Clear cached CL mappings so resolveGemRate re-resolves to profileIds.
UPDATE cl_card_mappings   SET cl_gem_rate_id = '', cl_condition = '';

-- Clear stale identifiers on purchases (repopulated by the next refresh).
UPDATE campaign_purchases SET gem_rate_id = '' WHERE gem_rate_id <> '';
```

`cl_sales_comps` rows keyed by the old hash become orphaned and are superseded
by profileId-keyed rows on the next comp refresh; no manual cleanup required.

## Verify (after the next CL refresh cycle)
Check `/api/admin/cardladder/status`:
- `resolved` > 0
- `certResolveFailed` collapses toward 0
- `updated` > 0
- `cardsMapped` climbs past 34
