# Campaign Analysis — Playbooks and Reference Rules

Load this file when routing to any follow-up playbook (Step 4), running the strategy doc sync or retrospective (Steps 5-6), or consulting recommendation rules, data conventions, or mutations.

## Contents

- [Step 4 — Follow-up playbooks](#step-4--follow-up-playbooks)
  - Playbook A — "What campaign updates should we make?" (tuning)
  - Playbook B — "What should we liquidate to pay our invoice?" (liquidation)
  - Playbook C — "Should we consider price adjustments on aging inventory?" (repricing)
  - Playbook D — "Does the strategy doc still match reality?" (doc reconciliation)
  - Playbook E — Single-campaign deep dive
  - Playbook F — "What niches are we missing?" (coverage gaps / new campaigns)
  - Playbook G — "How are our DH listings doing?" (marketplace)
- [Step 5 — Strategy doc sync](#step-5--strategy-doc-sync)
- [Step 6 — Retrospective](#step-6--retrospective)
- [Recommendation rules](#recommendation-rules) — Sizing, Stale-suggestion filter, Confidence bands, Hold verdict, Fill-drought hypothesis ranking, Capital guardrail, Sequencing, Popular-tier exclusion, Era-fit gate, Sub-$150 modern floor, Turnover gate, Cap-diagnostic, Throttle lever selection, Partner-ask verification
- [Data conventions](#data-conventions) — buy terms, CL-lag vs CL-lead framing, exit channels, net-proceeds math
- [Mutations](#mutations) — write endpoints by intent

## Step 4 — Follow-up playbooks

Route each user follow-up to the matching playbook below. The `references/advisor-tools.md` file catalogs the server-side AI advisor tools; the advisor endpoints (`POST /api/advisor/digest`, `POST /api/advisor/liquidation-analysis`, `POST /api/advisor/campaign-analysis`) run an LLM loop over those tools and can be used as a sanity check against playbook output when time permits.

### Playbook A — "What campaign updates should we make?"

Trigger phrases: *"what updates should we make", "campaign tuning", "parameter adjustments", "should we change buy terms", "what should we change in our campaigns"*.

This playbook is the most-asked question and the one where opener output most often disappoints. The answer must be substantive: per-campaign verdicts grounded in `/tuning` byGrade and `/insights` segment data, not generic suggestions echoed from the server.

Fetch in parallel (most should already be in the opener cache from Step 3):
- `GET /api/campaigns/{id}/tuning` for **every** active campaign — grade-level ROI, price-tier performance, `avgBuyPctOfCL`, `roiStddev`, `cv`. Run all calls in parallel, not sequentially.
- `GET /api/campaigns/{id}/fill-rate` for each active campaign — pegged-at-cap vs supply-constrained.
- `GET /api/portfolio/insights` — `byCharacter`, `byGrade`, `byPriceTier`, `byCharacterGrade`, `coverageGaps`. PARSE these segments; don't just list keys.
- `GET /api/portfolio/suggestions` — server-side suggestions (apply stale-suggestion filter before use).
- `GET /api/intelligence/campaign-signals` — per-campaign acceleration/deceleration.
- `GET /api/intelligence/niches?window=30d` — coverage-gap demand signal.
- `GET /api/opportunities/crack` and `GET /api/opportunities/acquisition` — cross-campaign arbitrage.
- `GET /api/portfolio/weekly-history?weeks=8` — trailing means for hold-verdict checks.
- `GET /api/campaigns/{id}/projections` — only when validating a specific candidate change worth sizing (heavy endpoint).

**Output structure.** Playbook A's value comes from per-campaign verdicts grounded in `/tuning` byGrade — not generic suggestions echoed from the server. The structure below is the minimum signal density that makes the response useful; trimming sections collapses it to a list.

State the **capital posture** once at the top (`Healthy / Tight / Critical` from the guardrail rule), then:

1. **Per-campaign verdict — every active campaign in canonical numeric order.** One of: `RAMP UP / TIGHTEN / HOLD / WIND DOWN / WATCH`. Each verdict carries one sentence of justification citing the metrics that drove it (e.g. `TIGHTEN — PSA 9 at 97.1% of CL on 92 fills, 32% ST, 0.2% ROI`). A `HOLD` must cite the trailing-mean from `/portfolio/weekly-history` per the hold-verdict rule. A campaign whose verdict is HOLD/WATCH still appears in the list — silence is not acceptable.
2. **Top parameter changes ranked by sized $ impact** (with confidence band; hold-verdict rule applied; capital guardrail applied to ramp-ups; **Throttle-lever-selection rule applied to any spend-reduction**). Each backed by `(campaign, grade)` data from `/tuning`, not generic suggestions. State the current value, proposed value, sample size (`n=N`), and projected impact (`Proj: +$X.XK/mo (H|M|L)`). When the proposal is a spend-reduction (cap cut, terms cut, inclusion-list narrow), name **both** cap and terms options with the cap-vs-terms tradeoff per the Throttle lever selection rule — don't pick one silently.
3. **Inclusion-list adds/trims from `/insights.byCharacter`** — characters with `soldCount ≥ 5` AND `roi ≥ 0.20` not yet covered (or undercovered). Per-campaign list of proposed adds with sized expected revenue. Also surface trims for high-`n` low-ROI characters dragging the portfolio (`soldCount ≥ 20` AND `roi < 0.05`). **Run the Era-fit gate** (Recommendation rules) on every proposed add before drafting the line — character year-of-first-release must overlap the campaign's `yearRange`. Filter `/insights.coverageGaps` rows for the open-net false-positive carve-out per the same rule.
4. **Coverage shifts** — niches the portfolio should expand into (`/intelligence/niches` rows with high `opportunity_score` and `current_coverage = 0`) or campaigns that should narrow (`/insights.byPriceTier` drag tiers). Proposed action per row.
5. **Cross-campaign arbitrage** — crack candidates and acquisition mispricings worth > $200 net. Capital-positive, bypass the guardrail.
6. **Stale-suggestion note** — one line: *"Filtered N stale server suggestions (campaigns updated within last 72h)."* (or *"No stale suggestions filtered."*).
7. **Default close — updated campaign list format.** Reproduce all canonical campaigns in the format spec'd below ("Output format: 'updated campaign list'"). This is the **default deliverable** for Playbook A — the operator uses it as a reviewable diff against the strategy doc. Don't replace it with prose, don't replace it with an email draft, don't trim to "only the changed ones." All canonical campaigns, every parameter, `Changed:` annotations or `No change`. Cross-reference each recommendation against the strategy doc's design intent and flag divergences. Approved changes apply via `PUT /api/campaigns/{id}` — see Mutations.
8. **Tail option (opt-in) — *"Want a Brady email draft for these changes?"*** The Brady email is opt-in only. Do **not** auto-draft. The default close is the campaign list at item 7; the email is a follow-on artifact the operator requests if they want one.

**Escalation: revocation.** If a campaign is critically underperforming (negative ROI with >20 observations, or health status "critical"), raise the possibility of revoking it entirely. Fetch `GET /api/portfolio/revocations` to check if any existing flags are pending. To create a new revocation flag: `POST /api/portfolio/revocations` with `{"segmentLabel": "...", "segmentDimension": "...", "reason": "..."}`. Then fetch the generated email via `GET /api/portfolio/revocations/{flagId}/email` for PSA notification. Only suggest revocation when tuning adjustments clearly aren't sufficient — this is a last resort, not a first response to a bad week.

#### Worked example

Synthetic numbers; the goal is to show shape — verdict tier, sized impact, inline rule citations (CL-lead, cap-diagnostic, capital guardrail, popular-tier exclusion, stale-filter), and a closing Sequence block.

> **User:** what updates should we make?
>
> **Assistant:**
> Capital posture: Tight ($42K outstanding, 6.1 weeks to cover, recovery ↗).
>
> **Per-campaign verdicts (canonical order):**
> - Vintage Core (C1): **TIGHTEN** — PSA 9 at 95.2% of CL on 47 fills, 28% ST, 1.1% ROI; CL-lead pattern.
> - Vintage-EX PSA 8 Precision (C2): **HOLD** — ROI 11.3% within ±10% of 11.8% trailing-mean (n=22).
> - EX/e-Reader Era (C3): **RAMP UP** — fill-rate pegged at 96%, ROI 18% on 31 fills (CV 14%).
> - Crystal/HGSS (C7): **WATCH** — fill 22%; cap-diagnostic check below before any move.
>
> **Top changes by sized $ impact:**
> 1. Vintage Core (C1) — narrow PSA 9 inclusion to drop Mid-Era $200–500 tier (n=23 at 0% ROI). Proj: +$1.4K/mo (M). Reason: avgBuyPctOfCL 95% vs 80% contract → CL-lead, narrow scope per Data conventions.
> 2. Crystal/HGSS (C7) — raise daily cap $5K → $8K. Proj: +$2.1K/mo (M). Cap-diagnostic: $5K cap vs ~$2,065 mid-fill, single fill eats 40%+, cap binding on spike days, NOT supply-thin.
> 3. EX/e-Reader Era (C3) — raise buy terms 78% → 80%. Proj: +$0.9K/mo (H). Capital guardrail: tight posture caveat — sizing assumes fill rate holds.
>
> **Inclusion-list adds (byCharacter, soldCount≥5, roi≥0.20, popular-tier excluded):**
> - Add Typhlosion to Vintage Core (n=8, roi 0.34). Add Feraligatr to EX/e-Reader Era (n=6, roi 0.28).
>
> **Stale-suggestion note:** Filtered 2 stale server suggestions (Vintage Core updated 2026-05-04, EX/e-Reader Era updated 2026-05-03).
>
> **Sequence:** (1) C1 inclusion narrow, (2) C7 cap raise, (3) C3 buy-term raise. Apply C1 first to reduce drag before sizing the ramps.
>
> *(Default close — updated campaign list format below — abbreviated here for brevity.)*
>
> Want a Brady email draft for these changes?

#### Output format: "updated campaign list"

This format is the **default close for Playbook A** (item 7 in Output structure). It also fires whenever the operator explicitly asks for an updated campaign list, an updated parameter list, or a summary of the proposed changes — but the default trigger is *every* Playbook A response, not just on explicit ask.

Reproduce **all canonical campaigns** in the format below — not just the ones being changed. For each campaign, show every parameter field and annotate it with either `Changed: <field> <old> → <new>` (one line per change) or `No change`. The operator uses this format as a reviewable diff against the strategy doc.

Use the canonical numbering from the config loaded at Step 0. Pull live field values from `GET /api/campaigns` so the list reflects current state, not the strategy doc's stated intent (they can disagree — that's exactly the signal Playbook D surfaces).

```
Campaign N — <Name>
- Sport: Pokemon
- Years: YYYY–YYYY
- Grade: PSA X–Y
- Price Range (CL Value): $A–$B
- CL Confidence: Z+
- Buy Terms: P%
- Daily Spend Cap: $D
- Inclusion List: <comma-separated card names, or "None (open net)">
- <Changed: Buy terms 80% → 77%   Proj: +$1.1K/mo (H)>   OR   <No change>
```

Every campaign in the canonical list appears in numeric order, even the ones with `No change`. If a field is not yet stored in the API (e.g. InclusionList for a campaign that's pure "open net"), show `None (open net)`. Every `Changed:` line carries the sized projection annotation `Proj: +$X.XK/mo (H|M|L)` — a compact form of the Sizing rule's canonical `est. +$X.XK/mo at current fill (Confidence: H|M|L)`, shortened here because the annotation lives inline on a one-line list item. `No change` lines need no annotation.

### Playbook B — "What should we liquidate to pay our invoice?"

Trigger phrases: *"liquidate", "pay the invoice", "cover invoice", "recover capital", "cash out"*.

Fetch in parallel:
- `GET /api/credit/invoices` — outstanding invoices, due dates, amounts, sell-through data per invoice (the response enriches each invoice with `pendingReceiptCents`, `sellThroughPct`, `soldCount`, `totalCount`)
- `GET /api/credit/summary` — outstanding balance, weeks to cover
- `GET /api/inventory` — global unsold inventory with aging
- `POST /api/sell-sheet` — target price, min price, suggested channel per card (note: this is POST, not GET)
- `GET /api/sell-sheet/items` — current contents of the persistent sell sheet (purchase IDs already queued)
- `GET /api/campaigns/{id}/expected-values` for each campaign with significant unsold capital — EV cents, EV/dollar, sell probability

The approach:

1. Pick the invoice to cover (next-due from `/credit/invoices`, or user-selected). Note the invoice's `pendingReceiptCents` (cards bought but not yet received) and `sellThroughPct` — these contextualize how much of the invoice's inventory is already moving.
2. Compute target recovery = `invoiceAmount × 1.1` for buffer. State this target at the top of the response ("Target: $X.XK to cover $Y.YK invoice + 10% buffer").
3. **Feasibility precondition — before ranking anything.** Filter `/api/inventory` to the in-hand set (items with `receivedAt` set, i.e. received and physically sellable). Sum their `buyCostCents`. If `in-hand × 1.1 < target`, this is a capital crunch — liquidation from on-hand alone can't close the gap. Do NOT pad the list with in-transit cards (they're not sellable yet). Instead, say so and present options: (a) **partial coverage** — best cards from in-hand toward a partial payment, (b) **wait for receipt** — list cards arriving in the next 1–7 days by expected-arrival date, or (c) **invoice timing** — suggest the user negotiate or defer the invoice. Pick the option(s) matching the gap size, don't just pick (a).
4. Exclude any purchase IDs already on the sell sheet (from `/sell-sheet/items`).
5. Rank in-hand candidate cards by (a) shortest expected days-to-sell, (b) highest EV/dollar, (c) best channel fit using the net-proceeds math in *Data conventions*.
6. Walk down the ranked list accumulating projected net proceeds until the running total meets the target (or exhausts in-hand inventory if the crunch precondition fired). Stop as soon as target is met — don't pad.
7. Present as a table: card, cert, recommended channel, recommended price, projected net proceeds, running total. End the table with a summary line: `Selected N cards from in-hand, $X.XK projected recovery vs $Y.YK target (Confidence: H|M|L based on days-to-sell sample)`. If the crunch precondition fired, the summary line reads: `⚠ Partial coverage: $X.XK from N in-hand cards vs $Y.YK target — short $Z.ZK`.

Respect the strategy doc's exit-channel hierarchy. Flag any card recommended into a channel it's gated out of. Liquidation is a capital-positive action — the capital guardrail in the Recommendation rules does NOT apply here.

**Action: add to sell sheet.** When the user approves cards from the table, add them to the persistent sell sheet via `PUT /api/sell-sheet/items` with `{"purchaseIds": ["id1", "id2", ...]}`. This queues them in the web UI's sell sheet view for pricing and listing. Confirm what was added: *"Added 8 cards to the sell sheet ($X,XXX projected recovery). You can review and adjust prices in the Sell Sheet tab."*

### Playbook C — "Should we consider price adjustments on aging inventory?"

Trigger phrases: *"aging inventory", "repricing", "stale listings", "cards that aren't moving", "price drift"*.

Fetch in parallel:
- `GET /api/inventory` — global unsold with days held, market signals, anomaly flags
- `GET /api/campaigns/{id}/expected-values` for each active campaign — AI's current view of where each card should price
- `GET /api/portfolio/channel-velocity` — average days to sell by channel
- `GET /api/admin/price-override-stats` — how AI suggestions are being handled (pending count, acceptance rate)

**AI suggestion triage.** Check `pendingSuggestions` from the stats endpoint. If there are pending AI price suggestions the user hasn't acted on, surface them first — these are pre-computed recommendations waiting for review. For each, the user can accept (`POST /api/purchases/{purchaseId}/accept-ai-suggestion`) or dismiss (`DELETE /api/purchases/{purchaseId}/ai-suggestion`). Present the acceptance rate (`aiAcceptedCount` vs `pendingSuggestions + aiAcceptedCount`) as context for how reliable the suggestions have been.

Then focus on cards where any of the following hold:
- Days held > 2× the channel velocity for their current listing channel
- Current list price has drifted more than 15% from the market signal
- AI-suggested price differs from listed price by more than 10%

Present a table of candidates: card, days held, current list, suggested list, suggested channel, reason, **projected days-to-sell delta** (how much faster the card moves at suggested price vs current), and **projected $ delta** (expected recovery change per card). Each row also carries a confidence band per the Recommendation rules — typically M or L because inventory samples are small. Ask the user which ones to queue for actual price updates. For each approved row, issue `PATCH /api/purchases/{purchaseId}/price-override` — see Mutations.

Repricing is a capital-positive action (faster turn on held inventory) — the capital guardrail does NOT apply.

### Playbook D — "Does the strategy doc still match reality?"

Trigger phrases: *"update the strategy doc", "refine our strategy", "does the strategy still match reality", "what should we change in the doc"*.

This playbook closes the feedback loop. Playbook A proposes changes to live campaign parameters; this one proposes changes to the **strategy document itself** so design intent stays aligned with observed reality.

Fetch in parallel:
- `GET /api/portfolio/health` + `GET /api/portfolio/insights` — current portfolio reality
- `GET /api/campaigns/{id}/tuning` for each active campaign — empirical optimal vs stated buy %, grade mix, price-tier performance
- `GET /api/portfolio/channel-velocity` — actual days-to-sell and channel mix
- `GET /api/credit/summary` — capital recovery rhythm

Re-read `docs/private/CAMPAIGN_STRATEGY.md` with fresh eyes, then walk each section and check whether the live data still supports what's written. For every divergence prepare a proposed edit with:

- **Section:** heading + rough line range
- **Current text:** quoted
- **What the data shows:** specific numbers with sample sizes
- **Proposed text:** concrete replacement wording
- **Confidence:** high / medium / low based on sample size and variance (this rating applies to the proposed doc edit itself, not the Recommendation rules H/M/L bands, which cover live-data recommendations)

Present the proposed edits as a numbered list. Apply only the ones the user approves — never silently edit the doc.

**Warn once per session:** `docs/private/` is not tracked in git (see `/workspace/CLAUDE.md`), so edits to the strategy doc are not versioned. Suggest the user copy the file before a major revision so the prior version is recoverable.

Areas most likely to have drifted (check these first):

1. Per-campaign buy threshold vs empirical optimal from tuning data
2. Exit channel hierarchy vs actual channel velocity and net proceeds
3. Risk triggers and operational cadence (credit-limit language in particular is known to be stale — remove any lingering references)
4. Margin formulas (the eBay fee and exit channel table were restructured; any baked-in formulas predating that are wrong)

### Playbook E — Single-campaign deep dive

Triggered when the user asks *"how's Wildcard doing?"* or similar. Resolve the name against `/api/campaigns` by matching on YearRange, GradeRange, or name substring to get the UUID — campaign IDs are string UUIDs, not integers.

Fetch in parallel for that UUID:
- `GET /api/campaigns/{id}` (detail)
- `GET /api/campaigns/{id}/pnl`
- `GET /api/campaigns/{id}/pnl-by-channel`
- `GET /api/campaigns/{id}/fill-rate`
- `GET /api/campaigns/{id}/inventory`
- `GET /api/campaigns/{id}/tuning`
- `GET /api/campaigns/{id}/days-to-sell`

If the campaign's phase is `pending`, also fetch `GET /api/campaigns/{id}/activation-checklist` — this returns a readiness assessment with pass/fail checks and warnings. Present the checklist results so the user can see what's blocking activation.

Short-circuit any endpoint that returns empty (fill-rate is empty on fresh campaigns). Present:

1. **Identity.** Match to the strategy doc name and section. Restate design intent briefly.
2. **P&L summary.** Total spend, revenue, fees, net profit, ROI, sell-through %.
3. **Channel performance.** Which channels? Are eBay fees eating margin?
4. **Fill rate.** Filling at the expected rate from the strategy doc?
5. **Inventory aging.** How many unsold? Any held > 30 days?
6. **Days-to-sell distribution.** Fast-turning or slow-turning?
7. **Tuning signals.** What do the numbers suggest vs the strategy doc?

Finish with 2-3 targeted follow-up questions.

### Playbook F — "What niches are we missing?" / new campaign design

Trigger phrases: *"what are we missing", "should we add a campaign", "coverage gaps", "new campaign", "campaign 11"*.

Fetch in parallel:
- `GET /api/portfolio/insights` — the `coverageGaps` array identifies profitable segments without active campaigns
- `GET /api/campaigns` — current campaign parameters for overlap check
- `GET /api/portfolio/health` — current portfolio capacity and capital position

For each coverage gap, the response includes a `segment` (with ROI, sell-through, avg days-to-sell from historical data), a `reason` (why it's a gap), and an `opportunity` (suggested action). Present the top gaps ranked by ROI, with caveats on sample size.

**Before proposing anything:** apply the capital guardrail. New campaigns are ramp-up — they deploy more capital. Under "tight" posture, caveat each proposal with the outstanding-balance context. Under "critical" posture, block new-campaign proposals entirely and redirect to defensive moves (liquidate, reduce caps).

For each promising gap (that survives the capital guardrail), sketch what a new campaign would look like:
1. Proposed name and next available canonical number (from the config loaded at Step 0)
2. Year range, grade range, price range based on the segment data
3. Suggested buy terms (reference similar existing campaigns)
4. Expected fill rate and daily spend cap, with sized projection: `est. +$X.XK/mo at proposed cap (Confidence: H|M|L based on segment sample size)`

Cross-reference against the strategy doc to check whether any gaps were intentionally excluded (e.g., sealed product, sports cards). If the user wants to proceed, a campaign can be created via `POST /api/campaigns` — see Mutations.

### Playbook G — "How are our DH listings doing?" / marketplace optimization

Trigger phrases: *"DH status", "DoubleHolo", "marketplace", "listings", "what should we push to DH", "inventory alerts"*.

DH (DoubleHolo) is both the sole price source and a key sales channel. This playbook surfaces marketplace intelligence and listing health.

Fetch in parallel:
- `GET /api/dh/status` — integration health: matched/unmatched/pending/dismissed counts, intelligence and suggestion freshness, API health
- `GET /api/dh/suggestions/inventory-alerts` — DH suggestions that match cards currently in inventory (actionable signals)
- `GET /api/dh/intelligence` (if the user asks about a specific card) — deep market intel: sentiment, forecast, grading ROI, recent sales, population data

Present:
1. **Integration health.** How many cards are matched vs unmatched vs pending push? If `unmatchedCount` is high, suggest running a bulk match.
2. **Inventory alerts.** DH flags cards in your inventory as "hottest cards" (demand spike) or "consider selling" (market signal). Surface these with the reasoning and confidence score. Cards flagged as hot with stale listings are the highest priority — size the opportunity: `est. +$X.XK recovery within N days (Confidence: H|M|L mapped from DH's own confidence score)`.
3. **Push queue.** Cards with `dh_push_status = "pending"` are waiting to be approved for listing. Prioritize by days-queued + projected $ recovery: `N cards, oldest queued D days, est. +$X.XK recovery (Confidence: H|M|L)`. Approve via `POST /api/dh/approve/{purchaseId}` — see Mutations.

When recommending DH as a sales channel (in any playbook), note that eBay listings now flow through DH — there's no separate eBay CSV export. DH handles multi-channel distribution. DH approvals are capital-positive (they turn inventory into sales), so the capital guardrail does NOT apply.

## Step 5 — Strategy doc sync

**This step is non-negotiable.** If any campaign parameters were changed, campaigns were added/removed/paused, or emails were sent to Brady during this session, the strategy document MUST be updated before the session ends. Failing to do this breaks the next session's analysis — the strategy doc is the persistent state that carries across sessions, not memory.

**What to update in `docs/private/CAMPAIGN_STRATEGY.md`:**

1. **Add a "Changes Submitted YYYY-MM-DD" section** (after the most recent changes section, before the Overlap Map) documenting:
   - What was removed/paused
   - What parameters changed on each campaign (old → new, with rationale from this session's analysis)
   - Full campaign blocks for any new campaigns
   - Budget impact table (before/after)
   - Context driving the changes
   - "What to watch next session" bullets

2. **Update the Quick-Copy Campaign Formats section** — every active campaign block must reflect current parameters. Mark removed campaigns under a "Removed Campaigns" subsection.

3. **Update the Campaign Overlap Map** — reflect any changes to which campaigns catch which scenarios, and note new gaps.

4. **Update the Budget Parameters table** at the top of "Current Campaign Structure."

5. **Mark any superseded "Proposed Adjustments" sections** with a status note so future sessions don't treat proposals as current state.

**Verification:** After writing, re-read the Quick-Copy section and confirm each campaign's parameters match what was sent to Brady. A mismatch here is the exact failure mode this step prevents.

## Step 6 — Retrospective

Every `/campaign-analysis` conversation must close with a short retrospective block — at the end of a session or when the user signals they're wrapping up ("thanks, that's it for tonight", "ok let's apply these"). Don't write it on every turn; once per session.

**What to capture.** Four buckets, each 1-3 bullets. Skip any bucket that's empty, don't invent filler:

1. **Data gaps encountered** — endpoints that returned empty (`intelligence_count: 0`, niches empty, signals empty, etc.), fields missing from responses, hypotheses we couldn't test from current data.
2. **DH-side asks** — things we believe DH should be populating but isn't, or new intelligence fields we'd want. These feed the next dated `docs/private/YYYY-MM-DD-dh-data-ask.md`.
3. **Client-side work that would unblock future analyses** — new aggregations, new heuristics, migrations, etc.
4. **Lessons about the operator's edge / thesis** — anything learned this session about how they think, what moves they respect, what corrections they pushed back on. This is the most valuable bucket and the easiest to skip — don't skip it.

**Where it goes.** Append to `docs/private/campaign-analysis-wishlist.md` in the **Retrospective log** section at the bottom. One bullet per session, dated. Also append any new items to the relevant tables above it (external data gaps / internal work / hypotheses). If an item already exists, don't duplicate — add a date-stamped "reconfirmed" note to the existing row instead.

**Show the user what you're appending** — a brief recap of the retrospective bullets in the response, so they can correct or amend before it lands in the file. Then write the file.

**One-artefact-per-session discipline.** Drafts of messages to partners (DH, PSA, LGS) go into dated files: `docs/private/YYYY-MM-DD-<partner>-data-ask.md` (or similar). Don't overwrite a previous day's file — keep the trail linear so we can track what was sent, what was answered, and what's still open.

## Recommendation rules

These rules are referenced by every playbook that emits a recommendation. Keeping them here avoids drift between playbooks.

### Sizing

Every parameter-change, new-campaign, or material tuning recommendation carries a projected $ impact, time horizon, and confidence band — uniformly. Format: `est. +$X.XK/mo at current fill (Confidence: H|M|L)`. When the projections endpoint can't produce a clean counterfactual (sparse data, wide variance), say so explicitly: `est. +$X/mo (Low confidence, N obs)`. For one-time-recovery actions (DH push approval, sell-sheet liquidation batch), replace `/mo at current fill` with `recovery`: `est. +$X.XK recovery (Confidence: H|M|L)` — the `recovery` variant marks a non-recurring event, not ongoing monthly income. Confidence band always sits inside parentheses, regardless of variant. Never drop the number silently — that makes recommendations impossible to prioritize.

**Projections time unit.** The `/api/campaigns/{id}/projections` response returns `medianProfitCents` as the projected median profit **per one Monte Carlo purchase cycle** (not per month). Convert to a monthly figure only when you have a defensible cycle-to-month mapping from the tuning data (e.g. campaign fills ~weekly → multiply by 4). When the mapping is unclear, report `est. +$X.XK per projection cycle` and note the cycle semantics; don't pretend `/mo`. The same `medianProfitCents` also appears in `p10ProfitCents` / `p90ProfitCents` — use the spread to inform the confidence band (wide p10↔p90 relative to median → Low).

**Projections endpoint quirks.** Three real responses to handle:
- **HTTP 422 with `{error: "insufficient_data", minRequired: 10, available: N}`** — the campaign has fewer than 10 completed sales, so projections aren't meaningful. This is the semantic "not enough data" signal. Do NOT emit a sized recommendation; instead say plainly: *"Projections unavailable: N/10 completed sales on Wildcard — run more before projections are meaningful. Falling back to tuning-endpoint grade-level review."* Then use `/api/campaigns/{id}/tuning` for whatever grade-level signal exists.
- `confidence: null` on every scenario — the field is often null on 200 responses. Treat as unmapped and fall back to the tuning endpoint's obs-count bands (per the Confidence bands rule).
- All-zero scenarios (every scenario has `medianROI: 0`, `medianProfitCents: 0`, `medianVolume: 0`) — Monte Carlo didn't converge even though the sample cleared the 422 threshold. Do NOT emit a sized recommendation; instead surface a Low-confidence hold-adjacent note: "projections couldn't converge on Wildcard (thin sample) — recommend manual grade-level review via `/api/campaigns/{id}/tuning` before parameter changes." This is a distinct failure mode from the hold verdict rule — the rule fires on weak signal; this fires on unusable signal.

### Stale-suggestion filter

`/api/portfolio/suggestions` recommendations are computed against currently-stored campaign params. Before surfacing any suggestion in the opener or Playbook A, check `campaigns[i].updatedAt` from `/api/campaigns`. If the targeted campaign was updated within the last 72 hours AND the suggestion targets a changed field (buy terms, daily cap, grade range, CL confidence, inclusion list), the suggestion is stale — drop it from the top-3.

State the filter outcome once per response: *"Filtered N stale server suggestions (campaigns updated within last 72h)."* If nothing was filtered, the line is *"No stale suggestions filtered."* — silence makes it impossible to tell whether the filter ran.

This rule exists because the highest-leverage tuning changes (drop terms, raise CL confidence) are the exact ones the operator most often makes manually between sessions, so the server's pre-computed adjustments are most likely to be stale precisely when they sound most authoritative.

### Confidence bands

| Band | Rule |
|------|------|
| **High (H)** | ≥30 observations AND coefficient of variation < 20% |
| **Medium (M)** | 10–29 observations, OR ≥30 observations with CV ≥ 20% |
| **Low (L)** | < 10 observations OR < 4 weeks of history |

Coefficient of variation = `stddev / mean` of the metric driving the recommendation (ROI, sell-through, or days-to-sell — whichever the recommendation is predicated on). The tuning endpoint returns `roiStddev` and `cv` on grade- and price-tier performance rows; use them directly. The `/api/campaigns/{id}/projections` and `/api/portfolio/suggestions` responses also return a `confidence` string ("low"/"medium"/"high") — treat that as a **hint**, not authoritative. The skill's obs-count rule above wins on disagreement: a server-labeled "medium" with 8 observations is Low per the table, not Medium. State the rule-applied band, not the server label. When multiple bands match (e.g. obs-count qualifies for Medium but history-length qualifies for Low), use the lower confidence band.

### "Hold" verdict rule

When signal is weak, recommend holding — explicitly — instead of synthesizing a change. A hold fires when *any* of the following is true for the metric driving the recommendation:

- Week-over-week delta is within the noise band. Use the rule-of-thumb: WoW delta within ±10% of the campaign's trailing-4-week mean. Pull the trailing-mean from `GET /api/portfolio/weekly-history?weeks=N` (default 8, max 52; the `weeks` param must be a positive integer or the endpoint returns 400). The response is a JSON array of `WeeklyReviewSummary` objects, newest first — average the four most recent entries to form the trailing-4-week mean. A σ-based check is also viable from this series when the user wants one.
- Proposed parameter change magnitude is < 3 percentage points AND confidence is Medium or Low.
- Sell-through drop is < 5pp AND observation count is < 20.

Say it out loud in the rule-of-thumb form: *"Hold — this week's ROI is 7%, within ±10% of the 8.2% trailing-mean. Noise, not signal. I'd keep current params."* Silence is not acceptable; the user learns *why* nothing is being changed.

### Fill-drought hypothesis ranking

When a campaign or segment goes dark — fill rate dropped >25% WoW, sales stalled for 2+ consecutive weeks, or a previously-active segment shows zero recent fills — do **not** list hypotheses as equal-weight alternatives. Walk the four canonical hypotheses below, score each by the evidence present, and present them in ranked order with one-line reasoning per rank.

The four canonical hypotheses, each with the evidence that favors it:

1. **Competition.** Favored when (a) DH or CL data shows the segment is contested, (b) recent CL trend on the segment is upward (others bidding up the anchor), (c) the segment is in the popular-tier or a known-contested niche, or (d) the operator's recent realized `avgBuyPctOfCL` on similar segments is climbing toward 100%.
2. **Supply lull.** Favored when (a) similar segments across the operator's portfolio are also slow (same era, same grade, same character family), (b) PSA submission cycles or set-release timing predict the dip, or (c) intelligence endpoints show flat or declining population growth on the segment.
3. **Cycle dip / submission shift.** Favored when (a) overall PSA throughput slowed (operator-wide signal across multiple campaigns), (b) the same campaign's prior years show the same dip in the same week-of-year, or (c) a known holiday / PSA event window is in play.
4. **Inclusion-list mismatch.** Favored when (a) the Step 1a inclusion-diff is nonempty, (b) recent fills include characters not on inclusion list (recent inclusion-list edit retroactively excluded them), or (c) the segment was recently restricted (grade range tightened, characters removed).

**Output shape.** State the top hypothesis first with its supporting evidence, the second-likeliest with its evidence and why it's lower-ranked, and a one-line "if it's the top one we'd see X next; if it's the second we'd see Y next" — making the next diagnostic step explicit. Don't list all four if the evidence cleanly points at one or two; rank what the evidence supports.

**Evidence-poor case.** If no signal meaningfully separates the hypotheses, say so explicitly — *"competition vs supply lull both fit the data; the discriminator would be [X — e.g. checking DH velocity on the segment, or comparing prior-year same-week fills]; can you check, or would you like me to dig into [X]?"* — but never default to a flat menu when evidence supports a ranking. Equal-weight presentation is the exception, not the default.

This rule was added because the skill defaulted to an equal-weight menu of competition / submission shift / cycle dip on the 5/4 Modern drought question; the operator did the ranking. The skill must move the conversation forward, not leave the operator to disambiguate.

### Capital guardrail

Checked before emitting any **ramp-up** recommendation — actions that deploy more capital. Ramp-ups include: raise buy terms, raise daily spend cap, propose a new campaign, expand an inclusion list. DH push approvals are excluded (they move existing inventory, not new spend); liquidation actions are excluded (they recover capital, not deploy it).

| Posture | Rule | Effect |
|---------|------|--------|
| Healthy | `weeksToCover ≤ 5` AND `recoveryTrend != "worsening"` AND `alertLevel != "critical"` | No caveat; proceed. |
| Tight | `weeksToCover > 5` OR `recoveryTrend == "worsening"` | Caveat: *"capital posture is tight: $X outstanding, N.N weeks to cover, trend ↗ — sizing the downside if fill rate under-performs"*. |
| Critical | `alertLevel == "critical"` | Block the ramp-up. Recommend defensive posture (liquidate, reduce daily cap, pause aggressive DH) instead. |

Data sources: `GET /api/portfolio/health`, `GET /api/credit/summary` (already fetched in Step 3).

### Sequencing

When two or more recommendations interact — a liquidation target sits in a campaign also getting a buy-term change, a DH push conflicts with a pending price override, a daily-cap change affects a campaign also proposed for ramp-up — end the response with a short numbered Sequence block explaining the order. Example:

> **Sequence:** (1) apply the Campaign 7 buy-term drop first — no point liquidating at current terms and then lowering. (2) Queue the Wildcard sell-sheet adds. (3) DH approvals last.

For independent recommendations, skip the Sequence block; the numbered list in the opener is enough.

### Popular-tier character exclusion

The operator's edge is on characters competing buyers *aren't* chasing. Never default-recommend adding any of these characters to a campaign inclusion list, regardless of how strong the character's segment ROI looks in the aggregate data:

**Popular tier (do not default-recommend):** Charizard, Pikachu, Blastoise, Venusaur, Mewtwo, Mew, Umbreon, Eevee, Lugia, Ho-Oh, Gengar, Rayquaza.

The assumption is these are already contested, already in your lists where they belong, and adding them elsewhere just bids against yourself and other Partner Offers buyers.

**Narrow-pocket exception.** A specific `(character, grade, era)` combination from the popular tier IS recommendable if `/insights.byCharacterGrade` shows it matches the CL-lag pattern (`avgBuyPctOfCL ≤ 0.80 AND roi ≥ 0.20 AND soldCount ≥ 3`). Examples that qualify: "Gengar PSA 6" (not Gengar in general), "Mew PSA 8 vintage" (not Mew in general). State the grade + era explicitly so the add is narrow, not broad.

**Positive tier to mine for edge:** 2nd-tier vintage/mid-era Pokemon the operator has explicitly flagged (Absol, Typhlosion, Feraligatr, Meganium) plus the broader "Other" character bucket from `byCharacter` — that bucket held 200 fills and 10% ROI on average in one sampled session, meaning the uncaptured long tail has signal.

### Era-fit gate (inclusion-list adds)

Before recommending OR executing any character add to a campaign's inclusion list, verify the character had TCG cards released within the campaign's `yearRange`. Applies to manual recommendations, echoed `/snapshot.suggestions` "Add top performers" entries, and `/insights.coverageGaps` rows. Era fit is a mandatory **pre-check**, not a post-recommendation sanity check — `/snapshot.suggestions` and `/insights.coverageGaps` sort by portfolio-wide ROI without filtering by era.

**Pokemon TCG generation reference (first-card year):**

| Generation | Era / Set      | First-card year |
|------------|----------------|-----------------|
| Gen 1      | Base Set       | 1999            |
| Gen 2      | Neo            | 2000–2002       |
| Gen 3      | EX-era         | 2003–2007       |
| Gen 4      | Diamond/Pearl  | 2007            |
| Gen 5      | Black/White    | 2011            |
| Gen 6      | XY             | 2014            |
| Gen 7      | Sun/Moon       | 2017+           |
| Gen 8      | Sword/Shield   | 2020+           |
| Gen 9      | Scarlet/Violet | 2023+           |

**Open-net false-positive carve-out.** `/insights.coverageGaps` reasons of "not in any active campaign inclusion list" are misleading for open-net campaigns (no inclusion list). Open net catches every character within other parameters (year, grade, price, confidence) and does not appear in inclusion-list coverage analysis. Before treating a character as uncovered, verify against open-net campaign scope. If the character is already filling on an open-net campaign with positive ROI, the "coverage gap" is a false positive.

**When the data signal seems era-mismatched** (e.g., a "vintage" coverage gap for a Gen 4+ character), trace actual fills via `GET /api/inventory` filtered to the character — check `cardYear` / `setName` to identify which era is producing the demand. Add the character to the era-matching campaign, not the campaign whose name happened to surface the signal.

This rule was added because the skill recommended adding Leafeon (first card 2007) and Rayquaza (first card 2003) to Vintage Core (C1, 1999–2003) and Vintage-EX PSA 8 Precision (C11, 1999–2007) based on portfolio-wide ROI from `/snapshot.suggestions` and `/insights.coverageGaps`. PUTs were applied before the user caught the era mismatch — both characters' actual high-ROI fills were modern alt-arts already caught by C4 / C10's open-net scope.

### Sub-$150 modern floor

Never recommend lowering floors or adding character pockets that would capture sub-$150 *modern* (2016+) supply. The combination of $3 flat PSA sourcing fee (2%+ of cost at that tier) and high price volatility on modern alt-arts makes this the structural loss zone. Sub-$150 vintage / mid-era / EX-era does NOT carry the same penalty — different price dynamics, lower volatility, same $3 fee but distributed over cleaner margin.

### Turnover gate

Any ramp recommendation must carry an expected days-to-sell ≤ 30 based on the segment's historical `avgDaysToSell` (or `bestChannel`'s days-to-sell if segment data is thin). If the segment's avgDaysToSell > 30, the recommendation becomes a patient-capital play — surface it only with an explicit caveat: *"Ramp candidate but avgDaysToSell is X days — requires cash to sit. Skip unless you have reserves."* Operator has thin cash reserves and worst-case-LGS@70% is the liquidation floor — ramp must turn.

### Cap-diagnostic rule

Before interpreting a campaign's multi-day cap utilization %, check whether the daily cap can fit multiple fills of the campaign's expected per-fill cost. **Low cap utilization on a high-per-fill-cost campaign ≠ "supply is thin."** It usually means "a single fill eats most of the cap, and spike-day second fills are getting lost to cap exhaustion."

The check:

1. Compute **expected per-fill buy cost** from the campaign's price range: `midpoint(priceRange) × buyTerms + $3 sourcing fee`. E.g., a $500-$5000 campaign at 75% has expected per-fill cost ~$2,065 mid-range, and top-of-range fills at $3,753.
2. Compare to daily cap:
   - If `cap ≥ 3 × expected per-fill cost`: cap has room for multi-fill days — low utilization genuinely indicates thin supply.
   - If `cap < 3 × expected per-fill cost`: cap is binding on spike days regardless of multi-day utilization. Recommend cap raise *before* concluding the campaign is supply-constrained.
3. Flag explicitly in the recommendation: *"C7 cap is $5,000 against expected per-fill cost of ~$2,065 (mid-range) — single fill of a top-of-range Crystal card consumes 75%+ of cap. Multi-day 20% utilization is consistent with 'cap eats one fill' rather than 'supply is thin.'"*

This rule was added because the skill initially recommended lowering C7's floor based on 20% multi-day utilization. The operator corrected: Crystal cards land $3K-$7K, so the $5K cap was the bottleneck, not the floor. The diagnostic check above would have caught this.

**Cap-cut binding check (inverse direction).** Before proposing a cap *reduction* on any campaign, verify the cap actually binds on observed daily spend. The forward analog of the supply-thinness check above — both directions require checking how the cap binds on observed spend before the cap becomes a lever.

Compute from `/campaigns/{id}/fill-rate`:

- `excess = sum(max(0, spendUSD - proposedNewCap))` across the observed window.
- `daysExceeded = count(days where spendUSD > proposedNewCap)`.

**If `excess < $500` over a 14-day window OR `daysExceeded < 25%` of observed days, the cap reduction is a no-op.** Skip it or pick a binding lever (price-floor raise, terms cut, revocation, pause).

**State the math inline** when surfacing or rejecting a cap-cut:

> *"C8 Gold Stars cap $8K → $5K: 0 of 4 observed days exceeded $5K, max ever $1,374. $0 saved — no-op. Skipping."*

This rule was added because the skill proposed cap reductions on Mid-Era (C6, $5K → $2K) and Gold Stars (C8, $8K → $5K) as part of a throttle plan. Actual binding analysis: C6 had 3 of 15 days over $2K (~$0–$1K saved over a 4-day pro-rated window); C8 had 0 of 4 days over $5K (the 4/30 raise to $8K had never bound — $0 saved). Only C10 (14 of 18 days exceeding $2K, ~$3K saved) was a real cap-cut candidate. The skill should have run the binding check before proposing C6 / C8 cuts.

### Throttle lever selection

When a recommendation reduces spending on a campaign — whether for capital pressure, invoice-cycle tightening, or pre-emptive throttle — present **both** cap reduction and buy-terms reduction as peer levers, with the tradeoff stated explicitly. Don't pick one silently.

| Lever | Effect | When it's the right choice |
|-------|--------|----------------------------|
| **Cap reduction** | Clips spike-day exposure. Margin per fill unchanged; typical-day fills unchanged; only spike-day fills are dropped. Run the Cap-cut binding check (Cap-diagnostic rule, inverse direction) before proposing a number — a cap that doesn't bind on observed spend saves $0. | Goal is risk control or smoothing the cash burn curve. Healthy campaign you want to keep filling, just not blow up the cap on a bad day. |
| **Terms reduction** | Shifts the entire fill distribution: reduces fill rate every day AND improves margin on residual fills. | Goal is intentional volume-kill — *not* margin recovery on a filling segment. |

Default presentation: name both levers, frame the choice as risk-control (cap) vs distribution-shift (terms), and let the operator pick. Especially in invoice-cycle / capital-tightening contexts, the operator may have a directional preference (e.g. terms cut on a CL-lead segment) that the skill won't surface if it picks cap silently.

**Pair with CL-lag/CL-lead discipline.** Terms cuts are appropriate as a *volume-kill* lever, not as a *margin-recovery* lever on a filling segment. A CL-lead segment (per Data conventions thresholds — `avgBuyPctOfCL ≥ 0.93 AND roi ≤ 0.05`) is a "narrow scope (year/price/confidence)" candidate, not a terms-cut candidate. The terms-as-volume-kill case is distinct from margin recovery — keep them separate in the rationale.

This rule was added because the skill defaulted to cap reduction when the operator wanted terms reduction on a 99%-BPCL-vs-75%-contract campaign during invoice-cycle week 2. Cap would have left the realized BPCL gap untouched; terms shifts the distribution down so fewer high-BPCL fills land at all.

### Partner-ask verification

Before drafting a data-ask to a third party (DH, PSA, CardLadder, etc.) based on "this endpoint returns 0" or "this field is empty," **verify the gap isn't on our side first.** An empty response usually has three possible causes, and only one of them is a partner gap:

1. **Partner doesn't have the data** — legitimate partner ask.
2. **Partner has it, but our scheduler / seed / trigger isn't pulling it** — local bug.
3. **Partner has it and we pulled it, but it's stored in a different field / table than we're reading** — local query bug.

Ask three questions before filing a partner-ask:

- Is there a scheduler or job that should be populating this field? If so, has it run recently? Check via logs, API health endpoints, or manually running the job.
- Is the data already in a related table or field that we're not surfacing? (E.g., PSA pop lives in `market_intelligence.population` even though `dh/status` shows `intelligence_count: 0`.)
- Does the partner's documented API already return this? If yes, the gap is our pull, not their provision.

Only items that clear all three checks go into the dated `docs/private/YYYY-MM-DD-<partner>-data-ask.md` draft. Items that fail any of them go into the internal-work table of the wishlist as a local-side fix.

This rule was added because an early DH draft included "intelligence_count: 0" and "no pop data" as DH asks. Both turned out to be local seeding bugs — the scheduler only refreshed existing rows and nothing seeded the table. Operator caught it and corrected the draft before sending.

## Data conventions

- **All monetary values are in cents.** Divide by 100 and format as `$X,XXX.XX`.
- **Buy terms** are decimals (`0.80` = 80% of CL value). These are contract terms PSA fills against at purchase moment.
- **`avgBuyPctOfCL` is NOT your buy terms.** The tuning endpoint's `avgBuyPctOfCL` (and the equivalent field on `/insights` segments) is **realized cost ÷ current CL value** — a post-purchase ratio that includes CL drift since fill. When it's materially different from your contract terms, you're seeing CL move, not terms change. Never phrase this as "you're buying at X% of CL" — say "realized cost is X% of current CL" or "CL has drifted Y pp since purchase." Confusing these two was a real, documented failure mode of this skill. The field is also a **mean of per-card ratios, not dollar-weighted** — a few high-ratio outliers (Japanese variants with CL mismatches, post-purchase CL crashes) can inflate it by 10–25 points. Before citing `avgBuyPctOfCL ≥ 0.90` as a headline driver, run the dollar-weighted cross-check from `/api/inventory` per the API footguns block in `SKILL.md`.
- **CL-lag vs. CL-lead framing.** The same `avgBuyPctOfCL` field tells two different stories depending on direction:
  - **CL-lag (edge captured):** `avgBuyPctOfCL < contract terms` → CL drifted *up* after purchase → you bought before CL caught up to market. Surface segments with `avgBuyPctOfCL ≤ 0.80 AND roi ≥ 0.20` as patterns to replicate.
  - **CL-lead (edge lost):** `avgBuyPctOfCL > contract terms` → CL drifted *down* after purchase → CL was above market, now correcting. Surface segments with `avgBuyPctOfCL ≥ 0.93 AND roi ≤ 0.05` as segments to narrow scope in (year, price, confidence), **not** as terms-cut candidates. Terms cuts reduce fill rate without fixing the root cause, which is CL unreliability on that segment.
- **ROI** is a decimal ratio (`0.08` = 8%).
- **Capital summary fields:** `outstandingCents`, `weeksToCover`, `recoveryTrend`, `alertLevel`. Operator-specific framing (what's "healthy", labels for alert levels) lives in the config file loaded at Step 0.
- **~1 week delay** between a PSA purchase being consummated and the card arriving. Campaigns with < 2 weeks of history and 0% sell-through aren't necessarily underperforming — the cards may not be in hand yet.
- **Canonical campaign numbering** is in the config file loaded at Step 0. Map names to API UUIDs via the name / year range / grade range fields on the campaign detail.

### Exit channels

| Channel | Sell price (% of market) | Fee | Availability |
|---------|--------------------------|-----|--------------|
| eBay (via DH) | 100% | 12.35% | Always — eBay listings flow through DoubleHolo, not direct CSV export |
| Shopify | 100% | ~2% | Always, but lower traffic than eBay |
| Card show | 80–90% | 0% | Not daily — only when a show is scheduled |
| LGS (local game store) | 70–80% | 0% | Varies by shop; liquidation backstop |

Net proceeds math when ranking channels for a liquidation or repricing recommendation:

- **eBay:** `market × 0.8765 − $3` (12.35% fee + listing/shipping friction)
- **Shopify:** `market × 0.98`
- **Card show:** `market × 0.85` (midpoint of 80-90%; only include when a show is actually upcoming)
- **LGS:** `market × 0.75` (midpoint of 70-80%)

Channel selection hierarchy when recommending liquidation:

1. Shopify first if the card has traffic signal or the user wants the clean ~98% recovery.
2. eBay for anything else that needs to move reliably at high volume.
3. Card show for high-value cards when a show falls inside the liquidation window.
4. LGS as the speed option — instant cash at 70-80%, use when recovery speed beats recovery percentage (e.g. covering an imminent invoice).

## Mutations

Every write endpoint the playbooks reach for, in one place. Use the purchase UUID (`id` field), not the cert number, for all purchase-level operations.

| Intent | Verb + path | Body | Playbook |
|--------|-------------|------|----------|
| Reassign purchase to a different campaign | `PATCH /api/purchases/{id}/campaign` | `{"campaignId":"..."}` | ad-hoc |
| Fix a missing or wrong buy cost | `PATCH /api/purchases/{id}/buy-cost` | `{"buyCostCents":18699}` | ad-hoc |
| Override a sale/list price | `PATCH /api/purchases/{id}/price-override` | `{"priceCents":..., "source":"manual"}` | C |
| Accept a pending AI price suggestion | `POST /api/purchases/{id}/accept-ai-suggestion` | — | C |
| Dismiss a pending AI price suggestion | `DELETE /api/purchases/{id}/ai-suggestion` | — | C |
| Queue cards on the persistent sell sheet | `PUT /api/sell-sheet/items` | `{"purchaseIds": ["id1", ...]}` | B |
| Approve a DH push for a purchase | `POST /api/dh/approve/{id}` | — | G |
| Raise a revocation flag | `POST /api/portfolio/revocations` | `{"segmentLabel":"...", "segmentDimension":"...", "reason":"..."}` | A |
| Generate the revocation notification email | `GET /api/portfolio/revocations/{flagId}/email` | — | A |
| Update campaign parameters | `PUT /api/campaigns/{id}` | Campaign fields | A |
| Create a new campaign | `POST /api/campaigns` | Campaign fields | F |

Never apply any of these silently — present the proposed change to the user and only fire the mutation after approval.
