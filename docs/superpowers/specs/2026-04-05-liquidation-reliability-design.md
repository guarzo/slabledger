# Liquidation Analysis Reliability Redesign

**Date**: 2026-04-05
**Status**: Draft
**Problem**: Liquidation analysis LLM job fails 76% of the time (13/17 calls) with `ERR_ADVISOR_MAX_ROUNDS`

## Background

The liquidation analysis runs a 3-round tool-calling loop against Azure AI (GPT-5.4).
PR #46 simplified from 18 tools to 8 and 8 channels to 3, but the failure rate remains
high. The weekly digest (same architecture) succeeds 92% of the time.

**Current stats:**
| Operation | Calls | Errors | Success | Latency | Tokens | Cost |
|-----------|-------|--------|---------|---------|--------|------|
| Weekly Digest | 13 | 1 | 92% | 182.3s | 428.3K | $1.76 |
| Liquidation Analysis | 17 | 13 | 24% | 206.9s | 484.4K | $1.73 |

## Root Causes

### 1. Round budget mismatch (primary)

`operationMaxRounds[OpLiquidation] = 3` but the prompt says "3-round tool budget."
The LLM interprets this as 3 rounds of tool calls. The code loop runs
`for round := 0; round < 3; round++` — if the LLM returns tool calls in all 3
iterations, it exits with `ERR_ADVISOR_MAX_ROUNDS` with no iteration left to write
the analysis.

The weekly digest survives because its tools naturally cluster into 2 rounds. The
liquidation prompt actively encourages Round 3 tool use ("escape hatch").

### 2. Prompt issues

