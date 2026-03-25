# AI Usage Audit

Date: 2026-03-25

Comprehensive review of Card Yeti's AI integration — prompts, architecture, cost, reliability, observability, and missing capabilities.

## 1. Prompt Engineering (highest ROI)

### Stale campaign data baked into system prompt
`baseSystemPrompt` in `internal/domain/advisor/prompts.go:4-42` hardcodes 6 campaigns with specific parameters (names, buy terms, price ranges). When campaigns change, the prompt is wrong. The LLM has tools to fetch this data live.

**Fix**: Remove the hardcoded campaign list from the system prompt. Instruct the LLM to call `list_campaigns` as its first action to get current state. This makes the prompt evergreen.

### No structured output enforcement
Advisor operations stream free-form markdown. The LLM sometimes deviates from the requested sections (e.g., skipping "Watch List" in digests or reordering). No validation that the response matches the requested structure.

**Fix**: For non-streaming use cases (`CollectDigest`, `CollectLiquidation` used by scheduler), consider JSON mode or at minimum post-process to validate section headers are present.

### Social caption length not enforced
`captionSystemPrompt` says "under 300 characters" but there's no enforcement. If the LLM generates a longer caption, it's saved as-is.

**Fix**: Add a length check after `parseCaption()` — truncate or log a warning.

### Purchase assessment uses raw campaign ID
In `purchaseAssessmentUserPrompt`, the campaign is referenced by ID (`%s`), but the LLM has no way to know which campaign that is without a tool call. The prompt should include the campaign name for context.

## 2. Token Efficiency & Cost

### Tool result truncation is crude
`toJSON()` in `advisortool/executor.go` truncates at 30KB by slicing JSON bytes, which can produce invalid JSON. The truncated partial field will contain broken JSON the LLM has to work around.

**Fix**: Truncate at a valid JSON boundary (e.g., truncate arrays by removing trailing elements) or summarize the data before sending.

### All 19 tools sent on every request
Every analysis type (digest, campaign, liquidation, purchase) sends all 19 tool definitions. Purchase assessment doesn't need `suggest_price` or `get_crack_candidates`. Digest doesn't need `evaluate_purchase`.

**Fix**: Create per-operation tool subsets. Reduces prompt tokens and prevents the LLM from calling irrelevant tools.

### No per-operation token tuning
`defaultMaxTokens = 4096` is used for all advisor operations. A digest might genuinely need 4K+, while campaign analysis may need less. Social captions already use 512 — advisor operations could be differentiated too.

### Large tool results aren't summarized
Global inventory aging for a portfolio with 100+ cards produces a massive JSON payload that eats input tokens on re-submission across tool rounds.

**Fix**: For known large-result tools (global_inventory, sell_sheet), consider a summary mode that returns aggregate stats + top-N cards instead of the full list.

## 3. Reliability & Error Handling

### Rate limit retry only works pre-stream
`client.go:111-114` avoids retrying after chunks have been emitted, but there's no user-facing notification. The SSE stream stops with an error and the frontend must handle an incomplete response.

**Fix**: Emit a specific error event type the frontend can display as "Rate limited, please try again in a few minutes."

### Tool execution has no per-tool timeout
`service_impl.go:188-215` runs tools concurrently but with no individual timeout. A stuck `LookupCert` call (which hits external APIs) could block the entire round indefinitely. Only the parent context deadline protects against this.

**Fix**: Add a per-tool-call timeout (e.g., 30 seconds) wrapping each `Execute()` call.

### Max tool rounds error isn't surfaced well
When the loop exhausts 5 rounds (`service_impl.go:230`), the error message doesn't include what the LLM was trying to do.

**Fix**: Include the last tool calls attempted and a summary of the conversation in the error.

### Social caption fallback text can be published
`service_impl.go:355` sets caption to `"(Caption generation failed...)"`. If this post gets auto-published, the placeholder becomes the Instagram caption.

**Fix**: Block publishing of posts with the placeholder caption. Check in `Publish()` or `SetPublishing()`.

## 4. Architecture & Code Quality

### Social domain coupled to advisor package
`social/service_impl.go` imports `advisor.LLMProvider`, `advisor.StreamEvent`, `advisor.TokenUsage`, etc. Social isn't an "advisor" feature — it's coupling two independent domains.

**Fix**: Extract shared AI types (`LLMProvider`, `StreamEvent`, `TokenUsage`, `CompletionRequest`, etc.) into a neutral domain package (e.g., `domain/ai` or `domain/llm`). Both `advisor` and `social` depend on this.

### Duplicated recordAICall pattern
Both `advisor/service_impl.go:236-260` and `social/service_impl.go:560-583` have nearly identical `recordAICall` methods. `ClassifyAIError` has been extracted — the full recording logic could follow.

**Fix**: Create a shared `RecordCall` helper function that both services call.

### LLM-suggested posts always typed as NewArrivals
`service_impl.go:146` hardcodes `PostTypeNewArrivals` for all LLM-suggested posts. The LLM picks themes but the post type metadata is always the same.

**Fix**: Have the LLM return a `postType` field in its JSON response, or infer it from the theme.

## 5. Observability & Monitoring

### No cost tracking
Token counts are recorded but never translated to estimated cost. With Azure pricing varying by model, adding an estimated cost column would help monitor spend.

**Fix**: Add a `costEstimateCents` field to `AICallRecord` based on model pricing (configurable per deployment).

### 7-day rolling window is the only view
The `ai_usage_summary` SQL view only shows 7 days. No historical trend analysis or month-over-month cost growth visibility.

**Fix**: Add a monthly summary view or make the time window configurable in the API.

### No alerting on degradation
If success rate drops or rate limiting spikes, there's no notification. The admin must manually check the AI Status tab.

**Fix**: Add threshold checks in the scheduler that log at WARN/ERROR level when success rate < 80% or rate_limit_hits > N in 24h.

## 6. Missing Capabilities

### No conversation memory across sessions
Each analysis starts fresh. The LLM can't reference previous recommendations or flagged cards.

**Fix (low effort)**: Include a summary of the previous cached analysis result in the system prompt when one exists.

### No feedback loop on AI suggestions
`suggest_price` saves recommendations, but whether the user accepted/dismissed them is never fed back.

**Fix**: Track accept/dismiss rates per operation and include aggregate stats in the system prompt.

## Priority Recommendations

### Phase 1 — Prompt & Architecture (high ROI, low risk)
1. Remove hardcoded campaigns from system prompt
2. Create per-operation tool subsets
3. Extract shared AI types into `domain/ai` or `domain/llm`
4. Consolidate duplicated `recordAICall` logic

### Phase 2 — Reliability (medium effort)
5. Add per-tool-call timeouts
6. Fix JSON truncation in `toJSON()`
7. Block publishing of placeholder captions
8. Add rate-limit-specific error events for frontend

### Phase 3 — Observability & Intelligence (higher effort)
9. Add cost estimation to tracking
10. Include previous analysis context in prompts
11. Track suggestion accept/dismiss feedback loop
12. Add degradation alerting
