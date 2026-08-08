# Codebase Tech-Debt Audit — Design

**Date:** 2026-08-07 (revised 2026-08-08 after independent review)
**Status:** Revised; pending implementation plan
**Branch:** `tech-debt-audit`
**Baseline revision:** `740976ecf80a4f2ccdaa611d7790ccaa95b48773`

## Problem

SlabLedger has undergone heavy feature development and at least one change of
product focus. The suspicion is accumulated tech debt: dead code, misnamed or
misplaced modules, stale abstractions, and documentation that no longer
describes the system.

Two confirmed findings motivate the audit. Both concern the same subsystem, and
both were verified against the code at the baseline revision:

| Evidence | Reality |
|---|---|
| `internal/adapters/clients/cardladder/` | **106** references outside its own package. Initialized in production at `cmd/slabledger/init_services.go:101`, scheduled at `internal/adapters/scheduler/cardladder_refresh.go:85`, routed at `internal/adapters/httpserver/routes.go:64`. Load-bearing. |
| `CLAUDE.md:137` | States DH is the "sole price source" and that CardHedger/PriceCharting/JustTCG were "removed on 2026-04-06". |
| `CLAUDE.md:34-58` | Omits five domain packages that exist: `demand`, `dhevents`, `dhpricing`, `liquidation`, `psacampaign`. |

So the architecture documentation describes a system meaningfully different from
the one that is running, and none of it is detectable by the repo's automated
checks. That is the debt this audit exists to find.

## Case study: how this design's first finding was wrong

The original draft of this spec led with a different exemplar: that
`internal/adapters/clients/marketmovers/` was orphaned dead code, based on a
directory listing plus a reference count of zero.

**It was false.** That path is not a Go package and is not in git. It exists
only in the author's main checkout, containing a single untracked scratch file,
`grade_probe_test.go.tmp`. Migration `000021_drop_market_movers` drops
`mm_sales_comps`, `mm_card_mappings`, and MM columns — but not
`marketmovers_config`, which is still created at
`000001_initial_schema.up.sql:513`.

Two further baseline figures in the original draft were also wrong:

| Claimed | Actual | Cause |
|---|---|---|
| 27 migrations | 25 | `grep up` matched `..._add_su`**p**`abase_....down.sql` |
| 26 `t.Skip` | 6 | matched `result.Skipped`, `t.Errorf("Skipped…`, `…DuplicateSkip` |
| 37 markers | 1 | case-insensitive match on unrelated substrings; scope undefined |

This matters more than the corrections themselves. The original draft argued
that false-positive dead code was the audit's central risk and that mechanical
proof was the mitigation — and then produced three false positives **using the
exact method it proposed**, within one working session, on the first attempt.

The failure was not carelessness about a conclusion; it was trusting substring
matching as evidence of reachability. A fleet of thirteen agents applying that
method across 100k LOC would produce a ticket queue whose false positives are
indistinguishable from true findings, because both arrive decorated with real
`file:line` citations.

Everything in the Evidence Protocol below is a response to this episode. It is
recorded here rather than quietly fixed because implementers need to know why
the protocol is strict.

### The same instinct, correctly executed

The suspicion behind the false finding was not baseless — it was aimed at the
wrong artifact. Applying the protocol to the *database* instead of a
nonexistent Go package yields a real candidate:

`marketmovers_config` is created at `000001_initial_schema.up.sql:514`, granted
RLS policies at `000003_supabase_security_and_perf_fixes.up.sql:169-170`, is
**not** dropped by `000021_drop_market_movers`, and has zero references in
tracked Go files. Its schema comment at line 513 —
`-- marketmovers_config: sqlite.MarketMoversConfig [marketmovers_store.go:17]` —
points at a SQLite-era store that no longer exists; four other migration lines
carry the same stale provenance.

This is recorded as a **candidate for the `db-schema` lens**, at confidence
`strong` rather than `mechanical`: string-built SQL and external consumers have
not yet been ruled out per the protocol. Promoting it is the lens's job, not
this document's.

The contrast is the lesson. Same hunch, same subsystem — but one version named a
path that was never in git, and the other names a table with a `CREATE`
statement, a line number, and a stated confidence limit.

## Baseline

Measured at revision `740976ec` with the exact commands given, so that any agent
can reproduce them and disagreement is resolvable rather than a matter of
opinion. Agents MUST NOT re-derive these by other means.

