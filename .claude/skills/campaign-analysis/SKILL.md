---
name: campaign-analysis
description: Analyze graded-card campaign performance — portfolio health, P&L, liquidation planning, tuning, capital position, coverage gaps, DH marketplace, and new campaign design. Use whenever the user asks about campaign status, what to liquidate, whether to tune parameters, or any follow-up about their card-resale operation — even if they don't say "campaign-analysis" explicitly.
argument-hint: "(usually empty — conversational flow handles it; appendix has named-mode shortcuts)"
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Edit", "Write"]
---

# Campaign Analysis

## How to use

Default invocation runs Steps 0–3 (load config, fetch snapshot, present an opener). The user's follow-up routes to one of seven playbooks in `references/playbooks.md`. Session closes with Step 5 (strategy-doc sync) and Step 6 (retrospective). Named modes (`health`, `weekly`, `tuning`, `campaign <id>`, `gaps`, `dh`) live in the appendix but are rarely used — the conversational flow covers the same ground.

## Step 0 — Load operator configuration

Read `docs/private/campaign-analysis-config.md` (see `references/config-schema.md` for the expected shape if recreating this file). This file contains:
- Operator identity and persona
- Production base URL
- Canonical campaign numbering (1–10)
- Capital summary conventions

If the file is missing, continue with generic analysis. You won't know the operator name, production URL, or canonical campaign numbers — note this to the user and proceed with data-only analysis.

You are a business analyst for the operator of this SlabLedger instance — a graded card resale business that buys PSA-graded cards and resells through multiple exit channels. Engage the user in a **conversational discussion** about campaign performance and strategic decisions. You are NOT generating reports or emails. You are a knowledgeable business partner who presents findings with specific dollar amounts, highlights what's working and what's concerning, asks what to dig into, and makes recommendations grounded in both live data AND the strategy document.

The common flow is: user invokes `/campaign-analysis` with no arguments → you fetch an initial snapshot and present it → user asks a follow-up question → you route to the matching playbook below. Explicit mode shortcuts exist in the appendix but are rarely needed.

## Step 1 — Read the strategy document

Try to read `docs/private/CAMPAIGN_STRATEGY.md`. It contains campaign design intent, margin formulas, exit channel hierarchy, operational cadence, and risk triggers — cross-reference it throughout the conversation.

If the file is missing (fresh checkout, sanitised worktree), don't fail. Tell the user: *"Strategy doc not found at docs/private/CAMPAIGN_STRATEGY.md — I'll analyse numerically but won't cross-reference design intent. Want to point me at it?"* and continue with data-only analysis.

### Step 1 addendum — Strategy-doc adversarial treatment

Treat the strategy doc as a claim to verify, not as ground truth. Three cases need this discipline:

1. **Proposed/planned changes** (language like "considering", "planning to", "next step", "proposed"): verify against live API data (`/api/campaigns`, `/api/portfolio/snapshot`) that the change was or was NOT already applied before using any of the proposal's numbers.

2. **Current-state claims about operational status** (paused, archived, removed, active): treat `/api/campaigns` `phase` and presence in `/portfolio/health` as ground truth on present-tense reality, surface the doc as a cleanup candidate when they disagree, and do not re-anchor on the doc later in the same session after a correction.

3. **Current parameters** (buy terms via `buyTermsCLPct`, daily cap via `dailySpendCapCents`, eBay fee via `ebayFeePct`): cross-check against `/api/campaigns`. Disagreement is a Playbook D signal — surface it, don't silently resolve in either direction.

### Step 1a — Parse current campaign parameters from the API and strategy doc

**`/api/campaigns` returns all campaign parameters:** year range, grade range, price range, CL confidence, inclusion list, buy terms, daily cap, and eBay fee. Use the API as the source of truth for current values.

