# Inventory Price Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit committed inventory prices using recent matching sales, then explicitly save a reviewed price from the selected B-style workspace without changing existing DH save behavior.

**Architecture:** Keep the evidence cache/worker and one backend assessment implementation in `internal/domain/showprep`. Add a non-persisting hypothetical-price endpoint, then compose a focused queue/detail view inside `/inventory` alongside normal inventory operations. Preserve recorded show-list history and the existing reviewed-price write, including DH synchronization and eligible auto-listing.

**Tech Stack:** Go 1.26, PostgreSQL 17, React 19, TypeScript, TanStack Query/Virtual, React Router, Vitest, Playwright. No new application dependency or schema migration.

**Spec:** `docs/specs/2026-09-16-inventory-price-review-design.md`

## Global Constraints

- "Assess the latest operator-committed SlabLedger price using `inventory.ResolveListingPriceCents`."
- "All monetary arithmetic uses integer cents; median qualification must preserve half-cent precision until comparison. Rounded display values must not decide qualification."
- "Choose the newest five eligible sales in that seven-date window. Include **all** sales tied on the fifth sale's date."
- "The wider 30-day history remains context. It cannot override a recent contradiction."
- "Browser navigation, search, filters, trial-price checks, and selection must not acquire provider comps."
- "No destructive migration or automatic acknowledgment/backfill."
- "Normal Inventory remains the default."
- "No merge, deployment, production repricing, or production backfill is authorized by this design."
- Preserve dark/copper styling, existing fonts, accessible controls, explicit financial confirmation, and the no-em-dash UI-copy convention.
- Domain code must not import adapters. Follow the flat inventory-sibling rule; use existing mocks from `internal/testutil/mocks`. Source files should remain under 500 lines, with 600 a hard limit.
- Rewrite the selected prototype as production components. Never commit its real inventory fixture or serve it in the application bundle.

---

## Decisions that must survive implementation

1. **One assessor, three consumers:** saved inventory evaluation, saved-list evaluation, and hypothetical preview. React displays server facts; it does not implement the support thresholds.
2. **Canonical target, compatible diagnostics:** `Evaluation.LocalPriceCents` already contains the resolved committed price. Keep `ListedPriceCents` as the stored DH amount; do not silently change its wire meaning. Both saved-list current value and new explicit acknowledgments use canonical local cents.
3. **Separate health from judgment:** incomplete/invalid/stale evidence cannot certify support. Limited evidence can still show a sharp contradictory sale; it must not read as reassurance.
4. **History is not rewritten:** existing acknowledged amounts/statuses remain their original observations. Compare them against the current canonical result; do not fabricate policy provenance or hide tags inside status strings. Include a current policy ID in business fingerprints, not a new historical schema.
5. **Preview is genuinely read-only:** use purchase/snapshot reader ports. Do not call `Service.Evidence`, which observes and persists association holds. Preview returns no list-mutation evaluation token.
6. **Save retains existing side effects:** use `setReviewedPrice(id, cents, 'manual')`, not the override API. A successful local save triggers existing asynchronous DH sync and eligible auto-listing; it is not proof of external completion. Last-writer-wins storage remains an acknowledged limitation, not an invented concurrency guarantee.
7. **No hidden UI state mutation:** active review card, dirty drafts, bulk selection, and saved-show membership are different state. Evidence publication or preview completion cannot acknowledge a selected version, replace a draft, or change the focused card.

## Preparation and verified environment

Worktree: `/home/tng/workspace/slabledger/.worktrees/review-price-support`; branch `investigate/price-support-705-707`. Reuse it, do not write to primary `main`. Implementation baseline is `6ca4b420`; `c05b78f6` adds the reviewed spec. Subsequent documentation commits are not an implemented feature.

Observed during planning: Go `1.26.5`, Node `26.5.0`, npm `11.17.0`, Docker available. This worktree has no `web/node_modules`; install its own lockfile dependencies. The old browser-fixture container `slabledger-cached-show-01a09dd6` does not exist and port 44620 was unbound. Do not blindly run the historical DB-reset recipe against a different resource. Git's configured hooks path `/workspace/.githooks` is missing in this environment; do not change config or bypass hooks. Run the tracked hook explicitly before implementation commits and stop if it fails for unrelated reasons.

```bash
git status --short
git log -3 --oneline
(cd web && npm ci)
TZ=UTC POSTGRES_TEST_URL= SHOW_PREP_RUNTIME_TEST_URL= SHOW_READINESS_E2E_URL= go test -race -count=1 -timeout 10m ./...
(cd web && npm test && npm run typecheck)
```

Before baseline frontend checks, keep the private throwaway files out of both staging **and** application lint/build discovery. Confirm `git ls-files -- web/src/react/pages/inventory-prototype/` is empty and `git check-ignore .superpowers/price-review-prototype/prototype-data.js` succeeds, then move that whole untracked directory to `.superpowers/price-review-prototype/`. The browser companion has its own already-served copies, so this does not remove the comparison. Update the archived prototype's local run command to its new path. Do not delete the comparison until the implementation is accepted.

