# Remove the Azure AI / advisor feature

**Date:** 2026-08-08
**Branch:** `remove-azure-ai`
**Status:** Approved for planning

## Problem

SlabLedger carries a complete LLM stack — an Azure AI client, an advisor service
with a tool loop, a scoring engine, and AI call tracking — whose only working
consumer is a local CLI subcommand the owner does not use.

The investigation that prompted this removal established:

- `internal/adapters/clients/azureai/` implements `ai.LLMProvider` and is
  constructed for real in `initializeAdvisorService`
  (`cmd/slabledger/init_services.go:66`), gated on `AZURE_AI_ENDPOINT` and
  `AZURE_AI_API_KEY` being non-empty.
- The two HTTP endpoints (`POST /api/advisor/digest`,
  `POST /api/advisor/liquidation-analysis`) and the `/advisor` SPA route are
  registered (`internal/adapters/httpserver/routes.go:213-215`) but **no
  frontend code calls them**. There is no `/advisor` route in
  `web/src/react/App.tsx`, so the SPA handler serves an index page that falls
  through to `<Route path="*">` and redirects to `/login`. The only thing that
  touches those endpoints is a Playwright *mock*
  (`web/tests/screenshot-all-pages.spec.ts:40`).
- Those endpoints are nonetheless live, authenticated, and **documented as
  supported API operations** (`docs/API.md:1560-1593`). "No frontend caller" is
  not "no caller" — see the API compatibility decision below.
- The only path anyone is known to invoke is
  `slabledger admin analyze <digest|liquidation>`
  (`cmd/slabledger/admin_analyze.go`).
- The scoring engine is not independently reachable. `adapters/scoring` is
  constructed only inside `initializeAdvisorService`, and `domain/scoring`'s
  only importers are `domain/advisor`, `advisortool`, the gap store, and the
  gap-cleanup scheduler that exists solely to prune what scoring writes.

Additional evidence that the feature has been decaying: `ai.OpCampaignAnalysis`
is declared but never emitted, and the `ai_calls` CHECK constraint still lists
three operations (`purchase_assessment`, `social_caption`, `social_suggestion`)
that no longer exist anywhere in the code.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Scope | Full removal, all four tiers | Tiers 1-3 are one unit; nothing survives independently |
| Postgres tables | Drop both via migration | Avoids permanent schema drift and orphaned RLS policies |
| `admin analyze` CLI | Remove | Owner confirmed it is unused |
| Documented HTTP API | **Accept the break** | `POST /api/advisor/digest` and `/api/advisor/liquidation-analysis` are documented but auth-gated and single-operator; no external consumer exists. Removing them is a deliberate, accepted incompatibility, not an oversight. |
| Existing AI price suggestions | Keep the data, drop the producer | `campaign_purchases.ai_suggested_price_cents` and its accept/clear paths are untouched. Suggestions already recorded stay reviewable and dismissable; nothing new is ever written. |

## End state

No LLM capability remains in the repository: no `LLMProvider` seam, no scoring
engine, no AI call tracking, no Azure configuration. `go.mod` loses
`github.com/openai/openai-go/v3` — verified to have no importer outside the
`azureai` package.

DH pricing, CardLadder, liquidation, and intelligence are untouched. None routes
through the advisor.

## Deletions

**Whole packages (45 files)**

| Package | Files |
|---|---|
| `internal/adapters/clients/azureai` | 6 |
| `internal/domain/advisor` | 12 |
| `internal/domain/ai` | 5 |
| `internal/adapters/advisortool` | 8 |
| `internal/adapters/scoring` | 2 |
| `internal/domain/scoring` | 12 |

**Individual files (9)**

- `internal/adapters/httpserver/handlers/advisor.go` and `advisor_test.go`
- `internal/adapters/httpserver/handlers/ai_status_handler.go`
- `internal/adapters/storage/postgres/ai_tracker.go`
- `internal/adapters/storage/postgres/gap_store.go`
- `internal/adapters/scheduler/gap_cleanup.go`
- `cmd/slabledger/admin_analyze.go`
- `internal/testutil/mocks/gap_store.go`
- `internal/testutil/mocks/advisor_service.go`

