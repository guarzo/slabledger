# Prompt: optimistic concurrency control for campaign update

> **Status:** not started. This file is a *prompt* — a self-contained task
> briefing to hand to an implementer (human or agent), not an approved plan.
> The design decision in "Open decision" below is deliberately left open and
> must be settled with the operator before code is written.

---

## The prompt

`PUT /api/campaigns/{id}` is a blind full-object overwrite. Any client that does
read-modify-write against a cached campaign can silently revert fields another
writer changed in between, and two operators saving concurrently always lose one
write with no error surfaced to either. Add optimistic concurrency control to
the campaign update path so a stale write fails loudly (HTTP 409) instead of
succeeding destructively.

Scope is the campaign update path only. Do not generalize to purchases, sales,
or other aggregates in this change.

---

## Why now

`web/src/react/pages/CampaignsPage.tsx` performs a clipboard-import upsert that
builds a full-object `PUT` from a campaign it found in the react-query-cached
list. As of commit on branch `form-field-a11y` that path re-reads the campaign
via `api.getCampaign(existing.id)` immediately before merging, which narrows the
window — but does not close it. The race is still open between the `GET` and the
`PUT`, and the re-read is a client-side convention that nothing enforces. The
server accepts any write from any client at any time.

---

## Evidence (verified file:line)

**The endpoint layer is already complete on the read side.** `HandleGetCampaign`
exists and is routed — the gap is purely write-side concurrency.

- `internal/adapters/httpserver/routes.go:110-114` — `GET/POST /api/campaigns`,
  `GET/PUT/DELETE /api/campaigns/{id}`.
- `internal/adapters/httpserver/handlers/campaigns.go:166-183` —
  `HandleGetCampaign`.
- `internal/adapters/httpserver/handlers/campaigns.go:186-212` —
  `HandleUpdateCampaign`. Decodes the body into an `inventory.Campaign`, stamps
  `c.ID = id`, calls the service, returns 200. No version, no `If-Match`, no
  conflict path.

**The service clobbers `UpdatedAt` before the repo sees it.** This is the
subtlety that dictates the shape of the fix: a client-supplied expected version
*cannot* ride in `Campaign.UpdatedAt`, because the service overwrites that field
on the way through. It needs a separate parameter or a separate field.

```go
// internal/domain/inventory/service_crud.go:35-41
func (s *service) UpdateCampaign(ctx context.Context, c *Campaign) error {
	if err := ValidateAndNormalizeCampaign(c); err != nil {
		return err
	}
	c.UpdatedAt = time.Now()
	return s.campaigns.UpdateCampaign(ctx, c)
}
```

**The store's zero-row case is already spoken for.**
`internal/adapters/storage/postgres/campaign_store.go:295-328` runs
`UPDATE campaigns SET ... WHERE id = $21` and maps `RowsAffected() == 0` to
`inventory.ErrCampaignNotFound`. Adding `AND updated_at = $22` (or
`AND version = $22`) makes that zero-row result ambiguous between *not found*
and *conflict*. The store must disambiguate — e.g. a follow-up existence check
inside the same transaction — and return a distinct sentinel for the conflict
case. Do not let a conflict surface as a 404.

**Two interfaces and two non-HTTP callers are affected.** Both interfaces
declare `UpdateCampaign(ctx context.Context, c *Campaign) error`:

- `internal/domain/inventory/repository_campaign.go:10`
- `internal/domain/inventory/service_interfaces.go:10`

These callers must **not** be forced to supply a client version — they are
server-side writers with no browser round-trip to carry one:

- `internal/adapters/httpserver/handlers/campaigns_psa.go:109`
- `cmd/psa-harvest/baseline.go:264`

So the version must be optional at the domain boundary (a nil/zero expected
version means "unconditional write", preserving today's behavior) or expressed
via a separate method. Pick one and be consistent; do not add a required
parameter that every internal caller passes a dummy value for.

**Sentinel error convention.** `internal/domain/inventory/errors.go:28,45,62` —
`ErrCampaignNotFound`, `IsCampaignNotFound`, `IsValidationError`, built with
`errors.NewAppError` + `HasErrorCode`. A new `ErrCampaignConflict` (or similar)
follows that pattern, with a matching `Is…` helper and a handler mapping to
`http.StatusConflict`.

**Migration.** `campaigns` currently ends with:

```sql
created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
```

(`internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql`).
Latest migration is `000025_psa_portal_catalog`; the next number is **`000026`**.
Style reference for an `ALTER TABLE ADD COLUMN` + backfill + commented migration:
`000024_campaign_targeting_axes.up.sql`. A down migration is required.

**Frontend already carries the field it would need.**
`web/src/types/campaigns/core.ts:31-32` declares `createdAt: string;
updatedAt: string;`. `CampaignsPage.tsx` currently destructures `updatedAt`
*away* before the `PUT` — that destructure must be revisited by whatever design
is chosen.

---

## Open decision (settle before implementing)

**`version BIGINT` column vs. `updated_at` compare-and-swap.**

- `updated_at` CAS needs no migration and no new field on the wire, but compares
  a timestamp that round-trips through JSON. Precision truncation and timezone
  normalization between Postgres `TIMESTAMP`, Go `time.Time`, and a JS string
  are a real risk: a mismatch fails *closed*, so the failure mode is spurious
  409s on legitimate saves rather than lost updates — annoying, not dangerous,
  but it will look like a bug to the operator.
- A monotonic `version BIGINT NOT NULL DEFAULT 1` column is exact, trivially
  comparable, and immune to timestamp semantics. It costs migration `000026`,
  a new field on `Campaign` and its TS mirror, and a `version = version + 1`
  in the `UPDATE`.

Recommendation: the `version` column. Raise it with the operator before writing
code — it changes the migration, the Go struct, the TS type, and the wire shape.

**Transport of the expected version.** Either a JSON field on the request body
or an `If-Match` header. `If-Match` is the HTTP-correct answer and keeps the
body a pure representation; a body field is less ceremony and matches how the
rest of this API talks. Decide once; do not support both.

---

## Acceptance criteria

- [ ] A `PUT` carrying a stale expected version returns **409**, and the stored
      row is unchanged.
- [ ] A `PUT` carrying the current expected version succeeds and advances the
      version.
- [ ] A `PUT` for a nonexistent id still returns **404**, distinctly from 409.
- [ ] A `PUT` with **no** expected version behaves exactly as today
      (unconditional overwrite), so `campaigns_psa.go:109` and
      `cmd/psa-harvest/baseline.go:264` need no change beyond compilation.
- [ ] The frontend surfaces the 409 to the operator as a "changed elsewhere,
      reload" message rather than a generic failure toast, and the
      `CampaignsPage.tsx` clipboard upsert forwards the version it just read.
- [ ] Table-driven tests cover all four server cases above, using mocks from
      `internal/testutil/mocks/` (Fn-field pattern, never inline) and asserting
      sentinels with `errors.Is`.
- [ ] `go test -race ./...`, `make check`, and the `web/` suite
      (`npx tsc --noEmit && npm test && npm run build && npm run lint`) all pass.
- [ ] `docs/SCHEMA.md` and `docs/API.md` updated to reflect the new column and
      the 409 response.

---

## Out of scope

- Concurrency control for purchases, sales, invoices, or any other aggregate.
- Server-side merge / three-way reconciliation. A conflict is reported, not
  resolved.
- Real-time notification of concurrent edits (websockets, polling).
- The `CampaignsPage.tsx` edit-form anomaly noted during the `form-field-a11y`
  work; explicitly deferred by the operator.