Every commit below uses an explicit file list, a staged diff check, and relevant passing tests. No `git add .`, hook bypass, production credentials, default database fallback, or pushes.

## File ownership and contracts

**New backend units:**
- `internal/domain/showprep/price_assessment_types.go`: policy ID and recent-facts value type.
- `internal/domain/showprep/price_assessment.go`: pure recent-group selection and price judgment.
- `internal/domain/showprep/service_preview.go`: cached read composition and public preview DTO.
- `internal/adapters/httpserver/handlers/showprep_preview.go`: one authenticated preview handler.

**New frontend units:**
- `web/src/js/api/priceReview.ts`: preview API and response validation.
- `web/src/react/queries/usePricePreview.ts`: cancellable, current-input-only preview query.
- `web/src/react/pages/price-review/priceReviewModel.ts`: UI labels/grouping/sorting using server facts only.
- `web/src/react/pages/price-review/usePriceReviewState.ts`: review filter/order/focus/drafts, owned above virtual rows.
- `web/src/react/pages/price-review/PriceReviewWorkspace.tsx`: responsive queue/detail composition.
- `web/src/react/pages/price-review/PriceReviewQueue.tsx`: fixed-row queue, focus and separate bulk checkboxes.
- `web/src/react/pages/price-review/PriceReviewPanel.tsx`: evidence, draft, preview and explicit save.
- `web/src/react/pages/price-review/price-review.css`: scoped layout, existing tokens, responsive states.

Keep pure backend assessment separate from snapshot validity/provenance checks in `Evaluate`; both consumers must pass through the same validity logic. Keep UI components separate from the existing 418-line `InventoryTab.tsx`; do not add the whole workspace to that file.

### Shared backend interface (introduced in Tasks 1–3)

```go
const PriceAssessmentPolicy = "recent-sales-v1"
// Add MixedEvidence Status = "mixed_evidence" to the existing Status constants.
type RecentPriceEvidence struct {
    WindowStart        string   `json:"windowStart"`
    WindowEnd          string   `json:"windowEnd"`
    SaleIDs            []string `json:"saleIds"`
    Count              int      `json:"count"`
    MedianCents        int      `json:"medianCents"` // rounded for display only
    LatestSaleDate     string   `json:"latestSaleDate"`
    LatestSaleCount    int      `json:"latestSaleCount"`
    LatestSaleMinCents int      `json:"latestSaleMinCents"`
    LatestSaleMaxCents int      `json:"latestSaleMaxCents"`
    GapPct             *float64 `json:"gapPct"` // display percentage, not qualification
}
func assessRecentPrice(askingCents int, eligibleSales []Sale, now time.Time) (Status, string, RecentPriceEvidence)
// Extend Evaluation with PolicyVersion string and Recent RecentPriceEvidence,
// JSON names policyVersion and recent. Its existing fields keep their meaning.
```

`SaleIDs` uses deterministic date-descending/ID ordering for presentation, including all cutoff-date ties; that ID ordering never purports to be intraday chronology. Invalid/stale evidence retains inspectable facts but clears `recent.gapPct`; the UI does not present its median as verified current support. No positive price also means no gap. Exact qualification uses a local `int64` twice-median, not a rounded/displayed median or a frontend percentage.

```go
type PricePreview struct {
    PurchaseID          string              `json:"purchaseId"`
    CurrentPriceCents   int                 `json:"currentPriceCents"`
    TrialPriceCents     int                 `json:"trialPriceCents"`
    Status              Status              `json:"status"`
    Reason              string              `json:"reason"`
    EvidenceNeedsReview bool                `json:"evidenceNeedsReview"`
    EvidenceReason      string              `json:"evidenceReason"`
    EvidenceVersion     string              `json:"evidenceVersion"`
    PolicyVersion       string              `json:"policyVersion"`
    Recent              RecentPriceEvidence `json:"recent"`
}
func (s *Service) Preview(ctx context.Context, purchaseID string, trialPriceCents int) (PricePreview, error)
// POST /api/show-prep/preview
// Request: {"purchaseId":"canonical-uuid","priceCents":240000}
// Response: PricePreview (no version/canAdd/canPack/list acknowledgment token).
```

Preview cents must be positive, finite JSON integers within JavaScript's safe-integer range (`1..9007199254740991`). Reject unknown/trailing fields and noncanonical UUIDs using existing handler helpers. Missing purchase is 404, invalid input 400, auth absent 401, unavailable composition 503, operational storage failure uses the existing sanitized 500 mapping. Domain validation must also reject invalid trial amounts for non-HTTP callers.

## Task 1: Pure recent-sales assessment, with regression fixtures

**Files:** Create the two backend units above and `internal/domain/showprep/price_assessment_test.go`; modify `types.go` only for `MixedEvidence`. No call-site policy switch yet.

**Consumes:** Existing `Sale`, `Status`, UTC `now`; eligible records after existing validation/deduplication.
**Produces:** `assessRecentPrice` and `RecentPriceEvidence` exactly as defined above.

