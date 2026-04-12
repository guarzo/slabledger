---
name: improve
description: Holistic codebase review — quality, maintainability, dead code, duplication, UX friction — produces a prioritized top-10 list of improvements. Use when the user asks about tech debt, what to refactor, code audit, codebase health, what needs fixing, improvement opportunities, or general code quality review.
argument-hint: "[backend | frontend | diff | since:<date> | <package-name>]"
---

# Codebase Improvement Review

Perform a holistic codebase review and produce a force-ranked top-10 list of the highest-impact improvements. Use this skill to continuously improve the SlabLedger codebase.

## Step 0: Parse argument and determine scope

Parse the argument to determine review scope:

| Argument | Scope | Phase 1 Commands | Phase 2 Categories |
|----------|-------|-----------------|-------------------|
| *(empty)* | Full codebase | All backend + frontend | All 10 categories |
| `backend` | Go backend only | Backend only | Categories 1-6, 8-10 |
| `frontend` | React frontend only | Frontend only | Categories 1-4, 7-8 |
| `diff` | Files changed since last `/improve` run | Scoped to changed files | All applicable |
| `since:<date>` | Files changed since date (e.g. `since:2026-04-10`) | Scoped to changed files | All applicable |
| `<package>` | Specific package deep dive | Backend scoped to package | All applicable, deep dive |

**Diff-aware mode:** For `diff` or `since:<date>`, use `git diff --name-only` to get the list of changed files. For `diff`, use the date from the last run stored in the memory file. For `since:<date>`, parse the date from the argument. Scope both Phase 1 and Phase 2 to only those files. This is useful as a lightweight post-work check rather than a full sweep.

**Package argument validation:** If the argument is not a recognized keyword (`backend`, `frontend`, `diff`, or `since:*`), verify it matches an existing directory under `internal/` (any level — `domain/`, `adapters/httpserver/`, `adapters/storage/sqlite/`, `adapters/scheduler/`, `platform/`, etc.) or `web/src/`. If no match, list available packages and ask the user to pick one.

## Step 1: Check for previous findings

Read the memory file at `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md` if it exists. Note what was found last time so you can compare — which issues persist, which improved, which are new. If the file does not exist, this is the first run.

## Step 2: Phase 1 — Quantitative Sweep

Run all available tooling to establish a factual baseline. Execute commands in parallel where possible. **Failures are findings, not blockers** — capture all output and continue.

### Backend commands (skip if scope is `frontend`)

Run these in parallel:

```bash
# Quality checks: lint + architecture import check + file size check
make check

# Tests + coverage data
go test -race -timeout 10m -coverprofile=coverage.out ./...

# Per-function coverage breakdown
go tool cover -func=coverage.out

# Structured lint findings
golangci-lint run --out-format json
```

For package-scoped runs, add the package path to test/lint commands:
```bash
go test -race -coverprofile=coverage.out ./internal/domain/<package>/...
golangci-lint run --out-format json ./internal/domain/<package>/...
```

### Frontend commands (skip if scope is `backend` or a specific package)

Run these in parallel:

```bash
cd web && npm run lint:strict
cd web && npm run typecheck
cd web && npm test
```

### Dependency health (all scopes)

```bash
# Check for unused Go modules
go mod tidy -diff 2>&1 || true

# Check for known npm vulnerabilities
cd web && npm audit --audit-level=moderate 2>&1 || true
```

Capture any findings — unused modules or vulnerabilities are findings, not blockers.

### Structural analysis (all scopes)

Use Glob and Grep to gather:

1. **File sizes** — Find all `.go` files (excluding `*_test.go` and `testutil/mocks/`) and count lines. Flag files approaching or exceeding the 500-line guideline.
2. **Coverage by package** — Parse the `go tool cover -func=coverage.out` output. Extract the overall coverage percentage (the last line: `total:`) and per-package coverage. Sort packages by coverage ascending. Flag any functions at 0% in critical packages (`campaigns`, `pricing`, `auth`, `storage/sqlite`).
3. **Test file gaps** — List packages that have no `_test.go` files at all.
4. **LOC distribution** — Count lines per top-level package (`internal/domain/*`, `internal/adapters/*`, `internal/platform/*`, `cmd/*`, `web/src/*`).

