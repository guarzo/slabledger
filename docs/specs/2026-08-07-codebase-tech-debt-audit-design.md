# Codebase Tech-Debt Audit — Design

**Date:** 2026-08-07
**Status:** Approved (design), pending implementation plan
**Branch:** `tech-debt-audit`

## Problem

SlabLedger has undergone heavy feature development and at least one change of
product focus. The suspicion is accumulated tech debt: dead code, misnamed or
misplaced modules, stale abstractions, and documentation that no longer
describes the system.

Two findings gathered during design confirm the suspicion, and confirm it fails
in **both** directions:

| Evidence | Reality |
|---|---|
| `internal/adapters/clients/marketmovers/` | **0** references outside its own package. Migration `000021_drop_market_movers` dropped its table. Orphaned code. |
| `internal/adapters/clients/cardladder/` | **106** references outside its own package. Very much alive, including a 525-line scheduler. |
| `CLAUDE.md` | States DH is the "sole price source" and that CardHedger/PriceCharting/JustTCG were "removed on 2026-04-06". |

So the documentation both **retains** a source that is gone and **omits** a
source that is load-bearing. Neither error is detectable by the existing
automated checks.

Similarly, `internal/domain/` contains `demand`, `dhevents`, `dhpricing`,
`liquidation`, and `psacampaign` — none of which appear in the architecture
section of `CLAUDE.md`.

## Baseline (measured 2026-08-07 on `tech-debt-audit`)

Established before any analysis, so findings are attributable and no agent
re-derives them:

```
go build ./...          OK
go vet ./...            OK
golangci-lint run       0 issues
check-imports.sh        PASS (no domain→adapter, no inventory cross-sibling)
check-file-size.sh      PASS with 4 warnings (>500 lines)
check-playwright        PASS
go test ./...           PASS (exit 0, no failures)
```

Packages: 53 total, of which 4 have no test files —
`internal/adapters/clients/dhlisting`, `internal/domain/observability`,
`internal/domain/storage`, `internal/testutil`.

Scale:

| Metric | Value |
|---|---|
| Go non-test LOC | 70,280 |
| Go test LOC | 61,378 |
| Go files | 676 |
| Go packages | 53 (4 with no test files) |
| Frontend TS/TSX files | 218 |
| Frontend LOC | 27,759 |
| Migrations | 27 |
| `.env.example` variables | 71 |
| Config struct fields | 99 |
| `t.Skip` occurrences | 26 |
| TODO/FIXME/HACK/deprecated markers | 37 |
| Commits in last 6 months | 521 |

**Calibration consequence:** every mechanical gate the repo owns already passes.
This audit is therefore scoped to what tooling cannot see — semantic
reachability, conceptual naming drift, stale abstraction, duplication, and
doc/reality mismatch. Restating lint output is not a finding.

## Goals

1. Produce a comprehensive, **evidence-backed** inventory of tech debt across
   the Go backend, frontend, database, and docs/config/tests.
2. Structure the work so independent agents can run in parallel without
   duplicating effort or contradicting each other.
3. Convert verified findings into actionable Linear tickets, one per
   independently-shippable fix unit.

## Non-Goals

- **No code changes.** The audit is strictly read-only. Tickets are the only
  output. (Explicitly chosen by the user over "also land safe deletions".)
- Not a bug hunt. Correctness defects are out of scope except where they *are*
  the debt (e.g. a stale abstraction that silently misbehaves).
- Not a performance audit.
- Not a redesign. Findings propose targeted remediation, not re-architecture.

## Design decisions

### Why a shared map phase (the central decision)

Three shapes were considered:

- **Lens fleet** — N agents, each with one analytical lens, each sweeping the
  whole repo. Catches cross-cutting themes, but every agent must hold ~100k LOC
  and their coverage overlaps heavily.
- **Territory partition** — each agent owns a subtree. Deep and bounded, but
  structurally *cannot* find dead code: an agent auditing `marketmovers` cannot
  see whether the rest of the repo references it. This shape would have missed
  the single strongest finding gathered during design.
- **Map → Lens → Verify → Tickets** — *chosen.*

The deciding constraint: **reachability is a global property.** No
territory-scoped agent can answer "is this dead?" and no lens agent should
re-derive the answer eight times with eight different results. Building the
reference map once, mechanically, converts "is this reachable?" from a judgment
call into a lookup.

### Verification bar

Every finding is independently challenged before it becomes a ticket. Claims
that something is dead or removable require **mechanical proof**, not argument:
zero references outside the unit, and the build passing without it.

This is a direct application of the project's stated preference: *"Cite output
before declaring work done. No vibes."* Unverified findings are demoted to a
"needs investigation" tier rather than silently dropped — a suspicion is worth
recording, but it is not worth a remediation ticket.

### Read-only enforcement

Agents receive read-only tool sets. The audit branch is expected to end with
only documentation added.

## Architecture

Four phases. Each phase's output is the next phase's input, written to
`docs/audit/` on the `tech-debt-audit` branch.

```
Phase 0  Ground truth        (sequential, mechanical, already partly complete)
              |
Phase 1  Scouts              (5 parallel)  → reference maps, data not opinions
              |
Phase 2  Lens auditors       (8 parallel)  → findings in fixed schema
              |
Phase 3  Adversarial verify  (parallel)    → confirm / refute / demote
              |
Phase 4  Consolidate         (sequential)  → dedup, cluster, rank → Linear
```

### Phase 0 — Ground truth