- [ ] Write table-driven tests using fixed September 16 dates and anonymized sale IDs. Include the complete eight-sale counterexample and the three trial asking prices:

```go
for _, tc := range []struct{ ask int; want Status }{
    {320000, BelowTarget}, {254000, MixedEvidence}, {240000, Supported},
} {
    status, _, recent := assessRecentPrice(tc.ask, []Sale{
        {ID:"a", Date:"2026-09-16", PriceCents:231500},
        {ID:"b", Date:"2026-09-16", PriceCents:202500},
        {ID:"c", Date:"2026-09-15", PriceCents:309937},
        {ID:"d", Date:"2026-09-15", PriceCents:242500},
        {ID:"e", Date:"2026-09-15", PriceCents:220000},
        {ID:"f", Date:"2026-09-15", PriceCents:233012},
        {ID:"g", Date:"2026-09-15", PriceCents:222500},
        {ID:"h", Date:"2026-09-15", PriceCents:232500},
    }, time.Date(2026,9,16,12,0,0,0,time.UTC))
    require.Equal(t, tc.want, status)
    require.Equal(t, 8, recent.Count)
    require.Equal(t, 232000, recent.MedianCents)
}
```

- [ ] Add exact 90%, half-cent-below, strictly-more-than-20%, zero price, zero/one sale (including one sharply low), seven-date start/end, old sales, permutation invariance, all cutoff-day ties, and recovery `$70 → $100 → $100` cases. Future/invalid records are tested at the existing validation seam in Task 2, not silently tolerated here.
- [ ] Run `TZ=UTC go test ./internal/domain/showprep -run TestRecentPrice -count=1`; observe RED for the absent helper before implementing it.
- [ ] Implement bounded selection, deterministic ordering, twice-median arithmetic, newest-day min/max/count, reasons, and classification in the exact precedence below. Always allocate an empty `SaleIDs` slice rather than JSON null.

```go
// After selecting the group and computing local int64 twiceMedian:
if recent.Count > 0 {
    recent.MedianCents = int((twiceMedian + 1) / 2)
    if askingCents > 0 {
        gap := 100 * (1 - float64(twiceMedian)/(2*float64(askingCents)))
        recent.GapPct = &gap
    }
}
switch {
case askingCents <= 0:
    return NoListedPrice, "No positive SlabLedger asking price", recent
case recent.Count == 0:
    return NoRecentComps, "No matching sales in the past seven UTC dates", recent
case recent.Count == 1:
    return ThinEvidence, "Only one recent matching sale; review its amount", recent
case 5*twiceMedian < 9*int64(askingCents):
    return BelowTarget, "Recent median below 90% of asking price", recent
case 5*int64(recent.LatestSaleMinCents) < 4*int64(askingCents):
    return MixedEvidence, "A newest-day sale is more than 20% below asking", recent
default:
    return Supported, "Recent matching sales support the asking price", recent
}
```

- [ ] Run focused tests and `TZ=UTC go test -race -count=1 ./internal/domain/showprep`; inspect diff, stage only the four Task 1 files, run `bash .githooks/pre-commit`, and commit `feat: assess asking prices against recent sales`.

## Task 2: Activate canonical assessment atomically across saved reads and shows

**Files:** Modify `internal/domain/showprep/types.go`, `internal/domain/showprep/evaluation.go`, `internal/domain/showprep/service_lists.go`, and their existing `evaluation_test.go`/`service_lists_test.go`; add `internal/domain/showprep/price_assessment_integration_test.go` and `internal/adapters/storage/postgres/showprep_price_policy_test.go`. Update directly affected existing showprep storage/service/HTTP fixture tests. Frontend modifications: `web/src/types/showprep.ts`, `web/src/js/api/showprep.ts`, `web/src/react/pages/show-preparation/showPrepLabels.ts`, `web/src/react/pages/show-preparation/ShowEvidence.tsx`, `web/src/react/pages/show-preparation/ShowMember.tsx`, `web/src/react/pages/ShowPreparationPage.tsx`, and their affected tests plus `web/src/react/pages/show-preparation/fixtures.test-support.ts`.

**Consumes:** Task 1 assessor; existing storage projection's `LocalPriceCents`; existing list locks/holds/replay checks.
**Produces:** `Evaluation.policyVersion/recent`, canonical statuses/values across saved reads, unchanged persisted history, wire acceptance and truthful transitional copy before the new workspace exists.

- [ ] Add integration regressions proving fresh 30-day median alone no longer passes, differing local/DH prices assess local, no local price cannot borrow DH/CL, healthy local price is not blocked by DH association ambiguity, and source failure still suppresses certification.

```go
p := Purchase{ID:"p", Grader:"PSA", Grade:10, ProfileID:"profile",
    LocalPriceCents:30000, ListedPriceCents:90000, PriceAssociationUnclear:true}
now := time.Date(2026,9,16,12,0,0,0,time.UTC)
start,end := Window(now)
s := &Snapshot{Identity:p.Identity(), Source:"cardladder", Complete:true,
    AttemptState:"complete", WindowStart:start, WindowEnd:end, RefreshedAt:now,
    Sales:[]Sale{{ID:"a",Date:end,PriceCents:30000},{ID:"b",Date:end,PriceCents:30000}}}
e := Evaluate(p,s,now)
require.Equal(t, Supported, e.Status)
require.Equal(t, 90000, e.ListedPriceCents) // still DH diagnostics
require.Equal(t, PriceAssessmentPolicy, e.PolicyVersion)
```

