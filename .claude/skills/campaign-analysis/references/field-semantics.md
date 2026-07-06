# Field Semantics Catalog

The canonical source of truth for what every API field on the production endpoints actually
represents and how it can and cannot be used in analysis. Every Layer-1 domain agent loads
this file on each invocation and attaches the matching row's `gotcha` text to the
`semantics_caveat` of any fact-sheet row that surfaces the field. The Layer-3 adversary
loads this file to enforce the `forbidden_uses` column against every synthesis claim.

## How agents use this file

1. **Layer-1 domain agents.** Before emitting a fact-sheet row, look up the underlying
   field path in this catalog. If a row exists, copy its `gotcha` verbatim into the
   fact-sheet row's `semantics_caveat`. If no row exists, set `semantics_caveat: null`
   (do not invent a caveat — surface the gap to the reviewer instead).
2. **Layer-3 adversary.** For every `[id:...]` citation in the synthesis draft, look up
   the corresponding fact-sheet row's `semantics_caveat`. Check whether the surrounding
   prose violates anything in the `forbidden_uses` column. If yes: redline with the
   caveat text quoted.
3. **Layer-4 reviewer.** When the operator corrects a mistake in-session, the reviewer
   may auto-append a new row here (Tier A). Schema must match.

## Row schema

Each row is a level-2 section (`## <field path>`) with five required subsections:

- **Data semantics** — what the value actually is (provenance, refresh model)
- **Gotcha** — one-sentence caveat copied verbatim into `semantics_caveat`
- **Allowed uses** — bullet list
- **Forbidden uses** — bullet list (the adversary enforces this)
- **Source** — endpoint(s) and jq path(s) where the field appears

---

## `inventory.items[].purchase.clValueCents`

**Data semantics.** Live-updated CardLadder value for the card, refreshed by the pricing
pipeline and stamped onto `purchase.clValueUpdatedAt`. It is the present value snapshot,
not a frozen-at-purchase value. There is no on-record historical CL value for the
purchase; the value at acquisition has been overwritten by the current refresh.

**Gotcha.** `clValueCents` is live-updated per `clValueUpdatedAt` and is NOT the CL
value at time of purchase. Any ratio `buyCostCents / clValueCents` is a present-tense
mispricing signal relative to TODAY's CL, not a realized-at-purchase percentage.

**Allowed uses.**
- Present-tense mispricing vs today's CL (e.g. "this card was bought for 60% of its
  current CL").
- Per-card unrealized-edge sorting.

**Forbidden uses.**
- Presenting `buyCostCents / clValueCents` as "realized buy % at time of purchase."
- Inferring "PSA paid X% of contract" from any per-card ratio against this field.
- Computing historical campaign performance from this field.

**Source.** `/api/inventory` → `.items[].purchase.clValueCents`; also surfaced in
`/portfolio/snapshot` performer rows.

---

## `tuning.byGrade[].avgBuyPctOfCL`

**Data semantics.** Mean of per-card ratios `buyCostCents / clValueCents` across the
SOLD subset, where `clValueCents` is the live value (see row above), NOT the value at
purchase. Computed server-side in the tuning endpoint.

**Gotcha.** Computed against current `clValueCents` (live-updated), not CL-at-purchase,
and only over sold cards. Treat as a directional present-mispricing indicator on the
sold subset, not as a realized buy percentage.

**Allowed uses.**
- Directional signal that the sold subset of a grade was acquired below or above
  today's CL.
- Cross-grade comparison within a single campaign tuning response.

**Forbidden uses.**
- Interpreting as "PSA is paying X% of contract terms."
- Recommending a term change from this value alone (always pair with a fill-rate or
  ROI fact from a different endpoint — see two-source rule).
- Aggregating across campaigns to claim portfolio-wide buying performance.

**Source.** `/api/campaigns/{id}/tuning` → `.byGrade[].avgBuyPctOfCL`.

---

