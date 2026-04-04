---
name: improve
description: Holistic codebase review — quality, maintainability, dead code, duplication, UX friction — produces a prioritized top-10 list of improvements
argument-hint: "[backend | frontend | <package-name>]"
---

# Codebase Improvement Review

Perform a holistic codebase review and produce a force-ranked top-10 list of the highest-impact improvements. Use this skill to continuously improve the SlabLedger codebase.

## Step 0: Parse argument and determine scope

Parse the argument to determine review scope:

| Argument | Scope | Phase 1 Commands | Phase 2 Categories |
|----------|-------|-----------------|-------------------|
| *(empty)* | Full codebase | All backend + frontend | All 8 categories |
| `backend` | Go backend only | Backend only | Categories 1-6, 8 |
| `frontend` | React frontend only | Frontend only | Categories 1-4, 7-8 |
| `<package>` | Specific package deep dive | Backend scoped to package | All applicable, deep dive |

**Package argument validation:** If the argument is not `backend` or `frontend`, verify it matches an existing directory under `internal/domain/` or `internal/adapters/clients/`. If no match, list available packages and ask the user to pick one.

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

### Structural analysis (all scopes)

Use Glob and Grep to gather:

1. **File sizes** — Find all `.go` files (excluding `*_test.go` and `testutil/mocks/`) and count lines. Flag files approaching or exceeding the 500-line guideline.
2. **Test coverage gaps** — For each non-test `.go` file, check whether a corresponding `_test.go` exists in the same directory. List packages with no test files at all.
3. **LOC distribution** — Count lines per top-level package (`internal/domain/*`, `internal/adapters/*`, `internal/platform/*`, `cmd/*`, `web/src/*`).

For package-scoped runs, restrict structural analysis to that package.

## Step 3: Phase 2 — Qualitative Analysis

Using Phase 1 data as a guide, do targeted code reading. Prioritize areas the numbers flagged — large files, low-coverage packages, lint warnings — then broaden.

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
- Review loading states, error states, and empty states across key pages
- Check for accessibility gaps (missing aria labels, keyboard navigation issues)
- Identify inconsistent UI patterns across pages

### Category 8: Documentation & API
- Spot-check that docs match current code (especially `docs/API.md` endpoints, `docs/SCHEMA.md` tables)
- Check for Go struct JSON tags that don't match TypeScript interfaces in `web/src/types/`
- Identify undocumented endpoints or misleading comments

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

## Step 5: Save findings to memory

Write the summary to `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md`:

```markdown
---
name: improve-findings
description: Latest codebase review findings from /improve skill — used for tracking improvement over time
type: project
---

## Last Run: [today's date]
**Scope**: [full | backend | frontend | <package>]

### Top Findings
1. [Title] — Status: open
2. [Title] — Status: open
...

### Key Metrics
- Go lint warnings: [N]
- Files over 500 LOC: [N]
- Packages with zero tests: [N]
- Frontend lint warnings: [N]
- Test failures: [N]
```

If `MEMORY.md` exists in the memory directory, ensure it has a pointer to this file. If `MEMORY.md` does not exist, create it:

```markdown
- [Improve Findings](improve_findings.md) — Latest /improve skill review results and metrics
```

## Step 6: Interactive follow-up

After presenting the top 10, ask:

> "Want to dig into any of these? Pick a number to discuss further, or say 'go' to start working on #1."

From there, be conversational:
- If the user picks a number, provide deeper analysis of that finding — show the specific code, explain the trade-offs, discuss approach options
- If the user says "go" or picks a number to work on, help implement the fix
- If the user wants to reorder priorities, adjust and re-present
- If the user is done, wrap up