- [ ] Add a real PostgreSQL legacy-list regression: preserve its original acknowledged cents/status, packing time, and last-command data; current canonical target changes its warning/value; only explicit valid acknowledgment updates recorded values. Assert exact retries do not increment version or repack. Exercise ambiguity-hold survival and canonical known value independently, not an `else if` that omits local value because a DH warning exists.
- [ ] Run the new domain/PG tests RED before switching call sites. PG setup is specified below; a skipped DB test is not RED or GREEN.
- [ ] Keep existing 30-day facts and health checks in `Evaluate`, then call the assessor on the deduplicated eligible records. Set `PolicyVersion` before computing `Version`; keep readiness after the fingerprint. Preserve DH warning fields but remove their precedence over a clear canonical local-price assessment.

```go
status, reason, recent := assessRecentPrice(p.LocalPriceCents, eligibleSales, now)
e.PolicyVersion, e.Recent = PriceAssessmentPolicy, recent
// Keep independent evidence health. With a price and unhealthy evidence,
// project NeedsReview/evidence reason; clear Recent.GapPct even if retained sales exist.
// With no price, retain NoListedPrice and still expose evidenceNeedsReview/reason.
```

- [ ] Replace list current-value/price-change and new add/acknowledgment amount reads with `e.LocalPriceCents`. Preserve raw old acknowledgments and all lock/replay/hold operations; do not modify migration files. Preserve `AmbiguousPriceCount` as a DH-warning count independent of canonical known-value aggregation, and update UI wording accordingly.
- [ ] Extend TS status/DTO validation for `mixed_evidence`, `policyVersion`, and `recent`. Update shared fixtures and labels: below_target → Asking above comps, thin/no_recent → Limited evidence, no_listed_price → No asking price. Update existing evidence/list copy and values now, so this commit does not leave a new policy described as DH/30-day-median support. Readiness labels remain collection state, not a substitute market judgment.
- [ ] Run `TZ=UTC POSTGRES_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_cached_test?sslmode=disable' go test -race -count=1 ./internal/domain/showprep ./internal/adapters/storage/postgres ./internal/adapters/httpserver/handlers ./internal/adapters/httpserver` after the owned DB setup below, then `(cd web && npm test -- src/js/api/showprep src/react/pages/show-preparation src/react/pages/ShowPreparationPage.test.tsx && npm run typecheck)`. Review all changed assertions for genuine policy changes rather than weakened integrity checks. Stage explicit paths, run tracked hook, commit `fix: use canonical recent-price assessment across inventory and shows`.

## Task 3: Read-only server preview and current-input query

**Files:** Create `service_preview.go`, `service_preview_test.go`, `handlers/showprep_preview.go`, `handlers/showprep_preview_test.go`, `web/src/js/api/priceReview.ts`, `priceReview.test.ts`, `web/src/react/queries/usePricePreview.ts`, `usePricePreview.test.tsx`. Modify `handlers/showprep.go` interface, `httpserver/routes_showprep.go`, `showprep_routes_test.go`, `web/src/types/showprep.ts`, and `web/src/react/queries/showPrepKeys.ts`. Reuse/extend the bounded body-reading helper in `web/src/js/api/showRefreshTransport.ts`; preserve its evaluate behavior and existing stream tests.

**Consumes:** `Evaluate`, reader methods on existing `Store`, Task 2 DTOs and policy ID.
**Produces:** `Service.Preview`, the HTTP contract above, `priceReviewAPI.preview(purchaseId, priceCents, options?)`, and `usePricePreview(purchaseId, draftCents, savedEvaluation)`.

- [ ] First write a service test with `mocks.ShowPrepStoreMock` readers supplying a purchase/snapshot. Set `ObservePriceAssociationsFn`, `BeginAttemptFn`, `FinishAttemptFn`, `WithinFn`, and a `ShowPrepSourceMock.FetchFn` to fail if called. Assert one purchase read and one snapshot read, unchanged supplied objects, correct outcomes at three asking prices, cancellation/error context, and JSON absence of `version`, `canAdd`, `canPack`.
- [ ] Add handler/router table cases for valid request, noncanonical UUID, absent/zero/negative/fractional/unsafe cents, unknown/trailing fields, 404, 401, 503 composition, storage failure, and cancellation. Run the new tests RED.
- [ ] Implement exactly one cached snapshot read and pure evaluation over a purchase value copy; retain current price separately. Do not expose the temporary evaluation's business version or invoke `Service.Evidence`.

