# Remove Azure AI / Advisor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every trace of the Azure AI / advisor feature from SlabLedger — the LLM provider, the advisor service and its tool loop, the scoring engine, AI call tracking, the two HTTP endpoints, the admin CLI subcommand, the frontend AI-usage panel, the two Postgres tables, and all supporting configuration and documentation — while leaving DH pricing, CardLadder, liquidation, intelligence, and existing `ai_suggested_price_cents` data untouched.

**Architecture:** The removal is ordered so that every commit compiles and every test passes. Go permits unreferenced *packages* to sit in the tree — only unused *imports* and unused *local variables* inside a file are compile errors — so "unwire the callers" (Tasks 1-4) is deliberately split from "delete the now-orphaned packages" (Task 5), and each half lands green on its own. Task 1 (frontend) touches no Go file and is independent of every other task, so it can run first or in parallel. The migration and the documentation sweep land last, after the code they describe is already gone.

**Tech Stack:** Go 1.26, hexagonal architecture, Postgres via `jackc/pgx/v5/stdlib`, `golang-migrate/migrate/v4` with `embed.FS`, React + TypeScript + Vite, Vitest, Playwright.

## Global Constraints

- Every task ends with its own commit; the tree must build and test green at **every** commit, not just the last one.
- Backend verification for any task touching Go: run `go build ./...` and then `go test -race ./...` from the repository root, and paste real output before committing.
- Frontend verification for any task touching `web/`: run `cd web && npm run build` and then `cd web && npm test`.
- Run the full quality gate `make check` (lint + `scripts/check-imports.sh` + file-size check + Playwright version check) before the final commit of the plan.
- Never edit `internal/adapters/scheduler/dh_orders_poll_test.go:187` or `internal/domain/inventory/core_types.go:543`. A naive grep for "advisor" flags both, but their only match is the ordinary English word "advisory" describing a card grade — there is no advisor dependency (spec, "Out of scope").
- `AIPricingTab` and price-override statistics are **out of scope** except for the single empty-state copy edit in Task 1; in particular the "Pending Suggestion Value" display stays.
- Existing `campaign_purchases.ai_suggested_price_cents` data and the store methods in `internal/adapters/storage/postgres/purchase_price_store.go` that accept and clear suggestions are untouched — the *producer* is removed, the *data* is kept, so recorded suggestions stay visible and dismissable and the count only ever decreases.
- The documented HTTP API break (`POST /api/advisor/digest`, `POST /api/advisor/liquidation-analysis`, `GET /api/admin/ai-usage`) is deliberate and accepted (spec, Decisions table). Do not add shims, deprecation stubs, or redirects.
- Do not rename, refactor, or reformat anything the removal does not require. No opportunistic cleanups.

---

### Task 1: Frontend removal

**Files:**
- Delete: `web/src/react/pages/admin/AIStatusTab.tsx`
- Modify: `web/src/react/pages/admin/StatsTab.tsx:1,10-13`
- Modify: `web/src/react/queries/useAdminQueries.ts:85-93`
- Modify: `web/src/react/queries/queryKeys.ts:30`
- Modify: `web/src/js/api/admin.ts:5,23,86-88`
- Modify: `web/src/types/apiStatus.ts:57-87`
- Modify: `web/tests/screenshot-all-pages.spec.ts:39-42`
- Modify: `web/src/react/pages/admin/AIPricingTab.tsx:107`

**Interfaces:**
- Consumes: nothing. This task touches no Go file and is independent of all other tasks in this plan.
- Produces (these names no longer exist after this task, so later tasks and any reviewer can rely on their absence):
  - Component `AIStatusTab` (and its file)
  - Hook `useAIUsage` from `web/src/react/queries/useAdminQueries.ts`
  - Query key `queryKeys.admin.aiUsage`
  - API method `getAIUsage` (both the `declare module` interface entry and the `proto` implementation)
  - Types `AISummary`, `AIOperationSummary`, `AIUsageResponse` from `web/src/types/apiStatus.ts`
  - The Playwright route mock for `**/api/advisor/**`

Every site below was verified by reading the file in the worktree; the line numbers and quoted content are current as of the branch point. There are **no other** references in `web/` — a grep for `AIStatusTab`, `useAIUsage`, `getAIUsage`, `ai-usage`, `aiUsage`, `AIUsageResponse`, `AIOperationSummary`, `AISummary`, and `advisor` across `web/src`, `web/tests`, and the root config files returns exactly the sites listed here. There is no test file for `StatsTab` or `AIStatusTab`.

- [ ] **Step 1: Delete the AI status panel component**

`web/src/react/pages/admin/AIStatusTab.tsx` is the only consumer of the `/api/admin/ai-usage` endpoint. Delete it outright:

```bash
git rm web/src/react/pages/admin/AIStatusTab.tsx
```

- [ ] **Step 2: Unwire `AIStatusTab` from the admin Stats tab**

Edit `web/src/react/pages/admin/StatsTab.tsx`. Remove the import on line 1 and the entire `<section>` wrapper that renders it (lines 10-14, including the trailing `<hr />` separator, so the remaining sections are not left with a leading rule).

Replace the whole file with:

```tsx
import { DHStatsPanel } from './DHStatsPanel';
import { CLStatsPanel, PSAStatsPanel } from './ProviderStatsPanel';
import { IntegrationHealthStrip } from './IntegrationHealthStrip';

export function StatsTab({ enabled = true }: { enabled?: boolean }) {
  return (
    <div className="space-y-8 mt-4">
      <IntegrationHealthStrip enabled={enabled} />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">DoubleHolo</h3>
        <DHStatsPanel enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Card Ladder</h3>
        <CLStatsPanel enabled={enabled} />
      </section>
      <hr className="border-[var(--surface-2)]" />
      <section>
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">PSA Sync</h3>
        <PSAStatsPanel enabled={enabled} />
      </section>
    </div>
  );
}
```

- [ ] **Step 3: Remove the `useAIUsage` React Query hook**

Edit `web/src/react/queries/useAdminQueries.ts`. Delete this entire exported function (lines 85-93) along with the blank line that follows it. It sits between `usePricingDiagnostics` and `usePriceFlags`; leave both of those untouched.

```ts
export function useAIUsage(options?: AdminQueryOptions) {
  return useQuery({
    queryKey: queryKeys.admin.aiUsage,
    queryFn: () => api.getAIUsage(),
    refetchInterval: 60_000,
    staleTime: 30_000,
    enabled: options?.enabled ?? true,
  });
}
```

- [ ] **Step 4: Remove the now-orphaned query key**

`queryKeys.admin.aiUsage` had exactly one consumer, the hook deleted in Step 3. Edit `web/src/react/queries/queryKeys.ts` and delete line 30 from the `admin` block:

```ts
    aiUsage: ['admin', 'aiUsage'] as const,
```

The surrounding entries (`priceOverrideStats` above, `priceFlags` below) stay.

- [ ] **Step 5: Remove `getAIUsage` from the API client**

Edit `web/src/js/api/admin.ts`. Three edits in one file.

(a) Line 5 — drop `AIUsageResponse` from the type import list. Replace the line with:

```ts
import type { APIUsageResponse, PricingDiagnosticsResponse, PriceOverrideStats, DHStatusResponse, DHBulkMatchResponse, DHFixMatchRequest, DHFixMatchResponse, DHRetryMatchResponse, DHPushConfig, DHReconcileTriggerResult } from '../../types/apiStatus';
```

(b) Line 23 — delete this entry from the `declare module './client'` interface block:

```ts
    getAIUsage(): Promise<AIUsageResponse>;
```

(c) Lines 86-88 plus the blank line after — delete this implementation. It sits between `proto.getPriceOverrideStats` and `proto.getBackup`; both stay.

```ts
proto.getAIUsage = async function (this: APIClient): Promise<AIUsageResponse> {
  return this.get<AIUsageResponse>('/admin/ai-usage');
};
```

- [ ] **Step 6: Remove the AI usage response types**

Edit `web/src/types/apiStatus.ts`. Delete lines 57-87 — the section comment and all **three** interfaces, plus the blank lines separating them.

`AISummary` goes too: it was checked, and `AIUsageResponse.summary` (line 84) was its only consumer anywhere in `web/`. Nothing outside this deleted block references it.

Delete this whole block, leaving the `PriceOverrideStats` closing brace above it and the `/** DH integration status types */` comment below it:

```ts
/** AI usage status types for the /api/admin/ai-usage endpoint */

export interface AISummary {
  totalCalls: number;
  successRate: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalTokens: number;
  avgLatencyMs: number;
  rateLimitHits: number;
  callsLast24h: number;
  lastCallAt?: string;
  totalCostCents: number;
}

export interface AIOperationSummary {
  operation: string;
  calls: number;
  errors: number;
  successRate: number;
  avgLatencyMs: number;
  totalTokens: number;
  totalCostCents: number;
}

export interface AIUsageResponse {
  configured: boolean;
  summary: AISummary;
  operations: AIOperationSummary[];
  timestamp: string;
}
```

Note: `PriceOverrideStats` immediately above this block has fields named `aiAcceptedCount`, `pendingSuggestions`, and `suggestionTotalUsd`. Those belong to the price-override feature, which is out of scope — **do not touch them**.

- [ ] **Step 7: Remove the Playwright advisor route mock**

Edit `web/tests/screenshot-all-pages.spec.ts`. The mock existed only because the advisor SSE endpoints failed without a configured AI provider; the endpoints are being removed, so the mock is dead. Delete lines 39-42 and the blank line above them. The `**/api/auth/user` mock immediately above stays.

```ts
  // Mock advisor SSE endpoints — these fail without a configured AI provider
  await page.route('**/api/advisor/**', async (route) => {
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' });
  });
```

After the edit the enclosing helper ends with the closing `});` of the auth mock, followed by the function's own `}`.

- [ ] **Step 8: Fix the `AIPricingTab` empty-state copy**

`web/src/react/pages/admin/AIPricingTab.tsx` stays — it renders manual price *overrides*, a non-LLM feature. But its empty state at line 107 tells the user to run an advisor that will no longer exist. Replace that one line:

```tsx
          <p className="text-xs mt-1">Use the $ button on inventory cards to set overrides, or run the AI advisor for pricing suggestions.</p>
```

with:

```tsx
          <p className="text-xs mt-1">Use the $ button on inventory cards to set overrides.</p>
```

Change nothing else in this file. In particular the "Pending Suggestion Value" card (lines 92-98) and the `stats.pendingSuggestions === 0` condition on line 104 both stay: recorded suggestions remain reviewable, only their producer is going away.

- [ ] **Step 9: Confirm no references survive**

Run this grep from the repository root. It must return **no output**:

```bash
grep -rn "AIStatusTab\|useAIUsage\|getAIUsage\|ai-usage\|aiUsage\|AIUsageResponse\|AIOperationSummary\|AISummary" web/src web/tests
```

Then confirm the only remaining `advisor` mention in `web/` is gone too:

```bash
grep -rni "advisor" web/src web/tests
```

This must also return no output. (`AIPricingTab.tsx:107` was the last one, fixed in Step 8.)

- [ ] **Step 10: Verify the frontend build**

TypeScript will catch any dangling import or type reference the grep missed. Run:

```bash
cd web && npm run build
```

This must exit 0. If it reports an unused-import or missing-type error, fix the cited file before moving on — do not proceed with a red build.

- [ ] **Step 11: Verify the frontend tests**

```bash
cd web && npm test
```

This runs `vitest run`. It must exit 0 with zero failures. There is no test covering `StatsTab` or `AIStatusTab`, so the expected result is that the pre-existing suite is unchanged — same test count minus nothing, all passing.

- [ ] **Step 12: Commit the frontend removal**

Stage exactly the files this task touched and commit:

```bash
git add \
  web/src/react/pages/admin/AIStatusTab.tsx \
  web/src/react/pages/admin/StatsTab.tsx \
  web/src/react/pages/admin/AIPricingTab.tsx \
  web/src/react/queries/useAdminQueries.ts \
  web/src/react/queries/queryKeys.ts \
  web/src/js/api/admin.ts \
  web/src/types/apiStatus.ts \
  web/tests/screenshot-all-pages.spec.ts

git commit -m "refactor(web): remove the AI usage panel and advisor test mock

Deletes AIStatusTab and everything that fed it: the useAIUsage hook, the
queryKeys.admin.aiUsage key, the getAIUsage API method, and the AISummary /
AIOperationSummary / AIUsageResponse types. Also drops the Playwright mock for
the advisor SSE endpoints and the advisor instruction in the AIPricingTab empty
state.

Price overrides and recorded AI price suggestions are unaffected."
```

Confirm the working tree is clean for `web/` afterwards with `git status --short web/`.

---

### Task 2: HTTP surface removal

**Files:**
- Delete: `internal/adapters/httpserver/handlers/advisor.go`
- Delete: `internal/adapters/httpserver/handlers/advisor_test.go`
- Delete: `internal/adapters/httpserver/handlers/ai_status_handler.go`
- Modify: `internal/adapters/httpserver/routes.go:57-59`, `:208-217`
- Modify: `internal/adapters/httpserver/router.go:35-36`, `:73-74`, `:148-154`, `:266-267`
- Modify: `cmd/slabledger/server.go:47-48`, `:192-193`
- Modify: `cmd/slabledger/handlers.go:13-14`, `:55-57`, `:73-76`, `:221-232`, `:271-272`, `:342-345`
- Modify: `cmd/slabledger/main.go:324`, `:429-431`

**Interfaces:**
- Consumes: nothing from earlier tasks. Task 1 (frontend) is independent — it removes
  the browser-side caller of `GET /api/admin/ai-usage`; this task removes the server
  side. Neither blocks the other.
- Produces (gone after this task, so later tasks must not reference them):
  - `handlers.AdvisorHandler`, `handlers.NewAdvisorHandler`, its `HandleDigest` and
    `HandleLiquidationAnalysis` methods
  - `handlers.AIStatusHandler`, `handlers.NewAIStatusHandler`, `HandleAIUsage`
  - `httpserver.Router.registerAdvisorRoutes`, the `advisorHandler` / `aiUsageHandler`
    Router fields, and the `RouterConfig.AdvisorHandler` / `RouterConfig.AIStatusHandler`
    config fields
  - `ServerDependencies.AdvisorHandler`, `ServerDependencies.AIStatusHandler`
  - `handlerInputs.AdvisorService`, `handlerInputs.AzureAIClient`, `handlerInputs.AICallRepo`
  - `handlerOutputs.AdvisorHandler`
