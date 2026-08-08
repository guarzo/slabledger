# Audit Agent Preamble — READ FULLY BEFORE ANY TOOL CALL

You are one agent in a read-only tech-debt audit of the SlabLedger codebase.

## Absolute rules

1. **READ-ONLY.** You must not edit, create, or delete any file under
   `internal/`, `cmd/`, or `web/src/`. You write exactly one output file, at
   the path your task names. Nothing else.
2. **Baseline revision is `740976ecf80a4f2ccdaa611d7790ccaa95b48773`.**
   Record it on every record you emit.
3. **Git-tracked files only.** Enumerate with `git ls-files`. Do NOT use `ls`
   or `find` as evidence. They surface untracked scratch files, build
   artifacts, and ignored directories that are not part of the codebase.
4. **Never assert reachability from your own search.** Reachability comes from
   the Phase 1 maps in `docs/audit/maps/`. If you need it and it is not there,
   report that as a gap — do not compute it yourself.
5. **Absence is never proven by a `file:line` citation.** A citation proves
   presence. To claim something is unused, cite the *command you ran and its
   empty output*.

## Why these rules exist

An earlier draft of this audit's design claimed
`internal/adapters/clients/marketmovers/` was orphaned dead code. It was false:
that path is not a Go package and is not in git — it was a single untracked
scratch file in one developer's checkout, surfaced by `ls`. Two baseline counts
were also wrong, because `grep up` matched "su**p**abase" and `grep t.Skip`
matched `result.Skipped`.

Three false positives, in one session, from the very method the document was
recommending. Substring matching is not evidence of reachability. You are
working under a strict protocol because the unstructured version demonstrably
fails.

## Go defeats naive reference counting — check all nine

This repository actively uses every mechanism below. Before claiming anything
is unused, answer each explicitly:

| # | Check | Example in this repo |
|---|---|---|
| 1 | Interface satisfaction | `internal/domain/arbitrage/service.go:80` — anonymous interface assertion |
| 2 | Functional options | `internal/domain/pricing/lookup/adapter.go:65` — `WithRequestCache`; deleting it still compiles, behavior silently degrades |
| 3 | Struct embedding | promoted methods have no direct call site |
| 4 | Serialization tags | `internal/domain/liquidation/types.go:23` — JSON tag is a runtime API contract with no Go caller |
| 5 | `//go:embed` | `internal/adapters/storage/postgres/embedded_migrations.go:9` — migrations have zero Go references |
| 6 | Build tags | `internal/integration/cardladder_test.go:4` — `//go:build integration`, excluded from default builds |
| 7 | Registration / DI wiring | `cmd/slabledger/init_services.go`, scheduler builders, `routes.go` |
| 8 | `init()` side effects | package imported only for effect |
| 9 | Reflection / string-keyed lookup / string-built SQL | table and column names as string literals |

## Search discipline

- Word-boundary anchored: `git grep -nE '\bSymbolName\b'`, never a bare substring.
- Search both tag sets: default, and `integration`.
- Include `_test.go` files — test-only usage is still usage, and is itself a finding of a different kind.
- State your searched universe (the `git ls-files` globs) and every exclusion.

## Confidence tiers

- `mechanical` — every element of the evidence protocol satisfied, all nine
  runtime checks answered. Only this tier may be ticketed unqualified.
- `strong` — compelling evidence, one or more checks unresolved. Say which.
- `suspected` — worth recording, not worth a ticket.

Downgrade under uncertainty. A demoted true finding costs little; a promoted
false one costs a reviewer's trust in the entire audit.

## Output discipline

- Emit valid JSON against your task's schema. Nothing else in the file.
- **Finding nothing is a valid, useful result.** Do not manufacture findings to
  appear productive. "No debt in area X, here is the search that establishes
  it" is a real deliverable.
- Restating `golangci-lint` output is not a finding. Lint is clean at baseline;
  your job is what tooling cannot see.
- Stay in your lane. If you spot something owned by another lens, emit it as a
  `pointer` record, not a finding.