For package-scoped or diff-scoped runs, restrict structural analysis to the relevant files.

## Step 3: Phase 2 — Qualitative Analysis

Using Phase 1 data as a guide, do targeted code reading. Prioritize areas the numbers flagged — large files, low-coverage packages, lint warnings — then broaden.

**Parallelize with sub-agents:** For full-codebase or backend scope, spawn up to 3 Explore agents in parallel, each responsible for a subset of categories. Share the Phase 1 data summary with each agent so they have context. Each agent should return its top findings using the format from Step 4.

- **Agent 1 — Structural concerns**: Categories 1 (Maintainability), 2 (Dead Code), 3 (Duplication)
- **Agent 2 — Correctness concerns**: Categories 4 (Code Quality), 5 (Tests), 6 (Architecture), 9 (Performance)
- **Agent 3 — Surface concerns**: Categories 7 (UX Friction), 8 (Documentation), 10 (Dependencies)

For narrow scopes (single package, diff, frontend-only), running in the main context is fine — sub-agents add overhead that isn't worth it for small reviews.

Review across these categories (skip categories not applicable to the current scope per the table in Step 0):

### Category 1: Maintainability
- Read files flagged as large in Phase 1 — identify natural split points (separate strategies, separate concerns, utilities)
- Look for functions with high cyclomatic complexity, deep nesting, long parameter lists
- Identify god-objects or services doing too much

### Category 2: Dead Code & Unused Dependencies
- Search for exported functions/types with no callers outside their own package
- Check for unused struct fields, constants, or interface methods
- Look for stale imports in `go.mod` or unused dependencies in `package.json`

### Category 3: Duplication
- Look for similar logic repeated across packages (fee calculations, error handling patterns, HTTP handler boilerplate)
- On the frontend, look for components with overlapping functionality

### Category 4: Code Quality
- Triage lint warnings from Phase 1 by severity
- Look for swallowed errors (bare `_` on error returns) or errors missing context wrapping
- Identify inconsistent patterns (some places use one approach, others use another)

### Category 5: Test Quality
- Focus on packages with low or zero coverage from Phase 1
- Look for tests that don't assert anything meaningful (happy-path only)
- Check for missing edge case coverage on critical business logic (campaigns P&L, pricing, CSV parsing)

### Category 6: Architecture
- Check for hexagonal violations or near-violations beyond what `check-imports.sh` catches
- Look for business logic leaking into adapter packages (HTTP handlers doing calculations, SQLite queries embedding business rules)
- Identify domain packages that have grown too broad and should be split

### Category 7: UX Friction (full codebase and frontend scope only)
- Run the Playwright screenshot suite to capture current UI state:
  ```bash
  cd web && npx playwright test tests/screenshot-all-pages.spec.ts --project=chromium
  ```
- Read screenshots from `web/screenshots/` (desktop) and `web/screenshots/mobile/` (mobile) — visually check for broken layouts, overlapping elements, missing loading/empty states, inconsistent spacing
- Check for accessibility gaps (missing aria labels, keyboard navigation issues)
- Identify inconsistent UI patterns across pages

### Category 8: Documentation & API
- Spot-check that docs match current code (especially `docs/API.md` endpoints, `docs/SCHEMA.md` tables)
- Check for Go struct JSON tags that don't match TypeScript interfaces in `web/src/types/`
- Identify undocumented endpoints or misleading comments

### Category 9: Performance & Concurrency
- Look for unbounded goroutines or missing context cancellation in long-running operations
- Check for N+1 query patterns in the SQLite layer (queries inside loops)
- Identify unbounded slice growth or missing pagination in list endpoints
- Look for blocking operations in hot paths that should be async