```
go build ./...          OK
go vet ./...            OK
golangci-lint run       0 issues          (gates defined at Makefile:139-149)
check-imports.sh        PASS (no domain→adapter, no inventory cross-sibling)
check-file-size.sh      PASS with 4 warnings (>500 lines)
check-playwright        PASS
go test ./...           PASS (exit 0, no failures)
```

| Metric | Value | Command |
|---|---|---|
| Go non-test LOC | 70,280 | `git ls-files '*.go' \| grep -v '_test\.go$' \| xargs wc -l \| tail -1` |
| Go test LOC | 61,378 | `git ls-files '*_test.go' \| xargs wc -l \| tail -1` |
| Go files | 676 | `git ls-files '*.go' \| wc -l` |
| Go packages | 53 (4 untested) | `go list ./... \| wc -l` |
| Frontend files | 218 | `git ls-files 'web/src/*.ts' 'web/src/*.tsx' \| wc -l` |
| Frontend LOC | 27,759 | as above, `xargs wc -l` |
| Migrations | 25 | `ls …/migrations/*.up.sql \| wc -l` |
| `.env.example` vars | 71 | `grep -cE '^[A-Z][A-Z0-9_]*=' .env.example` |
| `t.Skip` calls | 6 | `git ls-files '*_test.go' \| xargs grep -hE '\bt\.Skipf?\(' \| wc -l` |
| TODO/FIXME/HACK/XXX in comments | 1 | `git ls-files '*.go' '*.ts' '*.tsx' \| xargs grep -nE '(//\|/\*)[^\n]*\b(TODO\|FIXME\|HACK\|XXX)\b'` |
| Commits, last 6 months | 521 | `git log --oneline --since=6.months \| wc -l` |

Packages without test files: `internal/adapters/clients/dhlisting`,
`internal/domain/observability`, `internal/domain/storage`, `internal/testutil`.

The single TODO marker is at `internal/adapters/scheduler/price_refresh_test.go:25`
and records disabled scheduler tests — itself a candidate finding.

**Calibration consequence:** every mechanical gate the repo owns already passes.
This audit targets only what tooling cannot see — semantic reachability,
conceptual naming drift, stale abstraction, duplication, and doc/reality
mismatch. Restating lint output is not a finding.

**Untracked-file rule.** All analysis operates on **git-tracked files at the
baseline revision**, enumerated via `git ls-files`. Working-tree listings
(`ls`, `find`) are forbidden as evidence: they surface scratch files, build
artifacts, and ignored directories that are not part of the codebase. This rule
exists because violating it produced the false exemplar above.

## Goals

1. Produce a comprehensive, evidence-backed inventory of tech debt across the Go
   backend, frontend, database, and docs/config/tests.
2. Structure the work so independent agents can run in parallel without
   duplicating effort or contradicting each other.
3. Convert verified findings into actionable Linear tickets, one per
   independently-shippable fix unit.

## Non-Goals

- **No code changes.** Strictly read-only; tickets are the only output.
- Not a bug hunt, except where a defect *is* the debt.
- Not a performance audit.
- Not a redesign. Findings propose targeted remediation.

## Design decisions

### Topology: global map, then bounded auditors

Reachability is a global property: no agent scoped to a subtree can determine
whether the rest of the repo references its subject. That argues for computing a
reference map once, mechanically, before any judgment happens.

It does **not** argue against territory-scoped auditors. An earlier draft framed
this as map-versus-territory; that was a false dichotomy, since a territory
auditor can simply consume the global map. The real design constraint is
narrower and stands on its own:

> No agent may assert reachability from its own search. Reachability is read
> from the shared map, which is computed once.

Lenses below are therefore assigned **exclusive ownership** of subject kinds, so
two agents never adjudicate the same artifact and reach different verdicts.

### Evidence protocol (replaces "zero refs + build passes")

The original bar — zero references plus a passing build without the subject —
is both unexecutable under read-only tooling and insufficient. Go defeats naive
reference counting in ways this repository actively uses:

| Mechanism | Present at | Why counting fails |
|---|---|---|
| Functional options satisfying interfaces at runtime | `internal/domain/arbitrage/service.go:80`, `internal/domain/pricing/lookup/adapter.go:65` | Deleting `WithRequestCache` still compiles; behavior silently degrades |
| `embed.FS` assets | `internal/adapters/storage/postgres/embedded_migrations.go:9` | Migrations have no ordinary Go references |
| Build-tagged integration tests | `internal/integration/cardladder_test.go:4` | Excluded from default builds |
| JSON struct tags as API contract | `internal/domain/liquidation/types.go:23` | Field is a runtime contract; no Go caller |