## `tuning.byGrade[].roi`

**Data semantics.** Net profit divided by total spend, computed only over the SOLD
subset within the grade. Unsold cards are excluded from both numerator and denominator.

**Gotcha.** Sold-subset ROI only; ignores unsold inventory entirely. Sample size can
be tiny — always cite `tuning.byGrade[].sampleCount` alongside.

**Allowed uses.**
- Within-campaign grade comparison.
- Identifying grade rows where the sold subset is materially profitable.

**Forbidden uses.**
- Extrapolating to portfolio-wide ROI without weighting by sample size.
- Comparing across campaigns without normalizing for sold-subset size.

**Source.** `/api/campaigns/{id}/tuning` → `.byGrade[].roi`.

---

## `weeklyReview.spendThisWeekCents`, `salesThisWeek`, `revenueThisWeekCents`, `profitThisWeekCents`

**Data semantics.** Partial-week aggregates for the current ISO week, computed from
midnight Monday through "now." The companion field `daysIntoWeek` indicates how
many days have elapsed; values are NOT projected to a full-week equivalent.

**Gotcha.** Partial-week aggregates when `daysIntoWeek < 7`. Do NOT compare to
full-week trailing means or characterize the week's trajectory without surfacing
`daysIntoWeek` explicitly.

**Allowed uses.**
- Reporting the raw partial-week figure with explicit `daysIntoWeek` caveat.
- Pacing sanity check ("on pace for N if linear").

**Forbidden uses.**
- Direct comparison to `weeklyHistory[]` rows (those are full weeks).
- "Down vs trailing 4-week mean" framing.
- Trend characterization ("worst week in a month") without normalization.

**Source.** `/api/portfolio/snapshot` → `.weeklyReview.*`.

---

## `weeklyHistory[]`

**Data semantics.** Array of full ISO weeks, ordered oldest → newest, terminating
one week before the current partial week. Each row is a closed 7-day window.

**Gotcha.** Full weeks only; the current partial week is in `weeklyReview`, not here.

**Allowed uses.**
- Trailing-mean computation (4w, 8w).
- Week-over-week comparison.
- Trend characterization.

**Forbidden uses.**
- Treating the most recent row as "this week."

**Source.** `/api/portfolio/snapshot` → `.weeklyHistory[]`.

---

## `campaigns[].buyTermsCLPct`

**Data semantics.** Local mirror of the PSA-side contract buy terms (percent of CL the
PSA partner offers for an accepted card). Sourced from operator-entered campaign
config; updated when the operator records a term change.

**Gotcha.** Local mirror of PSA-side configuration. May drift from PSA's actual
contract if a recent change wasn't pushed through here. The operator is the
canonical source of truth, not this field.

**Allowed uses.**
- Reporting the on-record terms with explicit "per local config" framing.
- Cross-campaign term comparison.

**Forbidden uses.**
- Claiming "PSA is buying at the wrong terms" from any divergence between this
  value and per-card realized ratios (the divergence is more likely a stale local
  mirror or a live-CL artifact — see `clValueCents` row).
- Treating as ground truth without confirming with the operator when a contradiction
  appears.

**Source.** `/api/campaigns` → `[].buyTermsCLPct`.

---

## `health.campaigns[].roi` and `health.campaigns[].sellThroughPct`

**Data semantics.** Lifetime-to-date aggregates per campaign. NOT filtered by the
operator's `currentScope` window.

**Gotcha.** Lifetime figures; ignore the currentScope filter. Use `inventory`
+ purchase/sale joins for currentScope-filtered ROI.

**Allowed uses.**
- Long-horizon campaign health comparison.
- Identifying campaigns whose lifetime ROI is structurally negative.

**Forbidden uses.**
- Presenting as the current-period ROI.
- Mixing with currentScope-filtered metrics in the same comparison.

**Source.** `/api/portfolio/snapshot` → `.health.campaigns[].roi` and `.sellThroughPct`.

