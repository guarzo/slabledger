# Show-preparation readiness and inventory integration plan

**Status:** Tasks 1–4 implemented and approved. Task 5 local real-wire upgrade regression and documentation implemented; local gate results are recorded in `implementation-notes.md`. Parent final polish/independent critique and review remain pending. Controlled production verification, deployment and merge still require separate authorization.

**Goal:** Opening the show-preparation workflow produces useful price-support results without a manual cache-initialization ritual, while inventory keeps its existing compact layout and safe selection behavior.

**Architecture:** Keep the shared domain evaluator, verified evidence store, explicit refresh endpoint, and transactional list operations. Add machine-readable evidence lifecycle metadata and a bounded, feature-scoped client refresh coordinator. Reintegrate the UI into existing inventory controls and contextual selection-bar patterns, rather than introducing a new workspace or theme.

**Stack:** Go 1.26, React 19, TypeScript, TanStack Query, PostgreSQL. No new application dependencies.

**Baseline:** Deployed merge `07ca78b8` (PR #705). Worktree `.worktrees/show-preparation-readiness`, branch `fix/show-preparation-readiness`. The original feature branch is merged and must not be amended.

**Existing specification:** `docs/specs/2026-09-14-show-preparation-design.md`. This proposal supersedes its explicit-only refresh workflow and inventory presentation if approved. Its qualification, money, availability, ambiguity, and history guarantees remain binding.

## 1. Evidence and constraints

- Production initially returned 145 Needs review and 10 No listed price; all 155 lacked a verified evidence version or timestamp. One controlled refresh returned Supported in 0.67 seconds, with 12 matching sales. The existing source works; initialization is missing.
- `internal/adapters/storage/postgres/showprep_evidence.go` reads only the separate evidence table. Migration 46 creates it empty. The existing legacy comp store is not a certified replacement: its acquisition lacks complete-window provenance, and the inventory aggregate uses a different window.
- `internal/domain/showprep/evaluation.go` requires both current UTC-window coverage and freshness within 24 hours. A UTC midnight transition can invalidate evidence earlier than 24 hours.
- `web/src/react/pages/show-preparation/ShowRefresh.tsx` currently owns sequential refresh batches, but the action is mounted inside selection controls. `inventory/useInventoryState.ts` applies Support filtering before selection eligibility, creating a cold-start empty-view trap.
- `InventoryTab.tsx` intentionally captures observed evaluation versions. New background work must not silently advance them. Existing 409 recovery, exact command replay, ambiguity holds, and transactional safeguards remain unchanged.
- `web/src/react/providers/QueryProvider.tsx` disables ordinary focus refetch. A query's staleTime does not schedule renewal. Existing detail/aggregate cancellation-settlement ordering is tested and must be preserved.
- Live critique measured 91px existing row content plus a 78px show footer, a 194–217px top selection form instead of the existing 54px contextual bar, and cards displaced below 1,100px on mobile.
- Live/source typography and palette are copper, Fraunces, and JetBrains Mono. `DESIGN.md` is stale on these points; `web/src/css/tokens.css`, `base.css`, and the surrounding live inventory are authoritative. Do not restyle the app to match the stale document.

## 2. Proposed decisions, ordered by review risk

### A. Automatic readiness, scoped to the operator's task

Opening show-selection mode or activating a price-support filter starts bounded checking of missing or stale evidence. Normal inventory browsing, opening an evidence disclosure, list mutations, and ordinary query refetches do not independently trigger source acquisition.

Build the acquisition cohort from the current inventory/campaign, search, inventory tab, price band, and applicable availability settings **before applying Support filtering or selection**. This ensures Supported-first works on an empty cache without fetching unrelated inventory outside the operator's scope.

Automatically acquire evidence only for refreshable identities with a positive listed price. Missing-price cards remain visible with the correct explanation and can still be explicitly checked. Manual selection remains allowed when evidence is weak or absent, subject to existing availability and observed-version rules.

Expose a compact readiness line, such as `Checking comps 20/145`, with Cancel and explicit Check/Retry actions. Its counts describe the acquisition cohort separately from displayed matches and selected cards. Distinguish an incomplete check from a completed check that found no supported cards.

### B. One bounded client coordinator, not a new background service

Use one coordinator scoped to the existing QueryClient/tab lifetime, with one batch runner shared by automatic and manual checking. Page hooks own active runs; remounts must not create duplicate in-flight work or reset cancellation guards.

Proposed automatic limits:

- One active refresh request per tab, shared with manual checks.
- Existing maximum of 10 purchase IDs per request; one representative per normalized evidence identity.
- At most 200 distinct identities and 20 dispatched requests per tab session/current-UTC-window automatic budget, shared across focus, activation, and filter changes.
- Five-minute wall-clock limit per active run. Stop dispatching and abort/settle active work at the deadline; show remaining work rather than claiming completion.
- Existing server request-child deadlines, source pagination bound, configured limiter, and no-transport-replay policy remain unchanged.
- No automatic retries after request failure, invalid response, or failed/partial source results. Read back uncertain outcomes, stop the automatic run, and offer explicit Retry/Continue.
- Cancellation, a failed result, and the five-minute stop suppress automatic restart until an explicit Resume/Retry/Continue action. Focus changes and query invalidation must not undo a terminal stop. Selection pauses are distinct and can resume when selection clears.

**Prerequisite before automatic activation:** the refresh transport must retain cancellation forwarding and its timeout through full response-body consumption, including success and error JSON, not clean them up when headers arrive. The current shared `APIClient` releases those guards at headers (`web/src/js/api/client.ts`); prefer a refresh-scoped correction rather than changing every API caller. After each awaited response/body operation, check cancellation and run ownership before counting completed IDs, advancing retry state, publishing results, or dispatching another batch. A stalled body must not hold the coordinator indefinitely after Cancel or the five-minute deadline; a late final response must not turn a stopped run into success. Release transport resources and settle the stopped run before allowing another refresh. Server-side commits may still have occurred, so retain the uncertain-outcome/readback policy rather than claiming rollback.

These are best-effort per-tab bounds, not a global quota. Reloading the page creates a new client lifetime, and multiple tabs/processes can multiply work. Keep existing singleflight semantics; do not introduce detached contexts, browser-leader infrastructure, distributed leases, or durable retry queues in this repair.

### C. Preserve selection intent while checks finish

On first selection, pause future automatic batches. Let the active batch settle; a user Cancel/navigation still aborts it. Resume eligible remaining work after selection clears, unless the operator cancelled or a stop condition occurred.

Do not refresh captured selection versions, acknowledge evidence/price changes, or clear selections automatically. In-flight work and other tabs can still update evidence. Show which selected cards changed and require explicit review/reselection before adding them.

Keep displayed row membership/order stable against background evidence updates while selection is active; explicit search/filter/sort changes still apply. This is presentation stability, not frozen purchase data: sold, deleted, unavailable, or changed-version selections must immediately become non-addable. Show selected/outside-view counts and a Reveal selected action.

Use the existing contextual selection-bar placement and Escape/modal priority. Add and other conflicting writes must not race a local active refresh. Do not block ordinary checkbox selection merely because an unrelated evaluation read is pending.

### D. Separate evidence lifecycle from support status

Keep existing support statuses, reasons, monetary fields, and endpoints compatible. Add an optional `readiness` object to evaluation responses, shared by evaluate, evidence, refresh, and list-detail results:

| Field | Proposed meaning |
|---|---|
| `state` | `not_checked`, `current`, `stale`, `running`, `interrupted`, `failed`, `invalid`, or `unavailable` |
| `refreshEligibility` | `needed`, `not_needed`, `wait`, `retry_only`, or `unavailable` |
| `identityKey` | Existing normalized identity fingerprint used to coalesce purchases with the same evidence |
| `expiresAt` | Server-derived earlier of age expiry and next UTC-window boundary; empty when not applicable |
| `retryAt` | Server-derived bound for a persisted running attempt; empty when not applicable |

No schema migration is expected: derive metadata from existing snapshots, attempt columns, identity validation, and typed read outcomes. Do not parse human error strings or label provider configuration healthy without evidence. Configuration/source failures encountered during a check stop the run with an actionable error; repairing first-ever credential hot reload is outside this plan.

Derive interrupted status after 120 seconds from a valid persisted attempt-start timestamp, safely beyond the current maximum 80-second request budget. Interrupted and failed work requires an explicit retry, not an autonomous loop. Missing/future timestamps fail closed. Reads do not mutate attempt state or launch recovery.

A complete zero-sale window is `current`, not not-checked. Missing listed price remains a separate price condition even when evidence is failed or stale.

**Critical:** scheduling metadata must stay outside evaluation fingerprints. The current evaluator hashes its Evaluation value. Add characterization tests and construct/hash the existing business projection before attaching readiness metadata; avoid selection-version churn merely because time or eligibility metadata changes.

Compatibility: old responses without valid readiness remain usable for display and manual operations. They disable automatic acquisition only. Unknown/malformed optional readiness must not discard an otherwise valid evaluation or become No recent comps. Keep refresh request bodies unchanged. Deploy the additive backend contract before activating the automatic client path.

### E. Renew the current window, not just first-run data

While the relevant workflow is mounted, use show-scoped visibility/focus revalidation and bounded scheduling for the earliest applicable server-derived `expiresAt`, running-attempt `retryAt`, or UTC boundary. Expiry/focus events read current evaluations first, then reconsider acquisition under the same cohort, selection, cancellation, and budget rules.

A `retryAt` wakeup is **read-only observation**, not permission to retry source acquisition. Re-read the current attempt so the server can report completion or derive `interrupted` after its 120-second bound; the latter exposes explicit Retry instead of leaving Checking indefinitely. Replace obsolete boundaries when newer attempt metadata arrives, deduplicate coincident reads, and guard expired timestamps against immediate rescheduling loops. Ordinary evaluation queries have no polling that would provide this transition automatically.

Never perform catch-up loops while hidden or replay every missed timer after sleep. Revalidate once on return, including missed attempt boundaries. Handle client/server clock skew conservatively, with bounded timer scheduling and a re-read rather than treating client time as evidence authority.

At midnight, retained sales remain inspectable but unqualified until renewed. If selection is active, show the changed-data warning and defer new automatic batches. If an in-flight batch crosses midnight, report that a newer window is needed rather than misclassifying it as an outage or repeatedly replaying it.

### F. Reintegrate the visual surface

Approved task-specific shape brief:

- **Primary action:** identify suitable inventory, select slabs, add to a named show, then pack. Do not make cache administration or list creation dominate the inventory page.
- **Direction:** restrained live copper/dark theme; the operator uses a desktop for preparation and a phone while handling slabs. Anchors are the app's current inventory filter row, contextual selection bar, and inline row expansion. No new aesthetic lane or generated imagery is needed.
- **Inventory:** compact support filter and readiness summary alongside existing filters. Remove the permanent full-width evidence footer. Put one concise support indicator in the established price/status area and disclose full evidence on demand.
- **Selection:** contextual bottom bar only when selection exists. Reveal destination choice and New list progressively; retain stable creation UUIDs and the correct success destination.
- **Mobile:** preserve 44px touch targets, keep evidence inside the owning card, provide safe-area spacing, and prevent the fixed bar from covering the last row.
- **Shows:** a concise create-first empty state when no lists exist. Existing-list choice becomes primary once lists exist. Keep packing/history semantics; do not redesign the entire populated list surface without separate live evidence.
- **Copy/states:** Not checked, Checking, current support outcomes, stale, interrupted, source failure, no listed price, unavailable purchase, cancelled/partial progress, empty matches, hidden selection, success, and conflict all get distinct presentation. Never show bare `0 sales` as evidence of no market activity when no verified lookup exists. Success explanations are not warning yellow; format source listing-type enums for people.

## 3. Alternatives and tradeoffs

| Alternative | Disposition |
|---|---|
| Only bulk-refresh production once | Optional temporary unblock, not the fix; expiry/UTC rollover recurs. Not authorized by this planning task. |
| Explicit Check comps only, independent of selection | Lowest-risk fallback if automatic budgets/selection behavior are not approved; still fixes the cold-filter dead end but leaves recurrent operator work. |
| Bounded on-demand automatic readiness | Recommended balance: useful results without a startup ritual, while retaining request-bound work and visible limits. |
| Always-on server warming/renewal | Deferred: requires scheduler ownership, provider-load policy, shutdown behavior, and durable coordination beyond this repair. |
| Reuse legacy aggregates as verified evidence | Rejected: different time window and insufficient completeness metadata. |
| Consolidate legacy and verified acquisition | Possible future architecture work; historical rows still cannot be retrospectively certified. Not bundled into the UI repair. |
| Restyle the whole application | Rejected. Match the existing live inventory, not stale design documentation. |

## 4. Ordered implementation slices

Each slice starts with failing regressions and ends with focused verification and a reviewable commit. This is a design-level sequence; executable code-level tasks follow approval.

### Task 1: Evidence lifecycle contract, without automatic acquisition

**Create:** `internal/domain/showprep/readiness.go` and `readiness_test.go` for the derived lifecycle policy.

**Modify:** `internal/domain/showprep/types.go`, `evaluation.go`, `service.go`; `web/src/types/showprep.ts` and its API guards; focused evaluator/HTTP/contract tests.

Implement and document readiness metadata, server-derived expiry/interruption bounds, backward-compatible validation, and unchanged business fingerprints. Keep existing acquisition behavior until this contract passes tests.

**Gate:** cold versus complete-empty, retained failed data, invalid identity/read failures, window/age boundaries, interrupted attempts, old-response compatibility, and stable-version tests.

### Task 2: Shared bounded checking and first-run readiness

**Create:** `web/src/react/queries/showRefreshCoordinator.ts`, `showRefreshCoordinator.test.ts`, `useShowReadiness.ts`, and `useShowReadiness.test.tsx`. The coordinator owns bounded run state; the hook connects the active cohort, query cache, and page lifecycle.

**Modify:** `web/src/js/api/showprep.ts` and refresh transport tests, `ShowRefresh.tsx`, `useShowPrepQueries.ts`, `inventory/useInventoryState.ts`, and `InventoryTab.tsx` integration. Inspect `web/src/js/api/client.ts` but keep any transport correction refresh-scoped unless broader changes are separately justified.

First prove full-response timeout/cancellation and post-await run guards; merely extracting the current runner preserves its late-body defect. Then extract the shared batch runner without weakening cache settlement, expose the pre-Support cohort, and implement manual checking independently of selection. Only after those prerequisites pass, enable automatic activation, expiry/focus/`retryAt` observation, budgets, deduplication, progress, stop/retry, and selection pausing.

**Gate:** Supported-first on a cold store, duplicate identities, fresh-data skips, 200/10/5-minute limits, cancellation/remount, focus, UTC rollover, uncertain partial commits, terminal failures without loops, and absence of source calls from ordinary reads. Add real streaming-response regressions for headers received with a stalled body, explicit Cancel and deadline during body consumption, delayed final-batch completion after cancellation, and stalled error bodies. Assert prompt run settlement, no late progress/success/cache publication, no next batch or transport replay, and unchanged captured selection versions. With a controlled clock, mount already-running evidence and cross `retryAt` without focus changes: assert a read occurs, completed/interrupted state becomes visible, no refresh POST occurs, newer attempts supersede old timers, and hidden-tab catch-up/past-boundary handling cannot loop.

### Task 3: Inventory layout and selection recovery

**Create:** `web/src/react/pages/show-preparation/ShowSelectionBar.tsx` and its component tests.

**Modify:** `ShowInventoryControls.tsx`, `ShowEvidence.tsx`, `web/src/react/pages/show-preparation/show-preparation.css`, and inventory row/mobile presentation. Use `InventorySelectionBar.tsx` as the established pattern; extract only shared presentation if necessary, not financial action behavior.

Remove repeated footer/form chrome; integrate compact states, progressive list choice, hidden-selection recovery, row stability, keyboard exits, and mobile ownership/spacing. Keep captured versions and 409 behavior intact.

**Gate:** real-browser desktop/mobile comparison before committing the visual direction. Existing inventory outside show mode keeps its density; selected cards remain identifiable; supported results have no warning styling; changing data cannot silently authorize stale actions.

### Task 4: Empty Shows and workflow copy

**Modify:** `ShowPreparationPage.tsx`, `ShowListPicker.tsx`, `showPrepLabels.ts`, and related tests.

Implement create-first empty state, progressive list creation/rename, accurate readiness and source-action labels, and preserved retry UUID/success-link behavior. Verify populated-list behavior locally with fixtures; production had no saved lists during critique.

**Gate:** no-list/one-list/many-list states, add-to-list success, retry/conflict, packing/unpacking/acknowledgment/history, and no regression in ordinary inventory selection.

### Task 5: Upgrade-shaped integration and production verification

**Local implementation complete; rollout gate pending.** The committed test and
separate fixture-server command are documented in `web/tests/show-readiness-real.md`.
The browser uses real router/service/storage/source-adapter responses, never
intercepted evaluate/refresh/list answers. Production operations were not run.

Build an isolated test path with legacy comps and purchases present, migration 46 applied, and verified evidence empty. Use the real Go router/service/storage/source adapter against a controlled local CardLadder HTTP server and disposable PostgreSQL; the browser must not stub evaluate/refresh results.

Prove inventory → Supported-first → automatic acquisition → persisted results → shortlist → packing, then restart/readback and UTC rollover/recovery. Assert unchanged prices, purchases, sales, and listing state; list/packing changes must be only the explicit test actions.

After polish and fresh gates, repeat the Impeccable critique against actual rendered views. Update `docs/API.md`, `docs/USER_GUIDE.md`, the approved spec amendment, and `implementation-notes.md`. Correct only the stale design-context statements relevant to the chosen integration.

**Rollout gate:** user-authorized controlled production verification with a missing/stale identity and a fresh identity. Confirm qualification without unnecessary reacquisition, observe the initialization/recovery path, and confirm no financial mutations. Do not treat a mocked browser test or the existing warmed production card as proof of first-run behavior.

## 5. Verification strategy and acceptance criteria

- Domain/source/HTTP race tests plus optional-metadata compatibility and stable-fingerprint tests.
- Real PostgreSQL integration with an explicit disposable `POSTGRES_TEST_URL`; never production or a developer ledger.
- Frontend hook/component regressions, including existing 805-card ordering, cancellation settlement, hidden observed versions, stable UUID retry and success destination. Explicitly cover full refresh-body cancellation/deadlines and read-only `retryAt` transitions; existing evidence-query late-body publication coverage does not prove bounded refresh settlement.
- Chromium desktop/tablet/mobile, keyboard/escape/focus, empty/filter/error/current states, virtualized rows, safe-area bottom bar, and latest published source/evidence values.
- Full `go test -race -count=1 -timeout 10m ./...` with DB unset, separately isolated PostgreSQL suite, frontend tests/typecheck/lint/build, compatible-toolchain `make check`, and diff/path/file-size checks. Run broad Go discovery and browser artifact generation sequentially.
- `polish-core --fix`, inspect edits, rerun relevant verification, independent review, then `change-explainer` before PR completion.
- New source files remain under the 500-line guideline/600-line hard limit; no architecture-boundary violations or dependencies.

Visible acceptance criteria:

1. A cold Supported filter explains checking and obtains genuine results without selecting cards or creating a list first.
2. Missing evidence never looks like a completed zero-sale lookup; failures have an actionable, non-looping recovery path.
3. UTC rollover or age expiry does not silently leave green badges authoritative forever and does not silently change selected intent.
4. Ordinary inventory no longer has the added 78px evidence footer; entering show mode does not introduce a 200px setup form.
5. At 390×844, mode activation itself adds at most one compact readiness/control region, not an extra screen of setup; the selection bar does not occlude rows.
6. No automatic action reprices, delists, reserves, sells, packs, acknowledges, or creates a list.
7. Cancel and the five-minute limit settle a refresh even if headers arrived but the body stalled; late completion cannot report success or start more work.
8. An observed running attempt is re-read at its interruption boundary without requiring focus or midnight, and an interrupted result offers explicit Retry without an automatic acquisition loop.

## 6. Adaptation points and exclusions

Revisit the plan if real provider latency makes the proposed budget ineffective; simultaneous tabs need strict exclusion; prerequisite source configuration cannot be surfaced without a new domain capability; or row stability requires changes beyond the show-specific presentation. Do not silently add global scheduling, distributed locking, credentials hot reload, or a new cache architecture to resolve these findings.

Explicitly excluded: relaxed thresholds/matching/completeness, fake legacy certification, automatic ambiguity clearing, financial changes, global visual redesign, weakening CSP for critique overlays, unrelated CodeRabbit nits, and production bulk warming or deployment during planning.

## Execution authorization

The user approved proceeding with the recommended bounded automatic policy, manual-only retry after failure/interruption, selection-pause behavior, and inventory-plus-empty-Shows visual scope. Both independent-review findings are incorporated. Production bulk warming, deployment, and merge are not authorized by this execution request.
