# Campaign targeting edit path

**Date:** 2026-08-07
**Status:** Approved (design)

## Problem

PR #538 gave `inventory.Campaign` three independent targeting axes —
`targetLanguages` (empty = open net), `subjectFilterMode` + `subjects`, and
`deniedSpecs` — so SlabLedger is authoritative and pushes accurate campaigns to
the PSA portal with no direct data entry on the portal side.

`web/src/react/ui/CampaignFormFields.tsx` has exactly one mount: the create form
at `web/src/react/pages/campaigns/CampaignsTab.tsx:96`. Neither `CampaignsTab`
nor `CampaignsPage` has an edit handler. Once a campaign exists, its targeting is
unreachable from the UI, so the only way to change it is the portal — the exact
workflow #538 was built to eliminate.

## Backend: verified, no change needed

The user's item 1 asked for confirmation rather than assumption. Confirmed at
three layers:

1. **Validation.** `internal/domain/inventory/service_crud.go` — `CreateCampaign`
   (`:11`) and `UpdateCampaign` (`:35`) both call the same
   `ValidateAndNormalizeCampaign(c)`. The axes validate identically on update.
2. **Persistence.** `internal/adapters/storage/postgres/campaign_store.go:295-329`
   — the `UPDATE campaigns SET …` statement carries all four columns
   (`target_languages`, `subject_filter_mode`, `subjects`, `denied_specs`) as
   `$17`–`$20`.
3. **Translation.** `internal/domain/psacampaign/mapper.go:62-95` — `TranslateToDiff`
   already emits `subjectFilterType`, `selectedSubjects`, `deniedSpecs`, and
   `prepackagedSpecListIds`. An edited campaign diffs against the portal with no
   new mapper work.

This is a frontend-only change.

## Two hazards the request did not name

Both are consequences of existing backend semantics, and both shape the design.

**Full-row PUT.** `HandleUpdateCampaign`
(`internal/adapters/httpserver/handlers/campaigns.go:186-212`) decodes a whole
`inventory.Campaign` from the body and the SQL above sets every column. A field
omitted from the request body decodes as its zero value and is written as such.
Omitting `psaCampaignRequestId` silently unlinks the campaign from the portal;
omitting `expectedFillRate` zeroes an analytics input. The edit path must
therefore send the full existing campaign with form values layered on top —
never form values alone.

**The harvester writes the same fields.** `cmd/psa-harvest/baseline.go:158-161`
assigns `TargetLanguages`, `Subjects`, and `DeniedSpecs` on a campaign and calls
the same `UpdateCampaign` (`:264`). A baseline pull that reconciles a `-1` subject
into a real portal id can land while an edit form sits open; saving that stale
form would write the `-1` placeholders back over the reconciled ids. Hence the
concurrency guard below.

## Design decision (item 5): local save, separate push

Editing saves locally only. It does **not** auto-propose or auto-publish.

The reasoning is that the diff machinery already exists and already covers all
three axes — `PSAPublishModal` renders `subjectFilterType`, `selectedSubjects`,
`deniedSpecs`, and `prepackagedSpecListIds` from `TranslateToDiff`. Routing the
edit through the propose flow would duplicate an approval gate rather than add
one. Keeping them separate preserves a single, well-understood publish path:
edit → save → operator opens the PSA modal → *Check for changes* → *Publish*.

After a successful save on a campaign that carries a `psaCampaignRequestId`, the
success toast nudges the operator to publish. It does not open the modal for
them; publishing stays an explicit act.

## Scope

**In:** a UI edit path for all campaign fields the create form already exposes,
including the three targeting axes, reachable from every campaign row on all
three phases (active, pending, closed — a closed campaign's targeting is still
worth correcting before it is reopened).

**Out:**
- Any backend change.
- Any change to the bulk-paste update format. `CampaignsPage.tsx:43-54`
  documents why targeting is excluded from it; that stays true. The comment is
  updated only where it asserts "there is currently no edit surface for it after
  that," which this change makes false.
- Editing `deniedSpecs`. It remains portal-managed and read-only, exactly as the
  create form renders it (`CampaignFormFields.tsx:294-308`).
- Any ability to set or repair a subject's portal id by hand.

## Components

| File | Change |
| --- | --- |
| `web/src/react/queries/useCampaignQueries.ts` | Add `useUpdateCampaign()`, mirroring `useCreateCampaign()` (`:127`); invalidates `queryKeys.campaigns.all`. |
| `web/src/react/pages/CampaignsPage.tsx` | Own `editingId` state and a second `useForm` seeded from the campaign being edited; own the save handler and the staleness guard; update the paste-format comment. |
| `web/src/react/pages/campaigns/CampaignsTab.tsx` | Per-row **Edit** button; render the edit `CardShell` above the list, reusing `CampaignFormFields` with `showPhase` and `showFees`. |
| `web/src/react/ui/SubjectListEditor.tsx` | `-1` chips lose their remove control. |