```go
currentPrice := purchase.LocalPriceCents
trial := purchase
trial.LocalPriceCents = trialPriceCents
e := Evaluate(trial, snapshot, s.now())
return PricePreview{PurchaseID:purchase.ID, CurrentPriceCents:currentPrice,
    TrialPriceCents:trialPriceCents, Status:e.Status, Reason:e.Reason,
    EvidenceNeedsReview:e.EvidenceNeedsReview, EvidenceReason:e.EvidenceReason,
    EvidenceVersion:e.EvidenceVersion, PolicyVersion:e.PolicyVersion, Recent:e.Recent}, nil
```

- [ ] Add the authenticated route through existing registration, JSON-size/decode/UUID/error helpers. Define request validation once per boundary, with domain safeguards for direct service callers.
- [ ] Add strict TS validation and cancellable preview transport. Adapt the existing stream reader to accept a typed body without duplicating its stalled-body cancellation behavior; retain evaluate's existing retry contract. Do not add nested retry loops for preview. Query key includes purchase ID, trial cents, and the saved evaluation's business version plus evidence/policy identifiers. The business version matters when the committed price changes without any evidence change.

```ts
import { useQuery } from '@tanstack/react-query';
import { priceReviewAPI } from '../../js/api/priceReview';
import { showPrepKeys } from './showPrepKeys';
import type { ShowEvaluation } from '../../types/showprep';

export function usePricePreview(
  purchaseId: string, draftCents: number | null, savedEvaluation?: ShowEvaluation,
) {
  return useQuery({
    queryKey: [...showPrepKeys.all, 'price-preview', purchaseId, draftCents,
      savedEvaluation?.version, savedEvaluation?.evidenceVersion,
      savedEvaluation?.policyVersion],
    enabled: !!purchaseId && !!savedEvaluation && draftCents !== null
      && Number.isSafeInteger(draftCents) && draftCents > 0
      && draftCents !== savedEvaluation.localPriceCents,
    queryFn: ({ signal }) => priceReviewAPI.preview(purchaseId, draftCents!, { signal }),
    retry: false,
    staleTime: 0,
  });
}
```

The component compares current parsed input with its 300ms-debounced target. While different, render `Checking price…`, not the previous target's result. If preview's `currentPriceCents` differs from the draft baseline, surface the changed saved price and re-read the saved evaluation; do not publish the hypothetical result into its cache.

- [ ] Test 254000 then 240000 responses arriving in reverse order, card switch, canceled/stalled body, preview failure retaining draft, and no publication into saved evaluation/list caches. Run `TZ=UTC go test -race -count=1 ./internal/domain/showprep ./internal/adapters/httpserver/handlers ./internal/adapters/httpserver` and `(cd web && npm test -- src/js/api/priceReview.test.ts src/react/queries/usePricePreview.test.tsx src/js/api/showprep-stream.test.ts src/js/api/showEvaluationTransport.test.ts && npm run typecheck)`; stage explicit files, run tracked hook, commit `feat: preview trial asking prices without writes or acquisition`.

## Task 4: Production review model, state, and B-style workspace

**Files:** Create the seven frontend units listed in the file map and their focused tests: `priceReviewModel.test.ts`, `usePriceReviewState.test.tsx`, `PriceReviewWorkspace.test.tsx`, `PriceReviewPanel.test.tsx`. Create anonymized `fixtures.test-support.ts` in this directory. Use existing `AgingItem`, formatting helpers, UI controls, `useShowEvidence`, `usePricePreview`, and source-link sanitization.

**Consumes:** Authoritative `ShowEvaluation`/`PricePreview`, existing `AgingItem[]`, and callbacks from shared inventory state.
**Produces:** The view components and this state boundary (ordinary inventory remains unmounted from neither its owner nor its bulk selection state):

```ts
export type PriceReviewFilter = 'all'|'above'|'mixed'|'limited'|'supported'|'unavailable'|'unpriced';
export type PriceReviewSort = 'attention'|'supported'|'asking'|'recent'|'gap';
export interface PriceDraft { value: string; baselinePriceCents: number }
export function usePriceReviewState(
  items: AgingItem[], evaluations: Record<string, ShowEvaluation>, search: string,
): {
  filter: PriceReviewFilter; setFilter: (v: PriceReviewFilter) => void;
  sort: PriceReviewSort; setSort: (v: PriceReviewSort) => void;
  descending: boolean; setDescending: (v: boolean) => void;
  rows: AgingItem[]; counts: Record<PriceReviewFilter, number>;
  activeId: string | null; focus: (id: string) => void; move: (delta: -1|1) => void;
  drafts: Record<string, PriceDraft>; setDraft: (id: string, draft: PriceDraft) => void;
  clearDraft: (id: string) => void;
  outsideFilter: boolean;
};
export interface PriceReviewWorkspaceProps {
  items: AgingItem[]; evaluations: Record<string, ShowEvaluation>;
  review: ReturnType<typeof usePriceReviewState>;
  selected: ReadonlySet<string>; onToggleSelected: (id: string) => void;
  onSavePrice: (id: string, priceCents: number) => Promise<void>;
}
```

