# AI Advisor Tool Reference

## When to consult this file from SKILL.md

The default campaign-analysis flow hits raw HTTP endpoints directly. This reference matters when you want a pre-synthesized narrative instead of stitching one yourself — the server exposes LLM-backed advisor endpoints that run tool-calling loops against the tools catalogued below:

- `POST /api/advisor/digest` — portfolio-wide weekly intelligence (complement to Playbook B / weekly flow)
- `POST /api/advisor/campaign-analysis` — single-campaign health (complement to Playbook E)
- `POST /api/advisor/liquidation-analysis` — sell recommendations (complement to Playbook B)
- `POST /api/advisor/purchase-assessment` — buy/no-buy (if the user asks about a specific card)

Use an advisor endpoint when the user wants "a quick read" with a narrative framing, or as a sanity check against a manually-assembled playbook output. Use raw endpoints when you need the numbers in structured form to cite specifics or make concrete edits.

---

The advisor service (`internal/domain/advisor/`) orchestrates LLM tool-calling loops. The `CampaignToolExecutor` (`internal/adapters/advisortool/executor.go`) registers these tools for the LLM to call.

## Tool Catalog

### Portfolio-Level Tools (no campaign ID required)

| Tool | Description | Service Method |
|------|-------------|---------------|
| `list_campaigns` | List all campaigns with parameters, phase, stats | `ListCampaigns` |
| `get_global_inventory` | All unsold cards across campaigns with aging and signals | `GetGlobalInventoryAging` |
| `get_flagged_inventory` | Items flagged for revocation or review | `GetFlaggedInventory` |
| `get_sell_sheet` | Global sell sheet: target price, min price, channel per card | `GenerateGlobalSellSheet` |
| `get_portfolio_health` | Health scores per campaign: status, reason, capital at risk | `GetPortfolioHealth` |
| `get_portfolio_insights` | Cross-campaign segmentation by character, grade, era, tier | `GetPortfolioInsights` |
| `get_capital_summary` | Capital exposure: outstanding balance, recovery trend (requires financeService) | `GetCapitalSummary` |
| `get_weekly_review` | Week-over-week comparison: purchases, spend, sales, profit | `GetWeeklyReviewSummary` |
| `get_capital_timeline` | Daily capital deployment: spend, recovery, outstanding balance | `GetCapitalTimeline` |
| `get_channel_velocity` | Average days to sell and count by channel | `GetPortfolioChannelVelocity` |
| `get_campaign_suggestions` | DH-sourced buy suggestions for campaigns | `GetCampaignSuggestions` |
| `get_suggestion_stats` | AI price suggestion acceptance/dismissal statistics | `GetPriceOverrideStats` |
| `get_dashboard_summary` | Combined dashboard view (health + capital + weekly in one call) | `GetDashboardSummary` |

### Campaign-Level Tools (require `campaignId` parameter)

| Tool | Description | Service Method |
|------|-------------|---------------|
| `get_campaign_pnl` | P&L summary: spend, revenue, fees, net profit, ROI | `GetCampaignPNL` |
| `get_pnl_by_channel` | P&L broken down by sale channel | `GetPNLByChannel` |
| `get_campaign_tuning` | Performance by grade, price tier, buy threshold analysis | `GetCampaignTuning` |
| `get_inventory_aging` | Unsold cards with days held, market signals, anomaly flags | `GetInventoryAging` |
| `get_expected_values` | EV per unsold card: EV cents, EV/dollar, sell probability | `GetExpectedValues` |
| `get_deslab_candidates` | Deslab/crack recommendations: graded vs raw net, advantage, ROI | `GetDeslabCandidates` |
| `run_projection` | Monte Carlo simulation (1000 iterations) comparing parameters | `RunProjection` |

### Batch Tools (multiple campaigns)

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_expected_values_batch` | EV for multiple campaigns in one call | `campaignIds` (array) |
| `suggest_price_batch` | AI price suggestions for multiple items in bulk | `items` (array of `{purchaseId, priceCents}`) |

### Intelligence Tools (DH market data)

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_market_intelligence` | Deep market intel: sentiment, forecast, grading ROI, sales, population | Requires `WithIntelligenceRepo` |
| `get_dh_suggestions` | DH-specific suggestions for inventory | Requires `WithSuggestionsRepo` |
| `get_inventory_alerts` | Inventory alert report from DH signals | (none) |

### Cross-Campaign Analysis Tools

| Tool | Description | Service Method |
|------|-------------|---------------|
| `get_acquisition_targets` | Potential acquisition targets across campaigns | `GetAcquisitionTargets` |
| `get_deslab_opportunities` | DH deslab/crack opportunities across portfolio | `GetDeslabOpportunities` |

### Utility Tools (custom parameters)

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_cert_lookup` | PSA cert lookup: card details + market data | `certNumber` (string) |
| `evaluate_purchase` | EV analysis for hypothetical purchase | `campaignId`, `cardName`, `grade`, `buyCostCents` |
| `suggest_price` | Suggest sell price (saved for user review) | `purchaseId`, `priceCents` |
| `get_data_gap_report` | Scoring data gap report over last 7 days (requires `WithGapStore`) | (none) |

## Operation-to-Tool Mapping

Each advisor operation only sends relevant tools to the LLM (defined in `operationTools` map in `service_impl.go`):

### Digest (weekly intelligence)
`get_dashboard_summary`, `get_weekly_review`, `get_global_inventory`, `get_portfolio_insights`, `get_flagged_inventory`, `get_inventory_alerts`, `get_acquisition_targets`, `get_deslab_opportunities`, `get_dh_suggestions`, `get_expected_values_batch`, `get_campaign_tuning`, `get_campaign_pnl`

### Campaign Analysis (single campaign health)
`get_campaign_pnl`, `get_pnl_by_channel`, `get_campaign_tuning`, `get_inventory_aging`, `get_expected_values`, `get_deslab_candidates`

### Liquidation (sell recommendations)
`get_dashboard_summary`, `get_flagged_inventory`, `get_suggestion_stats`, `get_inventory_alerts`, `get_expected_values_batch`, `suggest_price_batch`

### Purchase Assessment (buy/no-buy)
`get_campaign_tuning`, `get_cert_lookup`, `evaluate_purchase`, `get_campaign_pnl`

## Usage Notes

- **Total tools registered:** 31
- Optional tools (only useful when the relevant service is injected):
  - `get_capital_summary`: requires `WithFinanceService`
  - `get_data_gap_report`: requires `WithGapStore` (returns error JSON if absent)
  - `get_market_intelligence`: requires `WithIntelligenceRepo`
  - `get_dh_suggestions`: requires `WithSuggestionsRepo`
- The `suggest_price` and `suggest_price_batch` tools are only available during liquidation analysis (they write to the database)
- `evaluate_purchase` is only available during purchase assessment
- Most tools return JSON; large results are automatically truncated by `toJSON()` (30KB limit, array halving)
- Tool results are capped at 12,000 chars per individual result to prevent input token bloat
- Tool execution has a 30-second timeout per call
- Tools execute concurrently within a single LLM round
