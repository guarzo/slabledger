# Inventory price review

Date: 2026-09-16
Status: Approved direction and assessment policy; written design awaiting review before implementation.
Baseline: `6ca4b420` (merged PR #707).

## 1. Purpose and approved scope

Make it possible to audit SlabLedger asking prices against recent matching sales, inspect contradictions, and explicitly adjust a price without losing one's place in inventory. Show preparation is a secondary use of the same assessment, not its organizing concept.

Keep the existing operational Inventory view. Add a focused **Price review** view within `/inventory`, using the selected prototype B: compact inventory queue on the left, persistent evidence and price-editing panel on the right. Use a direct `Inventory | Price review` switch, not another dropdown or show-selection mode.

The operator selected B because the pricing workflow does not fit comfortably into existing expanding rows. Do not reproduce the new workspace underneath each row. Do not replace unrelated inventory operations or add new campaign navigation in this change. The current production caller of `InventoryTab` is `GlobalInventoryPage` despite the component's historical directory name.

### Primary action

Find an asking price that warrants attention, inspect recent sales, try a candidate price, explicitly save if appropriate, and continue to the next card.

### Non-goals

- Automatic repricing, bulk price application, or treating a median as a price instruction.
- Changing sales, costs, P&L calculations, fees, reservations, delisting, or campaign behavior.
- A second comp collection pipeline, browser-triggered acquisition, scheduler redesign, or provider changes.
- A wholesale application redesign, rebranding, or moving ordinary inventory operations into a pricing-specific queue.
- Repairing price synchronization without a separately demonstrated bug.
- Promoting the throwaway prototype or its private fixture into production.

## 2. Canonical asking price

Assess the latest operator-committed SlabLedger price using `inventory.ResolveListingPriceCents`. This already resolves reviewed prices and overrides by their commit timestamps.

- DH is expected to mirror SlabLedger. A persistent mismatch is a synchronization defect, not a legitimate alternate pricing choice or a normal operator workflow.
- Do not use CL valuation, a suggested price, or unsaved input as the saved asking price.
- Without a positive committed asking price, show **No asking price**, while leaving available sales inspectable.
- A trial price is explicitly hypothetical until the existing save operation succeeds.
- Retain DH diagnostic fields and integrity holds where required by existing listing/packing behavior; do not use them as the target for the new price assessment.

## 3. Assessment policy

Compute the saved-price assessment, hypothetical-price preview, and show-list assessment from one backend implementation. All monetary arithmetic uses integer cents; median qualification must preserve half-cent precision until comparison. Rounded display values must not decide qualification.

### Evidence and recency

1. Retain the existing verified matching identity, sale validity, source provenance, complete current 30-UTC-date capture, and capture-freshness requirements.
2. Within that capture, consider the current UTC date and the preceding six UTC dates: seven calendar dates inclusive.
3. Choose the newest five eligible sales in that seven-date window. Include **all** sales tied on the fifth sale's date. This group can contain more than five sales.
4. Stored evidence has day precision. Never invent a within-day order or label an arbitrary tied record as the unique latest sale. Display the latest sale-day range/count when appropriate.
5. Preserve all eligible records, including low sales. Do not remove inconvenient observations as outliers to restore support.
6. The wider 30-day history remains context. It cannot override a recent contradiction. Existing separately labeled 90-day analytics are not silently relabeled as verified recent evidence.

### Thresholds and classification

A recent-group median **at least 90%** of asking meets the broad support threshold. A sale on the newest sale-date **more than 20% below** asking is a sharp contradiction; exactly 20% does not cross that strict boundary.

| Assessment | Condition |
|---|---|
| No asking price | No positive committed asking price exists. Evidence health is still reported separately. |
| Evidence unavailable/stale | Evidence fails the existing validity, completeness, provenance, or freshness requirements. Retained sales may be shown with an explicit warning, never certified as current support. |
| Limited evidence | Fewer than two sales qualify for the recent group, including zero. Show the actual count, dates, and any contradictory amount. This is not a positive or negative market conclusion. |
| Asking above comps | At least two recent sales exist and their median is below 90% of asking. |
| Mixed evidence | The recent median passes, but at least one sale on the newest date is more than 20% below asking. Never green. |
| Supported | At least two recent sales exist, their median passes, and there is no sharp newest-day contradiction. |

For two or more recent sales, the negative recent-median condition takes precedence over the mixed-evidence condition. Evidence health and the availability of an asking price remain independently inspectable rather than one hiding the other.

**Gap display:** `(asking - recent reference) / asking`, labeled explicitly as the recent reference being below or above asking. No percentage without a positive asking price or a trustworthy recent reference. A single sale is labeled a single sale, not a well-supported market valuation.

### Acceptance examples

- Asking $3,200; eight newest eligible sales spanning September 15–16 with median $2,320, newest-day sales $2,025/$2,315: **Asking above comps**, even if the 30-day median is $3,000.
- The same evidence at $2,540: **Mixed evidence**. At $2,400: **Supported**. These are policy classifications, not recommended listing prices.
- Asking $100; newest sale $79, preceding recent sales $100/$100: **Mixed evidence**.
- Asking $100; newest recent sales $100/$100, earlier sale $70: **Supported**. A historical dip must not permanently veto a subsequent recovery.
- Asking $1,625; one recent sale at $1,000: **Limited evidence**, visibly stating that the only recent sale is 38.5% below asking.
- Two high sales outside the seven-date window: **Limited evidence**, even after a fresh capture.
- A median exactly at 90% passes; a half-cent below fails. Same-date input permutations do not change results.

## 4. UX brief: focused price review

### Design direction

Retain the existing dark/copper product system. Restrained neutrals, copper for interaction, semantic red/amber/green only for assessment meaning. Existing system UI typography and tabular numeric typography; no new fonts, decorative motion, or decorative imagery.

Scene: a desktop operator auditing prices across many slabs in a short working session, needing to compare evidence and act without repeatedly opening and closing rows. References: SlabLedger's existing inventory vocabulary, Linear-style queue/detail navigation, and Stripe-style dense financial tables. The selected B prototype is the structural reference, not production code.

Fidelity and breadth: production-quality implementation of the focused inventory pricing workflow, alongside the unchanged operational inventory view. The two browser variants were interactive throwaway probes. Native image generation was unavailable; actual editable mockups were used instead.

### Navigation and scope

- Normal Inventory remains the default. A shareable `view=pricing` query parameter opens Price review without creating a separate top-level pricing/show page. Preserve unrelated URL parameters.
- Use the same underlying inventory cohort. Do not silently exclude unreceived or unlisted inventory from a pricing audit.
- Preserve search and bulk-selection intent when switching views. Assessment filtering and pricing sort belong to the review workspace; do not unexpectedly change the normal inventory's established default ordering.
- Remove the old show-specific support dropdown and duplicate support expansion. A compact assessment in normal inventory can link to the corresponding card in Price review.
- Pricing controls distinguish the committed asking price from market references. Sorting by asking must sort the amount displayed as asking, not an unrelated last-sale value.

### Queue

- Show identity/grade, asking price, recent reference with count/date context, assessment, and meaningful gap.
- Filter with direct count-bearing controls: All, Above comps, Mixed, Limited, Supported, Unavailable, Unpriced. No support dropdown.
- Offer pricing-attention-first and supported-first ordering, plus explicit asking/recent-reference/gap sorts.
- Pricing attention prioritizes contradicted asking prices, then uncertain cases, then supported cards. Within comparable assessments, put larger shortfalls first. Unknown evidence is not a zero-dollar comp or a zero-gap bargain.
- Search by card, cert, set, or campaign. Counts must refer to the displayed scope, not an unrelated global cohort.
- Preserve stable purchase identity and the active card as background evidence changes. Do not make an editor jump to another card because a refresh changes ordering.

### Persistent detail and price editor

- Present identity, saved asking price, assessment/reason, recent median or single-sale amount, recent group size/date range, and newest sale-day amount/range together.
- Show individual matching sales with dates, amounts, platforms, and safe source links. Make broader history progressive; put source diagnostics behind disclosure rather than ahead of the useful sales evidence.
- Permit entering a trial price and inspecting its hypothetical assessment before saving. A reference-price shortcut fills the draft only; it does not save or authorize repricing.
- Keep the saved amount and draft amount distinguishable. Show cost and clearly labeled before-fee arithmetic, not a guaranteed sale profit.
- Explicit Save uses the existing reviewed-price operation. It already triggers asynchronous DH price synchronization and auto-listing of eligible unlisted inventory. Make that existing consequence visible before saving; do not claim the HTTP save response proves DH synchronization completed. Report local save success only after persistence succeeds, then obtain authoritative refreshed inventory/assessment data.
- Preserve draft input through preview errors and evidence reloads. A stale preview must not appear current for newly typed input. Switching cards/views retains in-memory drafts; nothing autosaves.
- If a background inventory update changes the saved price while a draft exists, preserve the draft and surface the changed baseline for review rather than silently replacing either value.
- After a save changes filter membership, keep the active card visible in the detail panel with an outside-filter indication until the operator deliberately moves on. Do not silently advance as a side effect of a price change.
- Provide explicit next/previous navigation through the filtered queue.

### Shows and other inventory actions

Reuse the existing saved-list destination flow and observed-version safeguards. Queue selection and show-list membership are different concepts. Price evidence does not change physical packing eligibility. Review or price edits do not acknowledge stale bulk-selection versions on the operator's behalf.

Ordinary sales, DH matching/listing, and bulk operations remain available in normal Inventory. The pricing panel must not acquire a duplicate sale/matching workflow to become a replacement for all inventory operations.

### Mobile, keyboard, and key states

- Desktop: persistent side-by-side queue and detail. Narrow screens: a focused detail presentation with a clear return to the same queue position, rather than squeezing both panes or requiring two competing nested scroll regions.
- Keyboard-reachable controls, explicit selection state, visible focus, named form inputs and regions. Navigation shortcuts must not intercept editing keys.
- Handle empty inventory, no filter matches, no asking price, zero/one recent sale, mixed evidence, partial/stale/unavailable evidence, loading, preview failure, save failure, background changes, and removed/sold active cards.
- Do not add UI-level automatic replay after an uncertain save result. Preserve the existing price-save/transport contract, retain the draft, and recheck saved state before the operator deliberately retries.

## 5. Implementation boundaries

Reuse the existing evidence store and independent server worker from PR #707. Browser navigation, search, filters, trial-price checks, and selection must not acquire provider comps.

The existing show-preparation domain already owns the evidence evaluator and saved-list validation. Introduce the recent-price assessment there without a package-renaming/refactoring campaign. Expose general-purpose pricing terminology in the UI; do not create a competing frontend interpretation or second domain pipeline.

Keep existing API fields with their documented meaning where feasible: 30-day median/count and stored DH diagnostics remain distinguishable from the new recent-group summary and canonical asking target. `localPriceCents` already carries the resolved committed price; do not silently redefine `listedPriceCents`, which carries DH's stored price. Retain the existing status codes and add `mixed_evidence`. `no_recent_comps` and `thin_evidence` may both present as Limited evidence; do not equate that display grouping with a source failure.

Add authenticated `POST /api/show-prep/preview`, accepting purchase identity and positive integer trial cents. Read the purchase and cached evidence once using the existing reader ports, and run the shared assessment. This endpoint must not call the existing `Evidence` service method, which also persists observed association holds. Return current/trial price, evidence/policy identifiers, health, and recent facts, but no list-mutation evaluation token. Do not acquire comps, wake the worker, observe/write holds, or write financial/list state. Do not copy the prototype's JS evaluator into the React application.

Use the existing `setReviewedPrice(..., 'manual')` write and its DH synchronization/listing path. Do not substitute the override endpoint, whose side effects differ. No new background writer, automatic lowering, or new listing semantics. Changing the evaluator must not reprice anything.

Existing reviewed-price writes are last-writer-wins by purchase ID, without an atomic stale-draft guard. Detect observed background changes in the UI and preserve drafts, but do not promise compare-and-swap protection or add a new persistence/concurrency contract in this change. The remaining simultaneous-writer race is an explicit limitation.

### Historical compatibility

- Preserve stored list members, packing state, acknowledged amounts/statuses, retries, and durable price-association holds. No destructive migration or automatic acknowledgment/backfill.
- Derive current show-list assessment/value from the canonical SlabLedger target. Compare historical acknowledged values with the current target and surface changes; do not rewrite old acknowledgments to make warnings disappear.
- New explicit add/acknowledge commands record the canonical price and current assessment. Preserve the existing stale-command and exact-retry protections.
- Preserve the original meaning of legacy stored amounts/statuses as observations recorded at that time, not proof that the operator acknowledged the new policy. Do not fabricate historical policy provenance or encode hidden policy tags inside the status string. A separate historical policy-audit feature is not required for the current assessment and is outside this scope; no schema migration is planned.
- Include an explicit current assessment-policy version and changed business assessment inputs/results in observed-version validation. Collection scheduling metadata alone must still not invalidate selections.
- Keep DH association holds and observations intact for DH integrity warnings. They must not suppress an assessment or known value derived from a clearly identified purchase's committed local price.

## 6. Verification and rollout

Write regression tests before implementation of changed behavior. Use anonymized fixture identities and explicit cents/dates; never commit the prototype's private real inventory capture.

Required coverage:

1. Domain: every assessment and precedence, exact thresholds, half-cent precision, seven-date boundaries, complete 30-day capture health, stale/future evidence, input-order invariance and cutoff-day ties, recovery, and canonical reviewed/override target selection.
2. Preview/API: authenticated validation, current cached evidence only, no acquisition or financial/list writes, proposed-price changes using the identical assessor, invalid inputs, and bounded error handling.
3. Storage/list compatibility: prior acknowledgments retained, current warnings, new canonical acknowledgments, sold/removed/ambiguous inventory safeguards, stale selected versions and exact retries.
4. React: both inventory views, filters/counts/sorts, stable active identity, preview request ordering, draft retention, successful/failed price save, current assessment after save, show selection, and responsive keyboard flows.
5. Real browser: the supplied falling-market pattern, a supported card, a low single-sale case, an old-sale case, and failed evidence. Exercise price adjustment through the real local API against disposable data before claiming production integration works.
6. Appropriate focused Go races and frontend checks, then broader suites/typecheck/lint/build/architecture checks for the actual implementation range. Independently review the final implementation and rerun checks after fixes.

No merge, deployment, production repricing, or production backfill is authorized by this design. The current application remains unchanged until implementation and its verification are complete.

## 7. Review focus

Confirm that the dedicated review workspace preserves the operational inventory workflow, that negative recent evidence cannot be averaged away, and that preview/save/list reads all use the same canonical target and policy without silently altering saved history.

The implementation plan should lead with the evaluator/API/history boundaries, then describe the mechanical UI replacement. Do not repeat the prior PRs' mistake of proving acquisition/concurrency mechanics while omitting the operator's actual pricing judgment.