`ai_status_handler.go`, `gap_cleanup.go`, and `ai_tracker.go` have no test files.

## Wiring unwind

| File | What to remove |
|---|---|
| `cmd/slabledger/init_services.go` | `initializeAdvisorService` entirely |
| `cmd/slabledger/main.go` | gap store construction, advisor tool opts, advisor init call, `GapStore` and `AzureAIClient` deps |
| `cmd/slabledger/handlers.go` | `AzureAIClient`/`AICallRepo` fields, advisor + AI status handler construction and wiring |
| `cmd/slabledger/server.go` | `AdvisorHandler` and `AIStatusHandler` fields and assignments |
| `cmd/slabledger/admin.go` | `analyze` case (`:24`), help text (`:45`), usage example (`:54`) |
| `cmd/slabledger/init_schedulers.go` | `advisor` import (`:11`), `AdvisorService` and `AICallRepo` dep fields (`:38-39`), `AICallTracker` propagation (`:75`), `GapStore` dep field (`:54`) and its propagation (`:197-198`) |
| `internal/adapters/httpserver/routes.go` | `registerAdvisorRoutes`, the `/api/admin/ai-usage` route |
| `internal/adapters/httpserver/router.go` | handler fields (`:35-36`), config fields (`:73-74`), assignments (`:149-153`) |
| `internal/adapters/scheduler/builder.go` | `domain/ai` import (`:9`), `AICallTracker` dep field (`:43-44`), `NewGapCleanupScheduler` call and `GapStore` dep |
| `internal/platform/config/types.go` | 4 Azure fields + `AdvisorRefreshConfig` |
| `internal/platform/config/defaults.go` | `AdvisorRefresh` default block (`:76-78`) |
| `internal/platform/config/loader.go` | `AZURE_AI_*` and `ADVISOR_MAX_TOOL_ROUNDS` parsing |

## Migration `000032_drop_ai_advisor_tables`

Patterned on `000030_drop_marketmovers_config`. `000031` is the current head.

`ai_calls` carries **two dependent views**, both created in `000003` with
`security_invoker = true`: `ai_usage_summary`
(`000003_supabase_security_and_perf_fixes.up.sql:33-51`) and
`ai_usage_by_operation` (`:53-61`). `DROP TABLE` errors out on a dependent view
rather than cascading, so the views must be handled explicitly. `scoring_data_gaps`
has no dependent views.

**Up:** `DROP VIEW IF EXISTS public.ai_usage_summary` and
`public.ai_usage_by_operation` **first**, then drop `ai_calls` and
`scoring_data_gaps`. Prefer explicit view drops over `DROP TABLE ... CASCADE`:
cascade would silently take anything else that later attaches to these tables.
Table indexes and RLS policies do drop automatically with their tables — views
are the exception, not the rule.

**Down:** recreate both tables, their indexes, **and** the `TO service_role` RLS
policies with the matching REVOKEs — then recreate both views with
`WITH (security_invoker = true)` and re-apply their REVOKEs from `anon` and
`authenticated`. `000028` revokes on the views by name, not just the tables
(`000028_tighten_000003_rls_policies.up.sql:56-58`, `:82-87`), so a down
migration that restores only tables leaves the two views readable by
`anon`/`authenticated` even when the tables underneath are locked down. A down
migration that restores only the schema would likewise reopen the tables,
because `000003`'s original policy style defaulted to `TO PUBLIC`.
Role-dependent statements must be guarded on `pg_roles` so the migration still
applies on a local Postgres where Supabase roles do not exist.

## Frontend

