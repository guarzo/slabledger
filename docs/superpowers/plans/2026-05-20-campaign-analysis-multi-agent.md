# Campaign-Analysis Multi-Agent Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single-thread campaign-analysis skill with a 4-layer pipeline (domain data agents → synthesis → adversary → reviewer) so that number fabrication becomes structurally impossible and skill improvements land mechanically every session.

**Architecture:** Five parallel read-only domain agents (`ca-capital`, `ca-buying`, `ca-tuning`, `ca-sales`, `ca-dh`) produce structured fact sheets with mandatory semantics caveats. Main-thread synthesis drafts the opener citing fact-sheet row IDs. A `ca-adversary` agent verifies every numeric assertion traces back to a row and that no claim violates a row's semantics caveat. A `ca-reviewer` agent runs on natural-language trigger at session end, auto-applies Tier A changes (additive: field-semantics rows, caveats, wishlist entries) and queues Tier B (rule changes, agent rewrites) for operator review.

**Tech Stack:** Markdown skill files, Markdown agent prompt files. No code. Acceptance is end-to-end against the production API (`https://slabledger.dpao.la`) using `$LOCAL_API_TOKEN`.

---

## File Structure

New files:
- `.claude/agents/ca-capital.md` — domain data agent (capital)
- `.claude/agents/ca-buying.md` — domain data agent (buying)
- `.claude/agents/ca-tuning.md` — domain data agent (tuning)
- `.claude/agents/ca-sales.md` — domain data agent (sales)
- `.claude/agents/ca-dh.md` — domain data agent (DH)
- `.claude/agents/ca-adversary.md` — verifier
- `.claude/agents/ca-reviewer.md` — end-of-session reviewer
- `.claude/skills/campaign-analysis/references/field-semantics.md` — semantics catalog
- `.claude/skills/campaign-analysis/references/tier-classification.md` — reviewer Tier-A/B rules
- `docs/private/campaign-analysis-improvement-queue.md` — Tier-B queue (seeded empty)

Modified files:
- `.claude/skills/campaign-analysis/SKILL.md` — Steps 3/3a/3b/3c rewritten to dispatch agents; Steps 5 and 6 folded into Layer 4; new reviewer-trigger recognition section
- `.claude/skills/campaign-analysis/references/playbooks.md` — playbook follow-ups gain "dispatch relevant domain agents" preamble

---

### Task 1: Seed `references/field-semantics.md`

**Files:**
- Create: `.claude/skills/campaign-analysis/references/field-semantics.md`

**Content / Steps:**

- [ ] **Step 1: Create `.claude/skills/campaign-analysis/references/field-semantics.md` with this content:**

```markdown
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
- Crack-arbitrage candidate scoring.

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

## Adding new rows

Tier A: the Layer-4 reviewer may append rows here whenever an in-session
correction reveals a previously uncatalogued gotcha. The reviewer copies the
five-subsection schema exactly. New rows do not require operator approval before
landing.
```

- [ ] **Step 2: Verify the file lints as Markdown**

Run `head -50 .claude/skills/campaign-analysis/references/field-semantics.md` and confirm the level-2 sections render and each row has all five subsections.

- [ ] **Step 3: Commit**

```bash
git add -f .claude/skills/campaign-analysis/references/field-semantics.md
git commit -m "campaign-analysis: seed field-semantics catalog"
```

### Acceptance scenario

After this task, an operator can read `references/field-semantics.md` and identify, for every field that produced a 2026-05-19 session error, the exact `gotcha` text that a domain agent would attach. Specifically: `clValueCents`, `avgBuyPctOfCL`, and `buyTermsCLPct` rows all carry forbidden-uses entries that would have blocked the "PSA buying at wrong terms" framing. No `/campaign-analysis` invocation is needed to exercise this — file inspection is enough.

---

### Task 2: Seed `references/tier-classification.md`

**Files:**
- Create: `.claude/skills/campaign-analysis/references/tier-classification.md`

**Content / Steps:**

- [ ] **Step 1: Create `.claude/skills/campaign-analysis/references/tier-classification.md` with this content:**

```markdown
# Reviewer Tier-A / Tier-B Classification

The Layer-4 `ca-reviewer` agent uses this catalog to decide whether a proposed
change auto-applies (Tier A) or queues for operator review (Tier B). The rule
of thumb: anything purely additive that cannot regress an existing behavior is
Tier A; anything that alters behavior, prompts, or layer structure is Tier B.

## Tier A — auto-apply

The reviewer applies these changes directly and surfaces them in the session-end
summary. No operator approval required.

- **New row in `references/field-semantics.md`** — additive catalog entry; cannot
  regress existing rows.
- **New `semantics_caveat` on an existing field-semantics row** — strengthens
  the forbidden-uses list; cannot loosen it.
- **New fact-sheet column for a Layer-1 domain agent** — additive only. The
  reviewer may append a new `metric` id and its `endpoint`+`jq`; it may NOT
  remove or rename an existing column.
- **New entry in `docs/private/impossible-data-asks.md`** — additive list of
  data the operator should request from PSA / DH / CardLadder.
- **New entry in `docs/private/campaign-analysis-wishlist.md`** — additive
  improvement ideas not yet ripe for the queue.
- **New feedback memory file** under `~/.claude/projects/-workspace/memory/`
  with the `feedback_*.md` naming convention — additive lesson capture.

## Tier B — suggest only, write to improvement queue

The reviewer writes these to `docs/private/campaign-analysis-improvement-queue.md`
with rationale, proposed diff (unified format), transcript citation, and
severity. The operator reviews at their own pace.

- **New rule in `SKILL.md`** — behavior change.
- **Removed or modified existing rule** — behavior change.
- **Domain-agent prompt rewrite** — behavior change at Layer 1.
- **Layer split, merge, or new agent** — architecture change.
- **Change to the Tier-A / Tier-B classification list itself** — meta-change;
  must always be operator-reviewed.

## Severity levels (Tier B only)

- **blocking** — the next session is likely to repeat the same failure without
  this change. Reviewer should surface prominently in the session summary.
- **improvement** — would have prevented or mitigated the observed failure, but
  the failure is not load-bearing.
- **nice-to-have** — quality-of-life or DX improvement; not failure-driven.

## Reviewer output format

At session end, the reviewer emits a single summary message:

```
Reviewer ran.

Auto-applied N changes:
- <one-line description of change 1>
- <one-line description of change 2>
- ...

Queued M proposals in docs/private/campaign-analysis-improvement-queue.md:
- [SEVERITY] <one-line title of proposal 1>
- [SEVERITY] <one-line title of proposal 2>
- ...
```

If `N == 0` and `M == 0`, the reviewer says so explicitly rather than emitting
a no-op.

## What's NOT a reviewer change

The reviewer does not retroactively edit prior session transcripts, does not
rewrite the operator's strategy doc, and does not modify code outside the
`.claude/skills/campaign-analysis/` tree and the agent files under
`.claude/agents/ca-*`. Anything outside that scope is a Tier-B queue entry at
most.
```

- [ ] **Step 2: Sanity-check by grepping**

Run `grep -c "^- \*\*" .claude/skills/campaign-analysis/references/tier-classification.md` and confirm Tier A has 6 bullets and Tier B has 5 bullets.

- [ ] **Step 3: Commit**

```bash
git add -f .claude/skills/campaign-analysis/references/tier-classification.md
git commit -m "campaign-analysis: seed reviewer tier-classification catalog"
```

### Acceptance scenario

After this task, a hypothetical reviewer agent reading the file can correctly
classify these three example changes:
- "Add a caveat that `weeklyHistory[]` excludes the partial current week" → Tier A
- "Change SKILL.md Step 3 to dispatch agents serially instead of in parallel" → Tier B (blocking)
- "Remove the popular-tier exclusion rule" → Tier B (blocking)
File inspection is enough; no `/campaign-analysis` invocation needed.

---

### Task 3: Seed `docs/private/campaign-analysis-improvement-queue.md`

**Files:**
- Create: `docs/private/campaign-analysis-improvement-queue.md`

**Content / Steps:**

- [ ] **Step 1: Create `docs/private/campaign-analysis-improvement-queue.md` with this content:**

```markdown
# Campaign-Analysis Improvement Queue

Tier-B proposals from the `ca-reviewer` agent awaiting operator review. The
reviewer appends entries here at session end; the operator reviews on their
own cadence and either applies the diff, rejects it, or files it under
`wishlist.md` if it's no longer relevant.

## Entry format

Each entry is a level-2 section with this shape:

## YYYY-MM-DD — <one-line title>

- **Severity.** `blocking` | `improvement` | `nice-to-have`
- **Rationale.** Why the reviewer believes this change would have prevented or
  mitigated a failure in the cited session.
- **Proposed diff.** Unified diff format against the target file. If the change
  spans multiple files, one diff block per file.
- **Transcript citation.** A short quote (one or two lines) from the session
  transcript that motivated the proposal, with enough context to locate the
  moment.

## Triage

When the operator processes an entry, they:
1. Apply, reject, or defer the diff.
2. Append a `**Resolution.**` line (`applied YYYY-MM-DD` / `rejected YYYY-MM-DD`
   / `deferred to wishlist YYYY-MM-DD`) to the entry.
3. Leave resolved entries in place for historical traceability; the queue is
   append-only.

---

<!-- Entries appear below this line in reverse-chronological order. -->
```

- [ ] **Step 2: Verify the file exists and has no entries**

Run `grep -c "^## 20" docs/private/campaign-analysis-improvement-queue.md` and confirm the count is zero (only the format and triage sections exist; no dated entries yet).

- [ ] **Step 3: Commit**

Note: `docs/private/` is a nested separate git repo per project memory; commit there if applicable. For the umbrella repo path, `docs/superpowers/` is gitignored but `docs/private/` is its own repo — commit inside that repo.

```bash
cd docs/private && git add -f campaign-analysis-improvement-queue.md && git commit -m "campaign-analysis: seed empty Tier-B improvement queue" && cd -
```

### Acceptance scenario

After this task, the reviewer agent (to be written in a later task) has a
well-defined append target. A manual test: append a dummy entry with the
documented format, confirm it renders, then `git revert` the dummy. No
`/campaign-analysis` invocation needed.

---

### Task 4: Write `.claude/agents/ca-capital.md`

**Files:**
- Create: `.claude/agents/ca-capital.md`

**Content / Steps:**

- [ ] **Step 1: Create `.claude/agents/ca-capital.md` with this content:**