---

## `health.campaigns[].inHandCapitalCents == 0` portfolio-wide

**Data semantics.** Sum of `inHandCapitalCents` across all active campaigns is zero
when every received card has either sold or been transferred out. It is NOT a
broken-pipeline signal by default — it commonly reflects a healthy clear-out.

**Gotcha.** Portfolio-wide `inHandCapitalCents == 0` is often the real business
state (all received cards sold). Confirm with the operator before characterizing
as a pipeline gap.

**Allowed uses.**
- Reporting the figure with neutral framing.
- Asking the operator to confirm whether unsold-in-hand inventory exists.

**Forbidden uses.**
- Declaring "the pipeline is broken" without operator confirmation.
- Treating as evidence of a data-ingestion problem.

**Source.** Aggregate over `/api/portfolio/snapshot` → `.health.campaigns[].inHandCapitalCents`.

---

## `dh_status.dh_listings_count` vs `mapped_count`

**Data semantics.** Throughput-gated counters from the DH listing pipeline.
`mapped_count` reflects cards eligible for listing; `dh_listings_count` reflects
listings live on DH. The gap is the in-transit pipeline plus operator-side
gating.

**Gotcha.** Known throughput gap. Surface only when `dh_listing_gap` appears on
`operationalPriorities`; otherwise this is intentional gating, not a fix target.

**Allowed uses.**
- Confirming the gap is within expected range during a DH-focused playbook.
- Reporting `dh_listings_count` as the published-listing count.

**Forbidden uses.**
- Recommending "close the listing gap" as a generic improvement.
- Treating the gap as a profitability lever.

**Source.** `/api/dh/status`.

---

## `intelligence/campaign-signals.computed_at`

**Data semantics.** Timestamp of the most recent compute pass for the campaign
intelligence rollup. Cached server-side; refreshes on a scheduled job, not on
every read.

**Gotcha.** Stale if older than 7 days. Treat any derived signals as advisory
when `computed_at` exceeds the freshness threshold.

**Allowed uses.**
- Citing signals with freshness annotation.
- Triggering a refresh request when stale.

**Forbidden uses.**
- Using stale signals as the basis for an actionable recommendation without
  surfacing the staleness.

**Source.** `/api/intelligence/campaign-signals` → `.computed_at`.

---

## `intelligence/niches[].market.analytics_not_computed == true`

**Data semantics.** Flag indicating the supply and velocity sub-fields are nulls
for this niche because the underlying analytics job hasn't run. The demand
sub-score is independently computed and remains valid.

**Gotcha.** Supply/velocity fields are null when this flag is true; demand
score is still valid in isolation.

**Allowed uses.**
- Citing the demand score on its own.
- Surfacing the flag to the operator as a coverage gap.

**Forbidden uses.**
- Reporting null supply/velocity as zero.
- Computing a composite niche score without weighting around the missing fields.

**Source.** `/api/intelligence/niches` → `[].market.analytics_not_computed`.

---

## `inventory.items[].purchase.setName`

**Data semantics.** The set/product line the card belongs to as returned by the
pricing API. For Japanese parallel cards this field contains the string
"JAPANESE" (e.g. "JAPANESE BASE SET"); the English variant of the same character
will have a set name without that token.

**Gotcha.** Japanese parallel detection must use `setName` (contains "JAPANESE"),
NOT `cardName`. Using a regex on `cardName` will under-count JPN cards and
misidentify which specific cards are outliers.

**Allowed uses.**
- Filtering or grouping by language: `.setName | test("JAPANESE"; "i")`.
- Segmenting dollar-weighted buy-pct analysis into EN vs JPN cohorts.

**Forbidden uses.**
- Using `cardName` regex to detect Japanese parallels.
- Treating a JPN cohort with elevated buy% as a CL-match error — CL correctly
  matches the JPN card; the over-pays reflect real acquisition cost on JPN parallels.

