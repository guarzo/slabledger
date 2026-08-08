# Flat-Sibling Import Checker: Derive the Package Set

**Date:** 2026-08-08
**Status:** Design approved, pending implementation plan
**Ticket:** [SLA-12](https://linear.app/slabledger/issue/SLA-12/fix-the-flat-sibling-import-checker-it-skips-19-of-25-domain-packages)

## Problem

`scripts/check-imports.sh:39` enforces the flat-sibling rule against a hardcoded
list of six package names:

```bash
SUB_PACKAGES="arbitrage portfolio tuning finance export dhlisting"
```

The loop below it iterates that list rather than the filesystem, so a package not
named in it is never opened. `internal/domain/` holds 25 top-level directories
today (26 counting the nested `pricing/lookup`).
The list has not kept pace, and the gap is not theoretical: `internal/domain/dhpricing`
imports its sibling `internal/domain/dhlisting` in two production files
(`dhpricing/types.go:11`, `dhpricing/service.go:7`) and the check passes anyway.

The failure mode is worse than a missing rule. On the unmodified tree the script
prints:

```
Flat sibling rule check passed: no cross-imports between inventory sub-packages.
```

That is a false statement produced by a green check. Every "`make check` passes,
so the hexagonal invariant holds" claim — in CI (`.github/workflows/test.yml:53`),
in review, in prior plan documents — has rested on it.

## Evidence

Membership derived as *"any directory under `internal/domain/` with a non-test
`.go` file importing `internal/domain/inventory`"* yields **10** packages, not six:

```
arbitrage  demand  dhlisting  dhpricing  export
finance    portfolio  pricing/lookup  psacampaign  tuning
```

Scanning all 10x10 ordered pairs surfaces **exactly one** violation:

```
dhpricing -> dhlisting
  internal/domain/dhpricing/types.go:11
  internal/domain/dhpricing/service.go:7
```

Widening the rule does not cascade. The full cross-import graph between domain
packages is already hub-and-spoke: every sibling imports `inventory`, nothing
else crosses except this one edge, plus `dhevents` and `intelligence` acting as
pure leaves and `pricing/lookup -> pricing` (child to parent). `inventory` itself
imports no sibling.

### The one edge is an oversight, not a design

`dhpricing` was deliberately built *not* to import `dhlisting`. Its package doc
says so (`dhpricing/types.go:1-8`):

> Sibling of dhlisting; does not import it — the tiny price-resolution helper is
> inlined here.

`resolveListingPrice` and `overrideNewer` are duplicated in `dhpricing/service.go`
to honor that, and `DHReconcileResetter` is redeclared locally with the comment
"Declared here (rather than imported from dhlisting) to preserve the
flat-siblings invariant."

Someone paid real cost to keep this edge out, then imported `dhlisting` anyway
for exactly one thing: the four-field DTO `DHInventoryStatusUpdate`
(`dhlisting/types.go:135`). Both `dhlisting.DHInventoryLister` and
`dhpricing.DHPriceUpdater` declare `UpdateInventoryStatus` and are satisfied by
the *same* adapter (`adapters/clients/dhlisting/adapter.go:175`), so both must
name the same struct type. Go interface satisfaction is structural on the method
signature; redeclaring a look-alike struct in `dhpricing` would not work.

The package doc is therefore currently false.

## Design

### 1. The rule

**Membership is derived, not listed.** A *sibling* is any directory under
`internal/domain/` containing a non-test `.go` file that imports
`internal/domain/inventory`.

| | |
|---|---|
| Forbidden | sibling → sibling |
| Allowed | sibling → `inventory` (the hub, by construction of the membership test) |
| Allowed | sibling → leaf packages (`errors`, `observability`, `constants`, …) |
| Ungoverned | `inventory` → `dhevents` / `intelligence` — neither imports `inventory`, so neither is a sibling |

This is the rule `check-imports.sh:7-8` and `:64` already describe. The change is
that it becomes enforced against the real package set instead of a fixed six.

Considered and rejected: forbidding *all* cross-domain imports behind a
shared-leaf allowlist. It would require inventing three exemptions (`dhevents`,
`intelligence`, `pricing`) plus a child-to-parent carve-out for `pricing/lookup`
purely to keep the tree green — trading a stale list for a stale allowlist.

### 2. Fail closed

The defect being fixed is not "the list was wrong." It is **"the check verified
nothing and reported success."** A derived list can regress the same way: change
the module path, move the tree, and the derivation returns empty, the loop
iterates zero times, and the script exits 0 with a passing message.

**The invariant to assert is the number of comparisons performed, not the size of
the derived list.** A non-empty assertion is too weak: a derivation that yields
exactly one package is non-empty, produces zero distinct ordered pairs, compares
nothing, and still prints success — reproducing the very defect class this ticket
exists to close.

The script must therefore maintain a `pairs_checked` counter, incremented once per
ordered `(pkg, other)` comparison actually executed, and fail with a distinct
message if it is zero when the scan completes. Deriving fewer than two siblings is
itself the failure condition; there is no legitimate tree state in which this
script has nothing to compare.

This must be enforced *inside the script*, on every `make check` run — not as a
manual probe someone remembers to run once (Verification §3 exercises it, but a
human running a probe is not a property the checker holds). Without this we would
replace a hardcoded blind spot with a silent one, and the ticket's core lesson
would be lost.

### 3. Script changes — `scripts/check-imports.sh`

Replace the `SUB_PACKAGES=` line and its double loop with:

- derivation via `find` + `grep -l` over non-test `.go` files, mapped to
  directories and de-duplicated;
- a `pairs_checked` counter asserted `> 0` at the end of the scan (§2);
- a per-package scan using `find -maxdepth 1`, so the nested `pricing/lookup` is
  scanned separately from `pricing` rather than being conflated with it;
- guarding the empty-file-list case, so `grep` never falls through to reading
  stdin.

**`grep` exit-code handling.** `grep` distinguishes 1 ("no match") from ≥2
("error" — unreadable file, bad pattern). Under `set -euo pipefail` these need
opposite treatment, and the two obvious shapes are both wrong:

- a blanket `|| true` — what the current checker uses (`check-imports.sh:44-45`) —
  swallows exit 2, so an unreadable source file reads as "no violations here" and
  the scan silently loses coverage of that package;
- omitting it entirely kills the script on the *expected* no-match path, which is
  the common case for a clean tree.

Each `grep` invocation must therefore capture its status explicitly and branch on
it: `0` → violation found, record it; `1` → no match, continue; `≥2` → abort with
the failing path and the status, exactly as loudly as a real violation. A read
error is a coverage hole, and this script's whole purpose is to not report success
over a coverage hole.

Style follows `scripts/check-file-size.sh`: `set -euo pipefail`, `find` +
`while IFS= read -r`, no dependency on `git` being present.

Header comment 3 is rewritten to state the derived rule rather than naming six
packages.

### 4. Go changes — remove the violation

Move `DHInventoryStatusUpdate` from `internal/domain/dhlisting/types.go:135` into
`internal/domain/inventory`, which already owns the DH status vocabulary it is
used with (`DHStatusInStock` / `DHStatusListed`, `types_core.go:23`, consumed at
`dhpricing/service.go:107`). That makes `inventory` the right home for the shared
DH contract under the existing "sub-packages depend only on `inventory` for shared
types" guidance.

**Destination: a new file, `internal/domain/inventory/dh_status_update.go`.** Not
`types_core.go`, despite that being where the status constants live: it is already
562 lines against a 500-line guideline (`CLAUDE.md:163`) and a 600-line hard
failure in `scripts/check-file-size.sh`. Appending the DTO there spends part of a
21-line margin for no benefit and moves a `make check` failure closer. A focused
file for the DH status-update contract is also the split `CLAUDE.md:163` asks for
when a file outgrows the budget.

Both interfaces then reference `inventory.DHInventoryStatusUpdate` and the same
adapter continues to satisfy both.

Touched: **new** `inventory/dh_status_update.go`; `dhlisting/types.go`,
`dhlisting/dh_listing_service.go` (call sites at `:281`, `:349`),
`dhpricing/types.go`, `dhpricing/service.go`,
`adapters/clients/dhlisting/adapter.go:175`, and affected tests.

The `dhpricing` package doc becomes true again. The deliberately-inlined
`resolveListingPrice` / `overrideNewer` helpers stay as they are — collapsing
that duplication is a separate argument and is not made here.

### 5. Docs

`CLAUDE.md:94` and `CLAUDE.md:168` are restated as a derived rule instead of a
fixed six-name list, so the documentation cannot drift out of step with the
checker the same way again. Historical plan documents under `docs/plans/` are
left untouched; they record what was true when written.

## Verification

1. `bash scripts/check-imports.sh` exits 0 on the finished tree.
2. **Liveness, not vacuity** — temporarily add a sibling edge (e.g. `finance`
   importing `export`) and confirm the script exits 1 and names it; revert.
3. **Fail-closed** — two probes, both reverted after:
   (a) break the derivation so it returns empty, and confirm the script exits 1
   rather than printing a pass;
   (b) constrain the derivation to a single package, and confirm the script still
   exits 1 — zero pairs compared must not read as success.
4. **Coverage** — confirm the derived list contains all 10 packages, including
   `demand`, `dhpricing`, `psacampaign` and `pricing/lookup`, none of which the
   old list reached.
5. `make check` passes.
6. `go test -race ./...` passes.

No `web/` files are touched, so the frontend build and test run is not required
by this ticket's definition of done.

## Out of scope

- Forbidding all cross-domain imports (§1, rejected).
- Asserting `inventory` imports no sibling — currently true, but beyond the
  ticket's acceptance criteria.
- De-duplicating the inlined price helpers in `dhpricing`.
- Broader `CLAUDE.md` drift, which is owned by SLA-24.
