# Leaf/non-leaf taxonomy for internal/domain/ (SLA-48)

**Ticket:** [SLA-48](https://linear.app/slabledger/issue/SLA-48) · Follow-up from SLA-45 (flat-sibling escape hatch)
**Date:** 2026-08-08

## Problem

`scripts/check-imports.sh` enforces the flat sibling rule: inventory sub-packages must not
import each other, but *may* import the hub and "leaf packages" (`check-imports.sh:12-13`).
The script has no definition of a leaf package. Neither does any document in the
repository.

Two rules already lean on the term. SLA-45 declined to close its residual escape hatch
specifically because the fix "is a definition, not a check" — a package that stops
importing the hub arguably *has* become a leaf, and importing a leaf is legal under the
rule as written. You cannot reason about a target set nobody has defined.

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
outbound edges are subject to the same taxonomy and must be ruled on, or they read as
violations under any rule phrased as "non-leaf may not import non-leaf."

## Decision

### The definition

> A package under `internal/domain/` is a **leaf** if its transitive import closure within
> `internal/domain/` excludes `internal/domain/inventory`. It is **non-leaf** otherwise —
> the hub itself, plus every package that depends on it directly or transitively.
>
> Any domain package may import a leaf, or the hub. Importing any other non-leaf is a
> violation.

Fully derived; there is no list to maintain. Verified against the tree, it partitions all
26 packages:

```
non-leaf: inventory (hub), arbitrage, demand, dhlisting, dhpricing,
          export, finance, portfolio, pricing/lookup, psacampaign, tuning
leaf:     advisor, ai, auth, constants, dhevents, errors, intelligence,
          liquidation, llmutil, mathutil, observability, pricing, scoring,
          storage, timeutil
```

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

### Enforcement: already in place, no script change

**The existing checker already enforces this taxonomy.** Naming the rule revealed that
SLA-45's target-side scan had been implementing it all along.

The argument is an identity between two sets:

1. `check-imports.sh:63-81` derives the governed set as the packages importing the hub
   **directly**.
2. The taxonomy's non-leaf set is the packages depending on the hub **transitively**,
   minus the hub itself.

These differ only for a package that reaches the hub *through another non-leaf* — and such
a package imports a governed sibling, which `check-imports.sh:125-155` already flags. So on
any tree that **passes** the checker the two sets are identical, and on any tree where they
diverge the checker is already failing. `governed sibling ≡ non-leaf minus the hub` holds
exactly where it needs to.

Because pass 2 scans every non-test file under `internal/domain/` against every governed
sibling regardless of the importer's own membership, the rule it enforces is precisely:

> no package under `internal/domain/` may import a non-leaf other than the hub

which is the taxonomy. Nothing to add.

### Rejected: transitive (fixed-point) membership

An earlier draft proposed changing membership from direct to transitive hub dependency, on
the theory that a package could otherwise escape governance by routing its hub dependency
through an intermediary. **This was wrong, and is recorded here so it is not re-proposed.**

Transitive closure can only add a package that imports an already-governed package. That
import is itself a violation. So the fixed point is reached at iteration zero on every
conforming tree: the change is not "no behavior change today," it is provably a no-op
wherever the checker passes.

The accompanying regression fixture was also invalid — it duplicated existing Case 1
(`check-imports-test.sh:74-83`: `rogue → beta`, `beta → hub`, `rogue` ungoverned), which
already pins exactly that behavior.

Credit: caught in independent review, not by the author.

### On `go list -deps`

The definition is phrased over the transitive closure, but no code needs to compute it —
see the identity above. This matters because the checker deliberately never compiles:
`check-imports-test.sh:4-6` documents that its fixtures are directory trees containing the
right import *text*, with no `go.mod`. Any future proposal to resolve the taxonomy with
`go list` would require rewriting every fixture as a compilable module. The `go list`
command in **Measurement** above is for humans auditing the tree, not for the checker.

## Effect on SLA-45's residual limitation

**Re-assessed. Not closed.** The reason is now grounded in a definition rather than
deferred to this ticket.

If `dhlisting` severs *all* hub dependency it becomes a leaf under this taxonomy, and
`dhpricing → dhlisting` becomes legal — correctly so, by the rule as written. Whether a
package that has shed its hub dependency still *belongs* to the inventory family is a
question about intent, not topology. No derivation over the import graph can answer it,
and encoding the answer by hand is the hardcoded list this taxonomy exists to avoid.

The residual is a deliberate boundary of the approach, not an unfinished task. SLA-48 does
not narrow it: the rejected fixed-point change would not have narrowed it either.
`docs/specs/2026-08-08-flat-sibling-escape-hatch-design.md` is updated to say so, and to
stop pointing at SLA-48 as pending work.

## Changes to make

Documentation only. No behavior change.

- `internal/README.md` — canonical statement of the taxonomy, the six rulings, the
  derivation command, and the identity that makes the existing check sufficient.
- `CLAUDE.md` — short pointer from the existing flat-sibling section; no duplicated
  package list.
- `scripts/check-imports.sh` — extend the header comment (`:12-13`) so "leaf packages"
  cites the definition instead of leaving it undefined. Comment only.
- `docs/specs/2026-08-08-flat-sibling-escape-hatch-design.md` — Residual limitation
  section updated per above.

## Verification

- `./scripts/check-imports-test.sh` — all five existing cases still pass, unmodified.
- `./scripts/check-imports.sh` — still reports 10 sub-packages, 1610 checks.
- `make check`.
- No new fixture case: the behavior a new case would have pinned is already pinned by
  Case 1.

## Out of scope

- Any change to `internal/domain/` package structure.
- Any change to checker logic. This ticket names a rule that already exists; it does not
  add one.
- Re-litigating the six edges' design merits; this rules on their *legality* only.