- [ ] Write model/state tests RED: only server status decides grouping; normal search includes card/cert/set/campaign; filtered counts share the search scope; asking sorts local cents; gap uses server `recent.gapPct`, not rounded-median recomputation; unknown values sort last within their group. Attention precedence is above, mixed, limited, unavailable, unpriced, supported, with larger known gaps first. Supported-first puts supported rows first, then retains attention precedence for the rest.
- [ ] Write state tests holding dirty drafts/focus across live evidence changes, filters, reordering, and view-switch ownership. If an active item leaves a filter, retain its panel and mark it outside the filter; an explicit next/previous chooses the adjacent surviving queue item. When an item disappears, clear unsafe actions but retain the unsaved text with an unavailable-card message. A live sold/not-packable item must never gain show eligibility through the pricing view.
- [ ] Write panel tests RED with server-provided outcomes, not a local assessor:

```tsx
// In PriceReviewPanel tests, mocked preview response for 254000 is mixed_evidence;
// for 240000 it is supported. Use real hook/provider integration in the ordering test.
await user.clear(screen.getByLabelText('Asking price'));
await user.type(screen.getByLabelText('Asking price'), '2400');
expect(savePrice).not.toHaveBeenCalled();
await within(screen.getByRole('region', {name:'Trial price assessment'}))
  .findByText('Supported', {exact:true});
await user.click(screen.getByRole('button', { name: 'Save price' }));
expect(savePrice).toHaveBeenCalledWith('purchase-a', 240000);
```

- [ ] Implement the UI model without price qualification math. Keep invalid/failed/missing evaluations separate from sparse but healthy evidence. Reuse `centsToDollars`, `dollarsToCents`, `formatCents`, and verify parsed cents are positive safe integers before preview/save.
- [ ] Implement the workspace using B's structure, not its code. Give the workspace region the name `Price review`, the detail region `Price details`, the hypothetical-result region `Trial price assessment`, and the committed amount output the accessible label `Saved asking price`. Use queue controls with separate focus and bulk checkboxes; detail with saved asking/assessment/recent facts/newest-day range, safe sale links, progressive 30-day history, trial input and explicit save. Highlight low single-sale facts even when the status is Limited. Put operational diagnostics after evidence, with stale/partial context clear.
- [ ] Implement draft/preview/save states. Never turn a preview into a saved badge. Keep input intact on errors, show changed saved baseline while dirty, disable duplicate saves, and retain the active panel after success until explicit navigation. The panel states that saving syncs DH and can list eligible inventory; it does not promise remote completion or atomic optimistic concurrency.
- [ ] Desktop uses a persistent side panel. On narrow screens, selecting a card opens the focused panel with a Back to inventory list control and restored queue scroll/focus. Honor reduced motion, use text with semantic colors, and keep all controls keyboard-accessible. No production prototype switcher or new fonts/dependencies.
- [ ] Run `(cd web && npm test -- src/react/pages/price-review src/react/queries/usePricePreview.test.tsx && npm run typecheck && npm run lint)`. Inspect a local fixture render at desktop and 390px, then stage explicit paths, run tracked hook, commit `feat: add focused inventory price review workspace`.

## Task 5: Integrate the view without disrupting normal inventory

**Files:** Modify `GlobalInventoryPage.tsx`, `campaign-detail/InventoryTab.tsx`, `inventory/{useInventoryState,usePricingActions}.ts`, `inventory/InventoryHeader.tsx` as needed, and the existing evidence button/disclosure integration. Add `GlobalInventoryPage.priceReview.test.tsx` and update `show-preparation/ShowInventory.test.tsx`, selection/readiness regressions affected by replacing the old controls. Remove `ShowInventoryControls.tsx` only after replacing its `ShowFilters` type imports/callers; retain still-used saved-list evidence components.

**Consumes:** Task 4 components/state, existing `useInventoryState`, `toggleCard`/`captureSelection`, `InventorySelectionBar`, and `handleInlinePriceSave`.
**Produces:** `/inventory?view=pricing` and `review=<purchase-id>` navigation, ordinary Inventory default, unified state ownership and authoritative post-save invalidation.

- [ ] Write integration tests RED for default normal Inventory, deep link into review, view-switch URL updates preserving unrelated params, compact normal-row assessment opening the correct reviewed card, and search/selection/draft retention. Assert no duplicate support dropdown or row expansion for the new pricing workflow.

```tsx
// Render GlobalInventoryPage with real providers and mocked HTTP inventory/evidence.
await user.click(screen.getByRole('button', { name: 'Price review', exact: true }));
expect(screen.getByRole('region', { name: 'Price review' })).toBeVisible();
await user.click(screen.getByRole('button', { name: 'Inventory', exact: true }));
expect(screen.getAllByRole('button', { name: 'Sell', exact: true })[0]).toBeVisible();
// Assert URL view=pricing/review identity separately through the test router.
```

