# Cached Show Preparation Implementation Plan

> **For agentic workers:** Use subagent-driven-development, one implementer and a scoped review per task. Steps use checkbox syntax. No production operation is authorized by this plan.

**Status:** Local implementation and verification complete; final scoped code review SHIP at `5ce621c5` (2026-09-16). Push, deployment, production catch-up and production verification remain separately gated. Final review required one additional price-only-notice correction; its 16-case Chromium matrix passes.

**Goal:** Prepare evidence without a browser; let the operator select and pack from cached data with the provider unreachable.

**Architecture:** Retire browser acquisition and reuse the existing selected-items workflow. Add an evidence-only, application-owned worker using the existing verified source/store, a fenced PostgreSQL lease, durable due/retry state and existing scheduler machinery. Keep list transactions and qualification unchanged.

**Tech Stack:** Go 1.26, React 19/TypeScript, PostgreSQL, existing CardLadder client and TanStack Query. No application dependencies.

**Spec:** `docs/specs/2026-09-15-cached-show-preparation-design.md` (approved after independent SHIP design review).

## Local completion status (Task4 handoff)

| Task | Status |
|---|---|
| 1, cached operator containment/UI | Implemented and independently reviewed before Task4 |
| 2, fenced worker/store/migration47 | Implemented and independently reviewed before Task4 |
| 3, production composition/admin/identity | Implemented and independently reviewed before Task4 |
| 4, final product proof/docs | Local implementation and verification complete; parent whole-branch review still required |
| Deployment/backfill/production verification | Not authorized or performed; rollout incomplete |

Task1–3 checklists below are retained as the original execution plan, not new work.
Task4 evidence and exact commands are in `implementation-notes.md` and the real-wire
guide. Its separate seeded-cache and actual-worker modes must not be conflated.

## Global Constraints

- Every inventory/selection/filter/evidence/list/packing action causes zero CardLadder requests. A checkbox causes no request at all. Database-backed reads and explicit list writes remain allowed.
- No separate Show selection mode, acquisition progress, Cancel checking, Check selected, or Retry comps controls in inventory/packing.
- Exact normalized card/profile, grader and grade; complete 30 UTC dates including today; current window; at most 24 hours old. Integer cents and median >=90% of the current saved DH listed price; >=2 eligible matching sales for Supported, one meeting threshold for Thin evidence. Do not certify legacy aggregates.
- Preserve lists, packing history, observed versions, financial state, ambiguity holds, auth/session fixes and financial pending-state behavior.
- Worker: immediate startup scan, one-minute tick, one identity at a time, existing shared client/pacing and five-page bound, five-minute sweep budget, fair oldest-due continuation.
- Three automatic attempted/failed acquisitions per identity/window maximum; retries after 15 minutes then 60 minutes. Charge a started attempt so process death cannot bypass the limit. Current successful entries need no further same-window attempts. Admin Retry failed permits one explicit bounded reset/sweep.
- Source attempt budget <=60 seconds with 5 seconds reserved for persistence, both within the worker run. Lease TTL30 seconds, renewal every10 seconds; database time owns lease validity. Cancellation/lease loss prevents publication and subsequent acquisition.
- Persist job-level authentication hold until explicit credential configuration change or admin retry. Never cycle through the fleet on an auth error. No secret/error URL persistence or exposure. Explicit repair advances the lease epoch so late old-credential auth/status/publication cannot re-block the repaired configuration; normal run intent survives heartbeat and health updates.
- Coverage units are explicit: eligible/current/missing/stale/failed **identities**, and eligible/current/unresolved **cards**. Eligible identities are resolved normalized identities; eligible cards include unresolved physically scoped inventory. Never mix these denominators.
- New/changed persisted identities become due independently of UI. Include unsold non-refunded/non-closed-campaign inventory, even without DH price or receipt; unresolved identities remain honestly unavailable.
- Retire authenticated `POST /api/show-prep/refresh` with JSON410, no source work. Admin actions enqueue/wake the worker, never attach its lifetime to an HTTP request.
- Domain cannot import adapters or another inventory sibling. Source ideally <500 lines, hard limit600. No unrelated refactor, global transport fix, repricing, legacy ingest replacement or re-theme.
- Local reversible implementation/commits only. No push, deployment, production source calls or production backfill. Never use port4173, `make screenshots`, default developer DB or unqualified database variables.

