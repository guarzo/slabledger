# Campaign targeting edit path

**Date:** 2026-08-07
**Status:** Approved (design), revised 2026-08-07 after independent review

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

Note the scope of "no direct data entry on the portal side": `deniedSpecs` is a
documented exception. `docs/psa-harvester.md:385-390` defers spec *discovery*
because the portal's spec-catalog modal was never captured, so adding a new
denial is done in the portal and picked up on the next pull — "the one
intentional exception to 'no direct data entry in the portal.'" This design
inherits that exception rather than changing it.

## Backend: one-line fix, otherwise unchanged

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

Everything the edit path needs on the write side already works. One line of Go
does have to change, for a reason that has nothing to do with the form — see
"The harvester writes the same fields" below.

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

**The harvester writes the same fields — and lies about it.**
`cmd/psa-harvest/baseline.go:157-161` assigns `TargetLanguages`,
`SubjectFilterMode`, `Subjects`, and `DeniedSpecs` onto a copy of the campaign
and writes it at `:264`. A baseline pull that reconciles a `-1` subject into a
real portal id can land while an edit form sits open, and saving that stale form
would put the `-1` placeholders back over the reconciled ids.

The guard against that is `updatedAt` — but as written, `updatedAt` cannot see
this. `runBaselinePull` (`:206`) takes an `inventory.CampaignRepository`, not the
service, so it never reaches `service_crud.go:39` where `UpdateCampaign` stamps
`c.UpdatedAt = time.Now()`. `updated := existing` (`:157`) carries the old
timestamp forward untouched, `buildBaselineCampaign` never assigns it, and
`campaign_store.go:314` binds `c.UpdatedAt` verbatim to `$16`. The row's targeting
changes and its `updated_at` does not move.

This is a defect in its own right, independent of this feature: `updated_at` is
supposed to mean "when this row last changed," and for the one writer that
bypasses the service it does not. **The fix is to set
`updated.UpdatedAt = time.Now()` in `buildBaselineCampaign`.** One line, one call
site — `baseline.go:206` is the only direct `CampaignRepository` writer outside
the service wiring in `cmd/slabledger/init_inventory_services.go:165-202`.

Two alternatives were rejected. Routing the harvester through
`inventory.Service` would make `psa-harvest` construct the whole service graph to
gain exactly one timestamp assignment; the comment at `baseline.go:163-169`
explains why it deliberately calls `ValidateAndNormalizeCampaign` itself and
writes through the repository. A SQL precondition
(`WHERE id = $21 AND updated_at = $22`, zero rows affected → conflict) is the only
fully atomic option, but every caller would then have to handle a conflict —
including the harvester, which legitimately wants to win and would need a force
path or a retry loop. That is disproportionate machinery for a single-operator
tool, and the one-line fix does not foreclose it later.

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
worth correcting before it is reopened). Plus the one-line `UpdatedAt` fix in
`buildBaselineCampaign`.

**Out:**
- Any other backend change. No new endpoint, no schema change, no migration.
- Any change to the bulk-paste update format. `CampaignsPage.tsx:43-54`
  documents why targeting is excluded from it; that stays true. The comment is
  updated only where it asserts "there is currently no edit surface for it after
  that," which this change makes false.
- Editing `deniedSpecs`. It remains portal-managed and read-only, exactly as the
  create form renders it (`CampaignFormFields.tsx:294-308`) and as
  `docs/psa-harvester.md:385-390` prescribes until spec discovery ships.
- Any ability to set or repair a subject's portal id by hand.

## Components

| File | Change |
| --- | --- |
| `cmd/psa-harvest/baseline.go` | Set `updated.UpdatedAt = time.Now()` in `buildBaselineCampaign` so a baseline write moves the row's timestamp. |
| `web/src/react/queries/useCampaignQueries.ts` | Add `useUpdateCampaign()`, mirroring `useCreateCampaign()` (`:127`); invalidates `queryKeys.campaigns.all`. |
| `web/src/react/pages/CampaignsPage.tsx` | Own `editingId` state and a second `useForm` seeded from the campaign being edited; own the save handler and the staleness guard; update the paste-format comment. |
| `web/src/react/pages/campaigns/CampaignsTab.tsx` | Per-row **Edit** button; render the edit `CardShell` above the list, reusing `CampaignFormFields` with `showPhase` and `showFees`. |
| `web/src/react/ui/SubjectListEditor.tsx` | Removing a `-1` chip asks for confirmation; the removed name is then blocked from re-entry for the rest of the editor session. |

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
3. On **Save**: fetch the campaign fresh over the network and compare its
   `updatedAt` against `T0`. This must be an explicit refetch, not a read of the
   React Query cache — `useCampaigns` holds data fresh for
   `CAMPAIGN_STALE_TIME = 30_000` (`useCampaignQueries.ts:18`), and the racing
   writer is a separate process whose write the cache cannot observe. If the
   timestamps differ, abort the write and warn that the campaign changed
   underneath the form — naming the harvester baseline pull as the likely cause —
   leaving the form open so the operator can re-open it against fresh data.
   If the refetch itself fails, abort the write too and say the check could not
   be completed; failing open here would reintroduce the race the guard exists
   to close.
4. Otherwise `PUT /api/campaigns/{id}` with `{ ...existingCampaign, ...formValues }`.
   The spread of the full existing campaign is what protects
   `psaCampaignRequestId` and `expectedFillRate` from the full-row `UPDATE`.
   Note this is deliberately *not* the pattern at `CampaignsPage.tsx:311-312`,
   which strips `expectedFillRate` before spreading — that strip is correct for
   the paste path and wrong here.