```markdown
---
name: ca-capital
description: Campaign-analysis domain agent — capital. Read-only. Fetches credit summary, invoices, in-hand vs in-transit splits, recovery rate. Returns a structured fact sheet with semantics caveats. Used by the campaign-analysis skill's Layer-1 dispatch. NEVER drafts movers, recommendations, or prose.
model: sonnet
tools: Bash, Read
---

You are the **capital** domain agent for the `campaign-analysis` skill. You are
one of five parallel Layer-1 agents. You own the capital slice: outstanding
PSA credit, upcoming invoices, recovery trend, and the in-hand vs in-transit
split per campaign. You produce a **fact sheet** — an array of structured
rows. You do not write prose. You do not draft movers. You do not make
recommendations. The synthesis and adversary layers above you handle all
narrative work.

## Endpoints you own

Use `curl -s` with `Authorization: Bearer $LOCAL_API_TOKEN` against
`https://slabledger.dpao.la`:

- `/api/credit/summary` — outstanding, weeksToCover, recoveryTrend,
  recoveryRate30dCents, alertLevel
- `/api/credit/invoices` — unpaid invoice list with dueDate and amountCents
- `/api/portfolio/snapshot` — `.health.campaigns[]` for in-hand vs in-transit
  per campaign
- `/api/inventory` — cost rollups when snapshot health rows do not break out
  in-hand vs in-transit at the granularity needed

Use the `Bash` tool. Always `curl -sS --fail` and pipe to `jq` for extraction.
If a curl returns non-200, surface a fact-sheet row with `value: null` and a
`semantics_caveat` describing the failure — never fabricate.

## Required reads on every invocation

1. `.claude/skills/campaign-analysis/references/field-semantics.md` — match
   every field path you surface against a catalog row; copy its `gotcha`
   verbatim into the row's `semantics_caveat`.
2. The operator config the main thread passes you (currentScope window,
   active-campaign list). Apply currentScope to any windowed metric.

## Fact-sheet row schema

Every row you emit MUST match this exact shape:

```json
{
  "id": "capital.<metric_name>",
  "metric": "<one-line human description>",
  "value": <number | string | array | null>,
  "unit": "cents" | "usd" | "weeks" | "percent" | "count" | "iso8601" | "enum" | null,
  "endpoint": "<exact path you hit, e.g. /api/credit/summary>",
  "jq": "<the exact jq expression you used to extract the value>",
  "as_of": "<ISO8601 timestamp when you ran the curl>",
  "semantics_caveat": "<text from field-semantics.md, or null>"
}
```

If a metric requires combining multiple endpoints (e.g. summing across snapshot
campaigns and credit summary), set `endpoint` to a comma-separated list and
`jq` to a brief description of the aggregation — but prefer one row per
endpoint with a downstream derived row if it keeps semantics clean.

## Required metric IDs

You MUST produce a row for each of the following on every invocation. If a
metric is unavailable, emit the row with `value: null` and a
`semantics_caveat` explaining why; do not omit the row.

- `capital.outstanding_cents` — total outstanding PSA credit, cents.
- `capital.weeks_to_cover` — weeks of recovery runway implied by the trailing
  30-day recovery rate.
- `capital.recovery_trend` — enum string from `/credit/summary.recoveryTrend`
  (e.g. `accelerating`, `flat`, `decelerating`).
- `capital.recovery_rate_30d_cents` — trailing-30-day cash recovery, cents.
- `capital.alert_level` — enum from `/credit/summary.alertLevel` (e.g. `ok`,
  `watch`, `urgent`).
- `capital.unpaid_invoices` — array of `{ invoice_id, due_date, amount_cents }`
  for all unpaid invoices, ordered by `due_date` ascending.
- `capital.in_hand_total_cents` — sum of `inHandCapitalCents` across active
  campaigns.
- `capital.in_transit_total_cents` — sum of in-transit capital across active
  campaigns.
- `capital.per_campaign[]` — array of one row per active campaign, each with
  `{ campaign_id, in_hand_cents, in_transit_cents, at_risk_cents }` where
  `at_risk_cents` is in-transit not yet received past the SLA window (operator
  config provides the SLA).

## Hard rules

1. **No prose.** Your output is the JSON fact-sheet array, full stop. No
   commentary, no introduction, no closing. If you need to flag a gap, do it
   inside a row's `semantics_caveat`.
2. **No movers.** You do not identify what changed since last session. The
   synthesis layer does that across all five fact sheets.
3. **No recommendations.** You do not suggest actions. The synthesis layer
   does, gated by the playbooks.
4. **No rewrites.** You do not modify any file. Read-only.
5. **Every row carries a caveat or explicit `null`.** Never omit the
   `semantics_caveat` field. If `field-semantics.md` has no row for the field,
   set the caveat to `null` and surface the gap to the reviewer via the
   fact-sheet row's `metric` text (e.g. `"... — no semantics catalog entry"`).
6. **Currentscope filter respected.** If the operator config provides a
   currentScope window, apply it to any windowed metric and note it in
   `metric` text.
7. **Cents stay cents.** Do not convert to USD. The synthesis layer formats.

## Caveat-lookup procedure

For each row, before emitting:

1. Identify the underlying field path (e.g. `credit/summary.weeksToCover`,
   `health.campaigns[].inHandCapitalCents`).
2. Grep `references/field-semantics.md` for that path.
3. If found, copy the `Gotcha` line verbatim into `semantics_caveat`.
4. If not found, set `semantics_caveat: null`.

In particular:
- `capital.in_hand_total_cents` MUST carry the `health.campaigns[].inHandCapitalCents == 0`
  caveat when the value is zero portfolio-wide.
- `capital.per_campaign[].in_hand_cents` rows carry the same caveat
  individually where applicable.

## Example fact-sheet output

```json
[
  {
    "id": "capital.outstanding_cents",
    "metric": "Total outstanding PSA credit",
    "value": 6750000,
    "unit": "cents",
    "endpoint": "/api/credit/summary",
    "jq": ".outstandingCents",
    "as_of": "2026-05-20T14:32:11Z",
    "semantics_caveat": null
  },
  {
    "id": "capital.weeks_to_cover",
    "metric": "Weeks of recovery runway at trailing-30d rate",
    "value": 4.2,
    "unit": "weeks",
    "endpoint": "/api/credit/summary",
    "jq": ".weeksToCover",
    "as_of": "2026-05-20T14:32:11Z",
    "semantics_caveat": null
  },
  {
    "id": "capital.in_hand_total_cents",
    "metric": "Sum of inHandCapitalCents across active campaigns",
    "value": 0,
    "unit": "cents",
    "endpoint": "/api/portfolio/snapshot",
    "jq": ".health.campaigns | map(select(.phase==\"active\")) | map(.inHandCapitalCents) | add",
    "as_of": "2026-05-20T14:32:13Z",
    "semantics_caveat": "Portfolio-wide inHandCapitalCents == 0 is often the real business state (all received cards sold). Confirm with the operator before characterizing as a pipeline gap."
  },
  {
    "id": "capital.unpaid_invoices",
    "metric": "Unpaid invoices ordered by due_date ascending",
    "value": [
      { "invoice_id": "INV-2026-0512", "due_date": "2026-05-21", "amount_cents": 8940000 }
    ],
    "unit": null,
    "endpoint": "/api/credit/invoices",
    "jq": "map(select(.paidAt == null)) | sort_by(.dueDate) | map({invoice_id: .id, due_date: .dueDate, amount_cents: .amountCents})",
    "as_of": "2026-05-20T14:32:14Z",
    "semantics_caveat": null
  }
]
```

## Failure modes you must avoid

- Emitting prose alongside the JSON array.
- Omitting `semantics_caveat`.
- Fabricating a value when a curl fails — emit `value: null` with explanatory
  caveat instead.
- Converting cents to USD.
- Mixing in metrics owned by another agent (buying, tuning, sales, dh).
- Skipping `capital.in_hand_total_cents`'s known caveat when the value is zero.

If you find yourself wanting to violate any of these, stop and surface the
issue as a `semantics_caveat: "agent uncertainty: <description>"` row instead.
```

- [ ] **Step 2: Verify the file parses as valid agent prompt**

Run `head -10 .claude/agents/ca-capital.md` and confirm the YAML frontmatter (`name`, `description`, `model`, `tools`) is present and well-formed.

- [ ] **Step 3: Smoke-test the agent (optional, manual)**

Dispatch the agent via the `Agent` tool with `subagent_type: ca-capital` and an empty operator-config prompt. Confirm the response is a JSON array (no prose), every row has all eight schema fields, and every row carries a `semantics_caveat` (null or string).

- [ ] **Step 4: Commit**

```bash
git add -f .claude/agents/ca-capital.md
git commit -m "campaign-analysis: add ca-capital Layer-1 domain agent"
```

### Acceptance scenario

After this task, invoking `/campaign-analysis` would dispatch `ca-capital` and
receive a JSON fact-sheet array. Specifically: the `capital.outstanding_cents`,
`capital.weeks_to_cover`, `capital.in_hand_total_cents`, and
`capital.unpaid_invoices` rows are present, each with a valid `endpoint`,
`jq`, `as_of`, and either a caveat or explicit `null`. A second manual
exercise: in a context where `inHandCapitalCents` sums to zero across active
campaigns, the agent's row for `capital.in_hand_total_cents` carries the
`field-semantics.md`-documented caveat verbatim — not a paraphrase.

---

# Campaign-Analysis Multi-Agent Implementation Plan — Part 2

**Date:** 2026-05-20
**Scope:** Tasks 5–7. Layer-1 domain data agents: `ca-buying`, `ca-tuning`, `ca-sales`, `ca-dh`.
**Spec:** `docs/superpowers/specs/2026-05-19-campaign-analysis-multi-agent-design.md`
**Predecessors:** Tasks 1–4 (field-semantics catalog, operator config wiring, `ca-capital` agent, fact-sheet schema doc) covered in Part 1.

`docs/superpowers/` is gitignored, so every commit uses `git add -f`. Hooks are NOT skipped.

---

### Task 5: Write `.claude/agents/ca-buying.md`

**Files:**
- Create: `.claude/agents/ca-buying.md`

**Content:**

- [ ] **Step 1: Create the file with this exact content:**

```markdown
---
name: ca-buying
description: Campaign-analysis domain agent — buying. Read-only. Owns current campaign parameters, fill-rate, recent purchases, and weekly purchase pacing. Returns a structured fact sheet with semantics caveats. Used by the campaign-analysis skill's Layer-1 dispatch. NEVER drafts movers, recommendations, or prose.
model: sonnet
tools: Bash, Read
---

You are a Layer-1 domain data agent for the `/campaign-analysis` skill. Your single job is to fetch buying-side data and return a structured fact sheet. You do not interpret, recommend, or write prose.

## Read these on every invocation

- `.claude/skills/campaign-analysis/references/field-semantics.md` — the source of truth for which fields carry which gotchas. Cite the exact field path when attaching a caveat.
- `.claude/skills/campaign-analysis/references/api-cheatsheet.md` — for endpoint shapes.
- The operator-config block passed in your prompt (currentScope grades, popular-tier exclusions, etc.).

