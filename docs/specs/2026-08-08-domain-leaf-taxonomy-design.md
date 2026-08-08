# Leaf/non-leaf taxonomy for internal/domain/ (SLA-48)

**Ticket:** [SLA-48](https://linear.app/slabledger/issue/SLA-48) · Follow-up from SLA-45 (flat-sibling escape hatch)
**Date:** 2026-08-08

## Problem

`scripts/check-imports.sh` enforces the flat sibling rule: inventory sub-packages must not
import each other, but *may* import the hub and "leaf packages." The script has no idea
what a leaf package is. Neither does any document in the repository.

Two rules already lean on the term. SLA-45 declined to close its residual escape hatch
specifically because the fix "is a definition, not a check" — a package that stops
importing the hub arguably *has* become a leaf, and importing a leaf is legal under the
rule as written. You cannot enforce against a target set nobody has defined.

The constraint that makes this hard: hardcoding a leaf allowlist reintroduces the
stale-list failure mode SLA-12 was opened to eliminate. Any answer must be derived from
the tree.

## Measurement

Derived 2026-08-08 from the tree (re-derive before acting; these figures go stale):

```bash
go list -f '{{$p := .ImportPath}}{{range .Imports}}{{$p}} -> {{.}}
{{end}}' ./internal/domain/... \
  | grep -- '-> github.com/guarzo/slabledger/internal/domain'
```

42 `domain → domain` edges across 26 packages. Of these, 36 target either the hub
(`inventory`, 10 edges) or one of `errors`, `observability`, `constants`, `mathutil`,
`timeutil` (26 edges).

Six edges target something else:

| Edge | In SLA-48's ticket text? |
|---|---|
| `advisor → ai` | yes |
| `advisor → scoring` | yes |
| `dhlisting → dhevents` | yes |
| `pricing/lookup → pricing` | yes |
| `inventory → dhevents` | **no** |
| `inventory → intelligence` | **no** |

The ticket lists four because it counted only non-hub *sources*. The hub's own two
outbound non-leaf edges are subject to the same taxonomy and must be ruled on, or they
read as violations under any rule phrased as "non-leaf may not import non-leaf."

## Decision

### The definition

> A package under `internal/domain/` is a **leaf** if its transitive import closure within
> `internal/domain/` excludes `internal/domain/inventory`. It is **non-leaf** otherwise —
> the hub itself, plus every package that depends on it directly or transitively.
>
> Any domain package may import a leaf, or the hub. Importing any other non-leaf is a
> violation.

This is fully derived; there is no list to maintain. Verified against the tree, it
partitions all 26 packages, yielding exactly the 10 governed siblings plus the hub as
non-leaf:

```
non-leaf: inventory (hub), arbitrage, demand, dhlisting, dhpricing,
          export, finance, portfolio, pricing/lookup, psacampaign, tuning
leaf:     advisor, ai, auth, constants, dhevents, errors, intelligence,
          liquidation, llmutil, mathutil, observability, pricing, scoring,
          storage, timeutil
```

"Governed sibling" and "non-leaf other than the hub" are the same set by construction.
That identity is why one derivation can drive both rules.

### Rulings on the six edges

All six are **legal**: every target is a leaf.

| Edge | Target | Ruling |
|---|---|---|
| `advisor → ai` | leaf | legal |
| `advisor → scoring` | leaf | legal |
| `dhlisting → dhevents` | leaf | legal |
| `pricing/lookup → pricing` | leaf | legal |
| `inventory → dhevents` | leaf | legal |
| `inventory → intelligence` | leaf | legal |

No package moves. All 42 current edges conform to the taxonomy as written.

### Enforcement

Membership derivation in `check-imports.sh` changes from **direct** hub import to
**transitive** hub dependency:

- Build the direct `domain → domain` edge set by grep, as today.
- Seed the governed set with packages importing the hub directly.
- Iterate to a fixed point: if `X → Y` and `Y` is governed, govern `X`.

Everything downstream — the fail-closed `< 2 siblings` guard, the two-pass check
accounting, target-side enforcement — consumes the derived set unchanged.

Against the current tree this yields the same 10 packages, so there is **no behavior
change today**. It is forward hardening: a package can no longer leave the governed set by
routing its hub dependency through an intermediary.

### Why not `go list -deps`

`go list` would resolve the real build graph, but `check-imports-test.sh:4-6` documents
that the checker never compiles — its five fixture cases are directory trees containing
the right import *text*, with no `go.mod`. Adopting `go list` would require rewriting
every fixture as a compilable module, discarding that property and slowing the self-test.
Transitive closure over the grep-derived edge set needs no toolchain and preserves the
harness.

### Why not enforce leaf-targeting universally

A stricter rule — "no domain package may import any non-leaf except the hub" — is
implementable and currently has zero violations. It is out of scope because it would newly
constrain the `advisor`, `pricing`, and `scoring` families, which the flat sibling rule has
never governed, without evidence that those families have a problem. Recorded here as a
considered and rejected option, not an oversight.

## Effect on SLA-45's residual limitation

**Re-assessed. Not closed.** The reason is now grounded in a definition rather than
deferred to this ticket.

The transitive upgrade narrows the hatch: a package can no longer escape by re-routing its
hub dependency through an intermediary, because the intermediary's governance propagates
back to it.

It does not close it. If `dhlisting` severs *all* hub dependency, it becomes a leaf under
this taxonomy, and `dhpricing → dhlisting` becomes legal — correctly so, by the rule as
written. Whether a package that has shed its hub dependency still *belongs* to the
inventory family is a question about intent, not topology. No derivation over the import
graph can answer it, and encoding the answer by hand is the hardcoded list this taxonomy
exists to avoid.

The residual is therefore a deliberate boundary of the approach, not an unfinished task.
`docs/specs/2026-08-08-flat-sibling-escape-hatch-design.md` is updated to say so.

## Testing

- **New fixture Case 6** in `check-imports-test.sh`: `A → L`, `L → hub`, `A → beta` (a
  governed sibling). Under direct membership `A` is ungoverned and the `A → beta`
  violation is missed; under transitive membership both `A` and `L` are governed and the
  violation is flagged. This case fails on the old script and passes on the new one.
- **Cases 1–5 unmodified** — they are the regression signal that the derived set for a
  conforming tree is unchanged.
- `./scripts/check-imports.sh` still reports 10 sub-packages.
- `make check`, `go test -race ./...`.

## Documentation

- `internal/README.md` — canonical statement of the taxonomy, the rulings, and the derivation command.
- `CLAUDE.md` — short pointer from the existing flat-sibling section; no duplicated package list.
- `docs/specs/2026-08-08-flat-sibling-escape-hatch-design.md` — Residual limitation section updated.

## Out of scope

- Universal leaf-target enforcement outside the inventory family (rejected above).
- Any change to `internal/domain/` package structure.
- Re-litigating the six edges' design merits; this rules on their *legality* only.