`CampaignFormFields.tsx` itself needs no change. Its props already include
`showPhase` and `showFees`, and `EconomicsSection` (`:87-92`) already re-syncs
its local inputs from props, which is what seeding an edit form requires.

`web/src/js/api/campaigns.ts:53` already exposes
`updateCampaign(id, data: Partial<Campaign>)` → `PUT /campaigns/{id}`.

## Data flow

1. Operator clicks **Edit** on a row. The form seeds from that campaign's current
   values and captures its `updatedAt` as `T0`.
2. Operator edits. Existing portal ids on `subjects` ride through the form
   untouched — `SubjectRef` carries `{ id, name }` and nothing re-derives an id
   from a name. Only an operator-typed subject (id `0`) is resolved by name at
   push time.
3. On **Save**: re-read the campaign list, compare its `updatedAt` against `T0`.
   If they differ, abort the write and warn that the campaign changed underneath
   the form — naming the harvester baseline pull as the likely cause — leaving
   the form open so the operator can re-open it against fresh data.
4. Otherwise `PUT /api/campaigns/{id}` with `{ ...existingCampaign, ...formValues }`.
   The spread of the full existing campaign is what protects
   `psaCampaignRequestId` and `expectedFillRate` from the full-row `UPDATE`.
   Note this is deliberately *not* the pattern at `CampaignsPage.tsx:311-312`,
   which strips `expectedFillRate` before spreading — that strip is correct for
   the paste path and wrong here.
5. Toast, invalidate `campaigns.all`, close the form. If the campaign has a
   `psaCampaignRequestId`, the toast adds a nudge to publish.

## Legacy unreconciled subjects

A `-1` subject cannot be repaired from the UI; the id has to come from the portal
via the harvester baseline pull. In the edit form this means:

- A `-1` chip renders with no `×`. It cannot be removed, so an operator cannot
  "fix" one by deleting it and re-adding the same name at id `0` — which would
  push a name-resolved subject in place of one the portal has not yet reconciled.
- A `-1` chip suppresses the same-named catalog subject, in both the dropdown
  filter and the Enter-to-add path, so the campaign never ends up holding two
  entries with the same name.

The second behavior is **already implemented** and is not new work here.
`SubjectListEditor.tsx:53-58` and `:66-69` compare by name unconditionally, which
covers `-1` chips (likely fixed by `4515b4ef`, "fix: address CodeRabbit findings
on spec-list targeting"). The request described it as an open wart; it is not.
It becomes a regression test rather than a fix.

The existing amber banner (`:137-143`) stays. Edits to a campaign carrying a `-1`
subject save locally; publishing remains refused by `TranslateToDiff`
(`mapper.go:144`, `ErrLegacySubjectsUnreconciled`), surfacing as a 400 from
`campaigns_psa.go:196-213`, until the harvester reconciles.

## Error handling

| Condition | Behavior |
| --- | --- |
| `updatedAt` changed since the form opened | Abort the write; warn, naming the baseline pull; form stays open. |
| Validation 400 from `ValidateAndNormalizeCampaign` | Toast via `getErrorMessage`; form stays open with the operator's values. |
| Campaign carries a `-1` subject | Amber banner persists; save succeeds; publish is refused downstream until reconciled. |
| Network/5xx | Toast; form stays open. |

## Testing

| Suite | Assertion |
| --- | --- |
| `CampaignsTab.test.tsx` | Clicking **Edit** seeds the form from the campaign's current values, including the three axes. |
| `CampaignsTab.test.tsx` | Save sends a body that preserves `psaCampaignRequestId` **and** `expectedFillRate`. |
| `CampaignsTab.test.tsx` | Existing positive subject ids round-trip through save byte-for-byte. |
| `CampaignsPage` | A campaign whose `updatedAt` moved since the form opened aborts the `PUT`. |
| `SubjectListEditor.test.tsx` | A `-1` chip renders no remove control. |
| `SubjectListEditor.test.tsx` | A `-1` chip suppresses the same-named catalog subject in the dropdown and on Enter. |

Gates: `npm run typecheck` (`tsc --noEmit` — `npm run build` does not type-check),
`npm test`, and `go test ./...` to confirm the backend is untouched.

## Assumptions

- The **Edit** button appears on all three phases. A closed campaign's targeting
  is still worth correcting before reopening.
- Client-side optimistic concurrency via `updatedAt` is sufficient. A server-side
  guard would need a backend change, which is out of scope; the race window here
  is an operator leaving a form open across a harvester run, and a clear abort
  covers it.