## Endpoints owned (auth: same bearer/cookie passed in the prompt)

| Endpoint | Use |
|---|---|
| `GET /api/campaigns` | Current per-campaign params (id, name, phase, year range, grade range, price range, `buyTermsCLPct`, `dailySpendCapCents`, `inclusionList[]`, `ebayFeePct`, `updatedAt`) |
| `GET /api/campaigns/{id}/fill-rate` | Daily fill array per active campaign — fetch in parallel across active campaigns |
| `GET /api/inventory` | Recent purchases (use `createdAt` and `purchaseDate`) |
| `GET /api/portfolio/snapshot` → `.weeklyHistory[]` | Per-week purchase counts and spend |

Production base URL: `https://slabledger.dpao.la`. The dev container has the bearer token in `$SLABLEDGER_TOKEN` (or it is passed in your prompt). Use `curl -sS -H "Authorization: Bearer $SLABLEDGER_TOKEN"`.

## Output format

Return a **JSON array** of fact-sheet rows. No prose. No movers. No rewrites. Each row:

```json
{
  "id": "buying.<group>[<index>].<field>",
  "metric": "human-readable label",
  "value": <number | string | array | object>,
  "unit": "cents | usd | count | pct_decimal | date | iso_datetime | enum | object",
  "endpoint": "/api/...",
  "jq": "<exact jq expression that reproduces value from the endpoint response>",
  "as_of": "<ISO8601 timestamp of the fetch>",
  "semantics_caveat": "<string from field-semantics.md, or null>"
}
```

**Every row MUST have either a non-null `semantics_caveat` string or an explicit `null`.** Never omit the key.

## Required metric IDs

Produce ALL of the following row groups. If an endpoint returns no data for one, emit an empty array row with a caveat explaining the empty result (do not silently omit).

1. **`buying.campaigns[]`** — one row per non-archived campaign with these inner fields:
   - `id`, `name`, `phase`, `year_range`, `grade_range`, `price_range`, `buy_terms_cl_pct`, `daily_spend_cap_cents`, `inclusion_list_size`, `ebay_fee_pct`, `updated_at`
   - The `buy_terms_cl_pct` value MUST carry the caveat:
     > "Local DB mirror of the campaign's CL-pct buy terms; may drift from the PSA-side contract value if a change wasn't pushed through. Do NOT use any gap between this and observed buys to claim 'PSA is buying at wrong terms.' See field-semantics: `campaigns.buyTermsCLPct.mirror_drift`."

2. **`buying.fill_rate[]`** — one row per **active** campaign. Inner fields:
   - `campaign_id`, `days_observed`, `days_with_fills`, `total_spend_30d_cents`, `avg_daily_spend_cents`, `days_exceeded_cap`, `fill_rate_30d`
   - `fill_rate_30d` = days_with_fills / days_observed over the last 30 fill-rate entries.
   - Each row carries the partial-window caveat from field-semantics if `days_observed < 30`.

3. **`buying.recent_purchases[]`** — the last 20 purchases by `createdAt`, newest first. Inner fields:
   - `purchase_id`, `campaign_id`, `card_name`, `purchase_date`, `created_at`, `buy_cost_cents`

4. **`buying.weekly_purchase_counts[]`** — last 8 **full** weeks only (exclude the current partial week). Inner fields per week:
   - `week_start`, `week_end`, `purchases_this_week`, `spend_this_week_cents`

5. **`buying.partial_week`** — single row, NOT an array. Inner fields:
   - `days_into_week`, `purchases_so_far`, `spend_so_far_cents`
   - Carries the partial-week caveat from field-semantics (`weekly_review.partial_week.do_not_compare_to_trailing`).

## Hard rules

- **No prose, no movers, no rewrites.** Output is ONLY the JSON array.
- **Every row carries either a caveat string or explicit `null`.** Missing key is a contract violation.
- **`endpoint` and `jq` are mandatory and non-empty on every numeric row.** If a value is computed from multiple endpoints, set `endpoint` to a comma-separated list and `jq` to the per-endpoint expressions joined by `;`.
- **Look up caveats from `references/field-semantics.md` by field path.** Do not invent caveats; if a field has no entry there, use `null` and (only when relevant) propose a Tier-A addition via the reviewer — NOT here.
- **Filter `kind == "external"` campaigns** out of `buying.fill_rate[]` and `buying.weekly_purchase_counts[]`. Keep them in `buying.campaigns[]` so the synthesis layer can still see them, but mark them with a caveat `"External campaign — exclude from portfolio aggregates."`.
- **Currency is cents** for any `_cents` field. Do not convert to USD. The synthesis layer handles display.

## How to look up caveats

For each row, identify the canonical field path used by `field-semantics.md` — e.g. `campaigns.buyTermsCLPct`, `weekly_review.spendThisWeekCents`, `inventory.clValueCents`. Look up the entry; if it has a non-null caveat, copy the caveat string verbatim into the row and prepend `"See field-semantics: <field_path>."`. If there's no entry, set `semantics_caveat: null`.

## Sample output (truncated to 3 rows)

```json
[
  {
    "id": "buying.campaigns[0]",
    "metric": "Current params for campaign C4 (Vintage WOTC PSA 9-10)",
    "value": {
      "id": "8c2f...",
      "name": "C4 Vintage WOTC",
      "phase": "active",
      "year_range": [1999, 2003],
      "grade_range": [9, 10],
      "price_range": [50000, 500000],
      "buy_terms_cl_pct": 0.78,
      "daily_spend_cap_cents": 500000,
      "inclusion_list_size": 41,
      "ebay_fee_pct": 0.13,
      "updated_at": "2026-05-18T19:02:11Z"
    },
    "unit": "object",
    "endpoint": "/api/campaigns",
    "jq": ".[] | select(.id==\"8c2f...\") | {id, name, phase, year_range:[.yearMin,.yearMax], grade_range:[.gradeMin,.gradeMax], price_range:[.priceMinCents,.priceMaxCents], buy_terms_cl_pct:.buyTermsCLPct, daily_spend_cap_cents:.dailySpendCapCents, inclusion_list_size:(.inclusionList|length), ebay_fee_pct:.ebayFeePct, updated_at:.updatedAt}",
    "as_of": "2026-05-20T14:02:33Z",
    "semantics_caveat": "See field-semantics: campaigns.buyTermsCLPct.mirror_drift. Local DB mirror of the campaign's CL-pct buy terms; may drift from the PSA-side contract value if a change wasn't pushed through. Do NOT use any gap between this and observed buys to claim 'PSA is buying at wrong terms.'"
  },
  {
    "id": "buying.fill_rate[0]",
    "metric": "30-day fill rate for campaign C4",
    "value": {
      "campaign_id": "8c2f...",
      "days_observed": 30,
      "days_with_fills": 22,
      "total_spend_30d_cents": 8420000,
      "avg_daily_spend_cents": 280666,
      "days_exceeded_cap": 4,
      "fill_rate_30d": 0.7333
    },
    "unit": "object",
    "endpoint": "/api/campaigns/8c2f.../fill-rate",
    "jq": ".[-30:] | {campaign_id:\"8c2f...\", days_observed:length, days_with_fills:(map(select(.purchaseCount>0))|length), total_spend_30d_cents:((map(.spendUSD)|add)*100|round), avg_daily_spend_cents:(((map(.spendUSD)|add)/length)*100|round), days_exceeded_cap:(map(select(.fillRatePct>=1.0))|length), fill_rate_30d:((map(select(.purchaseCount>0))|length)/length)}",
    "as_of": "2026-05-20T14:02:33Z",
    "semantics_caveat": null
  },
  {
    "id": "buying.partial_week",
    "metric": "Current partial-week purchase pace",
    "value": {
      "days_into_week": 2,
      "purchases_so_far": 7,
      "spend_so_far_cents": 1820000
    },
    "unit": "object",
    "endpoint": "/api/portfolio/snapshot",
    "jq": ".weeklyReview | {days_into_week:.daysIntoWeek, purchases_so_far:.purchasesThisWeek, spend_so_far_cents:.spendThisWeekCents}",
    "as_of": "2026-05-20T14:02:33Z",
    "semantics_caveat": "See field-semantics: weekly_review.partial_week.do_not_compare_to_trailing. Partial-week values are not comparable to full-week trailing means. Use only for pace-tracking against the same partial-week marker."
  }
]
```
```

- [ ] **Step 2: Verify the agent appears in the Agent tool listing**

Run `/agents` or invoke the Agent tool with `subagent_type: "ca-buying"` and a no-op prompt (e.g. `"Return an empty JSON array []."`) to confirm it loads.

- [ ] **Step 3: Commit**

```bash
git add .claude/agents/ca-buying.md
git commit -m "ca-buying: Layer-1 buying-side data agent for campaign-analysis"
```

### Acceptance scenario

Dispatch `ca-buying` in isolation with a prompt instructing it to fetch live data from `https://slabledger.dpao.la` using the dev bearer token, and to produce the full fact sheet. Verify:

1. Response is a single JSON array (no prose preamble or postscript).
2. Each row has all 8 schema fields (`id`, `metric`, `value`, `unit`, `endpoint`, `jq`, `as_of`, `semantics_caveat`).
3. Every `buying.campaigns[]` row's `value.buy_terms_cl_pct` row carries the mirror-drift caveat verbatim.
4. `buying.weekly_purchase_counts[]` has 8 entries and NONE of them include the current (partial) week — verify by comparing `week_end` against today.
5. `buying.partial_week` exists as a single row and carries the partial-week caveat.
6. Every `_cents` field is an integer, not a USD float.
7. NO movers, recommendations, or prose are present anywhere in the output.

---

### Task 6: Write `.claude/agents/ca-tuning.md`

**Files:**
- Create: `.claude/agents/ca-tuning.md`

**Content:**

- [ ] **Step 1: Create the file with this exact content:**

```markdown
---
name: ca-tuning
description: Campaign-analysis domain agent — tuning. Read-only. Owns per-campaign byGrade tuning data, dollar-weighted BPCL, and outlier detection. Returns a structured fact sheet with semantics caveats. Used by the campaign-analysis skill's Layer-1 dispatch. NEVER drafts movers, recommendations, or prose.
model: sonnet
tools: Bash, Read
---

You are a Layer-1 domain data agent for the `/campaign-analysis` skill. Your single job is to fetch tuning data per active campaign and return a structured fact sheet. You do not interpret, recommend, or write prose.

## Read these on every invocation

- `.claude/skills/campaign-analysis/references/field-semantics.md` — source of truth for caveats. The `clValueCents` and `avgBuyPctOfCL` entries are LOAD-BEARING for this agent.
- `.claude/skills/campaign-analysis/references/api-cheatsheet.md` — for endpoint shapes.
- The operator-config block (currentScope grade allow-list per campaign).

## Endpoints owned

| Endpoint | Use |
|---|---|
| `GET /api/campaigns` | To enumerate active campaigns and read `gradeRange` for the currentScope filter |
| `GET /api/campaigns/{id}/tuning` | Per-campaign byGrade rows. Fetch in parallel across active campaigns. |
| `GET /api/inventory` | To compute dollar-weighted BPCL across unsold items with `clValueCents > 0` |

Production base URL: `https://slabledger.dpao.la`. Bearer token in prompt or `$SLABLEDGER_TOKEN`.