## Files and ownership

1. Task1 owns existing inventory/show UI and the retired route; establishes cached-only browser proof. It does not build a worker or fake fresh market data.
2. Task2 owns domain worker orchestration and PostgreSQL lease/due/publication primitives plus migration000047. Public contracts below feed Task3.
3. Task3 owns scheduler/runtime/credentials/admin coverage and verified identity handoff. It consumes Task2 contracts, not a duplicate acquisition implementation.
4. Task4 owns the final real-worker/no-browser and provider-blocked UI integration proof, migration/security verification and final documentation. It does not reopen unrelated UI design.

### Task 1: Read-only operator workflow and containment

**Files:** modify `cmd/slabledger/showprep.go`, `internal/adapters/httpserver/handlers/showprep.go`, affected actual-router/handler tests; `web/src/react/pages/campaign-detail/InventoryTab.tsx`, `inventory/InventorySelectionBar.tsx`, `inventory/useInventoryState.ts`, existing show-preparation controls/selection/evidence/list files, `ShowPreparationPage.tsx`, show query/API modules and actual callers of `showRefreshCoordinator`. Modify existing `cmd/slabledger/showprep_readiness_*_test.go` and `web/tests/show-readiness-real.cjs` to support a cached-only test mode. Add `web/tests/show-cached-workflow.spec.ts` or equivalent actual-backend driver and focused UI tests.

**Interfaces:** preserve evaluate/evidence/list HTTP DTOs and observed-version writes. The only intentional removed route behavior is provider acquisition through `/refresh`; return JSON410. Reuse the existing shared selected IDs and captured versions, and expose Add to show from the existing bulk action bar. Keep worker-independent cached reads and useful full-body read safeguards.

- [x] Write failing actual-router tests with a counting source. A request to `/refresh` must return410 and leave source calls/evidence/list/financial rows unchanged; unauthenticated access remains rejected. Implementation shape:

```go
func (h *ShowPrepHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
    writeError(w, http.StatusGone, "Comp collection is server-managed; reload the application")
}
```

The handler-facing interface must not require Refresh; `buildShowPrepHandler` supplies no source. Remove route-only deadline tests when their behavior is deliberately retired, not unrelated source cancellation tests.

- [x] Add UI RED assertions using actual InventoryTab and selection actions. Select/filter/open evidence/add/pack while all source-acquisition routes are rejected. Use the existing API fixtures only for focused component tests; the real-backend check below must not intercept evaluate/list results.

```ts
await page.getByRole('checkbox', { name: /Select/ }).first().check();
await expect(page.getByRole('button', { name: /Add to show/ })).toBeEnabled();
await expect(page.getByRole('button', { name: /Check selected|Cancel checking/ })).toHaveCount(0);
expect(acquisitionRequests).toEqual([]);
```

`acquisitionRequests` is the driver's request log filtered to the retired refresh route; also assert the real local source's counter remains zero. Count checkbox-triggered requests separately from initial/focus/expiry reads.

- [x] Implement one selected-items bar and progressive destination choice. Remove separate selecting mode and source controls, not saved-list or financial actions. Preserve source-price context, data-state labels, hidden selection recovery, keyboard/escape behavior and stable virtualization.
- [x] Remove the acquisition coordinator and its source-only callers/leases after repository-wide reference search. Preserve form-owned pending state and original API rejection/notification ownership. Retain narrow read-only focus/expiry observation if needed; it cannot be activated by a checkbox or trigger acquisition. Do not revert unrelated QueryProvider/Auth/read-transport fixes.
- [x] Update obsolete automatic-acquisition tests to assert the new contract; retain relevant version/cancellation/order/history regressions. A lower test count is not itself a failure when explicitly retired behavior is documented, but deleting unrelated safety coverage is.
- [x] For the actual-backend milestone, seed valid verified snapshots as a test precondition, disable/block the source, use the real router/service/Postgres and complete selection/create/add/pack. This proves cached use, not worker population. The old warm-up test mode is replaced by Task4, not claimed passing unchanged.
- [x] Run focused tests, full frontend/typecheck/lint/build, full UTC Go-race and compatible make check; commit the independently usable containment/UI change. Report actual source-request counters and financial invariants, not screenshots alone.