**Source.** `/api/inventory` → `.items[].purchase.setName`.

---

## `campaigns[].inclusionList` (character-keyed)

**Data semantics.** A list of character names (e.g. "Charizard", "Pikachu") that
gate which cards the PSA partner will submit for a campaign. The key is the
character name only — there is no language, set, or variant qualifier in the
schema.

**Gotcha.** English and Japanese variants of the same character share the same
character-name key. Whitelisting or blacklisting a character affects ALL language
variants of that character. There is no structural lever in the inclusion list to
restrict a campaign to English-only cards.

**Allowed uses.**
- Adding or removing a character across all its variants.
- Reporting which characters are in or out of scope.

**Forbidden uses.**
- Proposing a "whitelist English only" inclusion-list fix — the field is
  character-keyed; JPN Charmeleon and EN Charmeleon cannot be split by this field.
- Treating inclusion-list changes as a JPN-parallel exclusion mechanism.

**Source.** `/api/campaigns` → `[].inclusionList`; also surfaced in campaign
config endpoints.

---

## `purchase.clValueAtPurchaseCents` (frozen CL-at-buy)

**Data semantics.** The CardLadder value stamped ONCE at the moment of purchase and
never overwritten (unlike the live `clValueCents`). Set to `0` on rows that pre-date
the snapshot feature or were imported without a CL value. This is the denominator behind
`/analysis` `bpclAtBuy`.

**Gotcha.** `0` means "no snapshot," not "CL was zero." Rows with `0` are excluded from
`bpclAtBuy` aggregates; the endpoint's `coveragePct` tells you how much of the campaign
has a snapshot. Caveat thin coverage before treating `bpclAtBuy` as portfolio-wide.

**Allowed uses.**
- As the CL-at-buy denominator for clean buy-quality ratios (`buyCost ÷ clValueAtPurchase`).
- Distinguishing buy skill (this field) from post-purchase CL drift (`clValueCents`).

**Forbidden uses.**
- Treating a `0` as a real CL value (it is a missing-snapshot sentinel).
- Reading `bpclAtBuy` over thin coverage as a whole-campaign figure.

**Source.** `campaign_purchases.cl_value_at_purchase_cents`; aggregated into
`/api/portfolio/analysis` → `.campaigns[].bpclAtBuy`.

---

## `sale.forcedLiquidation`

**Data semantics.** Boolean heuristic: true when the sale went through a forced channel
(`inperson`, `local`, `cardshow`) within ≤6 days before an invoice due date. Operator-overridable
per sale. Drives the `pnl.discretionary` vs `pnl.forced` split in `/analysis`.

**Gotcha.** A heuristic, not ground truth — it can misfire. The endpoint reports BOTH
splits so a misflag is visible, not silent. When forced ROI looks anomalous, check whether
the flag over/under-captured before drawing a conclusion.