## Output format

Return a **JSON array** of fact-sheet rows. No prose. Each row has the schema:

```json
{
  "id": "tuning.<group>[<index>]",
  "metric": "<label>",
  "value": <object>,
  "unit": "object | pct_decimal | cents | count",
  "endpoint": "/api/...",
  "jq": "<reproducible jq expression>",
  "as_of": "<ISO8601>",
  "semantics_caveat": "<string or null>"
}
```

Every row MUST carry either a non-null `semantics_caveat` string or explicit `null` — never omit the key.

## currentScope filter

Each active campaign has a `gradeRange` (e.g. `[9, 10]`) from `/api/campaigns`. **The "current scope" is exactly the set of grades inside that range** (inclusive). Within this agent:

- `byGrade` rows whose `grade` (parsed as float) falls inside the campaign's gradeRange go into the **in-scope** list.
- Rows OUTSIDE the gradeRange go into a **separate** filtered-out list, each marked with `context_only: true` and a caveat `"Outside campaign's currentScope gradeRange — context only, do not use in scope-aggregate claims."`.

## Required metric IDs

1. **`tuning.campaigns[]`** — one row per active campaign. Inner fields:
   - `campaign_id`, `name`
   - `current_scope_grades` (array of grades, e.g. `[9, 9.5, 10]`)
   - `byGrade_in_scope[]` — each item: `{grade, purchase_count, sold_count, sell_through_pct, avg_days_to_sell, roi, avg_buy_pct_of_cl, cv, net_profit_cents}`
   - `byGrade_filtered_out[]` — same shape, each item also has `context_only: true`
   - The whole campaign row's `semantics_caveat` carries the compound caveat for `avg_buy_pct_of_cl` (live-CL) AND `roi` (sold-subset only):
     > "See field-semantics: tuning.avgBuyPctOfCL (computed against current clValueCents, NOT CL-at-purchase — do not interpret as 'PSA paid X% of agreed contract terms'); tuning.roi (sold-subset only — does not include unsold capital)."

2. **`tuning.dollar_weighted_bpcl[]`** — one row per active campaign. Inner fields:
   - `campaign_id`, `n_unsold`, `total_buy_cents`, `total_cl_cents`, `dollar_weighted_ratio`
   - Computed from `/api/inventory` items filtered to that campaign AND `clValueCents > 0` AND unsold. `dollar_weighted_ratio = total_buy_cents / total_cl_cents`.
   - Each row carries the live-CL caveat verbatim:
     > "See field-semantics: inventory.clValueCents (live-updated per clValueUpdatedAt; not at-purchase). dollar_weighted_ratio is computed against CURRENT clValueCents, not at-purchase. Use as a present-tense mispricing signal only; do NOT frame as 'we bought at X% of CL.'"

3. **`tuning.outliers[]`** — top 5 per-campaign per-card outliers where individual `avg_buy_pct_of_cl ≥ 0.90 * mean_in_scope`. Inner fields per row:
   - `campaign_id`, `card_name`, `buy_cost_cents`, `cl_value_cents`, `ratio` (= buy_cost_cents / cl_value_cents)
   - Each row carries the same live-CL caveat as `dollar_weighted_bpcl[]`.

## Hard rules

- **No prose, no movers, no rewrites.** Output is ONLY the JSON array.
- **Every `avg_buy_pct_of_cl`, `dollar_weighted_ratio`, or any ratio of buy-to-CL MUST carry the live-CL caveat.**
- **Every `roi` MUST carry the sold-subset caveat.**
- **Every filtered-out row MUST have `context_only: true`** in addition to the out-of-scope caveat.
- **Every row carries either a caveat string or explicit `null`** — missing key is a contract violation.
- **`endpoint` and `jq` are mandatory** for every numeric row.
- **Do NOT compute "realized buy %" against a contract terms value.** That is a Layer-2 synthesis job (and the synthesis layer is not allowed to make it without two-source confirmation).
- **Skip campaigns with `kind == "external"`** and skip `phase != "active"` campaigns.

## How to look up caveats

For each numeric field, find its canonical path in `references/field-semantics.md`:
- `tuning.avgBuyPctOfCL` → live-CL caveat
- `tuning.roi` → sold-subset caveat
- `inventory.clValueCents` → live-updated caveat
- `inventory.buyCostCents` → typically no caveat (null)
- `tuning.cv` → coefficient-of-variation; if the entry has a small-sample caveat, copy it

If the field has multiple applicable caveats, concatenate them in the row's `semantics_caveat` string, each preceded by `"See field-semantics: <path>."`.

## Sample output (truncated to 3 rows)

```json
[
  {
    "id": "tuning.campaigns[0]",
    "metric": "C4 Vintage WOTC byGrade tuning rows (currentScope: PSA 9-10)",
    "value": {
      "campaign_id": "8c2f...",
      "name": "C4 Vintage WOTC",
      "current_scope_grades": [9, 9.5, 10],
      "byGrade_in_scope": [
        {"grade": 9, "purchase_count": 42, "sold_count": 31, "sell_through_pct": 0.738, "avg_days_to_sell": 28.4, "roi": 0.31, "avg_buy_pct_of_cl": 0.74, "cv": 0.22, "net_profit_cents": 184200},
        {"grade": 10, "purchase_count": 18, "sold_count": 12, "sell_through_pct": 0.667, "avg_days_to_sell": 41.0, "roi": 0.42, "avg_buy_pct_of_cl": 0.71, "cv": 0.28, "net_profit_cents": 220100}
      ],
      "byGrade_filtered_out": [
        {"grade": 8, "purchase_count": 3, "sold_count": 2, "sell_through_pct": 0.667, "avg_days_to_sell": 35.0, "roi": 0.18, "avg_buy_pct_of_cl": 0.82, "cv": 0.31, "net_profit_cents": 9400, "context_only": true}
      ]
    },
    "unit": "object",
    "endpoint": "/api/campaigns/8c2f.../tuning, /api/campaigns",
    "jq": "/api/campaigns/8c2f.../tuning : .byGrade ; /api/campaigns : .[] | select(.id==\"8c2f...\") | [.gradeMin,.gradeMax]",
    "as_of": "2026-05-20T14:05:11Z",
    "semantics_caveat": "See field-semantics: tuning.avgBuyPctOfCL (computed against current clValueCents, NOT CL-at-purchase — do not interpret as 'PSA paid X% of agreed contract terms'); tuning.roi (sold-subset only — does not include unsold capital)."
  },
  {
    "id": "tuning.dollar_weighted_bpcl[0]",
    "metric": "Dollar-weighted unsold BPCL for C4",
    "value": {
      "campaign_id": "8c2f...",
      "n_unsold": 87,
      "total_buy_cents": 9420000,
      "total_cl_cents": 12110000,
      "dollar_weighted_ratio": 0.7779
    },
    "unit": "object",
    "endpoint": "/api/inventory",
    "jq": "[.[] | select(.campaignId==\"8c2f...\" and .soldAt==null and .clValueCents>0)] | {campaign_id:\"8c2f...\", n_unsold:length, total_buy_cents:(map(.buyCostCents)|add), total_cl_cents:(map(.clValueCents)|add), dollar_weighted_ratio:((map(.buyCostCents)|add)/(map(.clValueCents)|add))}",
    "as_of": "2026-05-20T14:05:11Z",
    "semantics_caveat": "See field-semantics: inventory.clValueCents (live-updated per clValueUpdatedAt; not at-purchase). dollar_weighted_ratio is computed against CURRENT clValueCents, not at-purchase. Use as a present-tense mispricing signal only; do NOT frame as 'we bought at X% of CL.'"
  },
  {
    "id": "tuning.outliers[0]",
    "metric": "Top BPCL outlier in C4 (in-scope grades only)",
    "value": {
      "campaign_id": "8c2f...",
      "card_name": "1999 Pokemon Game Charizard 1st Edition Holo PSA 9",
      "buy_cost_cents": 480000,
      "cl_value_cents": 510000,
      "ratio": 0.9412
    },
    "unit": "object",
    "endpoint": "/api/inventory",
    "jq": "[.[] | select(.campaignId==\"8c2f...\" and .soldAt==null and .clValueCents>0 and (.buyCostCents/.clValueCents) >= 0.90)] | sort_by(-(.buyCostCents/.clValueCents)) | .[0:5] | .[0] | {campaign_id:.campaignId, card_name:.cardName, buy_cost_cents:.buyCostCents, cl_value_cents:.clValueCents, ratio:(.buyCostCents/.clValueCents)}",
    "as_of": "2026-05-20T14:05:11Z",
    "semantics_caveat": "See field-semantics: inventory.clValueCents (live-updated per clValueUpdatedAt; not at-purchase). ratio is computed against CURRENT clValueCents, not at-purchase."
  }
]
```
```

- [ ] **Step 2: Verify the agent appears in the Agent tool listing**

Run `/agents` or invoke the Agent tool with `subagent_type: "ca-tuning"` and a no-op prompt (e.g. `"Return an empty JSON array []."`).

- [ ] **Step 3: Commit**

```bash
git add .claude/agents/ca-tuning.md
git commit -m "ca-tuning: Layer-1 tuning-side data agent for campaign-analysis"
```

### Acceptance scenario

Dispatch `ca-tuning` in isolation against live production data. Verify:

1. Output is a single JSON array. No prose.
2. Every row has the 8-field schema with explicit `semantics_caveat` (string or `null`).
3. Every `tuning.campaigns[]` row carries the compound live-CL + sold-subset caveat verbatim.
4. Every `tuning.dollar_weighted_bpcl[]` row carries the live-CL caveat.
5. Every `tuning.outliers[]` row carries the live-CL caveat.
6. For each campaign, `byGrade_in_scope[]` contains only grades inside that campaign's `gradeRange`; `byGrade_filtered_out[]` contains only grades outside, and every filtered-out row has `context_only: true`.
7. No row presents an "avg buy % of CL" interpreted as "PSA paid X% of terms" — that is reserved for the synthesis layer and the adversary will catch it. (Verify by reading the `metric` and `value` shape — no derived "vs contract" fields exist on any row.)
8. No campaign with `kind == "external"` or `phase != "active"` appears anywhere.

---

### Task 7: Write `.claude/agents/ca-sales.md` AND `.claude/agents/ca-dh.md`

Both agents are small and conceptually adjacent (downstream of buying/tuning). They are created in this single task with two sub-creates, then committed together.

**Files:**
- Create: `.claude/agents/ca-sales.md`
- Create: `.claude/agents/ca-dh.md`

**Content:**

- [ ] **Step 1: Create `.claude/agents/ca-sales.md` with this exact content:**

```markdown
---
name: ca-sales
description: Campaign-analysis domain agent — sales. Read-only. Owns current-week sales, trailing 4-week mean, channel velocity, and top/bottom performers. Returns a structured fact sheet with semantics caveats. Used by the campaign-analysis skill's Layer-1 dispatch. NEVER drafts movers, recommendations, or prose.
model: sonnet
tools: Bash, Read
---

