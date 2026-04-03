# Advisor Batch Tools & Round Budget Fix

**Date**: 2026-04-03
**Status**: Approved

## Problem

The AI advisor tool-calling loop intermittently fails with "exceeded maximum tool rounds" on liquidation and digest analyses. Root cause: per-campaign tools like `get_expected_values` require one LLM tool call per campaign. With 9 active campaigns, the LLM emits 9+ tool calls in round 2 (EV calls + `get_crack_opportunities` + `suggest_price` calls), and when it needs follow-up `suggest_price` calls for additional cards, round 3 doesn't exist.

The 2-round budget was set when the portfolio had fewer campaigns. As campaigns grew from 7 to 9 (and may reach 15), the round budget became too tight.

## Solution

Two-part fix: batch tool variants to reduce per-round tool call count, plus a safety round increase.

### 1. New Tool: `get_expected_values_batch`

Accepts an array of campaign IDs (or omit for all active campaigns). Returns a map of campaignId to expected values.

```json
// Parameters
{ "campaignIds": ["id1", "id2"] }  // optional, defaults to all active

// Response
{ "id1": [...EVs], "id2": [...EVs] }
```

Handler calls `campaigns.Service.GetExpectedValues` concurrently for each campaign. Per-campaign results are individually size-limited before merging to stay within the 12KB tool result cap.

### 2. New Tool: `suggest_price_batch`

Accepts an array of price suggestions. Executes all, returns per-item status.

```json
// Parameters
{ "suggestions": [{"purchaseId": "abc", "priceCents": 1500}, ...] }

// Response
{ "results": [{"purchaseId": "abc", "status": "ok"}, {"purchaseId": "def", "status": "ok"}] }
```

### 3. Bump `operationMaxRounds` from 2 to 3

For `OpLiquidation` and `OpCampaignAnalysis`. Batch tools should keep things within 2 rounds normally, but 3 prevents hard failures when the LLM needs follow-up calls.

### 4. Update Prompts

Update liquidation and digest tool strategy sections to reference batch variants:

- Round 2 should use `get_expected_values_batch` (one call) instead of N individual calls
- Round 2 should use `suggest_price_batch` (one call) for all repricing
- Round 3 available as escape hatch for follow-up

### 5. Register in Operation Tool Subsets

Add `get_expected_values_batch` and `suggest_price_batch` to `operationTools` for `OpLiquidation` and `OpDigest`.

## What Stays the Same

- Single-campaign `get_expected_values` and `suggest_price` remain (used by `OpCampaignAnalysis` and `OpPurchaseAssessment`)
- Tool-calling loop logic unchanged
- `maxToolResultChars` truncation still applies per tool result
- `defaultMaxToolRounds` (3) remains the global default

## Truncation Strategy

Each campaign's EV data is truncated to `maxToolResultChars / len(campaignIds)` before merging, so the combined batch result stays under the per-tool-result cap.

## Files Touched

| File | Change |
|------|--------|
| `internal/adapters/advisortool/tools_portfolio.go` | Add `registerGetExpectedValuesBatch()` and `registerSuggestPriceBatch()` |
| `internal/adapters/advisortool/executor.go` | Call new register methods in `registerTools()` |
| `internal/domain/advisor/service_impl.go` | Bump `operationMaxRounds` for Liquidation and CampaignAnalysis: 2 to 3 |
| `internal/domain/advisor/prompts.go` | Update liquidation + digest tool strategy sections |
| `internal/adapters/advisortool/executor_test.go` | Add batch tools to expected tool list |

No domain interface changes needed. Batch tools are adapter-level orchestration over existing `campaigns.Service` methods.