### Task 2: Fenced evidence worker and durable due state

**Files:** create `internal/domain/showprep/worker.go`, `worker_types.go`, `worker_test.go` and focused state tests; create `internal/adapters/storage/postgres/showprep_worker.go`, `showprep_worker_lease.go`, `showprep_worker_test.go`, `showprep_worker_lease_test.go`; migration `000047_showprep_worker.up.sql` and `.down.sql`; modify `showprep_evidence.go` only to share publication machinery without weakening old generation checks. Source and pure evaluation logic remain reused.

**Produces:** domain `EvidenceWorker`, `WorkerStore`, `WorkerStatus`, and `SourceProvider`. Concrete names/signatures consumed by Task3:

```go
type SourceProvider func(context.Context) (Source, error)
type WorkerStatus struct {
    State string `json:"state"`
    EligibleIdentities int `json:"eligibleIdentities"`
    CurrentIdentities int `json:"currentIdentities"`
    MissingIdentities int `json:"missingIdentities"`
    StaleIdentities int `json:"staleIdentities"`
    FailedIdentities int `json:"failedIdentities"`
    EligibleCards int `json:"eligibleCards"`
    CurrentCards int `json:"currentCards"`
    UnresolvedCards int `json:"unresolvedCards"`
    LastSweepAt string `json:"lastSweepAt"`
    RetryAt string `json:"retryAt"`
    Error string `json:"error"`
}
// Proposed new constructor; UUID factory keeps domain ownership generation injectable.
func NewEvidenceWorker(store WorkerStore, source SourceProvider, now func() time.Time, newOwner func() string) *EvidenceWorker
func (w *EvidenceWorker) RunOnce(context.Context) error
func (w *EvidenceWorker) Status(context.Context) (WorkerStatus, error)
func (w *EvidenceWorker) RequestRun(context.Context, bool) error // bool is explicit retry-failed intent
func (w *EvidenceWorker) CredentialsChanged(context.Context) error
```

WorkerStore is a narrow domain-owned interface for candidate reads, fenced job ownership, begin/finish attempts, due/retry/auth state, and coverage. Task2 defines its exact method signatures with tests and records them in its report before Task3 uses them. It must not expose purchase financial writes or CL mapping membership. Its PostgreSQL constructor is `NewShowPrepWorkerStore(*sql.DB)`.

- [x] Write RED integration tests with two independent database connections/worker instances. Assert one lease owner, renewal/release, expiry takeover and rejection of every old owner's begin/finish/health update. Use database time for ownership, injected domain time for window qualification. Test actual shared publication methods, not copied lease logic.
- [x] Add the bounded schema: one worker state/lease record with owner token, epoch, expiry, durable request/auth flags and coverage timing; evidence retry window/count/not-before fields keyed by existing normalized identity. Keep payload/business fingerprints unchanged. Mirror migration46's service_role-only grants/RLS protections; test up/down and existing data retention on a disposable DB. No generic queue table.
- [x] Implement short transactional ownership and publication checks. Lock the lease row while validating epoch/expiry and publishing a result; provider calls occur outside transactions. Require both lease ownership and existing latest-attempt generation. Retained successful payloads survive failed/partial attempts. A lost lease must not publish failure either.
- [x] Write domain RED cases for missing/current/empty/stale/partial/invalid/unresolved identities, UTC rollover, deduplication, no-price inventory, fair continuation, crash-charged attempts, 15/60-minute retry bounds, exhaustion/restart, explicit retry reset, unconfigured/auth-hold behavior and current-success exclusion.
- [x] Implement RunOnce: obtain unique per-run ownership, resolve the current source, scan due identities, fetch sequentially with cancellable source/persistence budgets, heartbeat under the run context, and stop on cancellation/lease loss/auth failure. Treat `Complete=false` as failure even with nil error. Source `ERR_PROV_AUTH` / `ERR_CFG_MISSING` classifications already exist in domain errors; use typed error codes, not string matching. Never persist raw provider errors.
- [x] Durable auth hold stops further fleet acquisition until CredentialsChanged/explicit admin retry. RequestRun stores/coalesces intent; it does not execute source work. Retry failed resets only failed/due records for one bounded sweep, not current evidence or safety holds.
- [x] Query current candidates from unsold, non-refunded, non-closed inventory without depending on `cl_card_mappings`; read verified identity fields and normalize using existing domain conventions. Recheck current scope before dispatch. Work already in flight may finish its identity snapshot if a card sells, but must never mutate the sale/purchase/list.
- [x] Run focused domain/store races and real two-connection fencing tests, migration/RLS tests, full Go race and make check. Commit and report the produced interface, schema/lock order, failure cases and exact gates.