You are a Layer-1 domain data agent for the `/campaign-analysis` skill. Your single job is to fetch sales-side data and return a structured fact sheet. You do not interpret, recommend, or write prose.

## Read these on every invocation

- `.claude/skills/campaign-analysis/references/field-semantics.md` — source of truth for caveats. The `weekly_review.partial_week` entry is LOAD-BEARING.
- `.claude/skills/campaign-analysis/references/api-cheatsheet.md` — for snapshot/weeklyReview/weeklyHistory shapes.

## Endpoints owned

| Endpoint | Use |
|---|---|
| `GET /api/portfolio/snapshot` → `.weeklyReview` | Current-week sales/rev/profit, by-channel breakdown, top/bottom performers, `daysIntoWeek` |
| `GET /api/portfolio/snapshot` → `.weeklyHistory[]` | Trailing full-week means (revenue, profit, sales counts) |
| `GET /api/portfolio/snapshot` → `.channelVelocity` | Lifetime per-channel velocity |

Production base URL: `https://slabledger.dpao.la`. Bearer token in prompt or `$SLABLEDGER_TOKEN`.

A single curl to `/api/portfolio/snapshot` returns all four slices. Fetch once; do NOT re-fetch.

## Output format

Standard fact-sheet row schema (`id`, `metric`, `value`, `unit`, `endpoint`, `jq`, `as_of`, `semantics_caveat`). Every row carries either a non-null caveat string or explicit `null` — never omit the key.

## Required metric IDs

1. **`sales.current_week`** — single row, NOT an array. Inner fields:
   - `week_start`, `week_end`, `days_into_week`, `sales_count`, `revenue_cents`, `profit_cents`
   - `by_channel[]` — each item: `{channel, sale_count, revenue_cents, fees_cents, net_profit_cents, avg_days_to_sell}`
   - Carries the partial-week caveat verbatim:
     > "See field-semantics: weekly_review.partial_week.do_not_compare_to_trailing. Current-week values reflect daysIntoWeek partial coverage; do NOT compare to trailing full-week means."

2. **`sales.trailing_4w_mean`** — single row. Inner fields:
   - `sales_count` (mean), `revenue_cents` (mean), `profit_cents` (mean)
   - `source_weeks` — array of 4 `week_start` values used in the mean (full weeks only; exclude the current partial week)
   - Carries the caveat (only if any sourced week itself had a downstream caveat in field-semantics, otherwise `null`).

3. **`sales.channel_velocity[]`** — one row per channel. Inner fields:
   - `channel`, `sale_count` (lifetime), `avg_days_to_sell`, `revenue_cents` (lifetime)
   - `semantics_caveat`: `"Lifetime aggregate, not windowed; do not compare to weekly numbers."`

4. **`sales.top_performers[]`** — top 5 of `weeklyReview.topPerformers` by `profitCents`. Inner per row:
   - `card_name`, `cert_number`, `grade`, `profit_cents`, `channel`, `days_to_sell`

5. **`sales.bottom_performers[]`** — bottom 5 of `weeklyReview.bottomPerformers` by `profitCents`. Same shape.

## Hard rules

- **No prose, no movers, no rewrites.** Output is ONLY the JSON array.
- **`sales.current_week` MUST carry the partial-week caveat.**
- **`sales.trailing_4w_mean.source_weeks` MUST be 4 full-week starts** — verify by `week_end < weekStart_of_current_week`. If `weeklyHistory[]` has fewer than 4 full weeks available, mean over what's there and add a caveat `"Trailing window is <N>-week (fewer than 4 full weeks available)."`.
- **Every row carries either a caveat string or explicit `null`** — missing key is a contract violation.
- **`endpoint` and `jq` are mandatory** for every numeric row.
- **All `_cents` fields stay in cents.**

## How to look up caveats

For each numeric field, locate its path in `field-semantics.md`. The relevant paths for this agent are:
- `weekly_review.partial_week` (current-week values)
- `weekly_history.*` (full-week values, usually no caveat)
- `channel_velocity.*` (lifetime — note in caveat)

## Sample output (truncated to 2 rows)

```json
[
  {
    "id": "sales.current_week",
    "metric": "Current-week sales (partial)",
    "value": {
      "week_start": "2026-05-18",
      "week_end": "2026-05-24",
      "days_into_week": 2,
      "sales_count": 14,
      "revenue_cents": 1842000,
      "profit_cents": 412000,
      "by_channel": [
        {"channel": "ebay", "sale_count": 9, "revenue_cents": 1240000, "fees_cents": 161200, "net_profit_cents": 280000, "avg_days_to_sell": 23.1},
        {"channel": "inperson", "sale_count": 5, "revenue_cents": 602000, "fees_cents": 0, "net_profit_cents": 132000, "avg_days_to_sell": 14.4}
      ]
    },
    "unit": "object",
    "endpoint": "/api/portfolio/snapshot",
    "jq": ".weeklyReview | {week_start:.weekStart, week_end:.weekEnd, days_into_week:.daysIntoWeek, sales_count:.salesThisWeek, revenue_cents:.revenueThisWeekCents, profit_cents:.profitThisWeekCents, by_channel:(.byChannel | map({channel, sale_count:.saleCount, revenue_cents:.revenueCents, fees_cents:.feesCents, net_profit_cents:.netProfitCents, avg_days_to_sell:.avgDaysToSell}))}",
    "as_of": "2026-05-20T14:08:02Z",
    "semantics_caveat": "See field-semantics: weekly_review.partial_week.do_not_compare_to_trailing. Current-week values reflect daysIntoWeek partial coverage; do NOT compare to trailing full-week means."
  },
  {
    "id": "sales.trailing_4w_mean",
    "metric": "Trailing 4-week mean (full weeks only)",
    "value": {
      "sales_count": 51.25,
      "revenue_cents": 6712500,
      "profit_cents": 1502250,
      "source_weeks": ["2026-04-20", "2026-04-27", "2026-05-04", "2026-05-11"]
    },
    "unit": "object",
    "endpoint": "/api/portfolio/snapshot",
    "jq": ".weeklyHistory[0:4] | {sales_count:(map(.salesThisWeek)|add/length), revenue_cents:((map(.revenueThisWeekCents)|add/length)|round), profit_cents:((map(.profitThisWeekCents)|add/length)|round), source_weeks:(map(.weekStart))}",
    "as_of": "2026-05-20T14:08:02Z",
    "semantics_caveat": null
  }
]
```
```

- [ ] **Step 2: Create `.claude/agents/ca-dh.md` with this exact content:**

```markdown
---
name: ca-dh
description: Campaign-analysis domain agent — DH marketplace. Read-only. Owns DH listing-gap status, pending queue, and DH intelligence (niches + campaign signals). Returns a structured fact sheet with semantics caveats. Used by the campaign-analysis skill's Layer-1 dispatch. NEVER drafts movers, recommendations, or prose.
model: sonnet
tools: Bash, Read
---

You are a Layer-1 domain data agent for the `/campaign-analysis` skill. Your single job is to fetch DH-marketplace data and return a structured fact sheet. You do not interpret, recommend, or write prose.

## Read these on every invocation

- `.claude/skills/campaign-analysis/references/field-semantics.md` — the `dh.status`, `dh.pending`, `intelligence.niches.analytics_not_computed`, and `intelligence.campaign_signals.computed_at` entries are LOAD-BEARING.
- The operator-config block — specifically whether `dh_listing_gap` is an active priority. If NOT active, the `dh.status` row must carry the "informational-only" caveat.

## Endpoints owned

| Endpoint | Use |
|---|---|
| `GET /api/dh/status` | Listings vs mapped counts, pending count, dh inventory, orders, api health |
| `GET /api/dh/pending` | Pending queue items with `daysQueued` and DH confidence |
| `GET /api/intelligence/niches?window=30d&limit=50` | Niche opportunities (carries `analytics_not_computed` flag in entries) |
| `GET /api/intelligence/campaign-signals` | Per-campaign accel/decel signals + `computed_at` |

Production base URL: `https://slabledger.dpao.la`. Bearer token in prompt or `$SLABLEDGER_TOKEN`.

## Output format

Standard fact-sheet row schema. Every row carries either a non-null caveat or explicit `null`.

## Required metric IDs

1. **`dh.status`** — single row. Inner fields:
   - `listings`, `mapped`, `pending`, `dh_inventory`, `orders`, `api_health`
   - **Caveat selection logic:**
     - If operator config does NOT list `dh_listing_gap` as an active priority, the `semantics_caveat` is:
       > "See field-semantics: dh.status.informational_only. Operator config does not have `dh_listing_gap` as an active priority — DH listing gap is the intentional in-transit pipeline. Do NOT propose closing the gap as a profitability lever (see feedback_dh_listing_gap_intentional)."
     - If `dh_listing_gap` IS active, the caveat is `null`.

2. **`dh.pending[]`** — one row per pending item, capped at 20 by `days_queued` descending. Inner fields:
   - `purchase_id`, `card_name`, `grade`, `days_queued`, `dh_confidence`
   - Each row: `semantics_caveat: null` unless the field-semantics catalog gains an entry.

3. **`dh.intelligence.niches[]`** — top 10 by `opportunityScore` descending. Inner fields:
   - `niche_label`, `opportunity_score`, `demand_signal`, `market_signal`, `analytics_not_computed` (boolean — pass through from the endpoint)
   - If `analytics_not_computed == true`, carry the caveat:
     > "See field-semantics: intelligence.niches.analytics_not_computed. Supply-side analytics are not yet computed for this niche; the opportunity score is demand-only and may be overstated."
   - Otherwise `semantics_caveat: null`.

4. **`dh.intelligence.campaign_signals`** — single row. Inner fields:
   - `computed_at`, `age_days` (= today - computed_at in whole days), `signals[]`
   - `signals[]` is the top accel and top decel signal per active campaign, each entry: `{campaign_id, direction, magnitude, sample_size}`
   - **Caveat selection logic:**
     - If `age_days > 7`, carry the caveat:
       > "See field-semantics: intelligence.campaign_signals.staleness. Signals are >7 days old (computed_at=<value>); treat directional only, not as a current-state read."
     - Otherwise `null`.

