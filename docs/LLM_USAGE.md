# LLM Usage

This document describes how SlabLedger integrates large language models — what features use them, how they are wired, and the parameters involved.

---

## Provider

**Azure AI** is the sole LLM provider. The client is implemented in `internal/adapters/clients/azureai/` and uses the **Responses API** (not Chat Completions) via the `github.com/openai/openai-go/v3` SDK.

### Configuration

| Env Var | Default | Purpose |
|---|---|---|
| `AZURE_AI_ENDPOINT` | `""` | Azure AI Foundry or Azure OpenAI endpoint |
| `AZURE_AI_API_KEY` | `""` | API key |
| `AZURE_AI_DEPLOYMENT` | `"gpt-5.4"` | Default model deployment (advisor) |

If `AZURE_AI_ENDPOINT` or `AZURE_AI_API_KEY` is empty, the advisor LLM features degrade gracefully to no-ops or rule-based fallbacks.

### API versions

- LLM (Responses API): `2025-04-01-preview`

### Retry and rate limiting

The client enforces a self-imposed rate limit of **2 req/sec** (configurable via `WithRateLimitRPS`). On errors it applies exponential backoff:

| Error type | Backoff intervals |
|---|---|
| Rate limit (429) | 30s → 60s → 120s |
| No capacity | 60s → 120s → 240s |
| Transient | 5s → 10s → 20s |

After 5 streaming attempts it falls back to polling `GET /responses/{id}` when `store=true` and a `responseID` was captured.

### Cost model

Tracked as `$2.50 / 1M input tokens` + `$10.00 / 1M output tokens` (GPT-4o-class pricing). Actual cost depends on the deployed model.

---

## Domain interface

`internal/domain/ai/` defines the interfaces that all LLM consumers depend on:

```go
type LLMProvider interface {
    StreamCompletion(ctx context.Context, req CompletionRequest, stream func(CompletionChunk)) error
}
```

`CompletionRequest` fields of note:

| Field | Purpose |
|---|---|
| `SystemPrompt` | Static instructions for the LLM |
| `Messages` | Conversation history |
| `Tools` | Tool definitions available this round |
| `Temperature` | Sampling temperature |
| `MaxTokens` | Output token cap |
| `ConversationState` | `PreviousResponseID` string for chaining Responses API calls |
| `ReasoningEffort` | `"low"`, `"medium"`, or `"high"` |
| `Store` | Store response server-side (enables `PreviousResponseID` chaining) |

---

## Features

### Advisor

**Package:** `internal/domain/advisor/`  
**Invocation:** on-demand via the streaming HTTP endpoints (`/api/advisor/digest`, `/api/advisor/liquidation-analysis`).

The advisor is a multi-round tool-calling agent. Each operation runs a loop:

```
LLM → tool calls (concurrent, 30s timeout each) → LLM → … (up to maxRounds)
```

Tool results are truncated to **12 000 chars** each before being sent back to the LLM. When `PreviousResponseID` chaining is available, full message history is not re-sent.

#### Operations and parameters

| Operation | Max rounds | Max tokens | Temperature | Reasoning effort |
|---|---|---|---|---|
| `GenerateDigest` | 4 | 32 768 | 0.3 | medium |
| `AnalyzeLiquidation` | 3 | 32 768 | 0.3 | medium |
| `AssessPurchase` | 1 | 32 768 | 0.3 | medium |

`ADVISOR_MAX_TOOL_ROUNDS` env var overrides the default per-operation value.

#### Tool registry (28 tools, defined in `internal/adapters/advisortool/`)

Each operation uses a filtered subset:

**Digest (4 rounds, 11 tools):** `get_dashboard_summary`, `get_weekly_review`, `get_global_inventory`, `get_portfolio_insights`, `get_flagged_inventory`, `get_inventory_alerts`, `get_acquisition_targets`, `get_dh_suggestions`, `get_expected_values_batch`, `get_campaign_tuning`, `get_campaign_pnl`

**Liquidation (3 rounds, 6 tools):** `get_dashboard_summary`, `get_flagged_inventory`, `get_suggestion_stats`, `get_inventory_alerts`, `get_expected_values_batch`, `suggest_price_batch`

**Purchase assessment (1 round, 4 tools):** `get_campaign_tuning`, `get_cert_lookup`, `evaluate_purchase`, `get_campaign_pnl`

#### Scored analysis

For `AssessPurchase`, a pre-computed `ScoreCard` is injected into the system prompt and a structured JSON schema is appended, forcing structured JSON output from the LLM.

---

## Scheduler summary

| Scheduler | Default trigger | Timeout | Retries |
|---|---|---|---|
| Advisor refresh | Daily at 04:00 UTC | 20 min per analysis | 2× with 5-min backoff |

---

## AI call tracking

All LLM calls are recorded via `ai.AICallTracker` (implemented by `postgres.AICallRepository`). Tracked operations include: `digest`, `liquidation`, `purchase_assessment`.

Usage stats are exposed via the `/api/ai/usage` endpoint.

---

## Wiring

`cmd/slabledger/init_services.go` assembles the dependency graph:

```
AZURE_AI_ENDPOINT + AZURE_AI_KEY
    └─ azureai.NewClient (deployment = AZURE_AI_DEPLOYMENT)
         └─ advisor.NewService(llmProvider, CampaignToolExecutor)
```