Accordingly, a **removal claim** requires all of:

1. **Subject identity** — fully-qualified symbol/table/field, not a path.
2. **Searched universe, stated** — the `git ls-files` glob searched and every
   exclusion, so the search is reproducible and its blind spots visible.
3. **Tag matrix** — searched with default tags *and* every build tag in the repo
   (minimally `integration`), plus `_test.go` files.
4. **Runtime-reachability checks**, each explicitly answered:
   interface satisfaction · struct embedding · serialization tags ·
   registration/DI wiring (`cmd/`, `init_services.go`, scheduler builders,
   route tables) · `init()` side effects · `embed` directives · reflection and
   string-keyed lookup · config/env binding · SQL identifiers referenced as
   strings.
5. **Negative-evidence limitation, acknowledged.** A `file:line` citation can
   prove presence but never absence. Absence claims cite the *command and its
   empty output*, at the baseline revision.

Claims failing any element are demoted to `suspected`, never dropped. A
suspicion is worth recording; it is not worth a remediation ticket.

Because the audit is read-only, "build passes without it" is **not** performed.
Removal is validated by the checklist above; actual removal-and-build is
deferred to the ticket's own acceptance criteria, where it belongs.

## Architecture

Four phases, outputs written to `docs/audit/` on this branch.

```
Phase 0  Ground truth        (sequential, mechanical — complete, above)
              |
Phase 1  Scouts              (5 parallel)  → reference maps; data, not opinions
              |            [COMPLETENESS GATE]
Phase 2  Lens auditors       (8 parallel)  → findings in fixed schema
              |
Phase 3  Adversarial verify  (parallel)    → confirm / refute / demote
              |
Phase 4  Consolidate         (sequential)  → dedup, cluster, rank → Linear
```

### Phase 1 — Scouts (5 parallel)

Scouts emit structured data, not judgments. A scout offering an opinion has
exceeded its role.

| Scout | Produces |
|---|---|
| `go-reference-map` | Every exported symbol, definition site, external reference count, under each build tag. Entry points reachable from `cmd/`. Registration sites. |
| `frontend-reference-map` | Component/hook/type inventory with reference counts; route table; API-call sites. |
| `db-map` | Table/column inventory from the 25 migrations, mapped to reading/writing code, including string-literal SQL. |
| `config-map` | Every config field and env var mapped to consumption sites; `.env.example` cross-reference. |
| `docs-map` | Every factual claim in `CLAUDE.md`, `docs/`, `internal/README.md`, with the code location that confirms or refutes it. |

### Completeness gate (between Phase 1 and 2)

Scout failure is silent and catastrophic: a scout that returns a truncated map
makes real code look unreferenced, and an empty Phase 2 finding set would
otherwise satisfy every downstream check. No lens starts until:

1. **Expected-count reconciliation.** Each scout declares totals; these must
   match the Phase 0 baseline (e.g. `go-reference-map` covers all 676 Go files
   in all 53 packages; `db-map` covers all 25 migrations). Mismatch = rerun.
2. **Schema validation.** Every record parses against the declared schema.
3. **Provenance.** Every record carries the baseline revision and the command
   that produced it.
4. **Sampled independent recomputation.** A verifier independently recomputes a
   random sample of records; any disagreement fails the gate.
5. **Explicit status.** Scouts report `complete` / `partial` / `failed`.
   `partial` and `failed` block dependent lenses; they never silently proceed.

A gate failure stops the phase. Partial fleet output is not merged.

### Phase 2 — Lens auditors (8 parallel, exclusive ownership)

Reachability comes from the map; lenses supply judgment. Each subject kind has
exactly one owning lens, eliminating the overlap in the original draft, where
dead-code duplicated the db, frontend, and config lenses.