Deterministic facts computed once and shared, so no agent re-derives them and
no two agents disagree about the baseline:

- Build, vet, lint, `make check` results (captured above).
- Full test suite result and per-package coverage.
- Go package import graph.
- Per-file last-modified date and 6-month churn count.
- File size census.

Output: `docs/audit/facts/`.

### Phase 1 — Scouts (5 parallel)

Scouts emit **structured data, not judgments**. A scout that offers an opinion
has exceeded its role.

| Scout | Produces |
|---|---|
| `go-reference-map` | Every exported symbol, its definition site, and its external reference count. Entry points reachable from `cmd/`. |
| `frontend-reference-map` | Component/hook/type inventory with reference counts; route table; API-call sites. |
| `db-map` | Table/column inventory from migrations, mapped to the code that reads or writes each. |
| `config-map` | Every config field and env var, mapped to consumption sites; `.env.example` cross-reference. |
| `docs-map` | Every factual claim in `CLAUDE.md`, `docs/`, and `internal/README.md`, with the code location that would confirm or refute it. |

### Phase 2 — Lens auditors (8 parallel)

Each lens reads Phase 0 + Phase 1 output plus the repo, and emits findings.

| Lens | Looks for |
|---|---|
| `dead-code` | Zero-reference packages, symbols, components, tables, config fields, env vars. |
| `naming-and-boundaries` | Names that no longer match behavior; code in the wrong package; concepts split across packages or merged that shouldn't be. |
| `architecture` | Hexagonal violations tooling misses (leaked concrete types through interfaces, domain logic in adapters, anemic pass-through layers). |
| `duplication` | Exact and near-duplicate logic, parallel implementations of one concept, copy-paste divergence. |
| `size-and-complexity` | Oversized files, god objects, functions with too many responsibilities, over-broad interfaces. |
| `db-schema` | Unused tables/columns/indexes, migrations for dropped features, `docs/SCHEMA.md` drift. |
| `frontend-health` | Unused components/hooks, TS type drift vs Go JSON tags, dead routes, duplicated API patterns. |
| `docs-config-tests` | Doc claims contradicted by code, dead env vars, skipped/flaky tests, untested packages, stale markers. |

### Finding schema

Every finding, from every lens, uses one shape so Phase 3 and 4 can process
them uniformly:

```json
{
  "id": "DEAD-001",
  "lens": "dead-code",
  "category": "dead-code|naming|architecture|duplication|size|db|frontend|docs",
  "title": "short imperative statement",
  "severity": "high|medium|low",
  "confidence": "mechanical|strong|suspected",
  "evidence": ["path/to/file.go:120 — what is there"],
  "claim": "what is wrong",
  "proposed_fix": "what to do about it",
  "blast_radius": ["files/packages affected"],
  "effort": "S|M|L"
}
```

`confidence: mechanical` is reserved for claims backed by reference counts or
build results — the only tier eligible for automatic ticket creation without
qualification.

### Phase 3 — Adversarial verification

Each finding is handed to an independent agent **prompted to refute it**, with
instructions to default to "refuted" under uncertainty. Delete-this claims must
be proven by zero-reference evidence and a passing build.

Outcomes: `confirmed` → ticket; `refuted` → dropped, with reason recorded;
`uncertain` → demoted to the investigation tier.

### Phase 4 — Consolidation and Linear

1. **Dedup.** Multiple lenses will surface the same underlying problem from
   different angles (the `marketmovers` case will appear in dead-code, docs, and
   db lenses). Merge into one finding, preserving all evidence.
2. **Cluster** into independently-shippable fix units. A unit is one mergeable
   PR — e.g. "Remove the `marketmovers` client and its config/env/doc surface"
   spans several findings but is one change.
3. **Rank** by risk × effort.
4. **File** one Linear ticket per fix unit (user-selected granularity), each
   containing: the claim, file:line evidence, proposed fix, blast radius,
   effort estimate, and verification steps.

Expected volume: ~15–30 tickets.

**Destination gate:** the target Linear team and project are confirmed with the
user before any ticket is created. Nothing is filed until then.

## Risks

| Risk | Mitigation |
|---|---|
| **False-positive dead code** — the primary risk. Reflection, build tags, code generation, `init()` side effects, and test-only usage all defeat naive reference counting. | Mechanical proof required; adversarial verifier; read-only means a wrong finding costs a ticket, not an outage. |
| Findings that are merely lint restatements | Baseline shows lint is clean; lenses are explicitly scoped to what tooling cannot see. |
| Agents contradicting each other on basic facts | Phase 0 computes shared facts once. |
| Scope creep into redesign proposals | Non-goals are explicit; findings must propose targeted remediation. |
| Ticket spam | Dedup and clustering happen before filing, not after. |
| Integration-tagged and skipped tests hide real usage | `db-map` and `go-reference-map` scouts include `_test.go` and `-tags integration` files in reference counting. |

## Verification

The audit is complete when:

1. Every Phase 2 finding has a Phase 3 verdict.
2. Every confirmed finding maps to exactly one Linear ticket or is explicitly
   folded into another.
3. Every ticket cites file:line evidence.
4. The `tech-debt-audit` branch contains only documentation — `git diff --stat`
   against `main` shows no changes under `internal/`, `cmd/`, or `web/src/`.
5. `make check` and the test suite still pass, unchanged from baseline.

## Open items

- Linear team/project destination — to be confirmed with the user before
  Phase 4 files anything.
