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
| `get_sell_sheet` | Global sell sheet: target price, min price, channel per card | `GenerateGlobalSellSheet` |
| `get_portfolio_health` | Health scores per campaign: status, reason, capital at risk | `GetPortfolioHealth` |
| `get_portfolio_insights` | Cross-campaign segmentation by character, grade, era, tier | `GetPortfolioInsights` |
| `get_credit_summary` | Outstanding balance, recovery trend, alert level, projected exposure | `GetCreditSummary` |
| `get_weekly_review` | Week-over-week comparison: purchases, spend, sales, profit | `GetWeeklyReviewSummary` |
| `get_capital_timeline` | Daily capital deployment: spend, recovery, outstanding balance | `GetCapitalTimeline` |
| `get_channel_velocity` | Average days to sell and count by channel | `GetPortfolioChannelVelocity` |
| `get_campaign_suggestions` | Data-driven suggestions for new/adjusted campaigns | `GetCampaignSuggestions` |
| `get_suggestion_stats` | AI price suggestion acceptance/dismissal statistics | `GetPriceOverrideStats` |

### Campaign-Level Tools (require `campaignId` parameter)

| Tool | Description | Service Method |
|------|-------------|---------------|
| `get_campaign_pnl` | P&L summary: spend, revenue, fees, net profit, ROI | `GetCampaignPNL` |
| `get_pnl_by_channel` | P&L broken down by sale channel | `GetPNLByChannel` |
| `get_campaign_tuning` | Performance by grade, price tier, buy threshold analysis | `GetCampaignTuning` |
| `get_inventory_aging` | Unsold cards with days held, market signals, anomaly flags | `GetInventoryAging` |
| `get_expected_values` | EV per unsold card: EV cents, EV/dollar, sell probability | `GetExpectedValues` |
| `get_crack_candidates` | Crack arbitrage: graded vs raw net, advantage, ROI comparison | `GetCrackCandidates` |
| `run_projection` | Monte Carlo simulation (1000 iterations) comparing parameters | `RunProjection` |

### Utility Tools (custom parameters)

| Tool | Description | Parameters |
|------|-------------|------------|
| `get_cert_lookup` | PSA cert lookup: card details + market data | `certNumber` (string) |
| `evaluate_purchase` | EV analysis for hypothetical purchase | `campaignId`, `cardName`, `grade`, `buyCostCents` |
| `suggest_price` | Suggest sell price (saved for user review) | `purchaseId`, `priceCents` |

## Operation-to-Tool Mapping

Each advisor operation only sends relevant tools to the LLM (defined in `operationTools` map in `service_impl.go`):

### Digest (weekly intelligence)
`list_campaigns`, `get_campaign_pnl`, `get_pnl_by_channel`, `get_campaign_tuning`, `get_inventory_aging`, `get_global_inventory`, `get_sell_sheet`, `get_portfolio_health`, `get_portfolio_insights`, `get_credit_summary`, `get_weekly_review`, `get_capital_timeline`, `get_channel_velocity`, `get_dashboard_summary`

### Campaign Analysis (single campaign health)
`list_campaigns`, `get_campaign_pnl`, `get_pnl_by_channel`, `get_campaign_tuning`, `get_inventory_aging`, `get_expected_values`, `get_crack_candidates`, `get_campaign_suggestions`, `run_projection`, `get_channel_velocity`, `get_credit_summary`

### Liquidation (sell recommendations)
`list_campaigns`, `get_global_inventory`, `get_sell_sheet`, `get_credit_summary`, `get_expected_values`, `get_inventory_aging`, `get_portfolio_health`, `suggest_price`, `get_cert_lookup`, `get_channel_velocity`, `get_capital_timeline`, `get_suggestion_stats`, `get_dashboard_summary`

### Purchase Assessment (buy/no-buy)
`list_campaigns`, `get_campaign_tuning`, `get_portfolio_insights`, `get_cert_lookup`, `evaluate_purchase`, `get_campaign_pnl`, `get_channel_velocity`

## Usage Notes

- **Total tools registered:** 21 (including `get_dashboard_summary`)
- Tools unused by any operation: none currently — all tools appear in at least one operation's subset
- The `suggest_price` tool is only available during liquidation analysis (it writes to the database)
- `evaluate_purchase` is only available during purchase assessment
- Most tools return JSON; large results are automatically truncated by `toJSON()` (30KB limit, array halving)
- Tool execution has a 30-second timeout per call
- Tools execute concurrently within a single LLM round