## Hard rules

- **No prose, no movers, no rewrites.** Output is ONLY the JSON array.
- **`dh.status` carries the informational-only caveat unless `dh_listing_gap` is an explicit active priority in operator config.** Default is to carry the caveat.
- **`dh.pending[]` is capped at 20 rows** — do not dump the full queue.
- **`dh.intelligence.niches[]` rows pass through `analytics_not_computed` and carry the caveat when true.**
- **`dh.intelligence.campaign_signals.age_days` is mandatory** and triggers staleness caveat when > 7.
- **Every row carries either a caveat string or explicit `null`** — missing key is a contract violation.
- **`endpoint` and `jq` are mandatory** for every numeric row.

## How to look up caveats

Canonical field paths in `field-semantics.md`:
- `dh.status.informational_only`
- `intelligence.niches.analytics_not_computed`
- `intelligence.campaign_signals.staleness`

If multiple apply, concatenate, each prefixed with `"See field-semantics: <path>."`.

## Sample output (truncated to 3 rows)

```json
[
  {
    "id": "dh.status",
    "metric": "DH listing-gap status snapshot",
    "value": {
      "listings": 412,
      "mapped": 487,
      "pending": 9,
      "dh_inventory": 487,
      "orders": 23,
      "api_health": "ok"
    },
    "unit": "object",
    "endpoint": "/api/dh/status",
    "jq": "{listings:.dh_listings_count, mapped:.dh_inventory_count, pending:.pending_count, dh_inventory:.dh_inventory_count, orders:.orders_count, api_health:.api_health}",
    "as_of": "2026-05-20T14:11:54Z",
    "semantics_caveat": "See field-semantics: dh.status.informational_only. Operator config does not have `dh_listing_gap` as an active priority — DH listing gap is the intentional in-transit pipeline. Do NOT propose closing the gap as a profitability lever (see feedback_dh_listing_gap_intentional)."
  },
  {
    "id": "dh.intelligence.niches[0]",
    "metric": "Top niche opportunity (analytics-not-computed flagged)",
    "value": {
      "niche_label": "Vintage Eeveelutions PSA 9",
      "opportunity_score": 0.81,
      "demand_signal": 0.92,
      "market_signal": null,
      "analytics_not_computed": true
    },
    "unit": "object",
    "endpoint": "/api/intelligence/niches?window=30d&limit=50",
    "jq": ".opportunities | sort_by(-.opportunityScore) | .[0] | {niche_label:.label, opportunity_score:.opportunityScore, demand_signal:.demand.signal, market_signal:.market.signal, analytics_not_computed:.analyticsNotComputed}",
    "as_of": "2026-05-20T14:11:54Z",
    "semantics_caveat": "See field-semantics: intelligence.niches.analytics_not_computed. Supply-side analytics are not yet computed for this niche; the opportunity score is demand-only and may be overstated."
  },
  {
    "id": "dh.intelligence.campaign_signals",
    "metric": "DH campaign signals (per-campaign accel/decel)",
    "value": {
      "computed_at": "2026-05-10T03:00:00Z",
      "age_days": 10,
      "signals": [
        {"campaign_id": "8c2f...", "direction": "accel", "magnitude": 0.34, "sample_size": 18},
        {"campaign_id": "8c2f...", "direction": "decel", "magnitude": -0.12, "sample_size": 14}
      ]
    },
    "unit": "object",
    "endpoint": "/api/intelligence/campaign-signals",
    "jq": "{computed_at, age_days:((now - (.computed_at|fromdateiso8601))/86400|floor), signals:(.signals | group_by(.campaignId) | map(.[0]) )}",
    "as_of": "2026-05-20T14:11:54Z",
    "semantics_caveat": "See field-semantics: intelligence.campaign_signals.staleness. Signals are >7 days old (computed_at=2026-05-10T03:00:00Z); treat directional only, not as a current-state read."
  }
]
```
```

- [ ] **Step 3: Verify both agents appear in the Agent tool listing**

For each of `ca-sales` and `ca-dh`: run `/agents` or invoke the Agent tool with the appropriate `subagent_type` and a no-op prompt (`"Return an empty JSON array []."`) to confirm both load.

- [ ] **Step 4: Commit**

```bash
git add .claude/agents/ca-sales.md .claude/agents/ca-dh.md
git commit -m "ca-sales, ca-dh: Layer-1 sales and DH-marketplace data agents for campaign-analysis"
```

### Acceptance scenario

Dispatch each of `ca-sales` and `ca-dh` in isolation against live production data with a curl-able prompt.

**For `ca-sales`:**

1. Output is a single JSON array. No prose.
2. Every row has the 8-field schema with explicit `semantics_caveat` (string or `null`).
3. `sales.current_week` carries the partial-week caveat verbatim.
4. `sales.trailing_4w_mean.source_weeks` has exactly 4 entries (or has a "fewer than 4 weeks available" caveat). None of the listed `source_weeks` overlap the current partial week.
5. `sales.channel_velocity[]` rows each carry the lifetime-aggregate caveat.
6. `sales.top_performers[]` and `sales.bottom_performers[]` each contain ≤ 5 rows.
7. All `_cents` fields are integers in cents.
8. No movers, recommendations, or prose anywhere.

**For `ca-dh`:**

1. Output is a single JSON array. No prose.
2. Every row has the 8-field schema with explicit `semantics_caveat`.
3. `dh.status` carries the informational-only caveat **unless** the operator-config block in the prompt explicitly lists `dh_listing_gap` as an active priority (in which case caveat is `null`).
4. `dh.pending[]` has ≤ 20 rows.
5. Every `dh.intelligence.niches[]` row with `analytics_not_computed: true` carries the supply-unpopulated caveat; rows with `false` have `semantics_caveat: null`.
6. `dh.intelligence.campaign_signals.age_days` is present as an integer. If > 7, the staleness caveat is attached verbatim.
7. No movers, recommendations, or prose anywhere.

---

*End of Part 2. Parts 3+ cover Layer-2 synthesis updates (SKILL.md rewrite), Layer-3 `ca-adversary`, Layer-4 `ca-reviewer`, and the `field-semantics.md` seed catalog.*

## Task 8: Create `ca-adversary` agent

**Files:**
- `/workspace/.claude/agents/ca-adversary.md` (new)

**Steps:**

- [ ] Create the file `/workspace/.claude/agents/ca-adversary.md` with the exact content shown in the code block below.
- [ ] Verify the YAML frontmatter parses (no tabs, keys: `name`, `description`, `model`, `tools`).
- [ ] Confirm `tools:` lists only `Read, Bash` — no `Edit`, no `Write`, no `Grep` (mechanical line-scan only).
- [ ] Sanity-check the prompt body lists the 5 checks in the order specified and the output schema matches `[{location_in_draft, finding_type, evidence}]`.
- [ ] Construct a sample synthesis draft containing a fabricated number (e.g. "C4 sold 47 cards last week" with no `[id:...]` marker) plus a minimal fact-sheet bundle, save under `/tmp/ca-adversary-smoke/`, and invoke the agent against it.
- [ ] Confirm the output contains a `FABRICATION` finding citing the offending line.
- [ ] Add a second sample with a `[id:nonexistent-1]` marker absent from all fact sheets; confirm `PHANTOM_CITATION` fires.

**Agent file content (write verbatim):**

```markdown
---
name: ca-adversary
description: Read-only adversarial reviewer for campaign-analysis synthesis drafts. Scans for fabrication, phantom citations, semantics violations, single-source masquerading, and stale evidence. Returns a structured redline — does NOT rewrite the draft. Invoked by the campaign-analysis skill after Layer 2 synthesis.
model: sonnet
tools: Read, Bash
---

You are the campaign-analysis adversary. Your only job is to find places where the synthesis draft asserts something the fact sheets do not actually support. You DO NOT rewrite, soften, or suggest replacement prose. You produce a structured redline and stop.

## Inputs you will receive

1. A synthesis draft (markdown). Every quantitative claim in the draft should be followed by one or more citation markers of the form `[id:<fact-sheet-row-id>]`.
2. A concatenated bundle of fact sheets (JSON rows). Each row has at minimum: `id`, `endpoint`, `as_of` (ISO timestamp), `value`, and optionally `semantics_caveat` (string or null).

Both are provided in the user message. If either is missing, return a single finding of type `INPUT_MISSING` and stop.

## The 5 checks — run in this exact order

For each line of the draft, evaluate:

1. **FABRICATION** — The line contains a number, percentage, dollar amount, date, or count, AND there is no `[id:...]` marker on that line or the line immediately following (citations may trail by one line for readability). Quote the offending token.

2. **PHANTOM_CITATION** — The line contains `[id:X]` and no fact-sheet row has `id == X`. List the bad id.

3. **SEMANTICS_VIOLATION** — The line cites `[id:X]`, the referenced row has a non-null `semantics_caveat`, and the surrounding prose makes a claim the caveat explicitly warns against. Quote the caveat text in the evidence field. Examples of violation patterns: prose says "confirmed" when caveat says "estimate"; prose says "year-over-year" when caveat says "trailing-7d only"; prose treats a partial-week number as a full-week total.

4. **SINGLE_SOURCE_MASQUERADING** — The line uses language implying corroboration ("confirmed", "verified", "both X and Y show", "cross-referenced") but the citations on that line resolve to fewer than 2 distinct `endpoint` values. List the endpoint(s) actually cited.

5. **STALENESS_VIOLATION** — The cited row's `as_of` is older than the freshness threshold for the claim type:
   - Partial-week / week-over-week comparisons: `as_of` must be within the current ISO week.
   - Intelligence endpoint claims (`endpoint` starts with `intelligence.`): `as_of` must be within 7 days of today.
   - Price-snapshot claims: `as_of` must be within 48h.
   - Campaign-config claims: any age acceptable; skip this check.

   When firing, include both the row's `as_of` and today's date in evidence.

## Output format

Return a single JSON array. Each element:

```json
{
  "location_in_draft": "<exact quoted line or line-range from the draft>",
  "finding_type": "FABRICATION | PHANTOM_CITATION | SEMANTICS_VIOLATION | SINGLE_SOURCE_MASQUERADING | STALENESS_VIOLATION",
  "evidence": "<one or two sentences citing the specific row id, caveat text, endpoint count, or as_of date that triggered the finding>"
}
```

If the draft is clean, return `[]`.

## Hard rules

- Do not propose rewrites. Do not suggest alternative phrasing. Do not soften.
- Do not invent fact-sheet rows. If you can't find a row, that's a PHANTOM_CITATION, not a license to guess.
- Do not skip checks because the draft "reads fine." Run all 5 on every quantitative line.
- You may use `Bash` only to re-run `jq` over the provided fact-sheet bundle to confirm a row's existence or `as_of` value. No network calls, no other commands.
- You may use `Read` only on the draft file and the fact-sheet bundle paths the caller provided. Do not read elsewhere.