- Still alive after this task (Task 4 removes them): the `initializeAdvisorService`
  function itself, the `advisorService` and `aiCallRepo` locals in
  `cmd/slabledger/main.go`, and the `schedulerDeps.AdvisorService` /
  `schedulerDeps.AICallRepo` fields they feed.

---

- [ ] **Step 1: Delete the three handler files**

`ai_status_handler.go` has no test file; `advisor.go` has `advisor_test.go`, which is
deleted with it.

```bash
git rm internal/adapters/httpserver/handlers/advisor.go \
       internal/adapters/httpserver/handlers/advisor_test.go \
       internal/adapters/httpserver/handlers/ai_status_handler.go
```

- [ ] **Step 2: Remove the `/api/admin/ai-usage` route from the admin block**

In `internal/adapters/httpserver/routes.go`, inside `registerAdminRoutes`, delete this
block (currently at `:57-59`, sitting between the `price-override-stats` block and the
`price-flags` block):

```go
		if rt.aiUsageHandler != nil {
			mux.Handle("GET /api/admin/ai-usage", rt.authMW.RequireAdmin(http.HandlerFunc(rt.aiUsageHandler.HandleAIUsage)))
		}
```

- [ ] **Step 3: Delete the `registerAdvisorRoutes` method**

Still in `internal/adapters/httpserver/routes.go`, delete the whole method plus its
doc comment (currently `:208-217`), which sits between `registerCampaignRoutes` and
`registerPricingAPIRoutes`:

```go
// registerAdvisorRoutes wires the AI advisor endpoints.
func (rt *Router) registerAdvisorRoutes(mux *http.ServeMux) {
	if rt.advisorHandler == nil || rt.authMW == nil {
		return
	}
	mux.Handle("POST /api/advisor/digest", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleDigest)))
	mux.Handle("POST /api/advisor/liquidation-analysis", rt.authMW.RequireAuth(http.HandlerFunc(rt.advisorHandler.HandleLiquidationAnalysis)))
	mux.HandleFunc("/advisor", rt.spaHandler.HandleIndex)
	rt.logger.Info(context.Background(), "AI advisor routes registered")
}
```

Leave the blank line separating the two surviving neighbours. Do **not** touch the
`"context"` import in this file — the other `register*` methods still call
`context.Background()`.

- [ ] **Step 4: Delete the call site of `registerAdvisorRoutes`**

The method is called from `Setup()` in `internal/adapters/httpserver/router.go`
(currently `:266-267`). Deleting the method without deleting the call does not compile.
Remove the comment and the call:

```go
	// AI Advisor routes
	rt.registerAdvisorRoutes(mux)

```

so that `registerCampaignRoutes` is followed directly by:

```go
	// Arbitrage opportunities routes
	rt.registerOpportunitiesRoutes(mux)
```

- [ ] **Step 5: Remove the two Router struct fields**

In `internal/adapters/httpserver/router.go`, in the `Router` struct (currently `:35-36`),
delete:

```go
	advisorHandler            *handlers.AdvisorHandler
	aiUsageHandler            *handlers.AIStatusHandler
```

- [ ] **Step 6: Remove the two RouterConfig fields**

Same file, in `RouterConfig` (currently `:73-74`), delete:

```go
	AdvisorHandler            *handlers.AdvisorHandler       // AI advisor; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler      // AI usage stats; nil = disabled
```

- [ ] **Step 7: Remove the two assignments in `NewRouter`**

Same file (currently `:148-155`), delete both blocks and the blank line between them:

```go
	if cfg.AdvisorHandler != nil {
		rt.advisorHandler = cfg.AdvisorHandler
	}

	if cfg.AIStatusHandler != nil {
		rt.aiUsageHandler = cfg.AIStatusHandler
	}

```

The local-API-token block above and the `cfg.PriceFlagsHandler` block below stay
separated by a single blank line.

- [ ] **Step 8: Remove the two `ServerDependencies` fields**

In `cmd/slabledger/server.go` (currently `:47-48`), delete:

```go
	AdvisorHandler            *handlers.AdvisorHandler         // AI advisor; nil = disabled
	AIStatusHandler           *handlers.AIStatusHandler        // AI usage stats; nil = disabled
```

- [ ] **Step 9: Remove the two router-config assignments in `server.go`**

Same file, in the `httpserver.RouterConfig{...}` literal (currently `:192-193`), delete:

```go
		AdvisorHandler:            deps.AdvisorHandler,
		AIStatusHandler:           deps.AIStatusHandler,
```

- [ ] **Step 10: Remove the three `handlerInputs` fields**

In `cmd/slabledger/handlers.go` (currently `:55-57`), delete:

```go
	AdvisorService       advisor.Service
	AzureAIClient        advisor.LLMProvider
	AICallRepo           *postgres.AICallRepository
```

Note: `schedulerDeps` in `cmd/slabledger/init_schedulers.go` has its *own*
`AdvisorService` and `AICallRepo` fields. Those are a different struct and are Task 4's
job — do not touch that file here.

- [ ] **Step 11: Remove `AdvisorHandler` from `handlerOutputs`**

Same file (currently `:73-76`). The struct becomes:

```go
type handlerOutputs struct {
	DHHandler *handlers.DHHandler
}
```

`hOut.AdvisorHandler` has **no consumer** — `cmd/slabledger/shutdown.go:47-49` only
waits on `hOut.DHHandler`. Nothing else to remove for this field.

- [ ] **Step 12: Remove the advisor and AI-status handler construction**

Same file (currently `:221-232`), delete both blocks, which sit between the pricing
diagnostics handler and the price flags handler:

```go
	// Advisor handler (if advisor was initialized)
	var advisorHandler *handlers.AdvisorHandler
	if in.AdvisorService != nil {
		advisorHandler = handlers.NewAdvisorHandler(in.AdvisorService, logger)
	}

	// AI status handler — only wire tracker when an LLM provider is configured
	var aiTracker ai.AICallTracker
	if in.AzureAIClient != nil {
		aiTracker = in.AICallRepo
	}
	aiStatusHandler := handlers.NewAIStatusHandler(aiTracker, logger)

```

- [ ] **Step 13: Remove the two `deps` assignments**

Same file, in the `deps := ServerDependencies{...}` literal (currently `:271-272`),
delete:

```go
			AdvisorHandler:            advisorHandler,
			AIStatusHandler:           aiStatusHandler,
```

(keep the surrounding `PricingAPIKey:` and `PriceFlagsHandler:` lines).

- [ ] **Step 14: Shrink the `handlerOutputs` literal**

Same file (currently `:342-345`). Replace:

```go
	out := handlerOutputs{
		DHHandler:      dhHandler,
		AdvisorHandler: advisorHandler,
	}
```

with:

```go
	out := handlerOutputs{
		DHHandler: dhHandler,
	}
```

- [ ] **Step 15: Drop the now-unused `advisor` and `ai` imports from `handlers.go`**

`internal/domain/advisor` was referenced only by the two fields deleted in Step 10;
`internal/domain/ai` only by `ai.AICallTracker` deleted in Step 12. Delete these two
lines from the import block (currently `:13-14`):

```go
	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/ai"
```

Leave every other import — `postgres` is still used elsewhere in the file.

- [ ] **Step 16: Discard the `azureAIClient` return value in `main.go`**

This is the subtle one. In `cmd/slabledger/main.go` (currently `:324`):

```go
	azureAIClient, advisorService, err := initializeAdvisorService(
```

`azureAIClient` is consumed *only* by the `AzureAIClient:` line in the `handlerInputs`
literal that Step 17 deletes. An assigned-but-unused local is a **compile error** in Go,
not a warning, so the two edits must land together. Change the line to:

```go
	_, advisorService, err := initializeAdvisorService(
```

Do **not** delete anything else here. `advisorService` and `aiCallRepo` remain live —
they still feed `schedulerDeps` at `:386-387`, and `initializeAdvisorService` itself
still exists. Task 4 removes those.

- [ ] **Step 17: Remove the three `handlerInputs` literal entries in `main.go`**

Same file, in the `createHandlers(ctx, handlerInputs{...})` literal (currently
`:429-431`), delete:

```go
			AdvisorService:       advisorService,
			AzureAIClient:        azureAIClient,
			AICallRepo:           aiCallRepo,
```

The `DemandRepo:` line above and the `CLClient:` line below stay.

- [ ] **Step 18: Confirm no reference survives**

Run this; it must print nothing:

```bash
grep -rn "advisorHandler\|aiUsageHandler\|AdvisorHandler\|AIStatusHandler\|HandleAIUsage\|registerAdvisorRoutes\|/api/advisor\|/api/admin/ai-usage\|AzureAIClient" \
  --include='*.go' cmd/ internal/adapters/httpserver/
```

Verified before writing this plan: `internal/adapters/httpserver/router_test.go` contains
no advisor or ai-usage assertions (its only tests are
`TestRoutes_GetCampaigns_Returns200` and `TestRoutes_GetHealth_Returns200`), and there is
no route-inventory test anywhere that enumerates registered routes. If the grep above
does surface a test, update it in this task rather than deferring — the tree must be
green at this commit.

- [ ] **Step 19: Re-format**

Removing fields from aligned struct definitions and literals changes gofmt's alignment
columns. Run:

```bash
gofmt -w cmd/slabledger/handlers.go cmd/slabledger/main.go cmd/slabledger/server.go \
         internal/adapters/httpserver/router.go internal/adapters/httpserver/routes.go
gofmt -l ./cmd ./internal
```

The second command must print nothing.

- [ ] **Step 20: Build**

```bash
go build ./...
```

Expected: clean, no output. The `advisor`, `ai`, `advisortool`, `scoring`, and `azureai`
packages still exist and still compile — Go does not error on a package with no importer.

- [ ] **Step 21: Test**

```bash
go test -race ./...
```

Expected: all packages pass, 0 failures. The `internal/adapters/httpserver/handlers`
package now has three fewer files and one fewer test file; `mocks.MockAdvisorService`
becomes unreferenced but still compiles (Task 5 deletes it).

- [ ] **Step 22: Commit**

```bash
git add internal/adapters/httpserver/handlers/advisor.go \
        internal/adapters/httpserver/handlers/advisor_test.go \
        internal/adapters/httpserver/handlers/ai_status_handler.go \
        internal/adapters/httpserver/routes.go \
        internal/adapters/httpserver/router.go \
        cmd/slabledger/server.go \
        cmd/slabledger/handlers.go \
        cmd/slabledger/main.go
git commit -m "refactor(http): remove the AI advisor and AI usage endpoints

Deletes the advisor and AI status handlers, the POST /api/advisor/digest,
POST /api/advisor/liquidation-analysis, and GET /api/admin/ai-usage routes,
the /advisor SPA route, and their wiring through the router config and
handler construction. The advisor service and AI call repository stay
constructed for now; the scheduler still consumes them."
```

---

### Task 3: CLI removal

**Files:**
- Delete: `cmd/slabledger/admin_analyze.go`
- Modify: `cmd/slabledger/admin.go:3-8`, `:16`, `:24-25`, `:44-48`, `:52-54`

**Interfaces:**
- Consumes: nothing. Independent of Task 2 — they touch disjoint sites in
  `cmd/slabledger/` (`admin.go` / `admin_analyze.go` vs. `handlers.go` / `main.go` /
  `server.go`). They can be done in either order.
- Produces (gone after this task): `adminAnalyze`, `parseAnalyzeFlags`, `analyzeFlags`,
  `buildStreamCallback`, `maskString`, and the `slabledger admin analyze` subcommand.
- **Ordering constraint for Task 4:** `admin_analyze.go:145` is the second of exactly two
  callers of `initializeAdvisorService` (the other is `cmd/slabledger/main.go:324`).
  Task 4 deletes that function, so this task must land first or Task 4's tree will not
  compile.

---

- [ ] **Step 1: Delete the analyze CLI file**

Every symbol in it is file-local. Verified: `maskString`, `parseAnalyzeFlags`,
`analyzeFlags`, and `buildStreamCallback` have no reference anywhere else in the repo,
and there is no test file for the admin CLI.

```bash
git rm cmd/slabledger/admin_analyze.go
```

- [ ] **Step 2: Remove the `analyze` case from the command switch**

In `cmd/slabledger/admin.go`, in `handleAdminCommand` (currently `:24-25`), delete:

```go
	case "analyze":
		return adminAnalyze(ctx, args[1:])
```

- [ ] **Step 3: Remove the now-orphaned `ctx` local**

`ctx` was read only by the `analyze` case. The three surviving cases (`version`,
`print-config`, `help`) do not use it, and an assigned-but-unused local is a compile
error in Go. Delete this line (currently `:16`) along with the blank line after it:

```go
	ctx := context.Background()

```

`handleAdminCommand` then reads:

```go
func handleAdminCommand(args []string) error {
	if len(args) < 1 {
		return showAdminHelp()
	}

	switch args[0] {
	case "version":
		config.PrintVersion()
		return nil
	case "print-config":
		return adminPrintConfig(args[1:])
	case "help", "--help", "-h":
		return showAdminHelp()
	default:
		return fmt.Errorf("unknown admin command: %s\n\nRun 'slabledger admin help' for usage", args[0])
	}
}
```

- [ ] **Step 4: Drop the now-unused `context` import**

`context.Background()` was the file's only use of the package. The import block
(currently `:3-8`) becomes:

```go
import (
	"fmt"

	"github.com/guarzo/slabledger/internal/platform/config"
)
```

- [ ] **Step 5: Remove the AI Advisor section from the help text**

In `showAdminHelp`, inside the raw-string literal, delete this block including the blank
line that follows it (currently `:44-48`):

```
    AI Advisor:
        analyze <type>           Run an advisor analysis locally
                                 Types: liquidation, digest
                                 Flags: --verbose, --dry-run

```

so `Configuration:` is followed directly by `    Help:`.

- [ ] **Step 6: Remove the usage example**

Same literal (currently `:52-54`). Replace:

```
EXAMPLES:
    slabledger admin print-config
    slabledger admin analyze liquidation --verbose`)
```

with:

```
EXAMPLES:
    slabledger admin print-config`)
```

Note the closing backtick-and-paren must move to the surviving last line — dropping it
is a syntax error.

- [ ] **Step 7: Confirm no reference survives**

Both commands must print nothing:

```bash
grep -rn "adminAnalyze\|parseAnalyzeFlags\|buildStreamCallback\|maskString\|analyzeFlags" --include='*.go' .
grep -rn "admin analyze" --include='*.go' .
```

- [ ] **Step 8: Build**

```bash
go build ./...
gofmt -l ./cmd
```

Expected: both clean, no output.

- [ ] **Step 9: Test**

```bash
go test -race ./...
```

Expected: all packages pass, 0 failures. No test covers the admin CLI, so this task
removes no test coverage.

- [ ] **Step 10: Verify the CLI still works**

```bash
go run ./cmd/slabledger admin help
```

Expected: the help text prints with the `AI Advisor` section gone and only
`slabledger admin print-config` under `EXAMPLES:`. Then:

```bash
go run ./cmd/slabledger admin analyze liquidation
```

Expected: `unknown admin command: analyze` followed by the usage hint.

- [ ] **Step 11: Commit**

```bash
git add cmd/slabledger/admin_analyze.go cmd/slabledger/admin.go
git commit -m "refactor(cli): remove the 'admin analyze' advisor subcommand

Deletes admin_analyze.go and unwires the subcommand from the admin
dispatcher, its help text, and its usage example. This clears the second
of two callers of initializeAdvisorService, which the construction unwire
removes next."
```

---

### Task 4: Construction and scheduler unwire

This task removes the last code that *constructs* the advisor stack. After it,
the six advisor/AI/scoring packages still exist on disk but nothing outside
themselves imports them. That is legal Go — only unused *imports* and unused
*local variables* are compile errors, an unreferenced package is not — which is
why deleting the packages is deferred to Task 5. Keeping the two apart means
this commit builds and tests green on its own.

**Files:**
- Modify: `cmd/slabledger/init_services.go:3-6` (top-of-file comment), `:8-26` (imports), `:49-97` (delete `initializeAdvisorService`)
- Modify: `cmd/slabledger/main.go:24` and `:29` (imports), `:308-333` (AI call repo, gap store, advisor tool opts, advisor init call), `:386-387` and `:402` (`schedulerDeps` literal fields)
- Modify: `cmd/slabledger/init_schedulers.go:11` (import), `:38-39` and `:54` (struct fields), `:75` (`buildDeps` assignment), `:197-199` (`GapStore` propagation)
- Modify: `internal/adapters/scheduler/builder.go:9` and `:19` (imports), `:43-44` and `:84-85` (`BuildDeps` fields), `:173-176` (gap cleanup scheduler registration)
- Delete: `internal/adapters/scheduler/gap_cleanup.go`
- Delete: `internal/adapters/storage/postgres/ai_tracker.go`
- Delete: `internal/adapters/storage/postgres/gap_store.go`

**Interfaces:**
- Consumes: Task 2 removed the `handlerInputs.AdvisorService` / `AzureAIClient` / `AICallRepo` fields from `cmd/slabledger/handlers.go` and rewrote `cmd/slabledger/main.go:324` to start with `_,`. Task 3 deleted `cmd/slabledger/admin_analyze.go`, the second caller of `initializeAdvisorService`. So when this task starts, `initializeAdvisorService` has exactly one caller left: `cmd/slabledger/main.go`.
- Produces: `initializeAdvisorService` is gone. `postgres.AICallRepository`, `postgres.NewAICallRepository`, `postgres.GapStore`, `postgres.NewGapStore`, `scheduler.GapCleanupScheduler`, `scheduler.NewGapCleanupScheduler`, `scheduler.BuildDeps.AICallTracker`, and `scheduler.BuildDeps.GapStore` are gone. After this task the only remaining references to `internal/adapters/clients/azureai`, `internal/domain/advisor`, `internal/domain/ai`, `internal/adapters/advisortool`, `internal/adapters/scoring`, and `internal/domain/scoring` are (a) from inside those six packages themselves and (b) from `internal/testutil/mocks/advisor_service.go` and `internal/testutil/mocks/gap_store.go`. Task 5 deletes all of that.

---

- [ ] **Step 1: Confirm the shared locals in `main.go` have other consumers**

`cmd/slabledger/main.go:311-323` builds `advisorToolOpts` from seven locals that
are *also* used elsewhere in the file. If any of them turned out to be
advisor-only, deleting this block would leave a `declared and not used` compile
error. Verify before deleting, not after:

```bash
cd /workspace/.worktrees/remove-azure-ai
for v in intelRepo suggestionsRepo arbSvc portSvc tuningSvc financeService exportService; do
  echo "=== $v ==="
  grep -n "\b$v\b" cmd/slabledger/main.go
done
```

Expected: every one of the seven has at least one hit outside the `311-333`
range — each is passed to `handlerInputs` around `:417-427`, and `intelRepo`,
`suggestionsRepo` also appear in the `schedulerDeps` literal. Confirmed on the
branch point: none of the seven is advisor-only, so all seven survive untouched.
If a future rebase changes that, the variable's declaration must be deleted in
this task too.

Note that `tuningSvc` appears twice inside the block being deleted
(`advisortool.WithTuningService` at `:319` and `scoringadapter.WithTuningService`
at `:327`) — both go, and `:419` keeps it alive.

- [ ] **Step 2: Confirm `AICallTracker` is already a dead field in the scheduler package**

`scheduler.BuildDeps.AICallTracker` is declared and assigned from `main`, but
nothing in the scheduler package ever reads it. Verify:

```bash
grep -rn 'AICallTracker' internal/adapters/scheduler/
```

Expected: exactly one hit, the declaration at `internal/adapters/scheduler/builder.go:44`.
Any other hit means a scheduler grew a real dependency on it since this plan was
written — stop and re-scope. (Verified at planning time: one hit.)

- [ ] **Step 3: Confirm the postgres and scheduler files being deleted have no test files and no other callers**

```bash
ls internal/adapters/storage/postgres/ | grep -E 'ai_tracker|gap_store'
ls internal/adapters/scheduler/ | grep gap_cleanup
grep -rn 'NewAICallRepository\|AICallRepository\|NewGapStore\|GapStore\|NewGapCleanupScheduler' \
  --include='*.go' . | grep -v '^./internal/domain/'
grep -rn 'BuildDeps\|scheduler.Build(' --include='*_test.go' .
```

Expected, verified at planning time:
- `ai_tracker.go` and `gap_store.go` only — **no** `ai_tracker_test.go`, **no** `gap_store_test.go`.
- `gap_cleanup.go` only — no `gap_cleanup_test.go`.
- The third grep hits only the files this task edits or deletes, plus
  `internal/testutil/mocks/gap_store.go` (`MockGapStore`, which Task 5 deletes and
  which has **zero** users — no test constructs it).
- The fourth grep returns **nothing**: no test builds a `BuildDeps` or asserts a
  scheduler count, so removing the gap-cleanup scheduler from `Build` breaks no
  assertion.

If a test file does exist for any of the three, it must be deleted in the same
step as its source or the package stops compiling.

- [ ] **Step 4: Delete `initializeAdvisorService` from `init_services.go`**

Delete `cmd/slabledger/init_services.go:49-97` in full:

```go
// initializeAdvisorService creates the Azure AI client and advisor service.
// All return values may be nil/zero if Azure AI is not configured. This is not
// an error.
func initializeAdvisorService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *postgres.DB,
	aiCallRepo *postgres.AICallRepository,
	campaignsService inventory.Service,
	scoringOpts []scoringadapter.ProviderOption,
	toolOpts ...advisortool.ExecutorOption,
) (llmProvider advisor.LLMProvider, advisorSvc advisor.Service, err error) {
	if cfg.Adapters.AzureAIEndpoint == "" || cfg.Adapters.AzureAIKey == "" {
		return nil, nil, nil
	}

	client, err := azureai.NewClient(azureai.Config{
		Endpoint:       cfg.Adapters.AzureAIEndpoint,
		APIKey:         cfg.Adapters.AzureAIKey,
		DeploymentName: cfg.Adapters.AzureAIDeployment,
	}, azureai.WithLogger(logger), azureai.WithCompletionTimeout(cfg.Adapters.AzureAICompletionTimeout))
	if err != nil {
		return nil, nil, fmt.Errorf("initialize azure ai client: %w", err)
	}
	llmProvider = client

	toolExec := advisortool.NewCampaignToolExecutor(campaignsService, toolOpts...)
	advisorOpts := []advisor.ServiceOption{
		advisor.WithLogger(logger),
		advisor.WithAITracker(aiCallRepo),
	}
	if cfg.AdvisorRefresh.MaxToolRounds > 0 {
		advisorOpts = append(advisorOpts, advisor.WithMaxToolRounds(cfg.AdvisorRefresh.MaxToolRounds))
	}

	// Scoring engine: pre-compute factor scores for advisor flows
	scoringProvider := scoringadapter.NewProvider(campaignsService, scoringOpts...)
	advisorOpts = append(advisorOpts, advisor.WithScoringDataProvider(scoringProvider))

	// Data gap tracking for scoring quality reports
	advisorOpts = append(advisorOpts, advisor.WithGapStore(postgres.NewGapStore(db.DB)))

	advisorSvc = advisor.NewService(client, toolExec, advisorOpts...)
	logger.Info(ctx, "AI advisor initialized",
		observability.String("deployment", cfg.Adapters.AzureAIDeployment))

	return llmProvider, advisorSvc, nil
}
```

Leave the blank line separating `initializePriceProviders` from
`initializeCardLadder` — the file goes from three functions to two.

- [ ] **Step 5: Fix the `init_services.go` imports and top-of-file comment**

Six imports become unused once Step 4 lands. `advisortool`, `azureai`,
`scoringadapter`, and `advisor` are obvious. **`fmt`, `inventory`, and `config`
are the non-obvious ones** — each was used *only* by `initializeAdvisorService`
(`fmt` at the old `:72`, `inventory.Service` and `*config.Config` in its
signature). `context`, `time`, `postgres`, `pricing`, `observability`,
`crypto`, `cardladder`, `dh`, and `dhprice` all survive.

Replace the header block (`:1-26`) with:

```go
package main

// init_services.go initializes optional external services: price providers and
// third-party integrations (Card Ladder). Core inventory/campaign services are
// initialized in init_inventory_services.go. Scheduler initialization is in
// init_schedulers.go.

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/platform/crypto"
)
```

The comment change also drops the stale "Market Movers" mention — that
integration was removed in migration `000030` and the comment was never updated.

- [ ] **Step 6: Remove the AI call repo, gap store, advisor tool opts, and advisor init call from `main.go`**

Delete `cmd/slabledger/main.go:308-333` — everything from the `// AI call
tracking` comment through the `if err != nil` block that follows the
`initializeAdvisorService` call. After Task 2, line `:324` reads `_,
advisorService, err := initializeAdvisorService(`:

```go
	// AI call tracking
	aiCallRepo := postgres.NewAICallRepository(db)

	// Build advisor tool options — inject intelligence repos.
	gapStore := postgres.NewGapStore(db.DB)
	advisorToolOpts := []advisortool.ExecutorOption{
		advisortool.WithIntelligenceRepo(intelRepo),
		advisortool.WithSuggestionsRepo(suggestionsRepo),
		advisortool.WithGapStore(gapStore),
		advisortool.WithArbitrageService(arbSvc),
		advisortool.WithPortfolioService(portSvc),
		advisortool.WithTuningService(tuningSvc),
		advisortool.WithFinanceService(financeService),
		advisortool.WithExportService(exportService),
	}

	_, advisorService, err := initializeAdvisorService(
		ctx, cfg, logger, db, aiCallRepo, campaignsService,
		[]scoringadapter.ProviderOption{
			scoringadapter.WithTuningService(tuningSvc),
		},
		advisorToolOpts...,
	)
	if err != nil {
		return err
	}
```

The `syncStateRepo` declaration immediately above (`:305-306`) stays; the
`// Initialize Card Ladder` block immediately below (`:335`) stays. Collapse the
resulting double blank line to one.

- [ ] **Step 7: Remove the three advisor fields from the `schedulerDeps` literal in `main.go`**

In the `sDeps := schedulerDeps{...}` literal (`:369-403` before this edit),
delete these three lines:

```go
		AdvisorService:             advisorService,
		AICallRepo:                 aiCallRepo,
```

and, near the end of the literal:

```go
		GapStore:                   gapStore,
```

`DHTombstoneStore: dhTombstoneStore,` becomes the last field. Leave the
surrounding fields and their gofmt alignment alone — gofmt will re-align the
whole literal in Step 13.

- [ ] **Step 8: Remove the `advisortool` and `scoringadapter` imports from `main.go`**

From the "Concrete implementations" import group, delete:

```go
	"github.com/guarzo/slabledger/internal/adapters/advisortool"
```

```go
	scoringadapter "github.com/guarzo/slabledger/internal/adapters/scoring"
```

Every other import in that group is still used. `postgres` in particular stays —
it is used dozens of times.

- [ ] **Step 9: Remove the advisor and gap-store fields from `schedulerDeps` in `init_schedulers.go`**

In `cmd/slabledger/init_schedulers.go`, delete these two struct fields (`:38-39`):

```go
	AdvisorService             advisor.Service
	AICallRepo                 *postgres.AICallRepository
```

and this one (`:54`):

```go
	GapStore                   *postgres.GapStore
```

Then delete the now-unused `advisor` import (`:11`):

```go
	"github.com/guarzo/slabledger/internal/domain/advisor"
```

`postgres` stays — it types a dozen other fields in the same struct.

- [ ] **Step 10: Remove the `AICallTracker` and `GapStore` propagation from `init_schedulers.go`**

In the `buildDeps := scheduler.BuildDeps{...}` literal, delete `:75`:

```go
		AICallTracker:              deps.AICallRepo,
```

Further down, delete the whole nil-guard at `:197-199`:

```go
	if deps.GapStore != nil {
		buildDeps.GapStore = deps.GapStore
	}
```

It sits between the DH push-relist block and the `// Wire PSA portal sync
(nil-safe)` block; those two stay.

- [ ] **Step 11: Remove the `AICallTracker` and `GapStore` fields and the gap-cleanup registration from `builder.go`**

In `internal/adapters/scheduler/builder.go`, delete from `BuildDeps` (`:43-44`):

```go
	// AI call tracking (used by various AI-related schedulers if present).
	AICallTracker ai.AICallTracker
```

and (`:84-85`):

```go
	// Scoring gap cleanup dependencies (optional)
	GapStore scoring.GapStore
```

In `Build`, delete the registration (`:173-176`):

