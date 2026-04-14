# Campaign-Analysis API Gaps

Development followups surfaced while validating the recommendation-first rewrite of the `campaign-analysis` skill against live production data (2026-04-14). Each entry is a real limitation that forced the skill's prose to route around the API. Fixing any of these would let the skill drop a defensive branch.

Ordered roughly by skill-quality impact.

## 1. `weekly-review` silently ignores `?weekOffset=N`

`GET /api/portfolio/weekly-review?weekOffset=N` accepts the parameter but returns the same current-week record for every value (0, 1, 4, 8 all produce the identical body).

Impact: the skill's Hold verdict rule can't compute a true σ across trailing weeks. The rule falls back to the rule-of-thumb (±10% of trailing-mean from this-week + last-week only), which undersells confidence when a longer history is available. The skill currently documents this as a known reality rather than a temporary workaround.

Fix options:
- Honor the `weekOffset` param so the skill can loop 0..3 to build a 4-week window.
- Or add `GET /api/portfolio/weekly-history?weeks=N` returning an array of weekly records.

## 2. `/api/credit/invoices` returns `amountCents: null` per row

Every invoice in the list endpoint has `amountCents: null`. The amount is only populated on `/api/credit/summary.nextInvoiceAmountCents` for the *next* invoice — there's no way to retrieve amounts for the second-next or third-next invoice programmatically.

Impact: the skill can only size the next invoice. When there are two unpaid invoices back-to-back (observed today: $65.7K due 4/16 and an unsized one due 4/30), the skill can't size the 2-week horizon without scraping. The prose currently notes "~$74.2K" with a rough estimate from `pendingReceiptCents`, which is an approximation, not the real invoiced amount.

Fix: populate `amountCents` on every row of the invoices endpoint.

## 3. `/api/portfolio/health` doesn't include in-hand vs in-transit breakdown

`totalUnsold` and `capitalAtRiskCents` are aggregates across both received and in-transit cards. The skill has to fetch `/api/inventory` and bucket by `purchase.invoiceDate` to distinguish them (items with invoiceDate ≤ today − 7 days are considered in-hand, using the ~1-week PSA receipt delay).

Impact: every `/campaign-analysis` run makes an extra (large) fetch and applies a heuristic. For campaigns that are 100% in-transit (observed today: Modern PSA 10), the headline `$10.7K at risk` is misleading because nothing is actually sellable.

Fix options:
- Add `inHandCapitalCents` / `inHandUnsoldCount` / `inTransitCapitalCents` / `inTransitUnsoldCount` fields per campaign on `/api/portfolio/health`.
- Or add portfolio-wide equivalents on `/api/credit/summary`.

## 4. `/api/dh/status` has counts but no card-level queue

`dh_status` exposes `pending_count`, `mapped_count`, `dh_inventory_count`, `dh_listings_count` as aggregates, but there's no endpoint that returns the card-level list of pending DH pushes with their recommended prices. The skill wants to size "approve the 33-pending backlog" with an expected $ recovery, and today it can't without scanning `/api/inventory` and joining against some DH-status field that isn't there on the purchase record.

Fix: add `GET /api/dh/pending` returning an array of `{purchaseId, cardName, recommendedPriceCents, daysQueued, dhConfidence}` for every card with `dh_push_status = "pending"`.

## 5. Tuning endpoint doesn't return variance / stddev

`GET /api/campaigns/{id}/tuning` returns per-grade-tier breakdowns with counts and ROIs but no per-tier standard deviation or coefficient of variation. The skill's Confidence bands rule specifies High as "≥30 observations AND CV < 20%" but always falls back to obs-count-only because CV is never computable.

Impact: confidence bands are effectively obs-count-based only. A campaign with 40 steady observations and a campaign with 40 wildly variable observations both land in High under the fallback.

Fix: compute and return `roiStddev` (or equivalent) per tier on `/api/campaigns/{id}/tuning`.

## 6. `/api/campaigns/{id}/projections` returns `confidence: null`

The scenario objects have a `confidence` field but it's always null in observed responses. The skill's rule says to map the server's confidence string directly to H|M|L when present — currently that branch never fires.

Impact: the skill falls back to tuning-endpoint obs-count, which in turn has its own issues (see #5). The projections endpoint is effectively confidence-less.

Fix: either populate `confidence` based on simulation inputs (sample size, variance, convergence), or remove the field and let the skill rely entirely on the tuning signal.

## 7. Projections returns all-zero scenarios for thin-sample campaigns

When a campaign has too little data for Monte Carlo to converge, `/api/campaigns/{id}/projections` silently returns `medianROI: 0`, `medianProfitCents: 0`, `medianVolume: 0` across every scenario (observed today on Wildcard, which has ~20 observations).

Impact: the skill has to heuristically detect "all zeros" to avoid proposing bogus sized recommendations from garbage data. An explicit signal would be cleaner.

Fix options:
- Return HTTP 422 with `{"reason": "insufficient_data", "minRequired": N, "available": M}` when the simulation can't run.
- Or add `insufficient_data: true` to the response body when convergence fails.

## 8. `/api/campaigns` includes the synthetic "External" bucket among active campaigns

`External` has `buyTermsCLPct: 0` and `dailySpendCapCents: 0` but `phase: "active"`. The skill has to filter by those field values to exclude it from the portfolio-at-a-glance line.

Fix: either set `phase: "synthetic"` (or similar) for External, or add a `synthetic: true` / `kind: "external"` flag so consumers don't have to heuristic-filter.

## 9. No explicit "week just started" signal in weekly-review

`/api/portfolio/weekly-review` returns the current week starting Sunday. On Monday morning (observed today: Sunday 4/13 → sales=0, spend=0), a consumer can't cheaply distinguish "the week has no activity because nothing happened" from "the week just started today." An explicit `weekDayIndex` (0–6) or `weekInProgress: true` field would disambiguate.

Fix: add `daysIntoWeek` or `weekInProgress: bool` to the response.

## 10. `/api/portfolio/suggestions.adjustments[].expectedMetrics` returns zeros

All of `expectedROI`, `expectedMarginPct`, `avgDaysToSell` come back as `0` in observed responses for the buy-term-lowering adjustments — even though the rationale text talks about meaningful liquidation losses the change would address. The skill can't size the adjustment's expected impact from this endpoint; it has to call the projections endpoint separately (which has its own problems — see #6 and #7).

Fix: populate the expected metrics, at least when the suggestion has concrete data points to project from. Currently the `expectedMetrics` object is dead weight.

---

## Tracking

When picking these up, each fix should come with a skill update that removes the defensive branch it enables. For example, fixing #3 (in-hand/in-transit in `/portfolio/health`) lets the skill drop the `/api/inventory` fetch + invoice-date bucketing from Step 3.

The skill's gap-handling prose is intentionally verbose so the behavior stays correct under today's constraints — but each defensive branch is carrying cost (extra API calls, more instructions for the model to juggle) that a proper backend fix would eliminate.