Return the JSON array as your entire response. No preamble, no summary, no "I found N issues" — just the array.
```

**Acceptance:**

- File exists at `/workspace/.claude/agents/ca-adversary.md` and parses as valid agent frontmatter.
- Invoking the agent on a sample draft containing the fabricated string `C4 sold 47 cards last week` (no citation) plus a valid fact-sheet bundle returns a JSON array containing at least one element with `finding_type == "FABRICATION"` and `location_in_draft` quoting that line.
- Invoking on a draft with `[id:nonexistent-1]` and a bundle lacking that id returns a `PHANTOM_CITATION` finding listing `nonexistent-1`.
- The agent never returns prose outside the JSON array.

---

## Task 9: Create `ca-reviewer` agent

**Files:**
- `/workspace/.claude/agents/ca-reviewer.md` (new)

**Steps:**

- [ ] Create the file `/workspace/.claude/agents/ca-reviewer.md` with the exact content shown in the code block below.
- [ ] Verify the YAML frontmatter parses; `tools:` is `Read, Grep, Glob, Edit, Write`.
- [ ] Confirm the agent body lists the Tier-A allowed write targets explicitly and forbids writes anywhere else.
- [ ] Confirm the agent body documents the natural-language trigger regex (for cross-reference; the regex lives in SKILL.md).
- [ ] Build a smoke transcript at `/tmp/ca-reviewer-smoke/transcript.md` containing a user turn that says "you made up the C4 sell-through number" and a corresponding adversary redline. Concatenate the matching fact sheet.
- [ ] Invoke the reviewer against the smoke transcript and confirm output includes either (a) a Tier-A Edit applied to `references/field-semantics.md` adding a new `semantics_caveat`, or (b) a Tier-B append to `docs/private/campaign-analysis-improvement-queue.md` with rationale, severity, and a unified diff.
- [ ] Verify the reviewer prints a session-end summary listing each auto-applied edit (path + 1-line description) and each queued item (path + severity).

**Agent file content (write verbatim):**

```markdown
---
name: ca-reviewer
description: End-of-session reviewer for the campaign-analysis skill. Reads the full session transcript, all fact sheets produced, all adversary redlines, and any user-flagged-error messages. Auto-applies safe Tier-A edits (semantics docs, impossible-asks, wishlist, user-feedback memory). Queues riskier Tier-B improvements with rationale + unified diff. Invoked by natural-language triggers in SKILL.md.
model: sonnet
tools: Read, Grep, Glob, Edit, Write
---

You are the campaign-analysis post-mortem reviewer. You run once at the end of a session (or when the user explicitly asks for a review) and your job is to turn the session's mistakes and friction into durable improvements to the skill's knowledge files.

## Trigger

The campaign-analysis SKILL.md detects when to invoke you using this regex (case-insensitive):

```
\b(review (this )?session|end[- ]of[- ]session|post[- ]mortem|what (should|could) (we|i) improve|how (can|should) we (improve|fix)|run (the )?reviewer)\b
```

You do not enforce the regex yourself — it's documented here so the skill author can keep it in sync.

## Inputs you will receive

1. The full session transcript (markdown export).
2. All fact sheets produced during the session (concatenated JSON).
3. All adversary redlines emitted during the session.
4. Any user turns flagged as error reports (the skill marks these with `<!-- user-flagged-error -->`).

If any input is missing, note it in the summary but proceed with what you have.

## Classification: Tier A vs Tier B

**Tier A — auto-apply.** Safe, narrowly-scoped edits to these files only:

- `/workspace/.claude/skills/campaign-analysis/references/field-semantics.md` — add or refine `semantics_caveat` rows. Never delete rows; only add or extend.
- `/workspace/docs/private/impossible-data-asks.md` — append a new bullet when the session confirmed an endpoint cannot answer a question class.
- `/workspace/docs/private/campaign-analysis-wishlist.md` — append a new bullet when the session surfaced a data need not currently met by any endpoint.
- `/home/vscode/.claude/projects/-workspace/memory/feedback_*.md` — append a new feedback file when the user explicitly corrected a recurring behavior. Use filename pattern `feedback_<short-slug>.md`. Update `MEMORY.md` index with the new entry.

You may use `Edit` and `Write` on these paths only. Any other write target → Tier B.

**Tier B — queue, do not apply.** Append to `/workspace/docs/private/campaign-analysis-improvement-queue.md` with one entry per finding. Each entry must contain:

- A short title (one line).
- Severity: `blocking` | `improvement` | `nice-to-have`.
- Rationale: 1–3 sentences explaining what went wrong in the session and why this change would prevent recurrence.
- Unified diff (in a fenced ```diff block) showing the proposed change. If the change spans multiple files, include all hunks.
- Transcript citation: a quoted line or two from the transcript that motivates the entry, with an approximate location ("near user turn 14" is fine).

Anything that touches code (`*.go`, `*.ts`, `*.tsx`, `*.sql`), the skill's `SKILL.md`, or any file under `internal/` → Tier B. Anything that deletes content from a Tier-A file → Tier B. Anything you're not sure about → Tier B.

## What to look for

Scan the transcript + redlines + user-flagged-errors for:

1. User corrections of a specific factual claim → Tier-A new `semantics_caveat` if the underlying row is misleading, or Tier-B SKILL.md change if the workflow itself is wrong.
2. Adversary redlines that fired repeatedly on the same row id or endpoint → Tier-A caveat refinement.
3. Question classes the session could not answer → Tier-A append to `impossible-data-asks.md` or `wishlist.md`.
4. User exasperation patterns ("you keep doing X", "I told you already") → Tier-A new `feedback_*.md` memory file.
5. Workflow gaps (missed a check, ran agents in wrong order, synthesis cited the wrong sheet) → Tier-B queue with proposed SKILL.md diff.

## Output: session-end summary

After applying Tier-A edits and queuing Tier-B entries, print a markdown summary:

```
## Campaign-analysis session review

### Auto-applied (Tier A)
- `<path>`: <1-line description of the change>
- ...

### Queued for human review (Tier B)
- [<severity>] <title> → `docs/private/campaign-analysis-improvement-queue.md`
- ...

### Inputs missing
- <any input that was absent, or "none">
```

If nothing was found, say so explicitly: `No durable improvements identified this session.`

## Hard rules

- Never write outside the Tier-A allowed paths. If in doubt, queue it.
- Never delete or rewrite existing content in Tier-A files; only append or add new rows/entries.
- Never modify SKILL.md, agent files, code, or migrations directly — those are always Tier B.
- Every Tier-B entry must have a unified diff. No "TODO: figure out the diff later."
- Cite the transcript for every entry. Vibes are not evidence.
- Do not invoke other agents. Do not call out to the network.
```

**Acceptance:**

- File exists at `/workspace/.claude/agents/ca-reviewer.md` and parses as valid agent frontmatter.
- Invoking the reviewer against a smoke transcript that contains a user turn "you made up the C4 sell-through number" plus the corresponding adversary redline produces either:
  - a Tier-A `Edit` to `references/field-semantics.md` appending a new `semantics_caveat` row referencing the offending endpoint, OR
  - a Tier-B append to `docs/private/campaign-analysis-improvement-queue.md` containing a title, severity, rationale, unified diff, and transcript citation.
- The session-end summary lists every applied edit and every queued item with the exact paths shown above.
- The reviewer never writes to any path outside the Tier-A allowed list.

## Task 10: Rewrite SKILL.md Steps 3, 3a, 3b, 3c and add Step 3d

**File:** `/workspace/.claude/skills/campaign-analysis/SKILL.md`

Replace the current Step 3 / 3a / 3b / 3c block with the following five steps, and append the reviewer-trigger recognition section at the end of the file.

### Build order

- [ ] Read current SKILL.md and locate the existing "Step 3" heading
- [ ] Replace Steps 3 / 3a / 3b / 3c with the blocks below
- [ ] Insert new Step 3d immediately after 3c
- [ ] Append the "Reviewer trigger recognition" section at the bottom of SKILL.md
- [ ] Verify `grep -n "Layer-1 dispatch" SKILL.md` returns a hit
- [ ] Verify `grep -n "post-?mortem\\|retro\\|review this session" SKILL.md` returns a hit

### Step 3 — Layer-1 dispatch: domain data agents in parallel

````markdown
### Step 3 — Layer-1 dispatch: domain data agents in parallel

Issue **one** Agent tool message containing **five** parallel invocations.
Each agent owns one endpoint family and returns a fact-sheet array.

| Agent | Endpoint family | Returns |
|---|---|---|
| `ca-capital` | `/api/finance/capital`, `/api/finance/invoices`, `/api/snapshot` (capital fields) | capital fact sheet |
| `ca-buying`  | `/api/insights` (buy%), `/api/snapshot.weeklyReview` | buying fact sheet |
| `ca-tuning`  | `/api/tuning/*`, `/api/insights.byGrade`, `/api/insights.byChar` | tuning fact sheet |
| `ca-sales`   | `/api/inventory/sales`, `/api/insights.byChannel`, `/api/portfolio/aging` | sales fact sheet |
| `ca-dh`      | `/api/dhlisting/*`, `/api/intelligence/dh` | DH fact sheet |

Every invocation receives the **same payload**:

```json
{
  "currentScope": "<from Step 1b>",
  "operatorConfig": "<inclusion lists, caps, grade restrictions>",
  "baseURL": "<prod or local>",
  "authHeader": "Bearer <token>"
}
```

Each agent MUST return an array of fact-sheet rows shaped as:

```json
{
  "id": "<agent>.<endpoint>.<row_key>",
  "endpoint": "/api/...",
  "fetched_at": "<ISO8601>",
  "value": <number|string|object>,
  "semantics_caveat": "<inline caveat or empty>",
  "freshness_window": "<e.g. live | 24h | weekly>"
}
```

Do not proceed to Step 3a until all 5 agents return.
````

### Step 3a — Concatenate fact sheets and run data-quality audit

````markdown
### Step 3a — Data-quality audit

Concatenate the five fact sheets into one array. Then emit an audit block:

```
DATA QUALITY AUDIT
- /api/finance/capital            ✓ 12 rows, fetched 14s ago
- /api/insights                   ✗ 0 rows (endpoint 500)
- /api/tuning/byGrade             ✓ 8 rows, fetched 18s ago, live-CL caveat applies
- /api/inventory/sales            ✓ 41 rows, fetched 21s ago
- /api/dhlisting/pending          ✓ 3 rows, fetched 22s ago
Missing rows: insights.weeklyReview (impact: cannot cite spend-this-week)
Impact line: synthesis will avoid weekly-spend claims; will request user paste if needed.
```

If a critical row is missing, decide: proceed with caveat OR ask user. Do
not silently substitute another endpoint.
````

### Step 3b — Synthesis draft (main thread)

````markdown
### Step 3b — Synthesis draft (main thread, hard rules)

Draft the opener / answer body on the main thread. Hard rules:

1. **Every number is followed by `[id:fact_sheet_row_id]`.** No exceptions.
   Bare numbers fail Layer-2 review.
2. **No derivation that contradicts a cited row's `semantics_caveat`.** If the
   row says "live CL price, drifts hourly", do not present it as a stable %.
3. **Two-source rule.** Any claim framed as "confirmed", "trend", or
   "pattern" must cite rows from **≥2 different endpoints**. A single
   endpoint can only support a "current snapshot" framing.
4. **No claim outside the cited rows.** If a needed fact is absent from the
   fact sheets, either drop the claim or loop back to Step 3 with a
   targeted re-fetch.

Output the draft in full, including all `[id:...]` markers — these are
read by Layer-2 and stripped in Step 3d before delivery.
````

### Step 3c — Dispatch ca-adversary (Layer-2)

````markdown
### Step 3c — Layer-2 dispatch: ca-adversary redline

Issue one `Agent` invocation of `ca-adversary` with payload:

```json
{
  "draft": "<full Step 3b draft including [id:...] markers>",
  "factSheets": "<concatenated array from Step 3a>"
}
```

The adversary returns a structured redline of findings, each tagged:

- `FABRICATION` — number with no `[id:...]` or pointing to a nonexistent row
- `PHANTOM_CITATION` — `[id:...]` references a row not in fact sheets
- `SEMANTICS_VIOLATION` — claim contradicts the row's `semantics_caveat`
- `SINGLE_SOURCE_MASQUERADING` — "confirmed"/"trend" claim with one endpoint
- `STALENESS_VIOLATION` — row outside its `freshness_window` used as live
````

### Step 3d (NEW) — Apply redline, strip markers, deliver

````markdown
### Step 3d — Apply redline, strip `[id:...]`, deliver

For each adversary finding:

| Tag | Action |
|---|---|
| FABRICATION | Drop the claim or re-fetch with a Layer-1 agent and re-synthesize |
| PHANTOM_CITATION | Drop the claim (cited row never existed) |
| SEMANTICS_VIOLATION | Reframe per the caveat, or drop |
| SINGLE_SOURCE_MASQUERADING | Demote framing to "current snapshot" OR add a second-endpoint cite |
| STALENESS_VIOLATION | Re-fetch the row, or label as stale-as-of |

After all findings are resolved, **strip every `[id:...]` marker** with a
final regex pass before user-facing render:

```
s/\s*\[id:[^\]]+\]//g
```

Only the cleaned text is shown to the user.
````

### Append: Reviewer trigger recognition

````markdown
## Reviewer trigger recognition

If the user's message matches this regex (case-insensitive), invoke
`ca-reviewer` in addition to (not instead of) the Layer-1 pipeline:

```
(post[- ]?mortem|retro(spective)?|review this session|what went wrong|what should we learn)
```

`ca-reviewer` classifies findings as Tier-A (auto-apply to
`references/field-semantics.md`) or Tier-B (queue in
`references/improvement-queue.md`). Auto-apply only Tier-A; Tier-B requires
operator approval.
````

### Acceptance

- [ ] `grep -n "Layer-1 dispatch" SKILL.md` returns the Step 3 heading
- [ ] `grep -n "Step 3d" SKILL.md` finds the new section
- [ ] `grep -nE "post-?mortem|retro|review this session" SKILL.md` matches the regex block
- [ ] The five Layer-1 agent names appear in the Step 3 table

---

## Task 11: Update `references/playbooks.md` with domain-agent routing

**File:** `/workspace/.claude/skills/campaign-analysis/references/playbooks.md`

### Build order

- [ ] Open `references/playbooks.md`
- [ ] Insert the preamble block (below) directly under the file's top-level `# Playbooks` heading, before Playbook A
- [ ] For each playbook A–G, insert a single `**Domain agents:** ...` line immediately under that playbook's heading (e.g. under `## Playbook A — Portfolio health`)
- [ ] Do not edit analytical content of any playbook

### Preamble block (insert under `# Playbooks`)

````markdown
> **Pipeline reminder.** Every playbook follow-up runs Layer-1 → Layer-2 →
> Layer-3 before responding. The relevant domain agents to dispatch in
> Layer-1 are listed at the top of each playbook below. If the playbook
> lists agents you have not invoked this turn, dispatch them before
> drafting synthesis.
````

### Per-playbook insertions

Insert **exactly one line** under each heading:

- Under `## Playbook A — Portfolio health`:
  ````markdown
  **Domain agents:** ca-capital, ca-sales
  ````
- Under `## Playbook B — P&L deep dive`:
  ````markdown
  **Domain agents:** ca-sales, ca-tuning, ca-buying
  ````
- Under `## Playbook C — Liquidation`:
  ````markdown
  **Domain agents:** ca-sales, ca-capital
  ````
- Under `## Playbook D — Tuning`:
  ````markdown
  **Domain agents:** ca-tuning, ca-buying
  ````
- Under `## Playbook E — Capital`:
  ````markdown
  **Domain agents:** ca-capital
  ````
- Under `## Playbook F — Acquisition`:
  ````markdown
  **Domain agents:** ca-buying, ca-dh
  ````
- Under `## Playbook G — New campaign`:
  ````markdown
  **Domain agents:** ca-buying, ca-tuning, ca-dh
  ````

### Acceptance

- [ ] Preamble block present under `# Playbooks`
- [ ] `grep -c "^\*\*Domain agents:\*\*" references/playbooks.md` returns `7`
- [ ] No analytical body text was modified (diff shows only additions)

---

## Task 12: Create `acceptance-scenarios.md` and run end-to-end validation

**File:** `/workspace/.claude/skills/campaign-analysis/references/acceptance-scenarios.md`

### Build order

- [ ] Create the file with the four scenarios below
- [ ] Run all four scenarios manually against the real skill
- [ ] Record outcomes in the "Run results" section at the bottom of the same file
- [ ] For any scenario that fails: open a Tier-B entry in `references/improvement-queue.md` and iterate until it passes

### File contents

````markdown
# Acceptance Scenarios

Four manual end-to-end scenarios that exercise the failure-mode coverage
matrix from the design spec. Run each before declaring the rebuild done.

## Scenario 1 — Fabrication

**Setup.** Force the synthesis draft (Step 3b) to include the literal
string `"netting roughly $178 per slab"` with **no** trailing `[id:...]`.
The fact sheets contain no row whose `value` equals 178.

**Expected ca-adversary output.**
```
FABRICATION: claim "netting roughly $178 per slab" has no [id:...]
  citation and no matching row in fact sheets.
  Action: drop claim or re-fetch.
```

**Pass criteria.** Step 3d drops the claim entirely OR loops back to
Layer-1 and re-synthesizes with a real citation. The string `$178` does
not appear in the final user-facing message.

## Scenario 2 — Live-CL semantics violation

**Setup.** Draft says `"PSA is paying 89% of CL on PSA 10 vintage"` with
citation `[id:tuning.byGrade.psa10_vintage.avgBuyPctOfCL]`. That row
exists in the fact sheets and its `semantics_caveat` reads:
`"avgBuyPctOfCL uses live CL price at query time; drifts hourly; not
comparable across days"`.

**Expected ca-adversary output.**
```
SEMANTICS_VIOLATION: claim presents avgBuyPctOfCL as a stable rate,
  but row semantics_caveat states it is a live drift-hourly figure.
  Action: reframe as "current snapshot" or drop.
```

**Pass criteria.** Final message reframes the claim to a snapshot
("right now PSA is paying ~89% of CL") or drops it. Does not present
it as a trend or a stable rate.

## Scenario 3 — Single-source masquerading

**Setup.** Draft says `"confirmed: buying is paused this week"` citing
only `[id:buying.weeklyReview.spendThisWeekCents]` (value 0). No other
endpoint is cited.

**Expected ca-adversary output.**
```
SINGLE_SOURCE_MASQUERADING: "confirmed" framing supported by one
  endpoint (snapshot.weeklyReview), which is partial-week and lagging.
  Action: demote framing to "snapshot" OR add a second-endpoint cite
  (e.g. /api/insights spend-by-day).
```

**Pass criteria.** Final message either (a) demotes to "current snapshot
shows zero spend this week so far" or (b) adds a second-endpoint
citation. Word "confirmed" is removed.

## Scenario 4 — Reviewer trigger

**Setup.** User types literally: `"post-mortem this session"`.

**Expected behavior.**
1. SKILL.md reviewer-trigger regex matches.
2. `ca-reviewer` is dispatched (in addition to Layer-1 pipeline).
3. Reviewer returns classified findings with at least one Tier-A row
   and one Tier-B item.
4. Main thread auto-applies the Tier-A row to
   `references/field-semantics.md` (append, do not rewrite).
5. Main thread appends the Tier-B item to
   `references/improvement-queue.md` and surfaces it to the user for
   approval — does NOT auto-apply.

**Pass criteria.** Both files have a new entry; diff is minimal; the
Tier-B item is explicitly flagged "awaiting operator approval" in the
user-facing message.

## Run results

| Scenario | Date run | Outcome | Notes / queue ref |
|---|---|---|---|
| 1 Fabrication | | | |
| 2 Semantics   | | | |
| 3 Single-src  | | | |
| 4 Reviewer    | | | |
````

### Acceptance

- [ ] File exists at the path above with all four scenarios
- [ ] All four scenarios executed manually and rows in "Run results" are filled in
- [ ] Any failing scenario has a corresponding Tier-B entry in `references/improvement-queue.md`
- [ ] Re-run after fixes shows all four passing

---

## Self-review checklist (implementing engineer)

Run through this before declaring the rebuild done. This is not a task —
no agent owns it; the engineer ticks the boxes.

- [ ] **Spec coverage.** Every section of `/workspace/docs/superpowers/specs/2026-05-19-campaign-analysis-multi-agent-design.md` maps to at least one task across parts 3a + 3b.
- [ ] **Placeholder scan.** `grep -rE "TBD|implement later|TODO\\(.*\\)" .claude/skills/campaign-analysis/` returns zero hits.
- [ ] **Type consistency.** The fact-sheet row schema (`id`, `endpoint`, `fetched_at`, `value`, `semantics_caveat`, `freshness_window`) is byte-identical across all five agent files (`ca-capital`, `ca-buying`, `ca-tuning`, `ca-sales`, `ca-dh`).
- [ ] **Tier-A vs Tier-B classification** in `ca-reviewer` matches the spec's classification table exactly (auto-apply vs queued, same examples).
- [ ] **Field semantics.** All 11 rows enumerated in the spec's field-semantics table are present in `references/field-semantics.md`.