```go
	// Scoring data gap cleanup scheduler (if gap store is provided)
	if deps.GapStore != nil {
		schedulers = append(schedulers, NewGapCleanupScheduler(deps.GapStore, deps.Logger))
	}
```

It sits between the access-log cleanup block and the `// Inventory snapshot
refresh scheduler` block; those stay. Then delete the two now-unused imports —
`:9` and `:19` were the only uses of each in the file:

```go
	"github.com/guarzo/slabledger/internal/domain/ai"
```

```go
	"github.com/guarzo/slabledger/internal/domain/scoring"
```

Also collapse the blank lines left behind so the struct does not end up with two
consecutive blank lines between field groups.

- [ ] **Step 12: Delete the three orphaned implementation files**

```bash
git rm internal/adapters/scheduler/gap_cleanup.go \
       internal/adapters/storage/postgres/ai_tracker.go \
       internal/adapters/storage/postgres/gap_store.go
```

`gap_cleanup.go` was the only other file in the scheduler package importing
`internal/domain/scoring`; `ai_tracker.go` was the only file in the postgres
package importing `internal/domain/ai`. Both packages now import neither.

- [ ] **Step 13: Format, build, test, and commit**

```bash
cd /workspace/.worktrees/remove-azure-ai
gofmt -w cmd/slabledger/main.go cmd/slabledger/init_services.go \
         cmd/slabledger/init_schedulers.go internal/adapters/scheduler/builder.go
go build ./...
go test -race -timeout 10m ./...
```

Run the whole tree, not just the three packages this task edited. The Global
Constraints require it, and this task removes a constructor and a scheduler job —
the callers that break are exactly the ones a narrow package selection would miss.

`go build ./...` must be clean. If it reports `declared and not used` for one of
the seven `main.go` locals, Step 1's assumption broke — go back and delete that
local's declaration too rather than adding a `_ = x`.

The six advisor packages still compile at this point (nothing was removed from
them); `go vet`/`go build` will not complain about them being unreferenced. That
is expected.

```bash
git commit -am "refactor: unwire advisor construction and gap-cleanup scheduler"
```

---

### Task 5: Delete the orphaned packages

Tasks 1-4 removed every reference to the advisor stack from outside itself. This
task deletes the packages. Nothing here changes behavior — if any step surfaces a
remaining importer, that is a defect in an earlier task, not something to work
around here.

**Files:**
- Delete: `internal/adapters/clients/azureai/` (6 files)
- Delete: `internal/domain/advisor/` (12 files)
- Delete: `internal/domain/ai/` (5 files)
- Delete: `internal/adapters/advisortool/` (8 files)
- Delete: `internal/adapters/scoring/` (2 files)
- Delete: `internal/domain/scoring/` (12 files)
- Delete: `internal/testutil/mocks/advisor_service.go`
- Delete: `internal/testutil/mocks/gap_store.go`
- Modify: `internal/testutil/mocks/README.md:86`
- Modify: `go.mod`, `go.sum` (via `go mod tidy`)

**Interfaces:**
- Consumes: Task 4's guarantee that the only remaining importers of these six packages are the packages themselves plus the two mock files deleted here.
- Produces: no LLM, advisor, or scoring code remains in the repository. `github.com/openai/openai-go/v3` leaves `go.mod`.

---

- [ ] **Step 1: Prove no importer remains outside the set being deleted**

Run this **before** deleting anything:

```bash
cd /workspace/.worktrees/remove-azure-ai
grep -rn --include='*.go' \
  -e 'internal/adapters/clients/azureai' \
  -e 'internal/domain/advisor' \
  -e 'internal/domain/ai"' \
  -e 'internal/adapters/advisortool' \
  -e 'internal/adapters/scoring' \
  -e 'internal/domain/scoring' \
  . | grep -v '^./internal/adapters/clients/azureai/' \
    | grep -v '^./internal/domain/advisor/' \
    | grep -v '^./internal/domain/ai/' \
    | grep -v '^./internal/adapters/advisortool/' \
    | grep -v '^./internal/adapters/scoring/' \
    | grep -v '^./internal/domain/scoring/'
```

**Note the trailing quote on `internal/domain/ai"`.** Without it the pattern is a
prefix match on `internal/domain/ai` and also matches `internal/domain/advisor`,
`internal/domain/arbitrage`, and every other `internal/domain/a…` import path,
burying the real signal in false positives. The closing quote pins it to the end
of an import string. The other five paths are unambiguous prefixes and need no
quote.

Expected result: **exactly two lines**, both from mocks deleted in Step 2:

```
./internal/testutil/mocks/gap_store.go:8:	"github.com/guarzo/slabledger/internal/domain/scoring"
./internal/testutil/mocks/advisor_service.go:6:	"github.com/guarzo/slabledger/internal/domain/advisor"
```

Any *other* line means an earlier task left a reference behind. Go fix that task
— do not delete the package anyway and do not patch the referring site here. A
reference from Task 1's frontend files or Task 6's config would mean those tasks
were skipped or reverted.

- [ ] **Step 2: Delete the six packages and the two mocks**

```bash
git rm -r internal/adapters/clients/azureai \
          internal/domain/advisor \
          internal/domain/ai \
          internal/adapters/advisortool \
          internal/adapters/scoring \
          internal/domain/scoring
git rm internal/testutil/mocks/advisor_service.go \
       internal/testutil/mocks/gap_store.go
```

That is 45 package files plus 2 mocks. `MockGapStore` (`gap_store.go`) had no
users at all even before this change; `MockAdvisorService` was used only by
`internal/adapters/httpserver/handlers/advisor_test.go`, deleted in Task 2.

- [ ] **Step 3: Update the mocks README**

`internal/testutil/mocks/README.md:86` lists the advisor mock in the service-mock
table. Remove this row:

```markdown
| `advisor.Service` | `MockAdvisorService` |
```

The rows above and below it (`dhlisting.Service` / `MockDHListingService` and
`social.Service` / `MockSocialService`) stay. Then confirm nothing else in the
README mentions the deleted mocks:

```bash
grep -n -iE 'advisor|gapstore|gap store|scoring|azure|llm' internal/testutil/mocks/README.md
```

Expected: no output. (Verified at planning time: the gap-store mock was never
documented, so line 86 is the only edit.)

- [ ] **Step 4: Tidy the module**

```bash
go mod tidy
git diff go.mod go.sum
```

`github.com/openai/openai-go/v3 v3.48.0` should leave `go.mod:10` — the spec
verified it has no importer outside `azureai`, and Step 2 deleted the only six
files that imported it (`azureai/client.go`, `azureai/stream.go`). `go.sum` will
shrink by that module and whatever it pulled in transitively.

Do not assume the diff — read it and report what actually moved. If `openai-go`
survives, something outside `azureai` imports it; find it with
`grep -rn 'openai-go' --include='*.go' .` before continuing. If a module you did
not expect drops out, check that it was genuinely advisor-only.

- [ ] **Step 5: Run the import checker on its own, before `make check`**

```bash
./scripts/check-imports.sh
```

Run it standalone first rather than waiting for `make check` to reach it. The
checker derives the inventory-sibling set from the package tree and **fails
closed** if fewer than two siblings are derived, or if the scan performs an
unexpected number of checks. This task deletes six packages in one commit and is
by far the most likely point in the whole plan to trip that invariant.

It should pass. The spec verified `internal/domain/scoring` is *not* an inventory
sibling — it does not import `internal/domain/inventory` — so removing it cannot
change the derived count of ten (arbitrage, demand, dhlisting, dhpricing, export,
finance, portfolio, pricing/lookup, psacampaign, tuning). If the checker fails
with a "fewer than two siblings" or unexpected-check-count error, do not adjust
the script; re-derive the set and find out what actually changed:

```bash
grep -rl --include='*.go' 'internal/domain/inventory' internal/domain \
  | grep -v _test.go | xargs -n1 dirname | sort -u
```

- [ ] **Step 6: Build, test, and commit**

```bash
go build ./...
go test -race ./...
```