- Delete `web/src/react/pages/admin/AIStatusTab.tsx`; unwire from `StatsTab.tsx`.
- Remove `useAIUsage` from `web/src/react/queries/useAdminQueries.ts`.
- Remove `getAIUsage` and its interface entry from `web/src/js/api/admin.ts`.
- Remove `AIOperationSummary` and `AIUsageResponse` from `web/src/types/apiStatus.ts`.
- Remove the advisor route mock from `web/tests/screenshot-all-pages.spec.ts`.

**`AIPricingTab` stays, with one edit.** It survives because it renders price
*overrides* — a manual, non-LLM feature — via `usePriceOverrideStats`. But it
also surfaces AI price suggestions ("Pending Suggestion Value",
`AIPricingTab.tsx:92-98`), and its empty state instructs the user to *"run the
AI advisor for pricing suggestions"* (`:107`) — an action that will no longer
exist.

Per the decision above, suggestion **data** survives and the **producer** does
not. The only production writers of `ai_suggested_price_cents` are
`advisortool` (`tools_portfolio_analysis.go:151`, `tools_portfolio_batch.go:29`,
both registered at `executor.go:326,335`), all of which are deleted; the store
methods that accept and clear suggestions
(`internal/adapters/storage/postgres/purchase_price_store.go`) are untouched.
So any suggestion already in the database stays visible and dismissable, and the
count only ever decreases.

Required edit: rewrite the empty-state line at `AIPricingTab.tsx:107` to drop
the advisor instruction, keeping the override guidance. Leave the suggestion
display itself intact.

## Documentation

- Delete `docs/LLM_USAGE.md`.
- `docs/SCHEMA.md`: replace both table sections **and both view sections**
  (`ai_usage_summary`, `ai_usage_by_operation` at `:1066-1070`) with tombstones
  in the existing `~~advisor_cache~~ — DROPPED (migration 000013)` style.
- `docs/API.md`: remove the AI Advisor section (`:1560-1593`).
- `docs/ARCHITECTURE.md`: remove **four** interface rows, not one — `advisor`
  `Service`, `advisor` `CacheStore`, `ai` `LLMProvider`, and `ai` `ToolExecutor`
  (`:255-258`) — plus the `ai/`, `advisor/`, `azureai/` tree entries; correct the
  scheduler line.
- `internal/README.md`: remove `scoring/` from the domain tree diagram (`:45`)
  and from the package table (`:96`).
- `CLAUDE.md`: remove the AI env var group and the LLM Usage doc link.
- `.env.example`: remove the four `AZURE_AI_*` entries.

`docs/LOOP.md` and `docs/USER_GUIDE.md` need no changes — verified they never
reference the advisor.

## Verification

1. `go build ./...`
2. `go test -race ./...`
3. `make check` — lint, import checker, file size, Playwright version
4. `cd web && npm run build && npm test`

Baseline before any change: build clean, **0 test failures**.

Watch the import checker specifically. It fails closed if fewer than two
inventory siblings are derived or if the scan performs an unexpected number of
checks. This was verified rather than assumed: deriving the sibling set on the
branch point returns exactly the ten packages CLAUDE.md documents (arbitrage,
demand, dhlisting, dhpricing, export, finance, portfolio, pricing/lookup,
psacampaign, tuning), and `domain/scoring` is not among them. Removing it
therefore cannot change the derived count.

## Risks

**The migration is the only irreversible step, and it ships on merge.** This
repository auto-deploys from `main` via Fly. Historical AI cost and token data
in `ai_calls` is destroyed at deploy time. Anyone who wants that data must
export it before the PR merges.

## Out of scope

- `domain/intelligence` suggestions — DH Tier 3 data, independently owned.
- The tuning, arbitrage, portfolio, finance, and export services — `advisortool`
  consumed them through the executor but does not own them.
- `AIPricingTab` and price-override statistics.
- `internal/adapters/scheduler/dh_orders_poll_test.go`. A naive grep flags it,
  but its only match is the ordinary English word "advisory" describing a grade
  (`:187`) — no advisor dependency. Do not touch it. The same false positive
  exists in `internal/domain/inventory/core_types.go:543`.