- [ ] Keep hooks and state owner mounted across the view switch. Compose the new workspace below shared search/scope controls; normal inventory retains its established filters/default order/sales/matching/bulk behavior. Keep review-specific filter/order out of normal inventory sorting. Replace the old support control/type coupling rather than leaving a hidden second filter active.
- [ ] Give the shared bulk selection bar the review queue's visible IDs when selecting all, but keep its exact observed-version capture rules. Focus is not selection. Opening a preview, changing a price, or receiving new evidence does not silently update selected versions. Reuse the existing show destination/add path.
- [ ] Reuse `handleInlinePriceSave` and the existing reviewed-price API. On success invalidate inventory plus the existing `showPrepKeys.all` cache family (evaluations, evidence, lists, preview) without publishing a hypothetical result as committed. On write failure retain draft and do not call success/advance behavior. Distinguish a confirmed save followed by a failed cache reload: show `Price saved; assessment could not be refreshed` and offer a read retry, not another automatic mutation or a false `Save failed`. Keep the saved-assessment region pending/stale until authoritative data arrives. Add regressions for that distinction and that no override API is used.
- [ ] Remove the old show-support dropdown/readiness-heavy header and duplicate price-support expansion from ordinary inventory. Retain compact status linking into review and optional collection diagnostics in the review panel. Do not delete the existing expansion still used by operational sale/pricing actions without checking its callers.
- [ ] Run `(cd web && npm test -- src/react/pages/GlobalInventoryPage.priceReview.test.tsx src/react/pages/show-preparation src/react/pages/campaign-detail/inventory src/react/pages/price-review && npm run typecheck && npm run lint && npm run build)` and inspect normal Inventory for regression at desktop/mobile. Stage explicit paths, run tracked hook, commit `feat: integrate price review alongside normal inventory`.

## Task 6: Real-wire acceptance, regression gates, documentation, cleanup

**Files:** Create `cmd/slabledger/price_review_e2e_test.go` and `web/tests/price-review-real.cjs`; reuse fixture ownership/reset/auth/source-control helpers from existing `showprep_readiness_*_test.go`. Update affected existing cached/worker browser selectors and fixture canonical prices without weakening provider/financial assertions. Update `docs/API.md`, `docs/USER_GUIDE.md`, `web/tests/show-readiness-real.md`, and append a clearly scoped section to `implementation-notes.md`.

**Consumes:** Fully integrated Tasks 1–5, real router/services, owned PostgreSQL fixture, built frontend, controlled external sources.
**Produces:** A repeatable `TestPriceReviewRealBrowser`, updated adjacent workflows, documented API/UX, and explicit evidence of intended versus forbidden writes.

- [ ] Add the real-browser test before declaring integration complete. `TestPriceReviewRealBrowser` explicitly uses the cached fixture setup with the worker disabled; it must not depend on an unset mode's default. Seed anonymized fresh evidence reproducing falling-market, supported, mixed, low-single-sale, old-sale, unpriced, and failed cases. Use known committed prices, not only DH prices. Launch owned Chromium like the existing harness, not the shared CDP browser.
- [ ] Keep source/provider blocked while exercising navigation/filter/sort/preview. Assert provider counters and whole purchase/sale/list rows unchanged through this phase. Cached API reads are allowed; preview must add no association-hold or financial writes.
- [ ] In a separately marked phase, explicitly save 240000 for the declining-market fixture through the actual reviewed-price route. Assert the intended reviewed-price persistence and refreshed assessment, plus existing controlled DH sync/list side effects for eligible cases. Attribute these intentional writes separately from immutable preview/navigation proof; no misleading whole-flow zero-write claim.
- [ ] Verify source-free post-save assessment, held draft/background change, still-selected stale show versions, explicit add/pack acknowledgment, normal sale/list controls, mobile return-to-queue focus, and a fresh-context deep link. Keep asynchronous HTTP failures visible and fixture cleanup owned.

```js
// price-review-real.cjs uses the owned real application fixture URL/auth, not route mocks.
await page.getByRole('button', {name:'Price review', exact:true}).click();
await page.getByRole('button', {name:/Review Declining fixture/}).click();
await page.getByLabel('Asking price').fill('2400');
await expect(page.getByRole('region', {name:'Trial price assessment'})
  .getByText('Supported', {exact:true})).toBeVisible();
await page.getByRole('button', {name:'Save price', exact:true}).click();
await expect(page.getByLabel('Saved asking price')).toHaveText('$2,400.00');
// Go owner then asserts reviewed_price_cents=240000 and exact permitted side effects.
```

- [ ] Update adjacent cached/worker modes to the new view/status/canonical-price contract. Keep legacy history, acquisition counters, authentication, cancellation, stale add/pack conflicts, and cleanup proofs. Do not retain tests that require the deliberately removed dropdown; replace them with equivalent operator actions. Do not rewrite unrelated scheduler/auth logic merely to satisfy test fixtures.
- [ ] Document seven-date/tie/strict-threshold policy, canonical asking, status meanings, read-only preview, save/auto-list behavior, old acknowledgment compatibility, query/selection/draft boundaries, and no production authorization. Record actual commands/results separately from planned checks.
- [ ] Run the focused and broader gates below, inspect final diff, use `polish-core --fix` on the actual implementation range, inspect safe fixes, and rerun affected gates. Obtain an independent implementation review and use `change-explainer` for the completion summary. Do not treat the earlier spec SHIP verdict as implementation review.
- [ ] Remove the archived `.superpowers/price-review-prototype/` directory and companion copies containing private data only after the selected layout has a verified production replacement and the operator no longer needs the comparison. Before cleanup, confirm the directories remain ignored and contain only this session's artifacts. Keep the approved spec/plan as the durable design record. Stage only implementation/docs/test files, run tracked hook, and commit `test: verify inventory price review end to end`.