### Task 3: Runtime ownership, credentials and operational surface

**Files:** create `internal/adapters/scheduler/showprep_refresh.go` and tests; modify `builder.go`, `builder_schedulers.go`, `cmd/slabledger/init_schedulers.go`, runtime/handler wiring, config types/defaults/loader; create worker HTTP handler/tests and route registration; modify current CardLadder credential-save notification and `cardladder_resolve.go` cached-hit identity handoff; add typed API/query/UI modules under the existing Admin integrations surface. Update `.env.example`, API/user/scheduler documentation with actual new capabilities.

**Consumes:** Task2's `EvidenceWorker` methods, `WorkerStatus`, `SourceProvider` and store constructor. **Produces:** application-owned scheduled service; authenticated coverage read and admin-only operational actions.

New proposed routes:

```text
GET  /api/show-prep/coverage                 authenticated, safe counts/state only
GET  /api/admin/show-prep/worker             admin status/diagnostics
POST /api/admin/show-prep/worker/run         admin, persist/wake normal due scan, HTTP202
POST /api/admin/show-prep/worker/retry       admin, persist explicit retry-failed intent, HTTP202
```

- [x] Write runtime RED tests: Start with no browser, empty verified store and configured source; first run starts immediately, then due scans follow a one-minute interval. Disabled/unconfigured still produce honest status without source requests. Stop/shutdown cancels and drains active work. Concurrent Run requests cannot become concurrent unleased sweeps.
- [x] Add a dedicated enabled-by-default `SHOW_PREP_REFRESH_ENABLED` gate and use existing loop/group/StopHandle patterns. Expose status when disabled. The loop may wake promptly after RequestRun, but durable intent survives an app restart and requests remain bounded/coalesced. Never call RunOnce under an HTTP request context.
- [x] Supply the current configured client/source at each sweep and share client pacing. Integrate explicit successful credential SaveConfig with CredentialsChanged and local wake; token refresh alone is not an operator auth-hold reset. Ensure all instances can observe newly saved configuration, including startup with no client. Do not persist/expose credential hashes or tokens in status. Verify this path using local fixtures, not live Firebase.
- [x] Fix only the verified identity handoff: persist new grader-specific GemRate enrichment on the purchase. A serial-only cached mapping is not verified identity; reuse requires the current purchase's existing verified profile to match. Missing/conflicting cache provenance falls back to existing grader-specific BuildCollectionCard. Failure must not promote the cache; missing identity stays unresolved and existing verified identity stays unchanged. No new provenance schema. Preserve grader/grade and ambiguity behavior. Test new purchase plus unproven cache, anchored cache reuse, and shared serials across graders.
- [x] Implement actual-router authorization and no-acquisition request tests for all four routes. Admin POST returns202 after recording intent, even with a held/unreachable provider. A non-admin cannot trigger or reset work. Coverage reads do not wake work, repair matching, or expose raw errors.
- [x] Add a compact evidence-worker section inside existing Admin integrations with enabled/configuration/state, coverage breakdown, last finished sweep and retry timing. Run now/Retry failed are here only, never inventory/packing. UI success means request accepted, not all evidence ready. Persist/recover last-run status using existing scheduler conventions plus authoritative coverage.
- [x] Replace the inventory comp-progress line with quiet coverage only when needed; no Cancel/Retry-source controls or click-triggered acquisition. Missing price and missing evidence counts are separate. Derive counts server-side for normalized identities and avoid pretending one refreshed card means full coverage.
- [x] Run runtime/handler/source/config/identity tests plus full Go and frontend gates. Report the exact client lifecycle and cross-instance credential behavior. Commit.

