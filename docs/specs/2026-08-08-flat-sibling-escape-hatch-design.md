# Flat-sibling checker: close the membership escape hatch (SLA-45)

**Ticket:** [SLA-45](https://linear.app/slabledger/issue/SLA-45) · Follow-up from SLA-12 (PR #558)
**Date:** 2026-08-08

## Problem

`scripts/check-imports.sh` derives its governed set from the *source* side of an edge: a
package is a sibling if one of its non-test `.go` files imports
`internal/domain/inventory`. Enforcement then scans only those packages' files.

A package therefore leaves enforcement by dropping its hub import, taking its outgoing
sibling edges with it. A refactor that removed `dhpricing`'s `inventory` import would
silently re-legalize the exact `dhpricing → dhlisting` edge SLA-12 deleted. Nothing
fails; the checker stops looking. Silence is indistinguishable from a pass.

## Decision

Keep the narrow membership test. Move enforcement to the **target** side.

- **Membership (unchanged):** a governed sibling is any directory under
  `internal/domain/` with a non-test `.go` file importing `internal/domain/inventory`.
- **Enforcement (changed):** flag any import of a governed sibling from any package
  under `internal/domain/`, whether or not the importer imports the hub.

### Why not broad membership

The ticket asks whether the derived set should instead be "every directory under
`internal/domain/` except the hub." Measured against the current tree, that flags **24
edges**, of which **20** are legitimate leaf dependencies (`errors`, `observability`,
`constants`, `mathutil`, `timeutil`). (Counting the hub's own imports of non-hub
packages, which the proposed all-file scan would also walk, the figure is 31.)
Making it usable requires a hardcoded *leaf
allowlist* — reintroducing the stale-hardcoded-list failure mode SLA-12 exists to
eliminate. The remaining four (`advisor → ai`, `advisor → scoring`,
`dhlisting → dhevents`, `pricing/lookup → pricing`) sit outside the inventory family and
appear intentional.

Whether `internal/domain/` needs a formal leaf/non-leaf taxonomy is a real question, but
a separate one. It is tracked as SLA-48.

### Cost of the chosen option

Zero triage. Every domain→domain edge was enumerated; **no package currently imports a
governed sibling**, so target-based enforcement surfaces no existing violations.

Governed siblings today (10): `arbitrage`, `demand`, `dhlisting`, `dhpricing`, `export`,
`finance`, `portfolio`, `pricing/lookup`, `psacampaign`, `tuning`.

## Design

### 1. Target-based scan (`scripts/check-imports.sh`)

Derivation of `siblings` is unchanged. The scan is restructured: instead of iterating
sibling packages and grepping their files, iterate **every** non-test `.go` file under
`internal/domain/`, resolve its owning package from its path, and flag any import of a
governed sibling other than its own package.

Resolving the owning directory per file also retires the `-maxdepth 1` nesting
workaround — `pricing/lookup` is attributed correctly by construction rather than by a
special case.

Hub→sibling edges need no special-casing: every governed sibling imports `inventory` by
definition, so `inventory` importing one back is an import cycle the Go compiler already
rejects.

### 2. Fail-closed accounting

`expected_pairs = n*(n-1)` no longer describes the scan shape. Replaced with a
file-oriented invariant: every non-test file is checked against every sibling except its
own package, so

```
checks_performed == Σ_files ( n_siblings − [owner ∈ siblings] )
```

Asserting equality (not merely `> 0`) still catches a traversal that lost files midway.
The `n_siblings ≥ 2` guard is retained — below that the rule cannot mean anything.

### 3. Self-test (`scripts/check-imports-test.sh`)

The checker is ~100 lines of non-trivial bash with fail-closed accounting and no test.

The script uses **relative** paths (`internal/domain`), so the test needs no
parameterization of the script under test: it builds a fixture tree in a temp directory
with an `internal/domain/<pkg>/*.go` layout, `cd`s there, and invokes the real
`check-imports.sh` by absolute path. Fixture files carry the real module prefix as
literal text — the checker greps, it never compiles.

Cases:

| # | Fixture | Expect |
|---|---------|--------|
| 1 | Importer of a governed sibling that does **not** import the hub (the escape hatch) | exit ≠ 0 |
| 2 | Classic sibling → sibling | exit ≠ 0 |
| 3 | Clean tree (siblings import only the hub) | exit 0 |
| 4 | Fewer than 2 siblings | exit ≠ 0 (fail-closed guard) |

Each failing case also asserts the **message names the offending edge**, so a script that
dies for an unrelated reason cannot score a pass.

Wired into `make check` ahead of `check-imports.sh` — test the tester first.

### 4. Documentation (`CLAUDE.md`)

Rule text updated to state the split explicitly: membership is derived from importing the
hub; enforcement applies to any importer of a governed sibling.

## Residual limitation (documented, not fixed)

This does not close the single-package escape hatch. It halves it.

Today both endpoints must be governed for an edge to be checked (`check-imports.sh:83`
and `:97` iterate the same derived set), so *either* package can escape by dropping its
hub import. Target-based enforcement removes the source-side escape: a governed target is
protected from every importer under `internal/domain/`, whether or not that importer is
itself governed.

The **target** can still escape unilaterally. If `dhlisting` drops its hub import
(`internal/domain/dhlisting/dh_listing_service.go:12`), `dhpricing → dhlisting` becomes
legal even though `dhpricing` remains governed. No mechanism here prevents that, and the
spec does not claim otherwise.

We accept this rather than fix it, because the fix is a definition, not a check: a
package that stops importing the hub arguably *has* become a leaf, and importing a leaf
is legal under the rule as written. Deciding when that is true requires the leaf/non-leaf
taxonomy for `internal/domain/`, which is SLA-48. Enforcing against a
target set we cannot yet define would mean hardcoding one — the failure mode SLA-12 was
opened to remove.

## Verification

- `bash scripts/check-imports-test.sh` — all four cases pass
- `make check` passes
- `go test -race ./...` passes
- Escape-hatch probe from the ticket reproduced against the *old* script (exits 0, the
  bug) and the *new* one (exits non-zero, fixed)

## Out of scope

- Leaf/non-leaf taxonomy for `internal/domain/` — SLA-48
- The four existing non-inventory edges surfaced by the broad option
- Any change to `internal/domain/` package structure; this is a checker + docs change