Both must be clean, with zero test failures — the spec's baseline. `go test
-race ./...` here is the full suite, not a subset: this is the commit where the
package set changes shape, so a package that silently depended on one of the six
through a test-only import would surface now and nowhere else.

```bash
git add -A
git commit -m "refactor: delete the advisor, AI, and scoring packages"
```

---

### Task 6: Configuration removal

**Files:**
- Modify: `internal/platform/config/types.go:101-103,106` (four `AzureAI*` fields on `AdapterConfig`)
- Modify: `internal/platform/config/types.go:161` (`AdvisorRefresh` field on the parent `Config` struct)
- Modify: `internal/platform/config/types.go:232-237` (`AdvisorRefreshConfig` doc comment + struct)
- Modify: `internal/platform/config/defaults.go:76-78` (`AdvisorRefresh` default block)
- Modify: `internal/platform/config/loader.go:167-169` (`ADVISOR_MAX_TOOL_ROUNDS` parsing + section comment)
- Modify: `internal/platform/config/loader.go:179-183,187` (`AZURE_AI_*` parsing)
- Modify: `.env.example:51-68` (the whole `AI — Azure OpenAI (Advisor)` section)

**Interfaces:**
- Consumes: Tasks 2–5 must have landed. Every remaining reader of these fields lives in
  code those tasks delete: `cmd/slabledger/init_services.go:62-94`
  (`initializeAdvisorService`, reads all four `AzureAI*` fields plus
  `cfg.AdvisorRefresh.MaxToolRounds`), `cmd/slabledger/main.go:324,430`,
  `cmd/slabledger/handlers.go:56,229`, and `cmd/slabledger/admin_analyze.go:77-78,156`
  (the `analyze` CLI prints `AzureAIEndpoint`/`AzureAIDeployment` in its dry-run branch —
  that whole file is the advisor CLI and is deleted by **Task 3**, so there is **no**
  admin-print edit in this task). `initializeAdvisorService` itself is deleted by
  **Task 4**. Running Task 6 first breaks the build.
- Produces: `internal/platform/config` no longer knows Azure or the advisor exists.
  `AdapterConfig` loses four fields, `Config` loses `AdvisorRefresh`,
  `AdvisorRefreshConfig` no longer exists, and `AZURE_AI_ENDPOINT`,
  `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`, `AZURE_AI_TIMEOUT`, and
  `ADVISOR_MAX_TOOL_ROUNDS` are no longer read by the binary or documented in
  `.env.example`.

**Verified before writing this task:**
- `internal/platform/config/validation.go` has **no** Azure/advisor reference — nothing to change there.
- `internal/platform/config/*_test.go` has **no** Azure/advisor reference — no test updates needed in this task.
- There is no `admin_print_config.go`; the only config-printing site is `admin_analyze.go` (deleted by Task 5).
- `time` stays imported in `types.go`: 35 other `time.` uses remain after `AzureAICompletionTimeout` goes (e.g. `ReadTimeout` at `:18`, `OrdersPollInterval` at `:114`).

---

- [ ] **Step 1: Remove the four `AzureAI*` fields from `AdapterConfig`**

In `internal/platform/config/types.go`, replace the `AdapterConfig` struct body. Deleting
`AzureAICompletionTimeout` removes the longest field name in the block, so gofmt
re-aligns every remaining field — paste the replacement exactly rather than deleting
lines in place.

Delete these four lines (`:101-103` and `:106`):

```go
	AzureAIEndpoint          string        // AZURE_AI_ENDPOINT - Azure AI Foundry endpoint URL
	AzureAIKey               string        // AZURE_AI_API_KEY - Azure AI API key
	AzureAIDeployment        string        // AZURE_AI_DEPLOYMENT - Model deployment name (default: gpt-5.4)
	AzureAICompletionTimeout time.Duration // AZURE_AI_TIMEOUT - Completion poll fallback timeout (default: 3m)
```

so the struct body reads exactly:

```go
type AdapterConfig struct {
	PSAToken        string // PSA_ACCESS_TOKEN - PSA cert lookup (comma-separated for rotation)
	PricingAPIKey   string // PRICING_API_KEY - Bearer token for pricing API auth
	GoogleOAuthEnv  string // GOOGLE_OAUTH_ENV - controls login button visibility ("production" shows it)
	LocalAPIToken   string // LOCAL_API_TOKEN - dev-mode bearer bypass; empty = disabled
	DHEnterpriseKey string // DH_ENTERPRISE_API_KEY - Bearer token for enterprise endpoints
	DHBaseURL       string // DH_API_BASE_URL
}
```

- [ ] **Step 2: Remove the `AdvisorRefresh` field from the parent `Config` struct**

In `internal/platform/config/types.go:161`, delete this single line:

```go
	AdvisorRefresh     AdvisorRefreshConfig
```

The surrounding fields keep their alignment — the column is set by
`DHAnalyticsRefresh` (`:165`), which is longer and stays.

- [ ] **Step 3: Delete the `AdvisorRefreshConfig` type**

In `internal/platform/config/types.go:232-237`, delete the doc comment and struct
together, plus the blank line separating it from `SnapshotEnrichConfig`:

```go
// AdvisorRefreshConfig configures the on-demand AI advisor service. The
// background refresh scheduler was removed when the /insights overview was
// retired; only the per-call tool-loop bound remains.
type AdvisorRefreshConfig struct {
	MaxToolRounds int // max LLM tool-calling rounds per analysis (default: 5)
}
```

- [ ] **Step 4: Remove the `AdvisorRefresh` default block**

In `internal/platform/config/defaults.go:76-78`, delete these three lines from the
`Default()` composite literal:

```go
		AdvisorRefresh: AdvisorRefreshConfig{
			MaxToolRounds: 5, // hard cap; prompt guides LLM to 2 rounds, service default is 3, 5 is safety margin
		},
```

The `SnapshotEnrich` block above it and the `CardLadder` block below it are unchanged.

- [ ] **Step 5: Remove `ADVISOR_MAX_TOOL_ROUNDS` parsing from the loader**

In `internal/platform/config/loader.go:167-169`, delete the section comment, the parse
call, and the trailing blank line:

```go
	// Advisor service
	envIntPositive("ADVISOR_MAX_TOOL_ROUNDS", &cfg.AdvisorRefresh.MaxToolRounds)

```

`envIntPositive` is still used elsewhere (e.g. `SNAPSHOT_ENRICH_BATCH_SIZE` at `:163`),
so no helper becomes dead.

- [ ] **Step 6: Remove `AZURE_AI_*` parsing from the loader**

In `internal/platform/config/loader.go`, delete the three env reads and the deployment
default (`:179-183`):

```go
	cfg.Adapters.AzureAIEndpoint = os.Getenv("AZURE_AI_ENDPOINT")
	cfg.Adapters.AzureAIKey = os.Getenv("AZURE_AI_API_KEY")
	cfg.Adapters.AzureAIDeployment = os.Getenv("AZURE_AI_DEPLOYMENT")
	if cfg.Adapters.AzureAIDeployment == "" {
		cfg.Adapters.AzureAIDeployment = "gpt-5.4"
	}
```

and the timeout parse (`:187`):

```go
	envDurationPositive("AZURE_AI_TIMEOUT", &cfg.Adapters.AzureAICompletionTimeout)
```

After both deletions the adapter block reads:

```go
	// Adapter API keys and tokens
	cfg.Adapters.PSAToken = os.Getenv("PSA_ACCESS_TOKEN")
	cfg.Adapters.PricingAPIKey = os.Getenv("PRICING_API_KEY")
	cfg.Adapters.GoogleOAuthEnv = os.Getenv("GOOGLE_OAUTH_ENV")
	cfg.Adapters.LocalAPIToken = os.Getenv("LOCAL_API_TOKEN")
	cfg.Adapters.DHEnterpriseKey = os.Getenv("DH_ENTERPRISE_API_KEY")
	envString("DH_API_BASE_URL", &cfg.Adapters.DHBaseURL)
	envIntPositive("DH_CACHE_TTL_HOURS", &cfg.DH.CacheTTLHours)
```

`os` and `envDurationPositive` both remain in use (`PSA_ACCESS_TOKEN` above,
`SNAPSHOT_ENRICH_INTERVAL` at `:161`).

- [ ] **Step 7: Remove the Azure/advisor section from `.env.example`**

Delete `.env.example:51-68` — the banner through the blank line before the
`HTTP Server` banner:

```
# -----------------------------------------------------------------------------
# AI — Azure OpenAI (Advisor)
# All three must be set together to enable AI features.
# OPTIONAL
# -----------------------------------------------------------------------------
AZURE_AI_ENDPOINT=""
AZURE_AI_API_KEY=""
AZURE_AI_DEPLOYMENT="gpt-5.4"

# Completion poll fallback timeout (e.g. "90s", "5m").
# OPTIONAL (default: 3m)
AZURE_AI_TIMEOUT="3m"

# Hard cap on LLM tool-calling rounds per advisor analysis. The prompt guides the
# model to 2 rounds and the service default is 3; this is the safety margin.
# OPTIONAL (default: 5)
ADVISOR_MAX_TOOL_ROUNDS="5"

```

Line 50 (blank, after `PRICING_API_KEY=""`) is followed directly by the
`# HTTP Server` banner.

- [ ] **Step 8: Verify no config-layer references survive**

Run from the repo root. Expected: **no output**.

```bash
grep -rnE 'AzureAI|AdvisorRefresh|AZURE_AI|ADVISOR_MAX_TOOL_ROUNDS' \
  internal/platform/config .env.example
```

If this prints anything, a step above was applied incompletely — fix before continuing.

- [ ] **Step 9: Build**

```bash
go build ./...
```

Expected: no output, exit 0. A failure here naming `cfg.Adapters.AzureAI*` or
`cfg.AdvisorRefresh` means Tasks 2–5 have not fully landed — do not patch the caller
here; go back and finish the earlier task.

- [ ] **Step 10: Check formatting**

```bash
gofmt -l ./internal/platform/config
```

Expected: no output. Output naming `types.go` means the `AdapterConfig` block in Step 1
was not re-aligned — re-paste the replacement from Step 1.

- [ ] **Step 11: Run the full test suite with the race detector**

```bash
go test -race -timeout 10m ./...
```

Expected: `ok` / `no test files` for every package, no `FAIL`. The config package's own
tests do not reference these fields (verified), so a config-package failure here is a
real regression.

- [ ] **Step 12: Commit**

```bash
git add internal/platform/config/types.go \
        internal/platform/config/defaults.go \
        internal/platform/config/loader.go \
        .env.example
git commit -m "refactor(config): drop Azure AI and advisor configuration

Removes the four AzureAI* fields from AdapterConfig, the AdvisorRefresh
field and AdvisorRefreshConfig type, their defaults, the AZURE_AI_* and
ADVISOR_MAX_TOOL_ROUNDS env parsing, and the .env.example section. The
last readers were deleted with the advisor packages, handlers, and CLI."
```

---

### Task 7: Drop the ai_calls and scoring_data_gaps tables (migration 000032)

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.down.sql`
- Create/Test: `internal/adapters/storage/postgres/migration_000032_test.go`

**Not in this task:** `docs/SCHEMA.md`. It documents both tables, both views and three
of their dropped indexes, but **Task 8 owns every documentation edit** and replaces those
sections with tombstones in the existing `~~advisor_cache~~ — DROPPED (migration 000013)`
style, as the spec requires. Do not edit or stage `docs/SCHEMA.md` here — two tasks
editing the same file with different outcomes is how one of them silently loses.

**Interfaces:**
- Consumes: the embedded migration set (`MigrationsFS` in `internal/adapters/storage/postgres/embedded_migrations.go`), current head `000031_restore_user_tokens_session_unique`. Consumes the existing package-level test helpers `requireTestDB`, `resetSchemaAndMigrate` (`testhelper_test.go`), `roleExists` (`migration_000027_test.go`), and `migrateToVersion` / `tableExists` / `tableColumns` (`migration_000030_test.go`).
- Produces: schema version 32, in which `ai_calls`, `scoring_data_gaps`, `ai_usage_summary` and `ai_usage_by_operation` no longer exist. Produces one new test-only helper, `viewExists`, available to later migration tests in the same package.

**Precondition — do this task LAST among the schema/code tasks.** `ai_tracker.go` (`ai_calls`, `ai_usage_summary`, `ai_usage_by_operation`) and `gap_store.go` + `internal/adapters/scheduler/gap_cleanup.go` (`scoring_data_gaps`) still issue SQL against these objects on `main`. If this migration lands before those files are removed and unwired, the binary compiles and starts, then fails at runtime the first time the scoring-gap cleanup scheduler or an AI-usage read fires. Verify before starting:

```bash
grep -rn "scoring_data_gaps\|ai_calls\|ai_usage_summary\|ai_usage_by_operation" \
  --include='*.go' . | grep -v '/migrations/'
```

The only acceptable remaining hit is a comment. On `main` today the hits are `internal/adapters/storage/postgres/ai_tracker.go:33,70,103`, `internal/adapters/storage/postgres/gap_store.go:34,56,66,73,100,149`, `internal/adapters/scheduler/gap_cleanup.go:12` (comment), and `internal/domain/ai/tracking.go:77` (comment).

> **THIS MIGRATION IS IRREVERSIBLE IN PRACTICE.** This repository auto-deploys from `main` via Fly (there is no manual deploy step to hold it at). The moment the merge commit lands, `RunMigrations` executes on startup and the production `ai_calls` history — every recorded operation, latency, token count and cost estimate — is destroyed. The `.down.sql` restores *structure only*; it cannot restore rows. **Anyone who wants that data must export it before the PR merges**, e.g.
> ```bash
> psql "$DATABASE_URL" -c "\copy (SELECT * FROM ai_calls ORDER BY id) TO 'ai_calls_export.csv' CSV HEADER"
> psql "$DATABASE_URL" -c "\copy (SELECT * FROM scoring_data_gaps ORDER BY id) TO 'scoring_data_gaps_export.csv' CSV HEADER"
> ```
> Get an explicit acknowledgement on the PR that this was done or consciously waived, exactly as 000030 recorded the discarded `marketmovers_config` credential row in its header comment.

---

- [ ] **Step 1: Confirm the migration head and the objects being dropped**

Do not assume the head number. Read it off the tree, and read the DDL you are about to reproduce in the down migration:

```bash
ls internal/adapters/storage/postgres/migrations/ | tail -6
sed -n '413,433p' internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql
sed -n '596,611p' internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql
sed -n '33,61p'  internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.up.sql
sed -n '240,270p' internal/adapters/storage/postgres/migrations/000003_supabase_security_and_perf_fixes.up.sql
```

Two facts from that reading drive the down migration and are easy to get wrong:

1. **`ai_calls` has two dependent views.** `000003` recreated `ai_usage_summary` and `ai_usage_by_operation` `WITH (security_invoker = true)`, both selecting `FROM ai_calls`. Postgres does **not** cascade through a view on a plain `DROP TABLE` — it raises `ERROR: cannot drop table ai_calls because other objects depend on it`. So the up migration must `DROP VIEW IF EXISTS` both *first*. Do **not** reach for `DROP TABLE ... CASCADE`: cascade would also silently take anything that attaches to `ai_calls` later, which is exactly the class of drop this repo has been cleaning up in 000021/000030. `scoring_data_gaps` has no dependent views (`grep -n scoring_data_gaps migrations/*.sql` shows no `CREATE VIEW` over it).

2. **The state to restore is post-000003, not 000001.** `000003` Fix 5 dropped `idx_ai_calls_timestamp`, `idx_ai_calls_operation` and `idx_scoring_gaps_factor` as unused (lines 262, 263, 268 of `000003_..._up.sql`), and `000003`'s own down recreates them. Since 000032 runs *after* 000003, restoring the 000001 index set would leave the database with three indexes that the head schema does not have. The correct restored state is: **`ai_calls` with no indexes beyond its primary key**, and **`scoring_data_gaps` with `idx_scoring_gaps_recorded` only**. This is the same reasoning 000030's down header gives for restoring the post-000028 RLS shape rather than 000003's original.

Corollary for the RLS shape: `000003:151-152,181-182` gave both tables a `TO PUBLIC` `"service role bypass"` policy; `000028` rewrote both to `TO service_role` and revoked `anon`/`authenticated`. Restore the **post-000028** shape.

- [ ] **Step 2: Confirm 000028 revokes on the views by name, not just the tables**

```bash
sed -n '40,60p' internal/adapters/storage/postgres/migrations/000028_tighten_000003_rls_policies.up.sql
sed -n '60,90p' internal/adapters/storage/postgres/migrations/000028_tighten_000003_rls_policies.up.sql
```

`000028`'s `targets` array contains `'ai_calls'` and `'scoring_data_gaps'` among the tables (lines 48 and 51) and `'ai_usage_summary'`, `'ai_usage_by_operation'` among the seven views (lines 56-57), with the header noting the views "hold no rows of their own but are still separate grantable objects". The loop body issues `REVOKE ALL ... FROM anon` (line 82-84) and `FROM authenticated` (line 86-88) on every target including the views.

This matters for the down migration: a `CREATE VIEW` grants nothing by itself, but Supabase's default privileges do — that is the whole reason 000028 had to revoke on views at all. A down migration that recreates the views without re-applying those REVOKEs would restore views readable by `anon`/`authenticated` while the `ai_calls` table underneath stays locked, which is worse than either end state. The down migration below therefore revokes on all four objects.

- [ ] **Step 3: Write the failing test first**

TDD: this test must be written and observed failing before either `.sql` file exists. With no 000032 present, `TestMigration000032_ObjectsDropped` fails on all four assertions (the objects are still there at head), and `TestMigration000032_DownRestoresStructure` fails at `migrateToVersion(db, 31)` → `migrateToVersion(db, 32)` because version 32 does not exist.

Create `internal/adapters/storage/postgres/migration_000032_test.go`:

```go
package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The four objects 000032 removes: the two AI/scoring tables and the two views
// over ai_calls that 000003 created WITH (security_invoker = true).
//
// The views are the reason the up migration cannot be two DROP TABLE lines.
// Postgres refuses to drop a table a view selects from, and DROP TABLE CASCADE
// would take whatever else happens to be attached at deploy time — so the views
// are dropped explicitly, first, by name.
const (
	aiCallsTable         = "ai_calls"
	scoringDataGapsTable = "scoring_data_gaps"
	aiUsageSummaryView   = "ai_usage_summary"
	aiUsageByOpView      = "ai_usage_by_operation"
)

// TestMigration000032_ObjectsDropped is the acceptance criterion: after the full
// embedded migration set applies, none of the four objects exist.
//
// tableExists and viewExists are separate probes on purpose — see viewExists.
func TestMigration000032_ObjectsDropped(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	assert.False(t, tableExists(ctx, t, db, aiCallsTable),
		"%s should not exist after migration 000032", aiCallsTable)
	assert.False(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"%s should not exist after migration 000032", scoringDataGapsTable)
	assert.False(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"view %s should not exist after migration 000032", aiUsageSummaryView)
	assert.False(t, viewExists(ctx, t, db, aiUsageByOpView),
		"view %s should not exist after migration 000032", aiUsageByOpView)
}

// TestMigration000032_DownRestoresStructure pins the *shape* the rollback
// restores, which the generic up/down/up roundtrip in migrations_test.go cannot:
// that test proves the SQL is valid, not that it puts the schema back.
//
// Three things are worth pinning here:
//
//  1. The views come back at all. A down migration that restored only the two
//     tables would leave ai_usage_summary and ai_usage_by_operation gone
//     forever, and nothing else in the migration set recreates them.
//  2. The RLS shape is the post-000028 one (TO service_role, anon and
//     authenticated revoked), not 000003's TO PUBLIC original. 000028 runs
//     before this migration; restoring the looser statement would reopen the
//     hole 000028 closed.
//  3. The views are revoked too. 000028 listed them among its targets because
//     Supabase's default privileges grant on views as well as tables, so a
//     recreated view is readable by anon unless it is revoked again.
//
// Not parallel: it steps schema_migrations backwards for the whole package.
func TestMigration000032_DownRestoresStructure(t *testing.T) {
	db := requireTestDB(t)
	ctx := context.Background()

	// Restore the package's shared database no matter how this test exits.
	t.Cleanup(func() { resetSchemaAndMigrate(t, db) })

	require.NoError(t, migrateToVersion(db, 31), "roll back to version 31")

	require.True(t, tableExists(ctx, t, db, aiCallsTable),
		"000032's down migration should recreate %s", aiCallsTable)
	require.True(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"000032's down migration should recreate %s", scoringDataGapsTable)
	require.True(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"000032's down migration should recreate view %s", aiUsageSummaryView)
	require.True(t, viewExists(ctx, t, db, aiUsageByOpView),
		"000032's down migration should recreate view %s", aiUsageByOpView)

	// Columns come back with the original 000001 definitions.
	assert.Equal(t,
		map[string]string{
			"id":                  "bigint",
			"operation":           "text",
			"status":              "text",
			"error_message":       "text",
			"latency_ms":          "bigint",
			"tool_rounds":         "bigint",
			"input_tokens":        "bigint",
			"output_tokens":       "bigint",
			"total_tokens":        "bigint",
			"timestamp":           "timestamp without time zone",
			"cost_estimate_cents": "bigint",
		},
		tableColumns(ctx, t, db, aiCallsTable),
		"restored %s should match its 000001 column definition", aiCallsTable)

	assert.Equal(t,
		map[string]string{
			"id":          "bigint",
			"factor_name": "text",
			"reason":      "text",
			"entity_type": "text",
			"entity_id":   "text",
			"card_name":   "text",
			"set_name":    "text",
			"recorded_at": "timestamp without time zone",
		},
		tableColumns(ctx, t, db, scoringDataGapsTable),
		"restored %s should match its 000001 column definition", scoringDataGapsTable)

	// Indexes come back in the post-000003 set, not the 000001 set. 000003's
	// Fix 5 dropped idx_ai_calls_timestamp, idx_ai_calls_operation and
	// idx_scoring_gaps_factor as unused; recreating them here would leave the
	// rolled-back database with three indexes head does not have.
	assert.Empty(t, tableIndexes(ctx, t, db, aiCallsTable),
		"restored %s should carry no secondary indexes (000003 dropped both)", aiCallsTable)
	assert.Equal(t, []string{"idx_scoring_gaps_recorded"},
		tableIndexes(ctx, t, db, scoringDataGapsTable),
		"restored %s should carry only the index that survived 000003", scoringDataGapsTable)

	// The views must select through to the restored tables, not merely exist.
	for _, view := range []string{aiUsageSummaryView, aiUsageByOpView} {
		var n int64
		require.NoError(t,
			db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+view).Scan(&n),
			"restored view %s should be selectable", view)
	}

	// RLS comes back in the post-000028 shape on both tables.
	for _, table := range []string{aiCallsTable, scoringDataGapsTable} {
		var enabled bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT c.relrowsecurity
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&enabled),
			"read relrowsecurity for %s", table)
		assert.True(t, enabled, "restored %s should have RLS enabled", table)

		rows, err := db.QueryContext(ctx, `
			SELECT policyname, roles::text
			FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1
			ORDER BY policyname`, table)
		require.NoError(t, err, "query policies for %s", table)

		var policies int
		for rows.Next() {
			var policy, roles string
			require.NoError(t, rows.Scan(&policy, &roles))
			policies++
			assert.Equal(t, "{service_role}", roles,
				"policy %q on restored %s must be scoped to service_role alone; %s "+
					"passes every role listed", policy, table, roles)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())

		// Where service_role is absent (a local or CI Postgres) the down
		// migration creates no policy at all: RLS enabled with zero policies is
		// the same effective deny, matching 000029 and 000030 off Supabase.
		if roleExists(ctx, t, db, "service_role") {
			assert.Equal(t, 1, policies,
				"restored %s should carry exactly one TO service_role policy", table)
		} else {
			assert.Zero(t, policies,
				"restored %s should have no policies where service_role is absent", table)
		}
	}

	// anon and authenticated hold no privilege on any of the four objects,
	// including the two views — 000028 revoked on views by name for exactly
	// this reason, and the down migration has to repeat it.
	//
	// Uses has_table_privilege rather than information_schema.role_table_grants,
	// matching migration_000027_test.go:108-141 and for the reason documented
	// there: role_table_grants lists only *direct* grants, so a privilege
	// reaching anon through a grant to PUBLIC or through role inheritance
	// slips past it and the assertion passes while the access is still live.
	// has_table_privilege answers the effective-access question this test
	// claims to be checking.
	tablePrivileges := []string{
		"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER",
	}
	for _, role := range []string{"anon", "authenticated"} {
		if !roleExists(ctx, t, db, role) {
			continue
		}
		for _, object := range []string{
			aiCallsTable, scoringDataGapsTable, aiUsageSummaryView, aiUsageByOpView,
		} {
			for _, privilege := range tablePrivileges {
				var allowed bool
				require.NoError(t, db.QueryRowContext(ctx,
					`SELECT has_table_privilege($1::text, format('public.%I', $2::text), $3::text)`,
					role, object, privilege).Scan(&allowed),
					"probe %s on %s for %s", privilege, object, role)
				assert.False(t, allowed,
					"restored %s must not be reachable by %s (effective %s remains; "+
						"check for grants to PUBLIC or an inherited role, not just direct grants)",
					object, role, privilege)
			}
		}
	}

	// Re-applying 000032 removes all four again, proving the rollback is not
	// one-way and that the view drops still precede the table drops.
	require.NoError(t, migrateToVersion(db, 32), "re-apply version 32")
	assert.False(t, tableExists(ctx, t, db, aiCallsTable),
		"%s should be gone again after re-applying 000032", aiCallsTable)
	assert.False(t, tableExists(ctx, t, db, scoringDataGapsTable),
		"%s should be gone again after re-applying 000032", scoringDataGapsTable)
	assert.False(t, viewExists(ctx, t, db, aiUsageSummaryView),
		"view %s should be gone again after re-applying 000032", aiUsageSummaryView)
	assert.False(t, viewExists(ctx, t, db, aiUsageByOpView),
		"view %s should be gone again after re-applying 000032", aiUsageByOpView)
}

// viewExists reports whether a VIEW of that name exists in public.
//
// This is NOT redundant with tableExists. tableExists filters relkind = 'r'
// (ordinary table), so it returns false for a view — every "the view is gone"
// assertion written against it would pass vacuously, including against a
// migration that never dropped the views at all. That is precisely the bug this
// task exists to avoid, so the view probe has to filter relkind = 'v'.
func viewExists(ctx context.Context, t *testing.T, db *DB, view string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1 AND c.relkind = 'v'
		)`, view).Scan(&exists), "probe existence of view %s", view)
	return exists
}

// tableIndexes returns the non-constraint index names on a public table, sorted.
// Primary-key and unique-constraint indexes are excluded so the assertion reads
// as "which secondary indexes survive", which is what 000003's Fix 5 changed.
func tableIndexes(ctx context.Context, t *testing.T, db *DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT ci.relname
		FROM pg_index i
		JOIN pg_class ct ON ct.oid = i.indrelid
		JOIN pg_class ci ON ci.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = ct.relnamespace
		WHERE n.nspname = 'public'
		  AND ct.relname = $1
		  AND NOT i.indisprimary
		  AND NOT i.indisunique
		ORDER BY ci.relname`, table)
	require.NoError(t, err, "read indexes for %s", table)
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
```

Run it and confirm it fails for the right reason (see Step 6 for the env var):

```bash
make test-postgres
```

Expected at this point: `TestMigration000032_ObjectsDropped` fails four assertions; `TestMigration000032_DownRestoresStructure` fails at `re-apply version 32`. If instead everything is *skipped*, `POSTGRES_TEST_URL` is unset — a skip is not a failing test and does not satisfy this step.

- [ ] **Step 4: Write the up migration**

Create `internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.up.sql`:

```sql
-- Drop the AI advisor's persistence: ai_calls, its two usage views, and
-- scoring_data_gaps.
--
-- The Azure AI advisor feature is removed in this change set. ai_tracker.go and
-- gap_store.go — the only code that ever read or wrote these objects — go with
-- it, along with the gap-cleanup scheduler that existed solely to keep
-- scoring_data_gaps from growing unbounded. What is left behind is schema
-- nobody owns, which 000028 was still dutifully maintaining RLS policies for.
--
-- ai_calls holds production history: one row per advisor invocation, with
-- latency, token counts and cost estimates. That history is NOT recoverable
-- after this runs, and this repository auto-deploys from main, so it is
-- destroyed on merge rather than on a deliberate operator action. Export it
-- before merging if it is wanted; the .down.sql restores structure only.
--
-- Order matters. ai_usage_summary and ai_usage_by_operation are views over
-- ai_calls (000003, security_invoker = true), and Postgres raises
--   ERROR: cannot drop table ai_calls because other objects depend on it
-- rather than cascading through them. They are dropped first, by name. DROP
-- TABLE ... CASCADE would also work today and is deliberately not used: it
-- would silently take anything that attaches to ai_calls between now and the
-- deploy, which is the failure mode 000021 and 000030 were cleaning up after.
--
-- Policies are dropped explicitly before each table so the table drop needs no
-- CASCADE, matching 000013_drop_advisor_cache and 000030_drop_marketmovers_config.

DROP VIEW IF EXISTS public.ai_usage_by_operation;
DROP VIEW IF EXISTS public.ai_usage_summary;

DROP POLICY IF EXISTS "service role bypass" ON public.ai_calls;
DROP TABLE IF EXISTS ai_calls;

DROP POLICY IF EXISTS "service role bypass" ON public.scoring_data_gaps;
DROP TABLE IF EXISTS scoring_data_gaps;
```

Note `DROP POLICY IF EXISTS ... ON public.ai_calls` is itself safe when the table is already absent in only one direction: `IF EXISTS` on a policy covers a missing policy, not a missing table. Both `DROP POLICY` statements here run while their table still exists, so the ordering above is the safe one — do not move them below the `DROP TABLE`.

- [ ] **Step 5: Write the down migration**

Create `internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.down.sql`. The column definitions are the originals from `000001_initial_schema`, including the `CHECK` constraints, which are part of the shape and cheap to restore. The index set and the RLS shape are the post-000003 / post-000028 ones, for the reasons in the header:

```sql
-- Rollback: recreate ai_calls, scoring_data_gaps and the two ai_calls views
-- (structure only -- the dropped rows are not restored).
--
-- Column definitions are the originals from 000001_initial_schema. Two things
-- are deliberately NOT the 000001 state:
--
--   Indexes. 000003's Fix 5 dropped idx_ai_calls_timestamp, idx_ai_calls_operation
--   and idx_scoring_gaps_factor as unused per Supabase telemetry. 000003 runs
--   before this migration, so recreating them here would leave a rolled-back
--   database with three indexes the head schema does not have. Only
--   idx_scoring_gaps_recorded survived, and only it comes back.
--
--   RLS. 000003 created both policies with no TO clause, which defaults to
--   TO PUBLIC and therefore admits anon and authenticated; 000028 rewrote them
--   TO service_role and revoked the default grants. Restoring 000003's looser
--   statement would reopen the hole 000028 closed.
--
-- The views are recreated WITH (security_invoker = true), as 000003 left them,
-- and are revoked from anon and authenticated alongside the tables. 000028
-- listed both views among its targets for exactly this reason: a view holds no
-- rows of its own but is still a separate grantable object, and Supabase's
-- default privileges grant on it. A view restored without the REVOKE would be
-- readable by anon while the table underneath stays locked.
--
-- Role-dependent statements are guarded on pg_roles so this also applies to a
-- local Postgres, where Supabase's roles do not exist. There, the tables end up
-- with RLS enabled and no policies, which is the same effective deny.

CREATE TABLE IF NOT EXISTS ai_calls (
    id                  BIGSERIAL PRIMARY KEY,
    operation           TEXT NOT NULL CHECK(operation IN (
        'digest', 'campaign_analysis', 'liquidation',
        'purchase_assessment', 'social_caption', 'social_suggestion'
    )),                                                                  -- AICallRecord.Operation (named string)
    status              TEXT NOT NULL CHECK(status IN ('success', 'error', 'rate_limited')),
    error_message       TEXT DEFAULT '',                                 -- AICallRecord.ErrorMessage string
    latency_ms          BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.LatencyMS int64
    tool_rounds         BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.ToolRounds int
    input_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.InputTokens int
    output_tokens       BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.OutputTokens int
    total_tokens        BIGINT NOT NULL DEFAULT 0,                       -- AICallRecord.TotalTokens int
    timestamp           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,             -- AICallRecord.Timestamp time.Time
    cost_estimate_cents BIGINT NOT NULL DEFAULT 0                        -- AICallRecord.CostEstimateCents int
);

CREATE TABLE IF NOT EXISTS scoring_data_gaps (
    id           BIGSERIAL PRIMARY KEY,
    factor_name  TEXT NOT NULL,                                        -- GapRecord.FactorName string
    reason       TEXT NOT NULL,                                        -- GapRecord.Reason string
    entity_type  TEXT NOT NULL,                                        -- GapRecord.EntityType string
    entity_id    TEXT NOT NULL,                                        -- GapRecord.EntityID string
    card_name    TEXT NOT NULL DEFAULT '',                             -- GapRecord.CardName string
    set_name     TEXT NOT NULL DEFAULT '',                             -- GapRecord.SetName string
    recorded_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP          -- GapRecord.RecordedAt time.Time
);

-- Post-000003 index set: the two ai_calls indexes and idx_scoring_gaps_factor
-- stay dropped.
CREATE INDEX IF NOT EXISTS idx_scoring_gaps_recorded ON scoring_data_gaps(recorded_at);

-- The views, exactly as 000003 left them.
DROP VIEW IF EXISTS public.ai_usage_summary;
CREATE VIEW public.ai_usage_summary WITH (security_invoker = true) AS
SELECT
    COUNT(*) AS total_calls,
    COUNT(CASE WHEN status = 'success' THEN 1 END) AS success_calls,
    COUNT(CASE WHEN status = 'error' THEN 1 END) AS error_calls,
    COUNT(CASE WHEN status = 'rate_limited' THEN 1 END) AS rate_limit_hits,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(input_tokens), 0) AS total_input_tokens,
    COALESCE(SUM(output_tokens), 0) AS total_output_tokens,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents,
    TO_CHAR(MAX(timestamp), 'YYYY-MM-DD HH24:MI:SS') AS last_call_at,
    COUNT(CASE WHEN timestamp > NOW() - INTERVAL '24 hours' THEN 1 END) AS calls_last_24h
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days';

DROP VIEW IF EXISTS public.ai_usage_by_operation;
CREATE VIEW public.ai_usage_by_operation WITH (security_invoker = true) AS
SELECT
    operation,
    COUNT(*) AS calls,
    COUNT(CASE WHEN status = 'error' OR status = 'rate_limited' THEN 1 END) AS errors,
    COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
    COALESCE(SUM(total_tokens), 0) AS total_tokens,
    COALESCE(SUM(cost_estimate_cents), 0) AS total_cost_cents
FROM ai_calls
WHERE timestamp > NOW() - INTERVAL '7 days'
GROUP BY operation;

ALTER TABLE public.ai_calls ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.ai_calls;

ALTER TABLE public.scoring_data_gaps ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS "service role bypass" ON public.scoring_data_gaps;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'service_role') THEN
        CREATE POLICY "service role bypass" ON public.ai_calls
            TO service_role USING (true) WITH CHECK (true);
        CREATE POLICY "service role bypass" ON public.scoring_data_gaps
            TO service_role USING (true) WITH CHECK (true);
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
        REVOKE ALL ON public.ai_calls FROM anon;
        REVOKE ALL ON public.scoring_data_gaps FROM anon;
        REVOKE ALL ON public.ai_usage_summary FROM anon;
        REVOKE ALL ON public.ai_usage_by_operation FROM anon;
    END IF;

    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        REVOKE ALL ON public.ai_calls FROM authenticated;
        REVOKE ALL ON public.scoring_data_gaps FROM authenticated;
        REVOKE ALL ON public.ai_usage_summary FROM authenticated;
        REVOKE ALL ON public.ai_usage_by_operation FROM authenticated;
    END IF;
END
$$;
```

- [ ] **Step 6: Re-run the migration test against a throwaway database**

The Postgres package tests are gated: `requireTestDB` (`testhelper_test.go:68`) reads **`POSTGRES_TEST_URL`** and calls `t.Skip` when it is unset, deliberately never falling back to a default DSN — the devcontainer's `DATABASE_URL` points at the developer's real database and this package drops schemas. A plain `go test ./...` therefore skips every test in this file.

Use the Makefile target, which provisions the throwaway database and sets the variable:

```bash
make test-postgres
```

That runs `POSTGRES_TEST_URL="$(POSTGRES_TEST_DSN)" go test -race -count=1 ./internal/adapters/storage/postgres/...` after checking that `POSTGRES_TEST_DSN`'s database name matches `POSTGRES_TEST_DB` — the interlock that stops a mismatched override from creating one database and dropping schemas in another. To run only this file's tests during iteration, still go through the same variable:

```bash
POSTGRES_TEST_URL="postgresql://slabledger:slabledger@postgres:5432/slabledger_test?sslmode=disable" \
  go test -race -count=1 -run TestMigration000032 ./internal/adapters/storage/postgres/...
```

Confirm from the output that the tests **ran** rather than skipped (`-v` shows `--- PASS`, not `--- SKIP`). A skip here is indistinguishable from a pass in the summary line and satisfies nothing.

Also confirm the package's existing generic roundtrip still passes — `migrations_test.go` applies the whole set up, then down, then up, and is what catches a `.down.sql` that is invalid SQL rather than merely wrong:

```bash
POSTGRES_TEST_URL="..." go test -race -count=1 -run TestMigrations ./internal/adapters/storage/postgres/...
```

- [ ] **Step 7: Verify the drop is complete and nothing else references the objects**

```bash
grep -rn "scoring_data_gaps\|ai_calls\|ai_usage_summary\|ai_usage_by_operation" \
  --include='*.go' . | grep -v '/migrations/' | grep -v migration_000032_test.go
```

Expected: no hits, or comments only. `internal/domain/ai/tracking.go:77` and `internal/adapters/scheduler/gap_cleanup.go:12` are comment-only references on `main` and should have been removed with their files by the earlier tasks; if either file still exists, that task is not finished and this one is premature.

Then the standard gates:

```bash
go build ./... && go test -race ./... && make check
```

`make check` runs the import checker, the file-size check and the Playwright version check; none of them look at SQL, so it will not catch a migration mistake — it is here to confirm this task did not break anything else.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.up.sql \
        internal/adapters/storage/postgres/migrations/000032_drop_ai_advisor_tables.down.sql \
        internal/adapters/storage/postgres/migration_000032_test.go

git commit -m "feat(db): drop ai_calls and scoring_data_gaps (migration 000032)

Removes the AI advisor's persistence along with the feature itself: the
ai_calls table, its two security_invoker views (ai_usage_summary and
ai_usage_by_operation), and scoring_data_gaps.

The views are dropped explicitly before the table rather than via DROP
TABLE CASCADE — Postgres errors on a dependent view instead of cascading
through it, and CASCADE would silently take whatever else attaches later.

The down migration restores structure only, in the post-000003/post-000028
shape: no ai_calls indexes, idx_scoring_gaps_recorded only, policies scoped
TO service_role, and anon/authenticated revoked on the views as well as the
tables.

This destroys production ai_calls history on deploy and is not reversible."
```

---

### Task 8: Documentation sweep and final verification

**Files:**
- Delete: `docs/LLM_USAGE.md`
- Modify: `docs/ARCHITECTURE.md` (ASCII box, package tree, interface table)
- Modify: `docs/SCHEMA.md` (two table sections → tombstones, two view sections → tombstones, FK graph)
- Modify: `docs/API.md` (`GET /api/admin/ai-usage` section, `## AI Advisor` section)
- Modify: `internal/README.md` (domain tree diagram, package table)
- Modify: `internal/testutil/mocks/README.md` (service-mock table row)
- Modify: `CLAUDE.md` (orientation bullets, env-var group, Documentation link)
- Modify: `.claude/skills/polish-all/SKILL.md` (segment rows 2 and 8, sample summary row)

**Out of scope — never edit:** `docs/audit/**`, `docs/plans/**`, `docs/specs/**`,
`docs/superpowers/**`. These are historical records of past work; they are supposed to
name things that no longer exist. The final grep in Step 24 excludes them by path.

**Verified clean, no changes needed:** `docs/LOOP.md`, `docs/USER_GUIDE.md`,
`docs/OPERATIONS.md`, `docs/SCHEDULERS.md`, `docs/DEVELOPMENT.md`, `docs/DH_INVENTORY.md`
(zero advisor/azure/llm/ai_calls hits, re-confirmed by Step 1's grep).

**Known false positives — DO NOT EDIT:**
- `internal/adapters/scheduler/dh_orders_poll_test.go:187` and
  `internal/domain/inventory/core_types.go:543` — the English word "advisory" describing
  a card grade.
- `docs/SCHEMA.md` lines containing "Supabase's advisor" / "an index advisor" (in the
  `user_tokens` narrative and the Dropped-indexes narrative) — that is Postgres's index
  advisor, unrelated.

**Interfaces:**
- Consumes: the completed source deletions from Tasks 1–7 (domain/advisor, domain/ai,
  domain/scoring, adapters/advisortool, adapters/scoring, adapters/clients/azureai,
  the advisor scheduler job, the advisor + ai-usage HTTP handlers, the AI config block,
  the `ai_calls` / `scoring_data_gaps` drop migration).
- Produces: a repository whose documentation makes no live claim about the advisor stack,
  plus the green build/test/lint/frontend gate that closes the whole change.

---

- [ ] **Step 1: Run the discovery grep and work from its output, not from line numbers below.**

  Tasks 1–7 have already shifted every line number in this fragment. Run this first and
  treat its output as the authoritative worklist:

  ```bash
  cd /workspace/.worktrees/remove-azure-ai
  grep -rniE 'advisor|azure|openai|llm|ai_calls|ai-usage|scoring' \
    --include='*.md' docs internal CLAUDE.md README.md \
    | grep -vE '^docs/(audit|plans|specs|superpowers)/'
  ```

  Expected: hits only in `docs/LLM_USAGE.md`, `docs/ARCHITECTURE.md`, `docs/SCHEMA.md`,
  `docs/API.md`, `internal/README.md`, `internal/testutil/mocks/README.md`, `CLAUDE.md`,
  plus the two false-positive classes listed above. If a file appears that is not in this
  fragment's file list, stop and report it rather than guessing at the edit.

- [ ] **Step 2: Delete `docs/LLM_USAGE.md`.**

  ```bash
  git rm docs/LLM_USAGE.md
  ```

  The whole 133-line file documents the Azure AI provider, the advisor tool loop, the
  28-tool registry, and the advisor scheduler. Nothing in it survives the removal.

- [ ] **Step 3: Remove the `LLM Usage` link from `CLAUDE.md`'s Documentation list.**

  Delete this exact line:

  ```
  - [LLM Usage](docs/LLM_USAGE.md) - Where models are called and what they cost
  ```

  It is the last bullet in the `## Documentation` list, directly above the blank line
  preceding "`ls docs/` for the rest — the list above is the durable set, not an inventory."

- [ ] **Step 4: Remove the `**AI**` env-var group from `CLAUDE.md`.**

  Delete this exact line from the `## Environment Variables` list:

  ```
  - **AI**: `AZURE_AI_ENDPOINT`, `AZURE_AI_API_KEY`, `AZURE_AI_DEPLOYMENT`
  ```

  The surrounding bullets (`**Required**`, `**DH**`, `**Auth**`, `**CardLadder**`,
  `**Schedulers**`) stay. `.env.example` itself is owned by the configuration task —
  do not edit it here, but Step 24's grep will catch it if that task missed it.

- [ ] **Step 5: Remove the three dead orientation bullets from `CLAUDE.md`'s Architecture section.**

  Delete these two lines verbatim:

  ```
  - `domain/scoring` — price scoring factors and profiles.
  ```

  ```
  - `domain/advisor`, `domain/ai` — AI advisor interfaces, tool loop, LLM provider.
  ```

  ```
  - `adapters/advisortool`, `adapters/scoring` — executor and provider implementations.
  ```

  The bullets on either side (`domain/intelligence`, `domain/liquidation`,
  `domain/dhevents`, `domain/storage`, `adapters/clients/httpx`,
  `adapters/storage/postgres`) are unaffected.

- [ ] **Step 6: Leave the leaf-utilities bullet in `CLAUDE.md` alone.**

  The bullet reads:

  ```
  - `domain/{constants,errors,llmutil,mathutil,observability,timeutil}` — leaf utilities.
  ```

  It is tempting to drop `llmutil` from it. Do not. `internal/domain/llmutil` is **not
  deleted by any task in this plan**, and it is **not part of this change at all**:
  `grep -rl 'domain/llmutil' --include='*.go' .` on `main` returns only
  `internal/domain/llmutil/strip_fences_test.go`, so the package was already orphaned
  before this work started. That is pre-existing dead code with its own removal decision
  to make, and the spec's "Out of scope" section governs — this task documents what this
  change removed, nothing else.

  This step is a deliberate no-op. Verify the directory still exists
  (`ls internal/domain/llmutil`) and move on.


- [ ] **Step 7: Re-pad the DOMAIN LAYER ASCII box in `docs/ARCHITECTURE.md`.**

  `LLMProvider` is listed on the interfaces line inside a box whose right border must stay
  aligned. Removing the text without adding the spaces back breaks the box. Current line
  (the last content line of the DOMAIN LAYER box):

  ```
  │  Interfaces: PriceProvider, PriceLookup, LLMProvider        │
  ```

  Replace with (`, LLMProvider` removed, thirteen spaces added so the closing `│` stays in
  the same column — verified: both strings are 63 characters wide):

  ```
  │  Interfaces: PriceProvider, PriceLookup                     │
  ```

  For context, the full box after the edit — no other line in it changes:

  ```
  ┌─────────────────────────────────────────────────────────────┐
  │                         DOMAIN LAYER                        │
  │          (Pure Business Logic - NO external deps)           │
  │                                                             │
  │  • Inventory Service       • P&L Analytics                  │
  │  • DH Pricing              • Market Direction Signals       │
  │  • Portfolio Health        • CSV Import                     │
  │  • Authentication          • Channel Fee Calculation        │
  │                                                             │
  │  Interfaces: PriceProvider, PriceLookup                     │
  └─────────────────────────────────────────────────────────────┘
  ```

- [ ] **Step 8: Remove the dead package-tree entries from `docs/ARCHITECTURE.md`.**

  Inside the `## Package Structure` fenced block, delete these three lines verbatim:

  ```
      ai/                     # LLMProvider, ToolExecutor interfaces
      advisor/                # AI advisor service and tool loop
  ```

  ```
        azureai/              # Azure AI completions and image generation
  ```

  and rewrite the scheduler line, which names the advisor job:

  ```
      scheduler/              # Background jobs (price refresh, session cleanup, advisor)
  ```

  →

  ```
      scheduler/              # Background jobs (price refresh, session cleanup)
  ```

  Note: this tree does **not** list `scoring/` — only `internal/README.md` does. Do not go
  looking for it here.

- [ ] **Step 9: Delete four rows from the domain-interface table in `docs/ARCHITECTURE.md`.**

  Delete these four consecutive rows verbatim:

  ```
  | `advisor` | `Service` | `service.go` | 6 | AI advisor analysis (streaming) |
  | `advisor` | `CacheStore` | `cache.go` | 5 | Advisor result persistence |
  | `ai` | `LLMProvider` | `llm.go` | 1 | LLM completion (Azure AI) |
  | `ai` | `ToolExecutor` | `tools.go` | 1 | Tool call execution |
  ```

  The rows above (`auth` `Repository`) and below (`observability` `Logger`) stay; the table
  header and alignment row are untouched.

- [ ] **Step 10: Leave the two historical decision records in `docs/ARCHITECTURE.md` alone.**

  Both of these mention "scoring" and are records of *past* removals, not live claims:

  - `**Decision**: New `inventory/` domain package … Removed unused scoring, opportunity
    detection, eBay deal detection, PSA population analysis.`
  - `**Decision**: Removed dead code (scoring engine, opportunity detection, eBay deals,
    PSA population, marketplace timing, monitoring/alerts) …`

  Editing them would falsify the change log. This step is a no-op — it exists so the
  implementer does not "fix" them on the strength of Step 1's grep output.

- [ ] **Step 11: Replace the `ai_calls` and `scoring_data_gaps` table sections in `docs/SCHEMA.md` with tombstones.**

  The repo's tombstone convention is set by the existing `advisor_cache` entry: a
  `### ~~`name`~~ — DROPPED (migration NNNNNN)` heading followed by one prose paragraph.
  Read it before writing, so the format matches exactly:

  ```bash
  grep -n -A3 'advisor_cache.*DROPPED' docs/SCHEMA.md
  ```

  Confirm the migration number the drop task actually used before writing it into the
  tombstones — the highest migration on `main` is `000031_restore_user_tokens_session_unique`,
  so the drop is expected to be `000032`, but verify:

  ```bash
  ls internal/adapters/storage/postgres/migrations/ | tail -4
  ```

  Replace the entire `### `ai_calls`` section — heading, the "Log of every AI (Azure OpenAI)
  call…" line, the full 12-row column table, the `**Indexes:** none …` line and the
  `**Foreign Keys:** none` line — with:

  ```
  ### ~~`ai_calls`~~ — DROPPED (migration 000032)

  Dropped in migration 000032 with the removal of the Azure AI advisor. Logged every LLM call — operation, status, latency, tool rounds, token counts, and estimated cost in cents. Its two indexes were already gone (dropped in migration 000003, listed under "Dropped indexes" below).
  ```

  Replace the entire `### `scoring_data_gaps`` section — heading, the "Records of missing
  data encountered during scoring/analytics." line, the 8-row column table, the
  `**Indexes:**` block and the `**Foreign Keys:** none` line — with:

  ```
  ### ~~`scoring_data_gaps`~~ — DROPPED (migration 000032)

  Dropped in migration 000032 with the removal of the scoring domain. Recorded missing data encountered during scoring — factor name, reason, entity type and id, card and set name. Its surviving index `idx_scoring_gaps_recorded` went with the table; `idx_scoring_gaps_factor` had already been dropped in migration 000003.
  ```

  Keep the surrounding `---` separators exactly as they are.

- [ ] **Step 12: Replace the two AI view sections in `docs/SCHEMA.md` with tombstones.**

  In the views list, replace:

  ```
  ### `ai_usage_summary`
  Aggregate AI call statistics for the last 7 days: total calls, success/error/rate-limited counts, token totals, and estimated cost.

  ### `ai_usage_by_operation`
  Per-operation breakdown of AI call counts, error rates, latency, token usage, and cost for the last 7 days.
  ```

  with:

  ```
  ### ~~`ai_usage_summary`~~ — DROPPED (migration 000032)
  Dropped in migration 000032 with `ai_calls`. Aggregated AI call statistics for the last 7 days: total calls, success/error/rate-limited counts, token totals, and estimated cost.

  ### ~~`ai_usage_by_operation`~~ — DROPPED (migration 000032)
  Dropped in migration 000032 with `ai_calls`. Per-operation breakdown of AI call counts, error rates, latency, token usage, and cost for the last 7 days.
  ```

  The preceding `expired_sessions` section and the following `---` / `## Dropped indexes`
  heading are unchanged.

- [ ] **Step 13: Remove `ai_calls` and `scoring_data_gaps` from the FK dependency graph in `docs/SCHEMA.md`.**

  In the fenced block under `## FK Dependency Graph`, in the
  `── Standalone tables (no FK dependencies) ──` list, delete the two bare lines:

  ```
  ai_calls
  ```

  ```
  scoring_data_gaps
  ```

  `api_calls` (directly above `ai_calls`) and `scheduler_run_stats` (directly below
  `scoring_data_gaps`) stay — the names are similar, so delete by exact match.

- [ ] **Step 14: Leave the "Dropped indexes" catalogue in `docs/SCHEMA.md` unchanged, and fix one stale column note.**

  Do **not** delete the `idx_ai_calls_operation`, `idx_ai_calls_timestamp`,
  `idx_scoring_gaps_factor`, or `idx_advisor_cache_type` entries. That table opens with
  "They are catalogued here because older docs, query plans, and commit messages still name
  them — none of the rows below exist in the current schema." It is the same class of
  historical record as the ARCHITECTURE.md decisions in Step 10, and the tables those
  indexes belonged to are now themselves documented as tombstones, so the rows remain
  internally consistent.

  One real edit in this area: the `campaign_purchases` column note claiming an LLM use that
  no longer exists. `PSAListingTitle` is still live — it flows into
  `internal/adapters/clients/dhprice/provider.go` for card matching — only the LLM framing
  is stale. Replace:

  ```
  | `psa_listing_title` | TEXT | NOT NULL DEFAULT '' | Raw PSA title for LLM fallback; added migration 000001 |
  ```

  with:

  ```
  | `psa_listing_title` | TEXT | NOT NULL DEFAULT '' | Raw PSA title used for DH card matching; added migration 000001 |
  ```

- [ ] **Step 15: Remove the `GET /api/admin/ai-usage` section from `docs/API.md`.**

  Delete from the heading `### `GET /api/admin/ai-usage`` through the closing ``` ``` `` of
  its JSON response body and the trailing `---` separator — roughly 40 lines, ending
  immediately before `## Card Catalog`. The section begins:

  ```
  ### `GET /api/admin/ai-usage`

  Auth: RequireAdmin

  Returns AI call usage statistics broken down by operation.
  ```

  Verify the boundary before cutting:

  ```bash
  awk '/^### `GET \/api\/admin\/ai-usage`/,/^## Card Catalog/' docs/API.md | head -50
  ```

  Leave the `---` that closes the *preceding* endpoint, and leave `## Card Catalog` intact.

- [ ] **Step 16: Remove the `## AI Advisor` section from `docs/API.md`.**

  Delete from `## AI Advisor` through the end of the
  `### `POST /api/advisor/liquidation-analysis`` subsection and its trailing `---`, ending
  immediately before `## Pricing API v1`. That covers: the SSE preamble, the event-shape and
  error-event JSON blocks, `POST /api/advisor/digest`, and
  `POST /api/advisor/liquidation-analysis`. Verify the boundary:

  ```bash
  awk '/^## AI Advisor/,/^## Pricing API v1/' docs/API.md
  ```

  `docs/API.md` has no separate table of contents, so there is no cross-link to clean up —
  confirmed by `grep -n 'Advisor\|ai-usage' docs/API.md` returning only the two section
  headings.

- [ ] **Step 17: Remove `scoring/` from `internal/README.md`.**

  Two edits. In the domain ASCII tree, delete this whole line (a full-line deletion, so no
  re-padding is needed — the `└── storage/` line already carries the closing corner):

  ```
   │    ├── scoring/        (price scoring factors)  │
  ```

  In the domain package table, delete this row:

  ```
  | `scoring/` | Price scoring factors and profiles |
  ```

  The `pricing/` row above and the `storage/` row below stay.

- [ ] **Step 18: Remove the advisor row from `internal/testutil/mocks/README.md`.**

  In the service-mock table, delete:

  ```
  | `advisor.Service` | `MockAdvisorService` |
  ```

  The `dhlisting.Service` row above and the `social.Service` row below are unaffected. If
  Tasks 1–7 also removed `social.Service` or `picks.Service` mocks, re-check those rows
  against `internal/testutil/mocks/` before committing — but do not remove them
  speculatively.

- [ ] **Step 19: Run `go build ./...`.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai && go build ./...
  ```

  Expected: no output, exit 0. Documentation edits cannot break this — a failure here means
  a source change from Tasks 1–7 is incomplete, not that Step 2–18 went wrong.

- [ ] **Step 20: Run `go test -race ./...`.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai && go test -race -timeout 10m ./...
  ```

  Expected: zero failures — a fully green run is the documented baseline for this repo, so
  any `FAIL` line is a regression to fix, not a pre-existing condition to wave through.
  Paste the tail of the output into the completion report; do not claim it passed without it.

- [ ] **Step 21: Run `make check`.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai && make check
  ```

  Expected: lint clean, `scripts/check-imports-test.sh` five fixtures pass,
  `scripts/check-imports.sh` clean, no file over 600 lines, Playwright version in step.

  If the import checker fails: **do not edit `scripts/check-imports.sh`.** It fails closed
  when fewer than two inventory siblings are derived, but `internal/domain/scoring` was
  never an inventory sibling — it does not import `internal/domain/inventory` — so deleting
  it cannot change the derived count of ten. Re-derive the set and find what actually moved:

  ```bash
  grep -rl --include='*.go' 'internal/domain/inventory' internal/domain \
    | grep -v _test.go | xargs -n1 dirname | sort -u
  ```

  Expected: ten directories — arbitrage, demand, dhlisting, dhpricing, export, finance,
  portfolio, pricing/lookup, psacampaign, tuning.

- [ ] **Step 22: Build and test the frontend.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai/web && npm run build && npm test
  ```

  Expected: a clean Vite production build and a passing test run. The frontend task removed
  the advisor UI and its API-client methods; this step confirms no page, route, or type
  still references them.

- [ ] **Step 23: Repoint the `polish-all` segment table away from the deleted packages.**

  `.claude/skills/polish-all/SKILL.md` hardcodes a twelve-row segment table that drives how
  the polish skill walks the codebase. Two rows name directories this change deletes:

  ```
  | 2 | `domain/advisor+social+scoring` | `internal/domain/advisor/`, `internal/domain/social/`, `internal/domain/scoring/` |
  | 8 | `adapters/scheduler+scoring+advisor` | `internal/adapters/scheduler/`, `internal/adapters/scoring/`, `internal/adapters/advisortool/` |
  ```

  Segment 8 keeps a real path (`internal/adapters/scheduler/`) once the other two are
  dropped. **Segment 2 does not** — its third path, `internal/domain/social/`, already
  does not exist on `main` (verify: `ls internal/domain/social` fails). So deleting
  `advisor/` and `scoring/` leaves segment 2 with zero existing directories, and the
  polish run would iterate an empty segment. It needs replacement paths, not just deletions.

  **Do not renumber the rows.** The literal `12` is hardcoded in at least eight other
  places in this file (`--segment N (1-12)`, `all 12 segments`, `Segment <N>/12`,
  `N/12 done`). Keep twelve rows; change only what this removal broke.

  Replace row 2 with real, currently-unlisted domain packages, and trim row 8:

  ```
  | 2 | `domain/demand+dhpricing+psacampaign+liquidation` | `internal/domain/demand/`, `internal/domain/dhpricing/`, `internal/domain/psacampaign/`, `internal/domain/liquidation/`, `internal/domain/dhevents/`, `internal/domain/llmutil/` |
  | 8 | `adapters/scheduler` | `internal/adapters/scheduler/` |
  ```

  Then update the sample summary row at `.claude/skills/polish-all/SKILL.md:286`:

  ```
  | domain/demand+dhpricing+psacampaign+liquidation | 5 | 2 | 8 | ✓ |
  ```

  (It is illustrative sample output, not data — only the segment name changes.)

  **Scope boundary — report, do not fix.** This file carries a lot of staleness that
  predates this change and is not ours to clean up here: segment 3 lists
  `internal/domain/favorites/`, `internal/domain/picks/`, and `internal/domain/cards/`;
  segment 4 lists `internal/domain/csvimport/` and `internal/domain/mmutil/`; segment 6 is
  `adapters/storage/sqlite`, which the Postgres cutover removed. None of those exist.
  Leave every one of them alone and note them to the team lead as a follow-up. This step
  fixes the segment *this change* emptied, and nothing else.

  Verify the edit:

  ```bash
  cd /workspace/.worktrees/remove-azure-ai
  grep -nE 'advisor|scoring|social' .claude/skills/polish-all/SKILL.md
  ```

  Expected: no output.

- [ ] **Step 24: Run the final repo-wide grep and confirm only permitted hits remain.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai
  grep -rniE 'advisor|azure|azureai|openai|llmprovider|toolexecutor|ai_calls|ai-usage|ADVISOR_MAX_TOOL_ROUNDS|AZURE_AI_' \
    --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=dist \
    --exclude-dir=audit --exclude-dir=plans --exclude-dir=specs --exclude-dir=superpowers \
    . | grep -viE 'advisory|index advisor|Supabase.s advisor'
  ```

  Expected surviving hits, and nothing else:
  - `docs/SCHEMA.md` — the `~~advisor_cache~~`, `~~ai_calls~~`, `~~ai_usage_summary~~`,
    `~~ai_usage_by_operation~~` tombstones and the `idx_ai_calls_*` /
    `idx_advisor_cache_type` rows in the Dropped-indexes catalogue.
  - `internal/adapters/storage/postgres/migrations/` — the historical `000013` advisor_cache
    drop and the new `000032` drop pair. Migrations are append-only history; never rewrite them.
  - `internal/adapters/storage/postgres/migration_000032_test.go` — the rollback test
    **Task 7 creates**. It necessarily names `ai_calls`, `scoring_data_gaps`,
    `ai_usage_summary`, and `ai_usage_by_operation` throughout: a test that proves the
    drop happened has to refer to what was dropped. Dozens of hits here are correct.
  - `.claude/skills/ui-screenshot-improve/workspace/trigger-eval.seed.json:20` — a tracked
    eval fixture whose sample query reads "why is the **advisor** response slow on the
    campaigns page…". It is synthetic prompt text used to test whether the screenshot skill
    triggers, not a reference to the advisor feature — nothing about it breaks when the
    code is gone. Leave it; editing eval fixtures to satisfy a grep changes what the eval
    measures. It survives the filter because it says "advisor", not "advisory".

  Anything else is a miss: fix it and re-run Steps 19–24.

- [ ] **Step 25: Stage and commit.**

  ```bash
  cd /workspace/.worktrees/remove-azure-ai
  git add docs/ internal/README.md internal/testutil/mocks/README.md CLAUDE.md \
          .claude/skills/polish-all/SKILL.md
  git status --short
  ```

  Confirm `docs/LLM_USAGE.md` shows as `D` and no unintended file is staged, then:

  ```bash
  git commit -m "docs: drop advisor, Azure AI, and scoring documentation

Deletes docs/LLM_USAGE.md and removes every live documentation claim about the
Azure AI advisor, the ai/advisor domain interfaces, and the scoring domain:
ARCHITECTURE.md layer diagram, package tree and interface table; API.md
/api/admin/ai-usage and AI Advisor endpoint sections; internal/README.md domain
tree and package table; the advisor service-mock row; and CLAUDE.md's AI env
group, orientation bullets, and LLM Usage link.

SCHEMA.md keeps the removed objects as tombstones in the existing
advisor_cache style (ai_calls, scoring_data_gaps, ai_usage_summary,
ai_usage_by_operation) and leaves the Dropped-indexes catalogue intact, since
both are deliberate historical records. docs/audit, docs/plans, docs/specs and
docs/superpowers are untouched for the same reason.

Also repoints the polish-all skill's hardcoded segment table, whose rows 2 and 8
named the deleted packages. Row 2 would otherwise have been left with no existing
directory at all, since internal/domain/social was already gone."
  ```

  No ticket reference: none of the seven other commits in this change set carry one,
  and the plan cannot invent an identifier it does not have.
