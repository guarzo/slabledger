---
name: campaign-analysis
description: Analyze campaign performance — portfolio health, P&L, sell-through, aging inventory, liquidation planning, tuning recommendations, capital position, DH marketplace optimization, coverage gaps, and new campaign design. Use whenever the user asks about campaign status, which cards to liquidate, whether to adjust parameters, aging inventory, invoice coverage, strategy doc refinement, DoubleHolo listings or intelligence, what niches to expand into, AI price suggestions, or any follow-up about Pokemon card campaigns — even if they don't say "campaign-analysis" explicitly.
argument-hint: "[optional: health | weekly | tuning | campaign <id-or-name> | gaps | dh]"
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Edit"]
---

# Campaign Analysis

## Step 0 — Load operator configuration

Read `docs/private/campaign-analysis-config.md`. This file contains:
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

## Step 3 — Fetch the initial snapshot (default entry point)

Fetch these in parallel:

- `GET /api/campaigns` — for name ↔ UUID resolution; filter out archived
- `GET /api/portfolio/health` — per-campaign status, reason, capital at risk
- `GET /api/portfolio/weekly-review` — week-over-week deltas
- `GET /api/portfolio/insights` — cross-campaign segmentation by character, grade, era, tier
- `GET /api/credit/summary` — outstanding balance, weeks to cover, recovery trend
- `GET /api/credit/invoices` — next invoice date and amount (matters for the liquidation playbook)

Present a short conversational opening:

1. **Most actionable finding first.** One sentence: what needs attention right now?
2. **Per-campaign health** with specific dollar figures: status, ROI, sell-through %, unsold count, capital at risk.
3. **Week-over-week deltas** with specific numbers, e.g. *"Purchases up 20% (47 vs 39), but profit down 5%"*.
4. **Capital position** framed as *outstanding balance / weeks to cover / recovery trend*. Do NOT frame this as distance to any credit limit — PSA credit limits are no longer a constraint.
5. **Next invoice** one sentence: amount and due date.
6. **Open question.** Always end with: *"What would you like to dig into — Wildcard, liquidation for the invoice, parameter updates, or something else?"*

Keep it concise. The goal is to prompt a follow-up, not dump a report.

#### Example opening turn

> **User:** /campaign-analysis
>
> **Assistant:** Quick read on the active campaigns. [Most performing campaign] is the standout — [ROI]% ROI, [sell-through]% sell-through, [N] unsold ($X,XXX at risk). [Underperforming campaign] is dragging — [ROI]% ROI, [sell-through]% sell-through after [N] days. Outstanding balance is $XX.XK with N.N weeks to cover at current velocity. Next invoice is $X.XK due [date]. Want to dig into [campaign name], talk through liquidation options for the invoice, or review parameter updates?

## Step 4 — Follow-up playbooks

Route each user follow-up to the matching playbook below. The `references/advisor-tools.md` file catalogs the server-side AI advisor tools; the advisor endpoints (`POST /api/advisor/digest`, `POST /api/advisor/liquidation-analysis`, `POST /api/advisor/campaign-analysis`) run an LLM loop over those tools and can be used as a sanity check against playbook output when time permits.

### Playbook A — "What campaign updates should we make?"

Trigger phrases: *"what updates should we make", "campaign tuning", "parameter adjustments", "should we change buy terms"*.

Fetch in parallel:
- `GET /api/campaigns/{id}/tuning` for each active campaign — grade-level ROI, price-tier performance, buy threshold analysis
- `GET /api/portfolio/suggestions` — server-side data-driven suggestions
- `GET /api/campaigns/{id}/projections` — Monte Carlo against alternative parameters (only fetch if tuning flags a specific change worth sizing)

Present per campaign:

1. Which grades or price tiers are dragging ROI (with data-point counts — caveat anything with <10 observations).
2. What the empirical optimal buy % looks like vs the current term.
3. Specific parameter change recommendations with confidence level.
4. Cross-reference each recommendation against the strategy doc's design intent — flag divergences.
5. A prioritized list of proposed edits. If the user approves any, we can apply them via `PUT /api/campaigns/{id}`.