### Task 4: Prove the product boundaries and finalize

**Files:** update the existing `cmd/slabledger/showprep_readiness_*_test.go` harness and `web/tests/show-readiness-real.cjs`/guide for actual worker and cached-only modes; retain cleanup/geometry helpers and tests. Add dedicated worker/upgrade integration cases if needed. Update `implementation-notes.md`, API/user/scheduler docs, spec/plan status.

- [x] RED: run the final regression against a no-worker/cold baseline and observe that verified evidence is not prepared with no browser. Then start the actual runtime scheduler/service/store/source adapter and wait on persisted results before creating any browser page. Do not seed successful snapshots for this worker-population test.
- [x] Separately stop/pause only the test worker and make provider endpoints fail. Drive actual inventory, filter, checkbox selection, evidence, create/add/list/pack paths against real Go/Postgres responses. Assert no provider requests and no calls to the retired acquisition route. Initial auth/data reads are allowed; checkbox actions alone do not cause requests.

```js
const sourceCallsBeforeUse = (await state()).calls.length;
await useCachedInventoryAndPacking(page);
expect((await state()).calls.length).toBe(sourceCallsBeforeUse);
expect(browserRequests.filter(r => r.path === '/api/show-prep/refresh')).toEqual([]);
```

`state` is the existing test-only control read; `useCachedInventoryAndPacking` is the extracted real-browser interaction flow defined in this task, not an API-response mock. Assertions additionally verify expected cards/prices/source sales/list membership and financial whole-row equality.

- [x] Prove late new/resolved identities are populated with the browser absent; restart preserves current evidence/retry/auth state; midnight schedules renewal; two workers/lease-loss publication are safe; failed/partial source results retain data and don't starve peers. No production clock/reset hooks.
- [x] Publish evidence/availability changes while selection is held. Confirm no lost selection or silent version advancement, explicit conflict on stale Add/Pack, retained history, and working financial actions while server collection is active.
- [x] Retain real desktop/mobile expand/collapse/scroll/breakpoint geometry, keyboard actions, named destination and success-link checks. Measure cached checkbox/filter responsiveness on the 155-card fixture; aim <100ms without provider-dependent waiting, report environment/timing rather than a flaky CI wall-clock assertion.
- [x] Run full Go race, disposable PostgreSQL/migration suite, full frontend tests/type/lint/build, actual browser flows, cleanup tests, architecture/file-size/docs checks, and proactive polish. Read final actual screenshots for the unified selection workflow; no invented overlays or unsupported visual score claims.
- [x] Final independent whole-branch review and scoped corrections (including the additional price-only-notice residual). Explain changed ownership, retired interfaces, schema/rollback implications, actual verification and remaining limits. Do not claim production success; separate authorized deployment/backfill and real-inventory verification are still required.

## Scope/self-consistency check

- Tasks1/3 share coverage UI: Task1 removes acquisition and keeps cached evaluations; Task3 adds only a read-only health summary.
- Tasks1/4 share the harness: Task1 introduces the cached-only mode; Task4 replaces old browser-driven warming with the real worker/no-browser mode.
- Tasks2/3 share exact worker public contracts above. Store-private signatures are finalized/tested in Task2, reported before Task3 uses them.
- Tasks2/4 share migration/ownership proof. Snapshot/business version behavior must remain unchanged under both.
- Task3 identity enrichment does not become a source dependency of list reads. Runtime source/config work is server-owned only.
- Every task carries both decisive product tests forward; no task can declare the whole feature complete merely because its unit suite passed.
