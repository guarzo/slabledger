# Improve Skill — Design Spec

**Date**: 2026-04-04
**Skill name**: `improve`
**Location**: `/workspace/.claude/skills/improve/SKILL.md`

## Purpose

A reusable skill for continuously improving the SlabLedger codebase. Performs a holistic review covering quality, maintainability, dead code, duplication, UX friction, test gaps, architecture, and documentation — then produces a force-ranked top-10 list of the highest-impact improvements.

## Invocation

```
/improve                    # Full holistic review of entire codebase
/improve backend            # Deep dive into Go backend
/improve frontend           # Deep dive into React frontend
/improve <package-name>     # Deep dive into a specific domain/adapter area (e.g., campaigns, pricing)
```

## Skill Frontmatter

```yaml
---
name: improve
description: Holistic codebase review — quality, maintainability, dead code, duplication, UX friction — produces a prioritized top-10 list of improvements
argument-hint: "[backend | frontend | <package-name>]"
---
```

All tools are allowed (Bash for commands, Read/Grep/Glob for code analysis, Agent for parallel work).

## Two-Phase Architecture

### Phase 1: Quantitative Sweep

Run all available tooling to establish a factual baseline. Execute in parallel where possible. Failures are findings, not blockers — capture all output.

**Backend commands**:
- `make check` — lint + architecture import check + file size check
- `go test -race -coverprofile=coverage.out ./...` — tests + coverage data
- `go tool cover -func=coverage.out` — per-function coverage breakdown
- `golangci-lint run --out-format json` — structured lint findings

**Frontend commands**:
- `cd web && npm run lint:strict` — ESLint with zero warnings
- `cd web && npm run typecheck` — TypeScript strict checking
- `cd web && npm test` — Vitest suite

**Structural analysis** (via Read/Grep/Glob):
- File count and LOC distribution by package
- Files approaching or exceeding the 500-line guideline
- Test file coverage gaps (source files with no corresponding `_test.go`)
- Packages with no tests at all

For focused runs, only execute commands relevant to the focus area. E.g., `/improve frontend` skips Go commands; `/improve campaigns` runs Go tooling but focuses coverage/lint output on that package.

### Phase 2: Qualitative Analysis

Using Phase 1 data as a guide, do targeted code reading across these categories:

1. **Maintainability** — Large files (identify natural split points), high cyclomatic complexity, deep nesting, long parameter lists, god-objects
2. **Dead Code & Unused Dependencies** — Exported functions/types with no external callers, unused struct fields/constants/interface methods, stale imports or dependencies in `go.mod`/`package.json`
3. **Duplication** — Similar logic repeated across packages, copy-pasted handler boilerplate, frontend components with overlapping functionality
4. **Code Quality** — Lint warnings triaged by severity, error handling gaps (swallowed errors, missing context wrapping), inconsistent patterns
5. **Test Quality** — Packages with low/zero coverage, tests that don't assert meaningfully, missing edge case coverage for critical business logic
6. **Architecture** — Hexagonal violations or near-violations, business logic leaking into adapters, domain packages growing too broad
7. **UX Friction** — Loading/error/empty states, accessibility gaps, inconsistent UI patterns (included by default in full-codebase mode and frontend focus; skipped in backend-only focus)
8. **Documentation & API** — Stale docs that don't match code, undocumented endpoints, missing Go↔TS type sync, misleading comments

For focused runs, apply only relevant categories. For package-level focus, go deeper on all categories within that scope.

## Output Format

### Top 10 Priorities

Each finding:

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

**Ranking logic**: Impact-weighted against effort. High-severity/small-effort items rank highest; low-severity/large-effort items rank lowest.

**Type distinction**:
- **Issue**: Objectively verifiable — lint failure, missing tests, architecture violation, dead code
- **Observation**: Judgment call — UX could be better, naming is confusing, API ergonomics

### Interactive Follow-up

After presenting the top 10:

> "Want to dig into any of these? Pick a number to discuss further, or say 'go' to start working on #1."

From there, conversational — user can reorder priorities, ask for more detail, or jump into fixing.

## Light Tracking

After each review, save a summary to the memory system at `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md`.

Format:
```markdown
---
name: improve-findings
description: Latest codebase review findings from /improve skill — used for tracking improvement over time
type: project
---

## Last Run: YYYY-MM-DD
**Scope**: full | backend | frontend | <package>

### Top Findings
1. [Title] — [Status: open | resolved | improved]
2. ...

### Key Metrics
- Go lint warnings: N
- Files over 500 LOC: N
- Packages with zero tests: N
- Frontend lint warnings: N
- Test failures: N
```

On subsequent runs, read this file first to compare. Note what improved, what regressed, and what persists. Not heavy diff tracking — just lightweight "last time we found X, is it still there?" comparison.

Update `MEMORY.md` with a pointer to this file if not already present.

## Scope Rules by Argument

| Argument | Phase 1 Commands | Phase 2 Categories | UX Category |
|----------|-----------------|-------------------|-------------|
| (none) | All backend + frontend | All 8 categories | Included |
| `backend` | Backend only | 1-6, 8 (no UX) | Excluded |
| `frontend` | Frontend only | 1-4, 7-8 (no architecture) | Included |
| `<package>` | Backend scoped to package | All applicable, deep dive | If frontend-adjacent |

**Package argument validation**: When a `<package>` argument is provided, verify it matches an existing directory under `internal/domain/` or `internal/adapters/clients/`. If no match, list available packages and ask the user to pick one.

## Skill File Structure

```
.claude/skills/improve/
└── SKILL.md          # Main skill definition
```

No reference files needed — the skill reads CLAUDE.md and existing project docs for context. Keeps it simple and self-contained.
