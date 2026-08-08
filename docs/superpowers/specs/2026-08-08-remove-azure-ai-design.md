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
- The sole path that reaches Azure is
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
| `cmd/slabledger/init_schedulers.go` | `GapStore` dep field (`:54`) and its propagation (`:197-198`) |
| `internal/adapters/httpserver/routes.go` | `registerAdvisorRoutes`, the `/api/admin/ai-usage` route |
| `internal/adapters/httpserver/router.go` | handler fields (`:35-36`), config fields (`:73-74`), assignments (`:149-153`) |
| `internal/adapters/scheduler/builder.go` | `NewGapCleanupScheduler` call and `GapStore` dep |
| `internal/adapters/scheduler/dh_orders_poll_test.go` | advisor references |
| `internal/platform/config/types.go` | 4 Azure fields + advisor refresh config |
| `internal/platform/config/loader.go` | `AZURE_AI_*` and `ADVISOR_MAX_TOOL_ROUNDS` parsing |

## Migration `000032_drop_ai_advisor_tables`

Patterned on `000030_drop_marketmovers_config`. `000031` is the current head.

**Up:** drop `ai_calls` and `scoring_data_gaps`. Their indexes and their RLS
policies from `000003`/`000028` are dropped automatically with the tables.

**Down:** recreate both tables, their indexes, **and** the `TO service_role` RLS
policies with the matching REVOKEs. A down migration that restores only the
schema would silently reopen two tables to `anon`/`authenticated`, because
`000003`'s original policy style defaulted to `TO PUBLIC`. Role-dependent
statements must be guarded on `pg_roles` so the migration still applies on a
local Postgres where Supabase roles do not exist.

## Frontend

- Delete `web/src/react/pages/admin/AIStatusTab.tsx`; unwire from `StatsTab.tsx`.
- Remove `useAIUsage` from `web/src/react/queries/useAdminQueries.ts`.
- Remove `getAIUsage` and its interface entry from `web/src/js/api/admin.ts`.
- Remove `AIOperationSummary` and `AIUsageResponse` from `web/src/types/apiStatus.ts`.
- Remove the advisor route mock from `web/tests/screenshot-all-pages.spec.ts`.

`AIPricingTab` **stays**. Despite the name it renders price-override stats via
`usePriceOverrideStats` and involves no LLM.

## Documentation

- Delete `docs/LLM_USAGE.md`.
- `docs/SCHEMA.md`: replace both table sections with tombstones in the existing
  `~~advisor_cache~~ — DROPPED (migration 000013)` style.
- `docs/API.md`: remove the AI Advisor section.
- `docs/ARCHITECTURE.md`: remove `LLMProvider` from the interface list and the
  `ai/`, `advisor/`, `azureai/` tree entries; correct the scheduler line.
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