- "3-round tool budget" is misleading — invites 3 rounds of tool calling
- No hard stop instruction after Round 2
- No "Do NOT" guardrails (digest has these, liquidation doesn't)
- No numbered output structure (digest has 7 sections)
- `get_global_inventory` and `get_sell_sheet` return overlapping per-card data
- `suggest_price_batch` guidance is front-loaded, encouraging extra rounds

### 3. Token bloat

484K tokens per call. 5 Round-1 tools at up to 12K chars each = 60K chars of tool
results, much of it redundant between `get_global_inventory` and `get_sell_sheet`.

### 4. No local testing

No way to run the analysis locally with verbose output. Prompt iteration requires
deploying and waiting for the scheduler.

## Design

Four components: inventory signals system, round budget fix, revised prompt, and CLI
debug harness.

### Component 1: Inventory Signals System

Compute procedural flags per unsold card server-side. These flags are pure business
logic — no LLM. They feed the card show tab, the inventory UI, and the LLM analysis.

#### Signals

| Signal | Criteria | Card Show Tab | LLM Input |
|--------|----------|---------------|-----------|
| `profit_capture_declining` | Recent last-sold (<=14d) + still profitable at market price + trend30d < 0 | Yes | Yes |
| `profit_capture_spike` | Price increase >10% in 30d + recent sales (>=2 in 30d) + still profitable | Yes | Yes |
| `crack_candidate` | Existing `GetCrackOpportunities()` logic — `isCrackCandidate && crackAdvantage > 0` | Yes | Yes |
| `stale_listing` | Held >14 days (purchase date) with no sale | No | Yes |
| `deep_stale` | Held >30 days (purchase date) with no sale | No | Yes |
| `cut_loss` | Deep stale + declining trend OR negative unrealized P/L | No | Yes |

#### Domain type

```go
// InventorySignals contains procedural flags for an unsold card.
// Computed server-side from market data, aging, and profitability.
type InventorySignals struct {
    ProfitCaptureDeclining bool `json:"profitCaptureDeclining,omitempty"`
    ProfitCaptureSpike     bool `json:"profitCaptureSpike,omitempty"`
    CrackCandidate         bool `json:"crackCandidate,omitempty"`
    StaleListing           bool `json:"staleListing,omitempty"`
    DeepStale              bool `json:"deepStale,omitempty"`
    CutLoss                bool `json:"cutLoss,omitempty"`
}
```

Added to `SellSheetItem` and the inventory aging response so both the UI and
LLM tools receive them.

#### Computation

Computed inside `enrichSellSheetItem()` in `service_sell_sheet.go` using existing
data: `MarketSnapshot` (trend30d, lastSoldCents, salesLast30d), purchase aging
(days held), and cost basis. Crack candidate data requires a cross-reference with
`GetCrackOpportunities()` results — pass crack results as a lookup map.

#### Card show tab impact

`recommendChannel()` in `service_sell_sheet.go` becomes:

```
Grade 7                          -> In Person
profit_capture_declining         -> In Person
profit_capture_spike             -> In Person
crack_candidate                  -> In Person
trend30d > 5%                    -> In Person  (existing rule)
Default                          -> eBay
```

Frontend `isCardShowCandidate()` simplifies to checking server-computed signals
instead of re-deriving from market data client-side.

#### New tool: `get_flagged_inventory`

Returns only cards with at least one signal set. Replaces `get_global_inventory`
in the liquidation tool set. Much smaller payload — only actionable cards, not the
full inventory.

### Component 2: Round Budget Fix

Change `operationMaxRounds[OpLiquidation]` from 3 to 4 in `service_impl.go:237`.

This gives:
- Rounds 0-1: tool calls (prompt's Round 1 + Round 2)
- Round 2: escape hatch tool call if needed
- Round 3: guaranteed analysis-only iteration

### Component 3: Revised Liquidation Prompt

The LLM's role changes from "analyze entire portfolio" to "make judgment calls on
pre-flagged cards."

#### Tool set (6 tools, down from 8)

| Tool | Round | Purpose |
|------|-------|---------|
| `get_dashboard_summary` | 1 | Credit health, portfolio overview |
| `get_flagged_inventory` | 1 | Only cards with signals (replaces `get_global_inventory`) |
| `get_suggestion_stats` | 1 | Prior suggestion acceptance rates |
| `get_inventory_alerts` | 1 | DH cross-reference alerts |
| `get_expected_values_batch` | 2 | EV data for campaigns with flagged cards |
| `suggest_price_batch` | 2 | Save repricing recommendations |

**Removed**: `get_sell_sheet` (overlapping with inventory), `get_crack_opportunities`
(now a signal on flagged cards).

#### System prompt

```
## Your Task: Liquidation Analysis
You receive pre-flagged inventory — cards already identified by the scoring engine as
needing action. Your job is to make judgment calls the engine cannot:

1. **Reprice stale listings** — cards flagged stale/deep_stale with no recent sales near
   our price. Determine a new price using market comps, sentiment, and EV data. Save via
   suggest_price_batch.

2. **Auction vs fixed price** — for stale cards, evaluate whether auction is better than
   fixed price. Favor auction when: fair value is uncertain (wide spread in recent comps),
   card has been listed 30+ days at fixed, or card is trending with potential for
   above-market bids. Favor fixed when: price is well-established and we just need a
   small adjustment.

3. **Cut-loss decisions** — cards flagged cut_loss. For each: quantify the carrying cost
   vs expected further decline. Recommend one of:
   - Drop online price to [specific amount]
   - Auction (starting price at [amount])
   - Sell in person at 75-80% of market to free capital immediately
   Show the math: holding cost per month vs markdown cost.

4. **Credit pressure adjustment** — if credit utilization is high (>80%), lower the bar
   for all liquidation actions. Cards you'd normally hold become sells.

Do NOT re-analyze cards flagged profit_capture_declining, profit_capture_spike, or
crack_candidate — those have clear procedural actions (sell in person / crack and sell).
Only mention them in your summary totals.

## Tool Strategy
You have a **2-round tool budget** and 6 tools.

**Round 1**: Call get_dashboard_summary, get_flagged_inventory, get_suggestion_stats,
and get_inventory_alerts together.

**Round 2**: Call get_expected_values_batch for campaigns with flagged cards.
If you have repricing recommendations, call suggest_price_batch.

**After Round 2, write your analysis immediately. Do NOT make additional tool calls.**

## Output Structure
1. **Credit Snapshot** — utilization %, alert level, urgency modifier
2. **Reprice Recommendations** — table: card, current price, new price, reasoning
3. **Auction Candidates** — table: card, why auction beats fixed, suggested start price
4. **Cut-Loss Actions** — table: card, cost basis, current market, recommended action,
   carrying cost math, capital freed
5. **Summary** — total capital recoverable, total markdown cost, net repricing impact,
   suggestion stats (how prior suggestions performed)
```

#### User prompt

```
Run a liquidation analysis on my flagged inventory.

Focus your judgment on three decisions:
1. What price should stale listings be set to?
2. Should any cards go to auction instead of fixed price?
3. Which cards should we take a loss on, and how?

Do not repeat data from the flags — I can see those in the UI.
End with totals: capital freed, markdown cost, and repricing count.
```

### Component 4: CLI Debug Harness

Add `slabledger admin analyze` command.

#### Usage

```bash
slabledger admin analyze liquidation            # Run analysis, print result
slabledger admin analyze liquidation --verbose   # Per-round tool calls, tokens, timing
slabledger admin analyze digest                  # Works for digest too
slabledger admin analyze digest --dry-run        # Don't save to cache or execute suggest_price_batch
```

#### Verbose output format

```
Round 1: calling get_dashboard_summary, get_flagged_inventory, get_suggestion_stats, get_inventory_alerts
  get_dashboard_summary    -> 1,240 chars (0.3s)
  get_flagged_inventory    -> 4,891 chars (1.2s)
  get_suggestion_stats     -> 342 chars (0.1s)
  get_inventory_alerts     -> 2,891 chars (0.4s)
  LLM response: 847 input tokens, 312 output tokens (4.2s)

Round 2: calling get_expected_values_batch, suggest_price_batch
  get_expected_values_batch -> 8,401 chars (2.1s)
  suggest_price_batch       -> 423 chars (0.2s)
  LLM response: 24,102 input tokens, 3,841 output tokens (18.3s)

Round 3: no tool calls -- writing analysis
  LLM response: 24,102 input tokens, 8,204 output tokens (31.2s)

OK Analysis complete (54.1s, 484,201 total tokens, ~$1.73)

--- OUTPUT ---
[analysis markdown]
```

#### Implementation

Wires up the same `initializeAdvisorService` from `init.go`. Calls
`advisorSvc.CollectLiquidation(ctx)` directly with a verbose stream callback
that prints per-round details. No changes to the advisor service — it already
emits `StreamEvent` with tool start/result events.

The `--dry-run` flag wraps the tool executor to skip `suggest_price_batch` writes
and skips saving to the advisor cache.

## Expected Outcomes

| Metric | Current | Expected |
|--------|---------|----------|
| Success rate | 24% | >90% (matching digest) |
| Token usage | 484K | ~200-300K (flagged cards only, no overlapping tools) |
| Cost per call | $1.73 | ~$0.80-1.00 |
| Latency | 207s | ~100-140s (fewer tools, smaller payloads) |
| Local testing | None | Full CLI with verbose output |

## Out of Scope

- UI changes for displaying signals (beyond card show tab) — separate spec
- Auction as a first-class sale channel in the DB — the LLM recommends auction
  but the channel remains "eBay"; auction is a listing strategy, not a channel
- Digest prompt improvements — working well at 92%, not broken
- Historical signal tracking / trending over time
