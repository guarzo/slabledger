# Show preparation from prepared data

**Status: approved for local implementation following independent SHIP design review.**
Baseline: deployed merge `3baaed37` (PR #706). This document supersedes the browser-owned acquisition portions of the two previous show-preparation designs. The user approved continuing locally; push, production backfill, deployment, and production access remain separately gated.

## 1. The product contract

**Find cards whose recent sales support their DH price, select them, and save a packing list. Preparing a show is not a data-refresh workflow.**

The operator's path is:

1. Open inventory. Existing cached data and support results load with the inventory view.
2. Optionally filter to Supported. This filters the loaded evaluations.
3. Use the existing card checkboxes. Selection is immediate and does not make a request.
4. Choose **Add to show** in the existing selected-items action bar. Choose an existing list or name a new one, then save.
5. Open that list and pack its cards.

There is no prerequisite Show selection mode. No Checking comps progress counter, Cancel checking, Check selected, or Retry comps control appears in inventory or packing. Opening evidence displays stored sales, not a provider lookup. Selection, filtering, sorting, navigation, opening evidence, adding to lists, and packing must cause **zero CardLadder requests**.

The application can make ordinary authenticated database-backed reads and explicit list writes. Those must not wait for, start, resume, or cancel a comp job. Existing durable price-association safety observations remain; “read-only” does not mean removing those safeguards.

### What the screen shows

Keep the existing inventory layout, copper theme, typography, filters, and row/card structure. Extend the existing selection bar rather than creating a second selection experience. Existing sale and DH actions retain their behavior.

Support has explicit price context: **DH listed $300 · Supported**. Reviewed price and CL/Market valuation remain separately labeled values. Expanding evidence shows the stored median, sales count, dates, source, and acquisition time.

Keep market outcomes separate from data availability:

| Condition | Display / filtering behavior |
|---|---|
| Current, complete evidence | Supported, Thin evidence, Below target, or No recent sales according to the existing evaluator. |
| Never successfully collected | No comp data. Not a Supported match; still selectable for a manual shortlist. |
| Old verified evidence | Out of date. Last successful sales remain inspectable with their date; not presented as current Supported evidence. |
| Latest due collection failed or was incomplete | Data unavailable, with the last successful evidence retained and dated. No per-card acquisition prompt. |
| No positive DH listing price | No DH price. Collecting comps cannot repair the missing price, so never say “select to check manually.” |
| Unresolved identity / ambiguous price association | Needs matching / Price association unclear. Preserve the existing hold and conservative qualification. |

These are presentation mappings over existing evaluation/readiness data, not permission to relabel unknown evidence as supported. A concise coverage notice can explain incomplete data once per view. It must distinguish current evidence, missing evidence, failed/stale evidence, and missing prices; “last updated” alone must not imply the entire inventory is ready.

A zero-match Supported view says there are no current matches under the chosen filters and separately identifies incomplete coverage. It does not start a job or force the operator to clear their other filters.

## 2. Who prepares the data

Use **one evidence-only server worker**, built on existing scheduler machinery and the existing verified source/store:

```text
Inventory identity enrichment ─┐
                              ├─> server worker ─> CardLadder ─> verified snapshots
Scheduled due-state scan ──────┘                                  │
                                                                  v
Current saved DH prices ───────────────────────────────> cached evaluation reads
                                                                  │
                                                     inventory / evidence / shows
```

There is deliberately **no arrow from a checkbox, filter, page mount, or list action to the worker**.

Do not attach this task to successful CL valuation or collection synchronization. Those paths have independent prerequisites, side effects, and early returns. Do not create a generic queue platform. Reuse the existing scheduler loop/configuration/shutdown conventions, source adapter, evidence generations, evaluator, and list storage. Add only the missing ownership, candidate selection, retry bookkeeping, and health integration.

### Worker lifecycle and scope

- Start with application scheduling, independently of browser activity. Perform an immediate due scan, then scan once per minute while enabled and configured.
- Scope is unsold, non-refunded purchases in non-closed campaigns, including not-yet-received cards. Deduplicate by normalized profile, grader, and grade. Browser filters and selected IDs are not inputs.
- Missing DH price does not prevent background comp collection. A later DH-price update can then be evaluated immediately from stored sales.
- Unresolved identities are reported as unavailable until existing identity enrichment supplies the required fields. That handoff must persist a verified profile on the purchase for both new and cached matches; the current cached-mapping resolver path needs explicit coverage so a known match cannot remain permanently absent from worker candidates. The worker does not guess a match or depend on CL collection membership.
- Fetch missing and expired evidence, not every card on every tick. A complete zero-sale result is a successful cache entry and is not refetched merely because its sale count is zero.
- Reconsider changed/new identities at the next scan. Recompute support against a changed stored DH price without fetching new comps.
- Fetch one identity at a time, reusing the current configured client/pacing and the existing five-page source bound, not constructing a fresh independently limited client for each batch. Limit each sweep to five minutes; remaining work carries into later scans in oldest-due order. No alphabetical first-N starvation.
- Give each lookup a bounded source context, followed by a separately reserved persistence budget, both children of the worker run. Application shutdown or lost job ownership cancels the run. No browser/request context owns this work.

### Freshness and retries

Keep the approved matching and qualification rules: exact card/variant/grader/grade; 30 UTC dates including today; verified complete coverage; at most 24 hours old; current UTC window; integer-cent median; at least two eligible matching sales and a median at least 90% of the current DH listed price for Supported. One eligible matching sale meeting that price threshold yields Thin evidence; a median below the threshold yields Below target.

This means yesterday's window expires at UTC midnight even if it is less than 24 hours old. The next worker scan renews it. During that catch-up interval, the UI shows dated stale evidence, not false freshness or a user-owned progress task. This design does **not** silently relax that rule to conceal a scheduling gap.

Proposed bounded failure policy: three automatic failed attempts per identity per UTC window, with retries no sooner than 15 minutes and then 60 minutes after failure. Persist the count/window and next-attempt time so redeploys do not reset retry limits. Missing configuration reports Unconfigured without attempting each card. Authentication rejection puts the worker on a persisted hold until credentials change or an administrator explicitly resumes it; it must not move on to failing another card every minute. Successful current evidence is not routinely rechecked before expiry.

The worker must treat incomplete source results as failures even if the adapter returned no transport error. It must never overwrite the last successful sale payload with an incomplete result or report an incomplete sweep as healthy.

### Single ownership

Multiple app instances or an operational Run now request must not start competing sweeps. Add a small PostgreSQL-backed worker lease with an owner token and expiry. Renew it while running, cancel on loss, and require the same valid ownership at publication. Retain existing per-identity attempt-generation fencing as well.

This is a bounded addition, not an existing guarantee: scheduler stats and process-local mutexes are not distributed ownership. Do not hold the existing show-list/publication transaction lock across provider calls. Database work remains short; network work happens outside transactions.

## 3. Cache and interface boundaries

Preserve `showprep_evidence` and its verified snapshots, existing lists/items, observed versions, and ambiguity holds. No financial schema or pricing calculation changes.

Reuse identity-oriented acquisition/persistence logic behind a narrow worker port. Do not make the worker simulate batches of selected purchase IDs through the HTTP refresh handler. The UI-facing service gets read/evaluate/list capabilities, **not a provider source capability**.

Remove the inventory/packing refresh calls. Retire `/api/show-prep/refresh` as a browser acquisition route, returning an authenticated JSON HTTP 410 response with no source work. This is an intentional compatibility change for stale clients and old direct-refresh scripts; operational refresh moves to the separate admin worker action. An already-started old-deployment request is bounded by shutdown/request deadlines, not adopted by a new browser session.

Keep authenticated evaluation/evidence/list reads and existing list-write contracts. Readiness remains useful display metadata; remove browser acquisition eligibility, quota, retry, and ownership machinery from its callers. Preserve useful transport cancellation and unrelated authentication/session fixes rather than blindly reverting the previous PR.

A small migration is expected for worker lease and durable retry metadata. Successful evidence payloads and business fingerprints need not change. There is no new generic task table populated by selections, and no duplicate replacement comp store.

## 4. Stable selection and packing

Checkboxes and filters use the already-loaded inventory/evaluation view. Add to show uses the existing observed-version contract. Missing or stale comp evidence is not a reason to prohibit a physically eligible manual shortlist.

Background publication cannot clear selections, silently acknowledge newer data, or make selected rows jump away. Retain selected IDs and their observed versions. A read-only view update may show that data changed; the operator reviews those changes before saving. If a required evaluation was not loaded, obtaining it is a database read, never a source fetch.

On Add/Pack, the server checks current purchase/campaign availability and the submitted version within the existing transaction. Changed inputs produce a specific conflict with the selection retained. Nothing is silently added at a new baseline. Sold/deleted/refunded/closed items remain protected. Not-received cards can be planned but not packed. Weak evidence does not become a blanket prohibition on manual packing.

The worker does not acquire a browser edit lock, and financial operations do not wait for it. It writes evidence, not purchase prices or availability. Version checks and existing transactions, not a tab-wide source/write exclusion mechanism, protect list actions. Remove source-only write leases and blockers after verifying their callers; preserve actual form pending-state and financial-write behavior.

Packing remains the existing saved-list workflow. No new pricing system, reservation system, or populated-list redesign is part of this recovery.

## 5. Operations and failure visibility

Reuse the existing Admin integrations surface and scheduler-stat storage, adding a **separate evidence-worker status** rather than presenting CL pricing success as evidence success.

Show enabled/unconfigured/idle/running/failed state, current/eligible identity counts, missing/stale/failed/unresolved counts, last sweep finish, and retry due time. Report completed-with-errors honestly. A recent success for one card is not complete inventory coverage.

An admin-only **Run now** action wakes this same leased worker and respects due-state limits. It accepts no selected-card cohort, returns promptly, and does not invoke the full CL price/collection refresh. An explicit **Retry failed** action may reset exhausted retry/backoff state for one bounded sweep after an operational repair; it does not enable a retry loop or refresh already-current identities. These are new worker-specific capabilities, not claims that the current admin refresh already does this.

Inventory may read a small coverage summary. It does not display the worker's Cancel/Retry controls and does not require the operator to visit Admin before using lists. Credential availability must be checked at run time so first-time configuration can activate the worker without a stale nil-source construction.

Provider outage: preserve dated data, report the operational failure, obey retry limits, and leave selection/list/packing usable. Never manufacture green badges or “0 sales” from unavailable evidence.

## 6. Alternatives and tradeoffs

- **Recommended: independent evidence-only worker using existing building blocks.** Adds real scheduling ownership/retry state, but removes browser job management and avoids coupling show data to pricing or CL collection sync.
- **Rejected: fetch on selection, Support filter, or page entry.** It makes the operator pay acquisition latency and creates navigation/selection/cancellation dependencies. Moving the same trigger one click earlier is not a fix.
- **Rejected as a shortcut: certify the existing legacy comp aggregate.** It has different windows and lacks required completeness/provenance. That would change the meaning of Supported without permission.
- **Not in this recovery: replace all legacy comp ingestion with a unified raw-sales platform.** It may eventually eliminate duplicated provider work, but would widen this repair considerably. Existing legacy and verified stores retain their distinct meanings; disclose and bound the extra acquisition load.

## 7. Delivery and proof

### Order

1. Remove UI acquisition triggers and per-selection refresh controls. Preserve the cached-data browsing/list path. This is containment, not a claim that empty-cache readiness is solved.
2. Implement and verify the server worker, durable ownership/retry state, and honest operational coverage using the current verified store/source.
3. Add **Add to show** to the existing selection experience; remove the separate mode and source-only coordinator coupling. Verify existing financial actions are unchanged.
4. After separate deployment approval, perform an initial server-owned catch-up and verify coverage before claiming the feature ready. Do not backfill by opening inventory or clicking cards.

### Acceptance tests that decide whether it works

1. **No browser present:** start from legacy comps plus an empty verified store. The worker alone obtains and persists evidence. New resolved inventory becomes due without any page being opened.
2. **Provider unavailable during use:** preload verified evidence, disable the worker, make every provider endpoint fail, then open inventory, filter Supported, select, inspect, create/add and pack. The workflow succeeds from cached data and explicit list writes. Provider call count remains **exactly zero**.
3. **Pure selection:** selecting, clearing, selecting all and changing filters send no acquisition requests and do not start a job. Cached filtering/selection should respond within 100 ms for the 155-card case; no network-dependent spinner is allowed on these actions.
4. **Correct worker boundaries:** missing/current/zero-sales/stale/failed/unresolved cases, UTC rollover, three-attempt retry limits, late credential configuration, restart, two worker instances, lease loss, cancelled source and stale publication. One bad identity cannot starve the rest.
5. **Concurrent publication:** change evidence or availability while cards are selected. Selection and packing history remain intact; changed-version Add/Pack returns the existing conflict, not silent rebinding or a hidden live lookup.
6. **Real integration and real use:** use actual Go routes/services/PostgreSQL/source adapter for acquisition tests, not mocked evaluation answers. Separately verify the cached UI path with source access blocked, desktop/mobile geometry, keyboard use, and unchanged financial rows.
7. **Authorized deployment check:** confirm the worker prepares the real inventory with no browser open, then verify the real browser performs zero comp-acquisition requests during selection. Unit-test totals or a review verdict are not substitutes for this check.

## 8. Evidence, scope, and approval

Repository findings that constrain this proposal:

- `web/src/react/pages/campaign-detail/InventoryTab.tsx`: currently activates acquisition for `selecting || support !== 'all'`; also already uses shared selected IDs and captured versions.
- `web/src/react/pages/campaign-detail/inventory/InventorySelectionBar.tsx` and `web/src/react/pages/show-preparation/ShowSelectionBar.tsx`: existing bulk selection and destination mechanics can be reused; a new selection system is unnecessary.
- `internal/adapters/scheduler/cardladder_gap_fill.go` and `internal/adapters/storage/postgres/comp_refresh.go`: legacy gap fill uses page zero, hardcoded PSA, sale-date staleness and a first-200 cutoff.
- `internal/adapters/storage/postgres/cl_sales_store.go`: legacy summaries use 90 days and do not record complete current-window acquisitions or successful empty lookups.
- `internal/adapters/scheduler/cardladder_refresh.go`: daily pricing/collection scheduling has gates and early returns before comp work; it is not an evidence-readiness guarantee.
- `internal/domain/showprep/types.go`, `repository.go`, and `service_refresh.go`: verified snapshot/source/store building blocks exist, but current orchestration is purchase/request oriented.
- `internal/adapters/clients/cardladder/showprep.go` and `internal/adapters/storage/postgres/showprep_evidence.go`: qualified traversal and attempt-generation publication already exist and should be reused.
- `internal/adapters/scheduler/loop.go`, `internal/adapters/storage/postgres/scheduler_stats_store.go`, and Admin integrations: scheduler/health infrastructure exists; evidence-specific status and distributed ownership do not.
- `internal/adapters/scheduler/cardladder_resolve.go`: the cached-mapping path can return before per-purchase identity backfill, so coverage must include that handoff rather than assume every known match is persisted.

No global re-theme, financial repricing, automatic selling/delisting, forced refresh on item interaction, evidence-rule relaxation, or unrelated transport rewrite. Product metadata contains older indigo wording; current DESIGN/source and the existing copper UI remain authoritative.

**Approved decisions:** cached-only operator workflow; independent server-owned population; existing qualification rules; bounded worker policy and its small persistence change. If implementation cannot meet them without materially broader infrastructure, stop and revise this design rather than hiding that expansion in a fix loop.