| Lens | Owns | Looks for |
|---|---|---|
| `dead-code-go` | Go symbols/packages | Unreferenced Go code, per the evidence protocol |
| `naming-and-boundaries` | package/symbol names | Names that no longer match behavior; code in the wrong package; concepts split or wrongly merged |
| `architecture` | layering | Violations tooling misses: concrete types leaked through interfaces, domain logic in adapters, anemic pass-through layers |
| `duplication` | cross-cutting | Exact and near-duplicate logic, parallel implementations, copy-paste divergence |
| `size-and-complexity` | files/functions | Oversized files, god objects, over-broad interfaces |
| `db-schema` | tables/columns/indexes/migrations | Unused schema, migrations for dropped features, `docs/SCHEMA.md` drift |
| `frontend-health` | TS/TSX symbols | Unused components/hooks, type drift vs Go JSON tags, dead routes, duplicated API patterns |
| `docs-config-tests` | doc claims, env vars, tests | Doc/code contradictions, dead env vars, skipped tests, untested packages |

Cross-kind observations are filed as a pointer to the owning lens, never as a
competing finding.

### Finding schema

```json
{
  "id": "DEAD-001",
  "lens": "dead-code-go",
  "revision": "740976ec",
  "subject": {"kind": "symbol|package|table|column|env|component|claim",
              "identity": "fully-qualified name"},
  "category": "dead-code|naming|architecture|duplication|size|db|frontend|docs",
  "title": "short imperative statement",
  "severity": "high|medium|low",
  "confidence": "mechanical|strong|suspected",
  "evidence": [{"claim": "...", "command": "...", "output": "...",
                "file_line": "path:120"}],
  "searched_universe": "git ls-files glob(s) searched",
  "exclusions": ["what was not searched, and why"],
  "build_tags": ["default", "integration"],
  "runtime_checks": {"interface_satisfaction": "...", "embedding": "...",
                     "serialization": "...", "registration": "...",
                     "reflection": "..."},
  "proposed_fix": "what to do",
  "blast_radius": ["files/packages affected"],
  "effort": "S|M|L",
  "acceptance_criteria": ["how the fixer proves the change is correct"],
  "verifier": {"verdict": "confirmed|refuted|uncertain", "rationale": "..."}
}
```

`confidence: mechanical` requires every element of the evidence protocol. It is
the only tier eligible for an unqualified ticket.

### Phase 3 — Adversarial verification

Each finding goes to an independent agent **prompted to refute it**, defaulting
to refuted under uncertainty. The verifier's specific job is the runtime-semantic
gap: it must actively look for the mechanisms tabulated above before accepting
any removal claim.

`confirmed` → ticket. `refuted` → dropped, reason recorded. `uncertain` →
investigation tier.

### Phase 4 — Consolidation and Linear

1. **Dedup** across lenses, preserving all evidence.
2. **Cluster** into independently-shippable fix units — one mergeable PR each.
3. **Rank** by risk × effort.
4. **File** one ticket per fix unit, carrying claim, evidence, proposed fix,
   blast radius, effort, and acceptance criteria.

Expected volume: ~15–30 tickets.

**Destination gate:** the target Linear team and project are confirmed with the
user before any ticket is created.

## Risks

| Risk | Mitigation |
|---|---|
| **False-positive dead code** — demonstrated, not hypothetical (see case study) | Evidence protocol with runtime-reachability checklist; git-tracked-only rule; adversarial verifier; read-only output |
| **Silent scout failure poisoning every lens** | Completeness gate: count reconciliation, schema validation, provenance, sampled recomputation, explicit status |
| Absence claimed via `file:line` | Absence must cite command + empty output at the baseline revision |
| Agents contradicting each other | Phase 0 facts computed once; exclusive lens ownership |
| Findings that restate lint | Baseline shows lint clean; lenses scoped to what tooling cannot see |
| Scope creep into redesign | Non-goals explicit; targeted remediation only |
| Ticket spam | Dedup and clustering precede filing |

## Verification

Complete when:

1. Phase 1 passed the completeness gate with all scouts `complete`.
2. Every Phase 2 finding has a Phase 3 verdict.
3. Every confirmed finding maps to exactly one ticket or is explicitly folded.
4. Every `mechanical` finding satisfies every element of the evidence protocol.
5. The branch contains only documentation — `git diff --stat main..HEAD` shows
   no changes under `internal/`, `cmd/`, or `web/src/`.
6. `make check` and the test suite pass, unchanged from baseline.

A non-empty finding set is **not** a success criterion. "No debt found in area X,
here is the search that established it" is a valid and useful outcome.

## Open items

- Linear team/project destination — confirmed with the user before Phase 4 files
  anything.
- Stray untracked `internal/adapters/clients/marketmovers/grade_probe_test.go.tmp`
  in the main checkout: trivial local cleanup, not a repo change, not a ticket.