**Allowed uses.**
- Separating discretionary sale economics from invoice-driven fire-sale drag.
- Explaining a "bad ROI" character as forced-liquidation contamination (R-025 #1).

**Forbidden uses.**
- Treating the split as exact when coverage is small.
- Ranking or excluding a character on `pnl.forced` economics — forced-sale outcomes are
  contaminated by cash-crunch timing, not buy quality.

**Source.** `campaign_sales.forced_liquidation`; aggregated into
`/api/portfolio/analysis` → `.campaigns[].pnl.{discretionary,forced}`.

---

## `analysis.campaigns[].bpclAtBuy` vs the old `avgBuyPctOfCL`

**Data semantics.** `bpclAtBuy.dollarWeighted` = `sum(buyCost) / sum(clValueAtPurchase)`
over the snapshot rows — buy cost divided by CL AT THE TIME OF PURCHASE. This is the clean
buy-quality metric the overhaul introduced. `meanDriftPct` reports CL drift since purchase
SEPARATELY, so drift never contaminates the buy-quality number.

**Gotcha.** The older endpoints' `avgBuyPctOfCL` (on `/tuning` and `/insights`) is
`buyCost ÷ current clValueCents` and remains CL-drift-contaminated — it measures
`terms × (CL_then / CL_now)`, not buy skill. Prefer `bpclAtBuy.dollarWeighted` from
`/analysis`. Only reach for `avgBuyPctOfCL` if `/analysis` is unavailable, and label it
as drift-contaminated when you do.

**Allowed uses.**
- Citing `bpclAtBuy.dollarWeighted` as realized buy quality (pair with contract
  `buyTermsCLPct`).
- Reading `meanDriftPct` as post-purchase market movement, not acquisition performance.

**Forbidden uses.**
- Presenting `avgBuyPctOfCL` (old endpoints) as clean buy quality.
- Ranking characters on either figure — buy quality is identical-by-construction at
  purchase (`terms × CL`); there is nothing to rank (R-025 #2, R-027 construction floor).

**Source.** `/api/portfolio/analysis` → `.campaigns[].bpclAtBuy`; contrast
`/api/campaigns/{id}/tuning` → `.byGrade[].avgBuyPctOfCL`.

---

## Retired ledger rules — semantic content absorbed here

The 2026-07 overhaul retired several data-hygiene Rules from the ledger because the
`/analysis` endpoint now guarantees what they policed. Their reasoning lives here:

- **R-009 (realized vs contract pairing).** Never write a buy% without saying which it is.
  Realized = `bpclAtBuy.dollarWeighted` (clean, `/analysis`) or the drift-contaminated
  `avgBuyPctOfCL` (old endpoints). Contract = `buyTermsCLPct` = the PSA-side bid ceiling.
  Realized > contract is a *diagnostic question* (CL drift, mix shift, JPN parallels), not a
  parameter recommendation (still enforced as skill hard-constraint 3 and ledger R-001).
- **R-010 (lifetime-cumulative on OLD endpoints).** `/tuning.byGrade` and
  `/insights.byCharacterGrade` are lifetime-cumulative and show fills at now-excluded grades.
  `/analysis` `inScopeByGrade` is already current-scope filtered server-side, so the manual
  filter ritual is gone — but if you fall back to the old endpoints, re-apply the current
  `gradeRange` / inclusion / `yearRange` / price filter from `/api/campaigns` yourself.
- **R-011 (insights field names).** `/insights` segment rows use `label` (not `character`)
  and `netProfitCents` (not `avgProfitCents`); jq projections of missing keys silently
  return null. `/analysis` uses explicit typed fields, so this only bites on the old
  endpoints — run `jq '.[0] | keys'` once before projecting from `/insights`.
- **R-024 (suggestion-engine bugs).** `/snapshot.suggestions` is a server-side engine with
  known bugs (`docs/private/2026-06-01-suggestion-engine-bugs.md`): it doesn't filter by era
  and sorts on portfolio-wide ROI. Before echoing a suggestion, check it against Decisions the
  operator already rejected (same campaign + lever + direction). `/analysis` does not emit
  suggestions, so this applies only when you pull `/snapshot` in a follow-up.
- **R-025 (contamination summary).** No character/grade-level buy-quality signal is derivable
  from this system's purchase/sale records. `roi`/`netProfitCents`/`avgMarginPct`/`sellThroughPct`
  are contaminated by forced invoice-driven liquidation (see `forcedLiquidation`); `avgBuyPctOfCL`
  is contaminated by post-purchase CL drift (see `clValueCents`); eBay-share is contaminated the
  same way as ROI. This is still a ⭐ gate in the ledger — character selection comes ONLY from
  operator domain judgment or external CL/MM market comps, never from ranking this data.

---

## Adding new rows

Tier A: the Layer-4 reviewer may append rows here whenever an in-session
correction reveals a previously uncatalogued gotcha. The reviewer copies the
five-subsection schema exactly. New rows do not require operator approval before
landing.