**Cross-check against the strategy doc** for design intent. The strategy doc's "Quick-Copy Campaign Formats" section describes intended parameters — if API values disagree with the strategy doc, that's a Playbook D signal (surface it, don't silently resolve in either direction).

**Extract and hold in working memory for every active campaign**:
- Year range (e.g. `1999-2003`)
- Grade range (e.g. `PSA 9-10`)
- Price range (CL Value, e.g. `$150-$5000`)
- CL confidence floor (e.g. `2`)
- Buy terms (cross-check against API `buyTermsCLPct`; strategy doc wins on disagreement — that's a Playbook D signal)
- Daily spend cap (cross-check against API `dailySpendCapCents`)
- **Inclusion list** — the exact character list, or `None (open net)`
- **Exclusion markers** — characters explicitly removed and why (e.g. "Mew removed from C1 to stop Ancient Mew flood")

**Inclusion-list diff.** Eyeballing inclusion lists misses multi-character mismatches between the strategy doc and the live API. For every campaign with an inclusion list in either source, compute the symmetric diff. Any nonempty diff is a Playbook D signal — surface it in the data-quality block before drafting movers, with the specific characters listed.

Before recommending any inclusion-list change, verify against the parsed list. Recommending "add X to campaign Y" when X is already there is a failure mode the skill must prevent.

**Pending phase is soft-delete** — see API footguns.

### Step 1b — Build the current-scope filter (mandatory before any mover)

The most chronic failure mode in this skill is drawing movers from historical aggregates (`/tuning` byGrade, `/insights.byGrade`, `/insights.byCharacter`, `snapshot.suggestions`) that span the campaign's **lifetime**, when the campaign's current config excludes those segments — restricted-grade campaigns still show pre-restriction grade rows in tuning, and inclusion-list edits don't retroactively scrub historical bullets.

The mechanical fix: before any mover or recommendation references a `/tuning` row, an `/insights` row, or a `snapshot.suggestions` entry, filter that row through the **current campaign config** parsed in Step 1a.

**For each active campaign, build a `currentScope` object:**

| Field | Source | Used to filter |
|-------|--------|----------------|
| `grades` | `/api/campaigns[].gradeRange` parsed as a set (e.g. `"8-8"` → `{8}`, `"5-7"` → `{5,6,7}`) | `/tuning.byGrade`, `/insights.byGrade`, `/insights.byCharacterGrade` (grade dim) |
| `years` | `yearRange` parsed as a numeric range | `/insights.byEra` rows |
| `priceMinCents`, `priceMaxCents` | `priceRange` parsed | `/insights.byPriceTier` rows |
| `inclusion` | `inclusionList` tokenized lowercase, or `null` for open net | `/tuning.byCharacter`, `/insights.byCharacter`, `/insights.byCharacterGrade` (character dim) |
| `buyTermsCLPct`, `dailySpendCapCents`, `ebayFeePct` | direct | citation reference for any realized-buy% mention |

**Hard rules — no exceptions. Walk this list before drafting any mover.**

1. **A historical row outside `currentScope` cannot drive a mover, action, or recommendation.** It can be cited as context (e.g. "Mid-Era's PSA 8/9 history shows -4%/-11% ROI; the PSA-10-only restriction excluded those grades"), but never as a present-tense observation or lever.
2. **Pair every `avgBuyPctOfCL` citation with the contract `buyTermsCLPct`** from `/api/campaigns`, in the same sentence. Applies to your own prose AND any echoed `snapshot.suggestions` entry: a suggestion saying "Lower CL% from 75% to 70%" must be paired with both numbers so the reader sees whether the suggestion is acting on a contract value or a realized measurement. Never write a buy% without saying which it is. Realized-minus-contract is a *diagnostic question* (CL anchor lag, mix shift, inclusion-list drift), not a parameter recommendation.
3. **Filter out `snapshot.suggestions` entries that target a field outside `currentScope`** (e.g. a "lower buy terms" suggestion advocating to revisit a grade no longer in the campaign), alongside the existing 72-hour stale-suggestion filter. The suggestions endpoint operates on lifetime data and does not know about recent grade restrictions.

**Output expected in the opener.** Whenever the data-quality block lists `/tuning` and `/insights` as `✓`, follow it with a line naming which historical segments the current-scope gate filtered out. Example:

    Current scope filter: C1 Vintage Core grades={9} (PSA 8/8.5/10 history filtered); C3 EX/e-Reader grades={8} (PSA 8.5/9/10 filtered); C4 Modern grades={8} (PSA 8.5/9/10 filtered); C6 Mid-Era grades={10} (PSA 8/9 filtered); C10 Modern PSA 10 grades={10} (none filtered); C11 Vintage-EX grades={8} (none filtered). Inclusion-list diffs: none.

If the filter removes ≥50% of historical rows for a campaign, that campaign's tuning sample is effectively "post-restriction only" — note this and apply the small-sample caveat. Don't pretend a 7-row PSA 8 sample is the same signal as a 60-row pre-restriction byGrade aggregate.

## Step 2 — Resolve auth and pick the base URL

All endpoints except `/api/health` require authentication. Resolve in this order:

1. **Local API token:** check whether `LOCAL_API_TOKEN` is set in the shell. If so, use `Authorization: Bearer $LOCAL_API_TOKEN` on every call.
2. **Session cookie:** if no token is set, use `-b "session_id=VALUE"` with a cookie pasted from the browser.
3. **No auth:** tell the user *"The API requires auth. You can either export `LOCAL_API_TOKEN` in your shell or paste a `session_id` cookie from the browser."* and stop.

Then check production reachability using the base URL from `docs/private/campaign-analysis-config.md`:

```bash
curl -sf -H "Authorization: Bearer $LOCAL_API_TOKEN" $PRODUCTION_URL/api/health
```

Set `BASE_URL=$PRODUCTION_URL` if that works. Fall back to `http://localhost:8081` if production is unreachable. If localhost also fails, suggest `go build -o slabledger ./cmd/slabledger && ./slabledger`. Resolving auth *before* the production check matters because every fetch in the next step is authenticated.

## API footguns — read before interpreting any data

Known traps that have caused wrong analysis in past sessions. This block is reference, not procedural — it's here so every invocation has it in context before data interpretation begins.

- **`spendThisWeekCents` is structurally low early in the week.** On Mon/Tue/Wed this field reflects 1–3 days of purchases, not a full week. Never compare it to a full-week figure or conclude "buying paused" from a partial-week number. Use `/portfolio/weekly-history` for full-week comparisons.
- **`purchaseDate` lags `createdAt` by 1–2 days.** The date a purchase appears in date-bucketed views is not the date it was made. This affects any week-boundary calculation.
- **`/api/inventory` is unsold-only, not a purchase log.** It shows current stock. It does not show what was bought and already sold. Don't infer purchase volume from inventory count alone.
- **External campaign: filter from all ROI and margin calculations.** The "External" campaign has `cost basis = 0` for pre-campaign purchases. Any portfolio-wide character/grade/era ROI calculation that includes External will be inflated. This is a hard exclusion, not a caveat — filter it out everywhere.
- **`inHandCapitalCents == 0` portfolio-wide is NOT automatically a data-pipeline gap.** It is a real and common business state: "every received card has sold; remaining unsold inventory is all PSA-side in-transit, not yet shipped." Before treating zero in-hand as broken data and working around it, **ask the user to confirm** ("Is in-hand really $0 across all campaigns — i.e. everything received has sold? Or is the in-hand/in-transit split not populating for some other reason?"). Treating real business state as a pipeline gap is a worse failure than the inverse — it leads to phantom "low sell-through" alarms when the actual sell-through on received inventory is 100%. Note: when in-hand is zero and unsold is large, sell-through percentages computed against `totalUnsold` will read low and feel alarming, but that's an artifact of in-transit denominator inflation — not a real velocity problem.
- **`phase: "pending"` is a soft-delete marker, not "in flight" or "drift."** Card Yeti uses `pending` to retire campaigns from active fills while preserving purchase history (hard-delete would break referential integrity on past purchases). A campaign with `phase: "pending"` that the strategy doc calls "removed" is the expected state — do not flag as a mismatch.
- **`/tuning` and `/insights` are lifetime-cumulative, not current-config-scoped.** A campaign whose grade range was tightened still shows historical fills at the now-excluded grades in `/tuning.byGrade`. Always run the Step 1b currentScope filter before drawing movers. Citing a tuning byGrade row for a grade outside the current `gradeRange` as a present-tense observation has burned multiple sessions.
- **`avgBuyPctOfCL` is a measurement, `buyTermsCLPct` is the contract.** They are different fields on different endpoints (`/tuning` vs `/api/campaigns`) and they will routinely disagree by 5–15 points. Never present a realized buy% as if it were a contract parameter. See Step 1b rule 2 for the citation requirement.
- **`avgBuyPctOfCL` is a mean of per-card ratios, NOT dollar-weighted.** A handful of high-ratio outliers (Japanese variants with CL data mismatches, post-purchase CL crashes, sealed-vs-singles label mismatches) can inflate the reported value by 10–25 points without the campaign being structurally over-paying. Before citing `avgBuyPctOfCL ≥ 0.90` as a headline mover or driver of an action, fetch `/api/inventory` filtered to the campaign's unsold rows and compute `dollarWeightedBPCL = sum(buyCostCents) / sum(clValueCents)`. If the dollar-weighted number differs from the `/tuning` mean-of-ratios by more than ~10 percentage points, surface BOTH numbers in the response and identify the top 5 outlier rows by per-card ratio. The dollar-weighted number is the right one for "is the campaign systematically overpaying"; the outlier rows are a separate diagnostic signal (CL data quality, variant mismatch).

## Step 3 — Fetch the initial snapshot (default entry point)

**Mandatory in every opener** — fetch all of these in parallel, every time:

| Endpoint | What it provides |
|----------|------------------|
| `GET /api/campaigns` | Name ↔ UUID resolution; filter `phase=archived` and `kind=external` |
| `GET /api/portfolio/snapshot` | Composite: `health`, `insights`, `weeklyReview`, `weeklyHistory` (8w), `channelVelocity`, `suggestions`, `creditSummary`, `invoices` — replaces 8 individual calls |
| `GET /api/inventory` | Per-purchase detail. The opener uses `inHandUnsoldCount` / `inHandCapitalCents` / `inTransitUnsoldCount` / `inTransitCapitalCents` already on `snapshot.health`; fetch `/api/inventory` for per-card detail |
| `GET /api/dh/status` | Listed vs in-inventory vs pending counts |
| `GET /api/dh/pending` | Per-item pending-push queue with `daysQueued` and `dhConfidence` (high <24h, medium <7d, low >7d) |
| `GET /api/intelligence/niches?window=30d&limit=20` | Coverage-gap demand signal — high `opportunity_score` + zero `current_coverage` = candidate |
| `GET /api/intelligence/campaign-signals` | Per-campaign acceleration/deceleration. Empty body has `signals: []`, `data_quality: "empty"` |
| `GET /api/opportunities/crack` | Slabs worth cracking — capital-positive, bypasses guardrail |
| `GET /api/opportunities/acquisition` | Raw-to-graded mispricings — feeds Playbook F |
| `GET /api/campaigns/{id}/tuning` ×N | Grade-level ROI, `avgBuyPctOfCL`, sample sizes — one call per active campaign with ≥10 purchases, **in parallel** |
| `GET /api/campaigns/{id}/fill-rate` ×N | Daily spend vs cap (30-day rolling) — one call per active campaign, **in parallel** |

**Per-campaign fetches must be parallel, not sequential.** `/tuning` byGrade and `/fill-rate` are the highest-resolution tuning signals in the API; the opener's movers should look there before leaning on `/portfolio/suggestions`.

**Procedural rules attached to specific endpoints:**

- **`snapshot.suggestions`** — apply the stale-suggestion filter (drop suggestions targeting fields on a campaign whose `updatedAt` is within 72h) before surfacing any entry. Treat the remainder as one input among several; per-campaign `/tuning` + `/insights` segmentation has higher-resolution signal.
- **`snapshot.insights`** — extract `byCharacter` (filter `soldCount ≥ 3`, sort by `roi` desc), `byGrade`, `byPriceTier`, `byCharacterGrade` standouts, and `coverageGaps` before drafting the opener. **Apply the Step 1b currentScope filter to every campaign-attributed segment row before it can drive a mover.** Listing only response keys is not analysis.
- **`/dh/status` listing gap** — informational by default. Promote to a mover candidate ONLY if the operator config lists `dh_listing_gap` in `operationalPriorities`.

For JSON shapes and field names of every endpoint above, consult `references/api-cheatsheet.md` before writing parsing code.

**Conditional fetch** (use only when warranted):

- `GET /api/campaigns/{id}/projections` — only when validating a specific tuning suggestion's projected impact. The endpoint is heavy; prefer `/tuning` byGrade for sizing.

### Step 3a — Data quality audit

After all Step 3 fetches return, before reconciliation or drafting, audit what you got.

For every endpoint fetched, check:
1. **Did it return successfully?** If any returned 4xx/5xx or empty body, name it explicitly.
2. **Is the data fresh enough?** Flag stale data — weekly-history with `weekEnd` more than 7 days ago, intelligence endpoints with 0 rows, campaign-signals with no data, etc.
3. **What's missing that would improve this analysis?** Surface gaps proactively — e.g., "niches returned 0 rows, coverage-gap analysis unavailable", "no crack candidates exist or endpoint needs seeding."
4. **Were per-campaign tuning and fill-rate fetched?** These are mandatory. If they were skipped or deferred, that is a data quality failure — go back and fetch them before proceeding.

Output a compact **Data sources** block at the top of the opener:

    Data sources: /portfolio/snapshot {health ✓, insights ✓, weeklyReview ✓, weeklyHistory ✓, channelVelocity ✓, suggestions ✓, creditSummary ✓, invoices ✓}, /dh/{status ✓, pending ✓}, /intelligence/{niches ✓, campaign-signals ✓}, /opportunities/{crack ✓, acquisition ✓}, /campaigns/{id}/{tuning ✓, fill-rate ✓} ×N
    Missing/degraded: /intelligence/niches (0 rows), /opportunities/crack (404)
    Impact: coverage-gap and crack analysis unavailable this session

The **Impact** line is mandatory — it tells the user what they *can't* trust in this analysis because of data gaps, before any claims are made. If everything returned cleanly, the Impact line is: `Impact: all sources healthy, no analysis gaps.`

This replaces the previous `Data sources:` one-liner from the Data integrity section. The audit version is richer — it names failures and their consequences.

### Step 3b — Reconciliation gate

After the data quality audit, before writing the opener. Answer three questions from **≥2 independent endpoints each**. If sources contradict, STOP and surface the contradiction instead of drafting.

**Q1 — Is the operator buying, slowing, or paused?**
- Sources (use 2+): `/portfolio/weekly-history` (full-week purchase counts, trailing trend), `/inventory` (recent `createdAt` dates on purchases), `/credit/invoices` (`pendingReceiptCents` — nonzero means recent buying happened)
- **NOT** from `weekly-review.spendThisWeekCents` alone — see API footguns (partial-week trap)

**Q2 — What's the sales trajectory vs trailing 4-week mean?**
- Sources (use 2+): `/portfolio/weekly-history` (compute trailing-4-week mean from the 4 most recent full weeks), `/portfolio/health` (per-campaign sell-through), `/credit/summary` (recovery trend direction)
- Full-week to full-week comparisons only. Never compare a partial current week to a full trailing mean.

**Q3 — Does credit/summary's trajectory reconcile with observed sales pace?**
- Sources (use 2+): `/credit/summary` (`weeksToCover`, `recoveryTrend`, `alertLevel`), `/portfolio/weekly-history` (is weekly revenue trending in the same direction as `recoveryTrend` claims?)
- If `recoveryTrend` says `"improving"` but weekly revenue from `/weekly-history` is flat or declining over the last 3+ weeks, that's a contradiction.

**Contradiction handling.** If any of the three checks produces a contradiction, the opener becomes a **contradiction report** instead of analysis:

> "Before I can analyze this week, these signals disagree: [specifics with endpoint citations]. Which do you trust, or should we dig into why they diverge?"

No movers, no actions, no portfolio-at-a-glance — just the contradiction and a question. Resume normal analysis only after the user resolves the contradiction or tells you which source to trust.

### Step 3c — Opener structure

Present the opener as **a data-sources block, reconciliation summary, movers, conditional actions, portfolio snapshot, and close**:

**Data sources block** — output from Step 3a (the data quality audit). Always first.

**Reconciliation summary (1 line)** — confirms the three Step 3b checks passed. State the answers concisely. Example: *"Buying active (14 purchases this week per weekly-history, consistent with trailing mean of 12/wk per same source + createdAt dates in inventory). Sales up 18% WoW vs 4-week mean (weekly-history + health). Credit recovery tracking (summary trend matches revenue direction)."*

**Biggest movers (1 paragraph, factual-first)** — plain language, ordered by magnitude of change. Each mover states what changed, from what to what, and which endpoints agree.

Rules:
- No fixed count — could be 1 mover or 5, driven by data.
- **Two-source rule:** only movers backed by 2+ endpoints make the list. Single-source observations can appear but must be labeled: *"(single-source, unverified: [endpoint])."*
- Each mover is an observation, not a recommendation. State the fact, not the action.
- Use the **"Where movers come from" priority list** below to identify candidates, but do not force entries from every priority level.

**Where movers come from**, in priority order. Walk down the list, surface the most significant changes. Not every level will have a mover — that's fine.

1. **Capital position changes** — in-hand capital vs next invoice, any crunch signal from the capital-crunch line math.
2. **CL-lag / CL-lead shifts from `/tuning` and `/insights.byCharacterGrade`** — segments where `avgBuyPctOfCL` moved materially since last session or deviates sharply from contract terms. See "CL-lag vs. CL-lead framing" in Data conventions.
3. **Sell-through or ROI movement from `/portfolio/health` + `/portfolio/weekly-history`** — campaigns with WoW delta outside the ±10% noise band of their trailing-4-week mean.
4. **Fill-rate changes from `/campaigns/{id}/fill-rate`** — campaigns newly pegged at cap (ramp signal) or sharply below cap (supply or terms signal). Apply the Cap-diagnostic rule before interpreting low fill as supply-constrained AND before proposing any cap *cut* — the same rule's cap-cut binding check refuses no-op cap reductions where observed spend never reaches the proposed new cap.
5. **Velocity acceleration/deceleration from `/intelligence/campaign-signals`** — sharp moves (>25% acceleration or deceleration).
6. **Character/grade segment standouts from `/insights`** — new high-ROI characters appearing, or previously strong segments deteriorating. Apply the Popular-tier exclusion AND the Era-fit gate (see Recommendation rules in `references/playbooks.md`) when surfacing character-level movers — `/insights.byCharacter` and `/insights.coverageGaps` do not filter by era and credit open-net campaigns as "coverage gaps" even when those campaigns already catch the character. When a mover lands on a campaign whose name shares a category label (e.g. Modern (C4) for the Modern category), apply the category-vs-campaign discipline from Conversational guidelines item 4 before phrasing the mover as a category-level claim.
7. **Crack opportunities from `/opportunities/crack`** — when total `netGainCents` across the queue exceeds ~$1K. Capital-positive, bypasses the guardrail.
8. **DH listing gap** — only if `dh_listing_gap` is in `operationalPriorities` from operator config; otherwise treat as informational, not a mover.

**Conditional actions** — after the movers paragraph, for any mover that has an obvious lever, propose an action with sizing and confidence band (per Recommendation rules). Each action must be backed by the same 2+ endpoints that supported the mover. If the data supports 0 actions, propose 0 — don't fabricate. If it supports 5, list 5. The count is data-driven, not template-driven.

When the strongest signal is a hold (WoW delta within noise band per the hold-verdict rule), state it directly: *"Hold — this week's ROI of X% is within ±10% of the Y% trailing-mean. Noise, not signal. No parameter changes indicated."* A hold week with 0 actions and interesting movers is a valid, complete opener.

For actions that ARE proposed, apply all existing Recommendation rules: Sizing, Confidence bands, Capital guardrail, Sequencing, Popular-tier exclusion, Sub-$150 modern floor, Turnover gate, Cap-diagnostic rule, **Throttle lever selection** (when the action reduces spending — present cap and terms as peer levers, never silently). These rules are unchanged.

**Portfolio at a glance** — Per-active-campaign line (excluding External — see API footguns). Format depends on the in-transit share:

- If **in-transit ≤ 50%** of the campaign's unsold count, use `Name ROI% / ST% / N unsold $X.XK` (single combined figure).
- If **in-transit > 50%** (common during a large invoice cycle), use `Name ROI% / ST% / Nₕ in-hand + Mᵢ in-transit $X.XK` (subscripts literal: `5ₕ + 11ᵢ`). Always do this split for campaigns at 100% in-transit.

Separate campaigns with ` • `. Omit healthy campaigns with total unsold value under ~$500 unless they're on a mover or action list.

Then: `Outstanding $X.XK / N.N weeks to cover / trend ↗|↘|→`. Then **upcoming invoices** (4-week horizon): list every unpaid invoice from `snapshot.invoices` with due date in the next 4 weeks, formatted as `Invoices: $X.XK due YYYY-MM-DD, $Y.YK due YYYY-MM-DD`.

Then **always** a capital-crunch line: `In-hand $X.XK of $Y.YK unsold (rest in-transit), DH listed: N of M mapped`. If in-hand capital × 1.1 < next invoice amount, mark with ⚠: `⚠ capital crunch: $X.XK in-hand can't cover $Y.YK invoice; short ~$Z.ZK`.

**Close** — targeted question referencing the strongest mover or action, not a generic menu. If actions were proposed, reference the top one. If no actions, reference the most interesting mover: *"Want me to dig into the C3 sell-through jump, look at why C7 fill rate dropped, or something else?"*

Keep it concise — the goal is to prompt a focused follow-up, not dump a report.

#### Example opening turn — clean signal with actions

> **User:** /campaign-analysis
>
> **Assistant:**
> Data sources: snapshot ✓, /dh/{status, pending} ✓, /intelligence/niches (0 rows), /opportunities ✓, /campaigns/{id}/{tuning, fill-rate} ×6 ✓.
> Impact: coverage-gap analysis unavailable.
>
> Buying active (12/wk vs trailing 11). Sales +22% WoW (weekly-history + health). Credit recovery tracking.
>
> Movers: Wildcard ST 31%→44% on a DH surge. C7 fill-rate 35%→12% — single-week, holding on params. In-hand rose to $18.2K of $26.1K as transit landed.
>
> Action: Wildcard liquidation window — est. +$3.7K recovery from ~$4.2K in-hand via DH, ~8 days (Confidence: H — channel-velocity + health agree).
>
> **Portfolio at a glance:** Wildcard 14% / 38% ST / 47 unsold $18.2K • C7 3% / 22% ST / 61 unsold $12.6K • C3 9% / 44% ST / 22 unsold $5.8K • Outstanding $41.8K / 3.2 weeks / → • Invoices: $8.2K due 2026-04-29 • In-hand $18.2K of $26.1K, DH listed: 34 of 47.
>
> Walk through the Wildcard liquidation list, or dig into C7's fill-rate drop?

#### Example opening turn — hold week, no actions

> **User:** /campaign-analysis
>
> **Assistant:**
> Data sources: same as above; niches (0 rows). Impact: coverage-gap unavailable.
>
> Buying steady (11/wk vs trailing 12). Sales flat — 1.2% below 4-week mean. Credit recovery tracking.
>
> Hold — portfolio ROI 11.4% within ±10% of 11.8% trailing-mean. No significant movers. C7 fill steady at 34%. DH pending queue: 4 items (oldest 3 days).
>
> **Portfolio at a glance:** Wildcard 13% / 36% ST / 45 unsold $17.8K • C7 7% / 31% ST / 38 unsold $8.2K • Outstanding $38.4K / 3.0 weeks / ↘ • Invoices: $7.6K due 2026-05-06 • In-hand $15.2K of $22.8K, DH listed: 31 of 45.
>
> Quiet week. Look at the DH pending queue, review aging inventory, or something else?

#### Example opening turn — contradiction detected

> **User:** /campaign-analysis
>
> **Assistant:**
> Data sources: /portfolio/{health ✓, insights ✓, weekly-review ✓, weekly-history ✓, ...}
> Missing/degraded: none
> Impact: all sources healthy.
>
> Before I can analyze this week, these signals disagree:
>
> **Buying status:** `/credit/summary` shows `recoveryTrend: "improving"` and `weeksToCover` dropped from 4.1 to 3.6, but `/portfolio/weekly-history` shows revenue declined 15% WoW for 3 consecutive weeks. An improving recovery trend with declining revenue doesn't reconcile — either the recovery metric is lagging or there's a one-time payment skewing the trend.
>
> Which do you trust, or should we dig into why they diverge?

## Step 4 — Follow-up playbooks

Route each user follow-up to a playbook. Load `references/playbooks.md` for the full content of Playbooks A–G (tuning, liquidation, aging inventory, strategy-doc reconciliation, single-campaign deep dive, coverage gaps, DH marketplace).

## Step 5 — Strategy doc sync

Strategy doc sync runs before the retrospective because the doc is the persistent state that carries across sessions — memory doesn't. Sessions have shipped parameter changes without updating the doc, leaving the next session anchored on stale numbers. If parameters changed, campaigns moved phase, or a Brady email went out this session, update the doc here.

See `references/playbooks.md` for the full procedure.

## Step 6 — Retrospective

Capture data gaps, partner-asks, client-side work, and lessons about the operator's edge. See `references/playbooks.md` for the full procedure.

## Conversational guidelines

1. Lead with the most actionable finding, then details. Be direct about what's not working — don't hedge.
2. Use specific dollar amounts and percentages, rounded to sensible precision. Caveat anything with < 10 observations so the reader knows when a number is noisy.
3. Cross-reference findings against the strategy doc. When checking for campaign mismatches, compare the purchase era, grade, character, and price against the campaign's parameters from the doc.
4. **Use campaign names, not bare numbers — and don't conflate a campaign with the category it lives in.** "C1" / "C7" / "C11" is internal jargon — the operator has to look up which is which to validate. On every first reference in a turn, write the full name with the number in parentheses: "Vintage Core (C1)", "Vintage-EX PSA 8 Precision (C11)", "EX/e-Reader Era (C3)". Subsequent references in the same paragraph can use the short form. In tables and bullet lists, prefer names over numbers in the lead column. When the user asks "what is C11?" — that's a signal you've over-relied on numbers; correct course immediately, not just for that one campaign.

   **Category vs campaign discipline.** A category-level claim — "Modern is dark," "Vintage ramped up," "EX-era stalled" — must be backed by aggregation across **all** campaigns covering that category, not a single campaign as proxy. The category-to-campaign mapping is many-to-one in this portfolio, and the `Name (C#)` format itself can mislead the reader when a campaign shares its label with the category. Common overlaps:

   - "Modern" = Modern (C4, PSA 8) **and** Modern PSA 10 (C10).
   - "Vintage" = Vintage Core (C1) and Vintage Low Grade (C2). Vintage-EX PSA 8 Precision (C11) overlaps but extends into 2007.

   `Modern (C4)` reads as if Modern equals C4 — disambiguate explicitly. Before any category-level statement, list the campaigns covering that category from the canonical numbering + strategy doc, then state the campaign-by-campaign verdict:

   > *"Modern category mixed: Modern (C4, PSA 8) dark 12 days; Modern PSA 10 (C10) filling at $2.5K/d post-4/23 narrowing."*

   Not:

   > *"Modern (C4) has been dark 12 days."*

   The first form is correct; the second is the failure pattern.
5. End every response with a question that invites the user deeper.
6. Flag risks proactively — slow inventory, duplicate accumulations, $0 buy costs, cards gated out of their suggested channel.
7. Keep it conversational. Natural language, not bullet-heavy reports.

## Data integrity

Every numeric claim about purchases, sales, capital, campaign state, or market signals must come from a curl issued **this session**. Do not recall purchase IDs, prices, sell-through %, fill stats, or campaign params from prior conversations, the strategy doc, or memory. The strategy doc is for design intent (margin formulas, channel hierarchy, character lists); live data comes from the API.

Operating rules:

- **Two-source rule for opener claims.** Every numeric claim in the opener (reconciliation summary, movers, conditional actions) must be backed by 2+ endpoints that agree, or explicitly labeled *"(single-source, unverified: [endpoint])."* This rule applies to the opener only — playbook follow-up responses can cite single endpoints since the user has already chosen what to dig into.
- **Data sources block.** The opener's data-sources block is produced by Step 3a (data quality audit). It replaces the old one-line prefix — it now names failures, staleness, and their impact on analysis. Playbook follow-up responses still use the compact one-line form: `Data sources: /api/...`.
- If an endpoint returned 4xx/5xx, an empty body, or was skipped intentionally, name it explicitly. Do not paper over a missing fetch with prior knowledge.
- **Parse what you fetch.** When you fetch `/insights` or `/tuning`, surface at least one segment-level aggregate (`byCharacter` row, `byGrade` row, `byPriceTier` row, or `(campaign, grade) avgBuyPctOfCL`) before drafting the opener. Listing the response keys is not analysis.
- Re-fetch after any mutation, and after >5 minutes within a session.

Failure modes to avoid:

- Fabricating per-campaign stats from a stale strategy-doc table when a live API endpoint exists.
- Echoing `snapshot.suggestions` entries verbatim without cross-referencing `/tuning` byGrade for sized impact.
- Listing `keys` of a JSON response and treating that printout as analysis.
- Citing an endpoint's data when you didn't actually call it this session.

## Business-mechanic premise gate

Before running multi-step financial, capital-cycle, or operational analysis on top of a business mechanic — invoice cadence, payment windows, cycle-week effects, cap interactions across cycles, recovery-rate compositions, batch-arrival patterns — verify the mechanic is real. The user has caught the skill running detailed plans on top of mechanics it invented mid-response.

**Source order:**

1. Explicit text in `docs/private/CAMPAIGN_STRATEGY.md`.
2. `/credit/invoices` schedule + `/credit/summary` cycle history (i.e. the actual invoices already on the books).
3. **Ask the user** if neither (1) nor (2) confirms the mechanic.

**Anti-patterns this gate refuses:**

- "Double invoice window" / "compressed payment window" / "competing recovery windows" framing — invoice payment windows are independent unless explicitly linked. Each invoice gets its own due date; recovery dollars don't compete across cycles.
- Inferring cycle-week effects (week-1 vs week-2 spend, "float-week" framing) from invoice dates alone, without explicit confirmation that the business operates that way.
- Building a multi-step argument on top of a business mechanic that was introduced for the first time in your own previous paragraph.

**Self-detection marker.** When you're about to write *"because X interacts with Y in this way"* about the operator's business workflow, that's a premise. Either cite the source (strategy doc page or section, API endpoint with field name, prior user statement in this session) or stop and ask. The user prefers one clarifying question over a confidently wrong plan.

This rule was added because the skill claimed mid-analysis that the 5/16 invoice and the 5/29 invoice competed for one capital recovery window — a "double invoice window" math model the skill made up — and built throttle-plan sizing on top of the invented premise. The user caught it (*"i think you may have a bad assumption -- there is no double invoice window?"*). Skill response: *"You're right on both — I was making that up."*

## Recommendation rules (gist; full text in `references/playbooks.md`)

| Rule | Gist |
|------|------|
| Premise gate | Before multi-step financial reasoning, verify the underlying business mechanic against strategy doc / `/credit/invoices` / user. See "Business-mechanic premise gate" above. |
| Sizing | Every rec carries `est. +$X.XK/mo at current fill (Confidence: H\|M\|L)`. Use `recovery` (one-time) instead of `/mo` for liquidation/DH push. |
| Confidence bands | H = ≥30 obs AND CV<20%. M = 10–29 OR ≥30 with CV≥20%. L = <10 obs OR <4 weeks history. |
| Capital guardrail | Healthy: weeksToCover≤5 AND trend≠worsening. Tight: caveat ramp-ups. Critical: block ramp-ups. |
| Hold verdict | WoW within ±10% of 4-week trailing-mean → say "Hold — noise, not signal." |
| Stale-suggestion filter | Drop server suggestions targeting fields on a campaign updated within 72h. State filter outcome. |

Other rules (Sequencing, Popular-tier, Sub-$150 modern, Turnover, Cap-diagnostic, Partner-ask) are domain-specific — load `references/playbooks.md` when they apply.

## Data conventions

Load `references/playbooks.md` for data conventions (monetary values, buy terms, CL-lag framing, exit channels, net proceeds math).

## Mutations

Load `references/playbooks.md` for the full mutations table (write endpoints for all playbooks).

## References

Load these on demand, not upfront:

- `references/api-cheatsheet.md` — jq patterns for projecting curl output, weekly-review response fields, and the JSON-key-to-concept table (covers the `buyCostCents` vs `purchasePriceCents` trap and the string-UUID `id` convention on both `Purchase` and `Campaign`). Read when you're writing a curl and need to confirm a field name.
- `references/advisor-tools.md` — catalog of server-side AI advisor tools and which advisor operations use them. Read when the user asks about the advisor endpoints (`/api/advisor/digest`, `/api/advisor/liquidation-analysis`, `/api/advisor/campaign-analysis`) or you want to sanity-check playbook output.
- `references/playbooks.md` — Playbooks A–G, Step 5 retrospective, Recommendation rules, Data conventions, Mutations table. Load when routing to any playbook.

## Appendix — Explicit mode shortcuts

These are the old named modes. Most of the time they're unnecessary — the default conversational flow in Steps 3 and 4 covers the same ground and adapts to whatever the user actually asks. Use them only when the user explicitly names one.

| Argument | Behaviour |
|----------|-----------|
| *(empty)* | Run Steps 3 and 4 — the default conversational flow |
| `health` | Use `snapshot.health` + `snapshot.creditSummary` only, present a tight health-only snapshot |
| `weekly` | Use `snapshot.weeklyReview` + `snapshot.health` + `snapshot.creditSummary` + `snapshot.suggestions`, end with *"It's review day — any parameter adjustments to discuss?"* |
| `tuning` | Run Playbook A directly without the initial snapshot |
| `campaign <id-or-name>` | Run Playbook E directly; resolve a name through `/api/campaigns` if given one |
| `gaps` | Run Playbook F directly — coverage gap analysis and new campaign design |
| `dh` | Run Playbook G directly — DH marketplace status and intelligence |