### Category 10: Dependency Health
- Review `go mod tidy -diff` output from Phase 1 for unused modules
- Review `npm audit` output from Phase 1 for known vulnerabilities
- Check for pinned vs floating dependency versions that could cause supply chain risk

## Step 4: Synthesize and rank — Top 10

From all Phase 1 and Phase 2 findings, force-rank to the **10 highest-impact items**. Use this ranking logic: severity weighted against effort. High-severity/small-effort items rank highest; low-severity/large-effort items rank lowest.

Present each finding in this exact format:

```
### #N: [Short title]
**Category**: Maintainability | Dead Code | Duplication | Quality | Tests | Architecture | UX | Docs
**Type**: Issue | Observation
**Severity**: High | Medium | Low
**Effort**: Small (< 1hr) | Medium (1-4hr) | Large (4hr+)
**Location**: file_path:line_number (or package name for broad issues)

[2-3 sentence description of the problem and why it matters]

**Suggested approach**: [1-2 sentences on how to fix it]
```

**Type definitions:**
- **Issue**: Objectively verifiable — lint failure, missing tests, architecture violation, dead code
- **Observation**: Judgment call — UX could be better, naming is confusing, API ergonomics

If there are previous findings from Step 1, note for each item whether it is **new**, **persists** from last run, or represents a **regression**.

## Step 5: Save findings to memory and clean up

Write the summary to `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md`. When updating an existing file, **preserve the metrics history table** — append a new row and keep the last 5 rows. For findings, update the status of resolved items (add the PR number if known) and add new items.

```markdown
---
name: improve-findings
description: Latest codebase review findings from /improve skill — used for tracking improvement over time
type: project
---

## Last Run: [today's date]
**Scope**: [full | backend | frontend | diff | <package>]

### Metrics History
| Date | Scope | Coverage | Lint | 500+ LOC | nil,nil | npm audit |
|------|-------|----------|------|----------|---------|-----------|
| [today] | [scope] | [N%] | [N] | [N] | [N] | [N] |
| [prev] | [scope] | [N%] | [N] | [N] | [N] | [N] |

### Top Findings
1. [Title] — opened [date], status: open
2. [Title] — opened [date], status: open
3. [Title] — opened [prev date], status: resolved (PR #NNN)
...

### Key Metrics (current run)
- Go test coverage: [N%]
- Go lint warnings: [N]
- Files over 500 LOC: [N] ([list them])
- Packages with zero tests: [N]
- Frontend TS type errors: [N]
- Frontend lint warnings: [N]
- Test failures: [N]
- return nil, nil in production: [N]
- npm audit vulnerabilities: [N]
```

If `MEMORY.md` exists in the memory directory, ensure it has a pointer to this file. If `MEMORY.md` does not exist, create it:

```markdown
- [Improve Findings](improve_findings.md) — Latest /improve skill review results and metrics
```

**Clean up artifacts:**

```bash
rm -f coverage.out
```

## Step 6: Interactive follow-up

After presenting the top 10, offer these options:

> "What next? You can:
> - **Pick a number** to dig deeper into a specific finding
> - **'go'** to start working on #1
> - **'quick wins'** to tackle all Small-effort items in sequence
> - **'compare'** to see how metrics changed over time
> - **'diff'** to see what changed since the last run"

From there, be conversational:
- If the user picks a number, provide deeper analysis of that finding — show the specific code, explain the trade-offs, discuss approach options
- If the user says "go" or picks a number to work on, help implement the fix
- If the user says "quick wins", filter to Small-effort items and work through them in order
- If the user says "compare", show the metrics history table with trend arrows
- If the user says "diff", run `git diff` from the last run date and summarize what changed
- If the user wants to reorder priorities, adjust and re-present
- If the user is done, wrap up