**Escalation: revocation.** If a campaign is critically underperforming (negative ROI with >20 observations, or health status "critical"), raise the possibility of revoking it entirely. Fetch `GET /api/portfolio/revocations` to check if any existing flags are pending. To create a new revocation flag: `POST /api/portfolio/revocations` with `{"segmentLabel": "...", "segmentDimension": "...", "reason": "..."}`. Then fetch the generated email via `GET /api/portfolio/revocations/{flagId}/email` for PSA notification. Only suggest revocation when tuning adjustments clearly aren't sufficient — this is a last resort, not a first response to a bad week.

#### Output format: "updated campaign list"

When the user asks for an **updated campaign list** (or an updated parameter list, or a summary of the proposed changes), reproduce **all canonical campaigns** in the format below — not just the ones being changed. For each campaign, show every parameter field and annotate it with either `Changed: <field> <old> → <new>` (one line per change) or `No change`. The user uses this format as a reviewable diff against the strategy doc.

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
- <Changed: Buy terms 80% → 77%>   OR   <No change>
```

Every campaign in the canonical list appears in numeric order, even the ones with `No change`. If a field is not yet stored in the API (e.g. InclusionList for a campaign that's pure "open net"), show `None (open net)`.

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
2. Compute target recovery = `invoiceAmount × 1.1` for buffer.
3. Exclude any purchase IDs already on the sell sheet (from `/sell-sheet/items`).
4. Rank candidate cards by (a) shortest expected days-to-sell, (b) highest EV/dollar, (c) best channel fit using the net-proceeds math in *Data conventions*.
5. Walk down the ranked list accumulating projected net proceeds until the running total meets the target.
6. Present as a table: card, cert, recommended channel, recommended price, projected net proceeds, running total.

Respect the strategy doc's exit-channel hierarchy. Flag any card recommended into a channel it's gated out of.

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

Present a table of candidates: card, days held, current list, suggested list, suggested channel, reason. Ask the user which ones to queue for actual price updates. For each approved row, issue `PATCH /api/purchases/{purchaseId}/price-override` with `{"priceCents": ..., "source": "manual"}`.

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
- **Confidence:** high / medium / low based on sample size and variance

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

For each promising gap, sketch what a new campaign would look like:
1. Proposed name and next available canonical number (from the config loaded at Step 0)
2. Year range, grade range, price range based on the segment data
3. Suggested buy terms (reference similar existing campaigns)
4. Expected fill rate and daily spend cap

Cross-reference against the strategy doc to check whether any gaps were intentionally excluded (e.g., sealed product, sports cards). If the user wants to proceed, a campaign can be created via `POST /api/campaigns`.

### Playbook G — "How are our DH listings doing?" / marketplace optimization

Trigger phrases: *"DH status", "DoubleHolo", "marketplace", "listings", "what should we push to DH", "inventory alerts"*.

DH (DoubleHolo) is both the sole price source and a key sales channel. This playbook surfaces marketplace intelligence and listing health.

Fetch in parallel:
- `GET /api/dh/status` — integration health: matched/unmatched/pending/dismissed counts, intelligence and suggestion freshness, API health
- `GET /api/dh/suggestions/inventory-alerts` — DH suggestions that match cards currently in inventory (actionable signals)
- `GET /api/dh/intelligence` (if the user asks about a specific card) — deep market intel: sentiment, forecast, grading ROI, recent sales, population data

Present:
1. **Integration health.** How many cards are matched vs unmatched vs pending push? If `unmatchedCount` is high, suggest running a bulk match.
2. **Inventory alerts.** DH flags cards in your inventory as "hottest cards" (demand spike) or "consider selling" (market signal). Surface these with the reasoning and confidence score. Cards flagged as hot with stale listings are the highest priority.
3. **Push queue.** Cards with `dh_push_status = "pending"` are waiting to be approved for listing. If any are held up, the user can approve them via `POST /api/dh/approve/{purchaseId}`.

When recommending DH as a sales channel (in any playbook), note that eBay listings now flow through DH — there's no separate eBay CSV export. DH handles multi-channel distribution.

## Conversational guidelines

1. Lead with the most actionable finding, then details. Be direct about what's not working — don't hedge.
2. Use specific dollar amounts and percentages, rounded to sensible precision. Caveat anything with < 10 observations so the reader knows when a number is noisy.
3. Cross-reference findings against the strategy doc. When checking for campaign mismatches, compare the purchase era, grade, character, and price against the campaign's parameters from the doc.
4. End every response with a question that invites the user deeper.
5. Flag risks proactively — slow inventory, duplicate accumulations, $0 buy costs, cards gated out of their suggested channel.
6. Keep it conversational. Natural language, not bullet-heavy reports.

## Recommendation rules

These rules are referenced by every playbook that emits a recommendation. Keeping them here avoids drift between playbooks.

### Sizing

Every parameter-change, new-campaign, or material tuning recommendation carries a projected $ impact, time horizon, and confidence band — uniformly. Format: `est. +$X.XK/mo at current fill (Confidence: H|M|L)`. When the projections endpoint can't produce a clean counterfactual (sparse data, wide variance), say so explicitly: `est. +$X/mo (Low confidence, N obs)`. Never drop the number silently — that makes recommendations impossible to prioritize.

### Confidence bands

| Band | Rule |
|------|------|
| **High (H)** | ≥30 observations AND coefficient of variation < 20% |
| **Medium (M)** | 10–29 observations, OR ≥30 observations with CV ≥ 20% |
| **Low (L)** | < 10 observations OR < 4 weeks of history |

Coefficient of variation = `stddev / mean` of the metric driving the recommendation (ROI, sell-through, or days-to-sell — whichever the recommendation is predicated on). If the tuning endpoint doesn't return variance for a given metric, fall back to obs-count-only bands and say so: `(Medium confidence, obs-count only — variance not available)`. The `/api/campaigns/{id}/projections` response also returns its own `confidence` string ("low"/"medium"/"high") that can be mapped directly to H|M|L when that endpoint is the source. When multiple bands match (e.g. obs-count qualifies for Medium but history-length qualifies for Low), use the lower confidence band.

### "Hold" verdict rule

When signal is weak, recommend holding — explicitly — instead of synthesizing a change. A hold fires when *any* of the following is true for the metric driving the recommendation:

- Week-over-week delta is within 1σ of historical WoW variance. If rolling variance isn't available from the endpoint, use a rule-of-thumb: WoW delta within ±10% of the campaign's trailing-4-week mean. (Computing a true σ requires 4 sequential calls to `/api/portfolio/weekly-review?weekOffset=N` — only do that if the user specifically asks; default to the rule-of-thumb.)
- Proposed parameter change magnitude is < 3 percentage points AND confidence is Medium or Low.
- Sell-through drop is < 5pp AND observation count is < 20.

Say it out loud — in the default rule-of-thumb form: *"Hold — this week's ROI is 7%, within ±10% of the 8.2% trailing-4-week mean. Noise, not signal. I'd keep current params."* (The σ form is used only when you actually computed σ via 4× `weekOffset` calls.) Silence is not acceptable; the user learns *why* nothing is being changed.

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

## Data conventions

- **All monetary values are in cents.** Divide by 100 and format as `$X,XXX.XX`.
- **Buy terms** are decimals (`0.80` = 80% of CL value).
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

## References

Load these on demand, not upfront:

- `references/api-cheatsheet.md` — jq patterns for projecting curl output, weekly-review response fields, and the JSON-key-to-concept table (covers the `buyCostCents` vs `purchasePriceCents` trap and the string-UUID `id` convention on both `Purchase` and `Campaign`). Read when you're writing a curl and need to confirm a field name.
- `references/advisor-tools.md` — catalog of server-side AI advisor tools and which advisor operations use them. Read when the user asks about the advisor endpoints (`/api/advisor/digest`, `/api/advisor/liquidation-analysis`, `/api/advisor/campaign-analysis`) or you want to sanity-check playbook output.

## Appendix — Explicit mode shortcuts

These are the old named modes. Most of the time they're unnecessary — the default conversational flow in Steps 3 and 4 covers the same ground and adapts to whatever the user actually asks. Use them only when the user explicitly names one.

| Argument | Behaviour |
|----------|-----------|
| *(empty)* | Run Steps 3 and 4 — the default conversational flow |
| `health` | Fetch `/api/portfolio/health` + `/api/credit/summary` only, present a tight health-only snapshot |
| `weekly` | Fetch `/api/portfolio/weekly-review` + `/api/portfolio/health` + `/api/credit/summary` + `/api/portfolio/suggestions`, end with *"It's review day — any parameter adjustments to discuss?"* |
| `tuning` | Run Playbook A directly without the initial snapshot |
| `campaign <id-or-name>` | Run Playbook E directly; resolve a name through `/api/campaigns` if given one |
| `gaps` | Run Playbook F directly — coverage gap analysis and new campaign design |
| `dh` | Run Playbook G directly — DH marketplace status and intelligence |