5. Toast, invalidate `campaigns.all`, close the form. If the campaign has a
   `psaCampaignRequestId`, the toast adds a nudge to publish.

## Legacy unreconciled subjects

A `-1` subject cannot be *repaired* from the UI; a portal id has to come from the
portal via the harvester baseline pull. But repairing and removing are different
acts, and only the first is forbidden. In the edit form:

- **Removing a `-1` chip is allowed, behind an explicit confirmation.** Removal
  does not manufacture an id — it drops targeting. Forbidding it has a real cost:
  a single `-1` refuses the entire campaign's push (`mapper.go:144`), so an
  operator who genuinely no longer wants that subject would otherwise be stuck
  waiting on a harvester run to land an unrelated change. The confirmation is
  there because the act is destructive and its purpose is to unblock a push.
- **Once removed, that name is blocked from re-entry for the rest of the editor
  session** — in the dropdown filter and the Enter-to-add path alike. This is the
  part that preserves the rule. Remove-then-retype *is* the hand repair, and it is
  the one path that can fail silently: `SubjectID` matches names with
  `strings.EqualFold` (`resolver.go:146`), so a retyped name that also belongs to
  a *different* portal subject resolves to that subject and pushes wrong
  targeting — the `-1 Machamp` versus `22210 Machamp` case. Every other outcome is
  loud: `SubjectID` never silently returns `0` for an unresolved name
  (`resolver.go:144-151`), it returns `ErrUnknownSubject`, which surfaces as a 400
  (`campaigns_psa.go:204`).
- **A `-1` chip suppresses the same-named catalog subject**, in both the dropdown
  filter and the Enter-to-add path, so the campaign never holds two entries with
  the same name.

The third behavior is **already implemented** and is not new work here.
`SubjectListEditor.tsx:53-58` and `:66-69` compare by name unconditionally, which
covers `-1` chips (likely fixed by `4515b4ef`, "fix: address CodeRabbit findings
on spec-list targeting"). The request described it as an open wart; it is not.
It becomes a regression test rather than a fix.

**Known limit:** the post-removal block is session-scoped — it lives in
`SubjectListEditor` component state and dies when the form unmounts. Closing and
reopening the form clears it, after which the name can be added at id `0`.
Persisting it would mean backend state to guard against an operator mistake in a
single-operator tool, which is not worth it — but the hole is real and is
recorded here rather than papered over.

The existing amber banner (`:137-143`) stays. Edits to a campaign carrying a `-1`
subject save locally; publishing remains refused by `TranslateToDiff`
(`mapper.go:144`, `ErrLegacySubjectsUnreconciled`), surfacing as a 400 from
`campaigns_psa.go:196-213`, until the harvester reconciles — or until the
operator removes the offending subject.

## Error handling

| Condition | Behavior |
| --- | --- |
| `updatedAt` changed since the form opened | Abort the write; warn, naming the baseline pull; form stays open. |
| Staleness refetch fails | Abort the write; say the check could not be completed; form stays open. |
| Validation 400 from `ValidateAndNormalizeCampaign` | Toast via `getErrorMessage`; form stays open with the operator's values. |
| Campaign carries a `-1` subject | Amber banner persists; save succeeds; publish is refused downstream until reconciled or the subject is removed. |
| Network/5xx | Toast; form stays open. |

## Testing

| Suite | Assertion |
| --- | --- |
| `cmd/psa-harvest` (Go) | `buildBaselineCampaign` returns a campaign whose `UpdatedAt` is newer than the one it was given. |
| `CampaignsTab.test.tsx` | Clicking **Edit** seeds the form from the campaign's current values, including the three axes. |
| `CampaignsTab.test.tsx` | Save sends a body that preserves `psaCampaignRequestId` **and** `expectedFillRate`. |
| `CampaignsTab.test.tsx` | Existing positive subject ids round-trip through save byte-for-byte. |
| `CampaignsPage` | A campaign whose `updatedAt` moved since the form opened aborts the `PUT`. |
| `CampaignsPage` | The staleness check issues a network fetch and does not satisfy itself from cached campaign data. |
| `CampaignsPage` | A failed staleness refetch aborts the `PUT`. |
| `SubjectListEditor.test.tsx` | Removing a `-1` chip requires confirmation; declining leaves it in place. |
| `SubjectListEditor.test.tsx` | After a confirmed `-1` removal, that name cannot be re-added in the dropdown or on Enter. |
| `SubjectListEditor.test.tsx` | A `-1` chip suppresses the same-named catalog subject in the dropdown and on Enter. |

Gates: `npm run typecheck` (`tsc --noEmit` — `npm run build` does not type-check),
`npm test`, and `go test ./...` — the last now covers a real backend change, not
just a no-regression check.

## Assumptions

- The **Edit** button appears on all three phases. A closed campaign's targeting
  is still worth correcting before reopening.
- Client-side optimistic concurrency plus an honest `updated_at` is sufficient.
  It is not atomic: a harvester write landing between the staleness refetch and
  the `PUT` still wins silently. That window is milliseconds, against the minutes
  a form sits open, and closing it entirely means a SQL precondition every caller
  must handle. Revisit if the harvester ever runs continuously rather than
  periodically.
- Blocking a removed `-1` name only for the editor session is enough friction to
  stop the accidental hand repair. It does not stop a determined one.

## Review history

Codex reviewed this spec against the repository on 2026-08-07 and returned
REVISE. Its blocking finding — that the `updatedAt` guard could not observe the
harvester write it was built to catch — was correct and is fixed above by the
`buildBaselineCampaign` change. Its cache-staleness and legacy-removal findings
are also incorporated. Its scope finding on `deniedSpecs` was downgraded to a
citation: `docs/psa-harvester.md:385-390` already sanctions that exception.