### Owned database setup and verification commands

The old documented container is absent. At execution time recheck the name/port before creating a new owned resource; if either is occupied, stop and identify it rather than reset/reuse it blindly. This uses the exact database guard conventions already present, not a default developer DB.

```bash
docker ps -a --filter name=slabledger-price-review-test --format '{{.ID}} {{.Names}} {{.Ports}}'
ss -ltn '( sport = :44620 )'
docker run -d --name slabledger-price-review-test \
  --label slabledger.task=inventory-price-review \
  -e POSTGRES_USER=showprep -e POSTGRES_PASSWORD=showprep_test \
  -e POSTGRES_DB=showprep_readiness_e2e -p 127.0.0.1:44620:5432 postgres:17
# Wait until pg_isready succeeds; record container ID and verify loopback port/label.
docker exec slabledger-price-review-test pg_isready -U showprep -d showprep_readiness_e2e
docker exec slabledger-price-review-test createdb -U showprep -O showprep showprep_cached_test
docker exec slabledger-price-review-test createdb -U showprep -O showprep showprep_runtime_test
docker inspect slabledger-price-review-test --format '{{.Id}} {{json .Config.Labels}} {{json .NetworkSettings.Ports}}'
docker exec slabledger-price-review-test psql -U showprep -d showprep_readiness_e2e \
  -c "SELECT datname,pg_get_userbyid(datdba) FROM pg_database WHERE datname IN ('showprep_readiness_e2e','showprep_cached_test','showprep_runtime_test');"
```

Use separate storage/runtime/browser DBs; package tests reset public schema. Clear inherited DB variables before supplying explicit test URLs. Run browser phases separately from parallel-package tests, with new artifact directories. These commands are planned execution, not results already obtained:

```bash
unset DATABASE_URL POSTGRES_TEST_DSN POSTGRES_ADMIN_URL LOCAL_DB_URL SUPABASE_URL SHOW_READINESS_E2E_URL
POSTGRES_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_cached_test?sslmode=disable' \
SHOW_PREP_RUNTIME_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_runtime_test?sslmode=disable' \
TZ=UTC go test -race -count=1 -timeout 10m ./...
(cd web && npm test && npm run typecheck && npm run lint && npm run build)
make check
node --test web/tests/show-readiness-browser-checks.cjs

unset POSTGRES_TEST_URL SHOW_PREP_RUNTIME_TEST_URL
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_ARTIFACTS=/tmp/price-review-real-20260916 \
TZ=UTC go test -race -count=1 -v -timeout 10m ./cmd/slabledger -run '^TestPriceReviewRealBrowser$'
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_MODE=cached SHOW_READINESS_ARTIFACTS=/tmp/price-review-adjacent-cached-20260916 \
TZ=UTC go test -race -count=1 -v -timeout 10m ./cmd/slabledger -run '^TestShowReadinessRealBrowser$'
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_MODE=worker SHOW_READINESS_ARTIFACTS=/tmp/price-review-adjacent-worker-20260916 \
TZ=UTC go test -race -count=1 -v -timeout 10m ./cmd/slabledger -run '^TestShowReadinessRealBrowser$'
git diff --check
```

Record container ownership in the updated runner documentation. Stop/remove only the container created for this task after its tests and artifact collection are complete; do not touch the unrelated running project databases. If a gate fails due to an unrelated hook/configuration issue, report and ask rather than fix unrelated code or use `--no-verify`.

## Dependency and review order

`Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6`.

Each task owns a separate testable deliverable and review gate. Do not deploy intermediate commits: the feature is complete only after the corrected assessor, existing-list compatibility, preview, workspace, and real-wire verification agree. A frontend worker may develop Task 4 against the fixed Task 3 contract while its transport tests finish, but one worker owns shared `InventoryTab` integration and no agent silently changes the agreed DTOs.

## Plan self-review checklist

- [x] Mapped spec §§1–2/4 to Tasks 4–5; §3 to Tasks 1–2; §5 preview/save to Tasks 3/5; §5 history to Task 2; §6 to Task 6.
- [x] Marked proposed units Create and checked 20 existing integration paths. During execution, resolve remaining grouped fixture updates by their actual callers/failures rather than deleting assertions indiscriminately.
- [x] Checked shared DTO/policy/status/cents/hook/component contracts. Preview keys include committed-price business changes; assertions target the preview region rather than an unrelated Supported row.
- [x] No unfinished placeholder markers found. Planned commits exclude the private prototype and test secrets. No deployment or production-data operations are included.
