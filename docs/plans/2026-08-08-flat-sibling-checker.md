# Flat-Sibling Import Checker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `scripts/check-imports.sh` derive its flat-sibling package set from the repository layout, fail closed when it verifies nothing, and remove the one real violation it currently misses (`dhpricing → dhlisting`).

**Architecture:** Two independent changes. First, move the shared `DHInventoryStatusUpdate` DTO from `internal/domain/dhlisting` into `internal/domain/inventory` (the hub every sibling already imports), which deletes the only sibling-to-sibling import edge in the tree. Second, replace the checker's hardcoded six-name list with a filesystem-derived set, guarded by a `pairs_checked == n*(n-1)` assertion so a broken or truncated derivation fails loudly instead of printing a pass.

**Tech Stack:** Bash (`set -euo pipefail`, shellcheck-clean), Go 1.26, hexagonal architecture.

**Spec:** [`docs/specs/2026-08-08-flat-sibling-checker-design.md`](../specs/2026-08-08-flat-sibling-checker-design.md)
**Ticket:** [SLA-12](https://linear.app/slabledger/issue/SLA-12/fix-the-flat-sibling-import-checker-it-skips-19-of-25-domain-packages)

## Global Constraints

- Module path is `github.com/guarzo/slabledger`. Import strings in the checker must match it exactly.
- The flat-sibling rule governs **non-test** `.go` files only. `*_test.go` is excluded from both derivation and violation scanning.
- A *sibling* is any directory under `internal/domain/` holding a non-test `.go` file that imports `internal/domain/inventory`. Today that derives exactly 10: `arbitrage demand dhlisting dhpricing export finance portfolio pricing/lookup psacampaign tuning`.
- `scripts/check-imports.sh` must stay shellcheck-clean (`shellcheck scripts/check-imports.sh` exits 0) and must not depend on `git` being present.
- Keep Go source files under 500 lines (`CLAUDE.md:163`); `scripts/check-file-size.sh` hard-fails above 600. `internal/domain/inventory/types_core.go` is already 562 lines — **do not add to it**.
- Every task ends on a green tree **at its commit boundary**, gated by what that task actually touches. Task 1 changes Go source: `go build ./...`, `go test -race ./...`, and `make check` must all pass before committing. Tasks 2 and 3 change only a shell script and markdown — no Go source — so `make check` is the gate there. Note `make check` runs lint + import check + file-size check and **does not** run Go tests (`Makefile:145-149`); it is not a substitute for `go test -race` on a task that touches Go.
- No `web/` files are touched, so the frontend build/test run is not required.

---

### Task 1: Move `DHInventoryStatusUpdate` into the inventory hub

Removes the only sibling-to-sibling import in the tree.

**Steps 2-7 are one atomic refactor and the tree does not compile in between.** Step 2 deletes the type from `dhlisting` while `dh_listing_service.go:281`, `dh_listing_service_test.go:85`, `dhpricing/types.go:57`, and `adapters/clients/dhlisting/adapter.go:175` still reference the old name; the build only comes back at Step 7. Do not run `go build` or `go test` mid-sequence and conclude something is wrong — the green-tree guarantee is at the commit boundary (Step 11), not between steps. The old checker passes throughout, so no import-check regression can hide here either.

**Files:**
- Create: `internal/domain/inventory/dh_status_update.go`
- Modify: `internal/domain/dhlisting/types.go:130-140` (remove type), `:150` (signature)
- Modify: `internal/domain/dhlisting/dh_listing_service.go:281`, `:349`
- Modify: `internal/domain/dhpricing/types.go:11` (drop import), `:57` (signature)
- Modify: `internal/domain/dhpricing/service.go:7` (drop import), `:115`
- Modify: `internal/adapters/clients/dhlisting/adapter.go:175`
- Test: `internal/domain/dhlisting/dh_listing_service_test.go:85,528,534,561,592,594,596,645,836`
- Test: `internal/domain/dhlisting/inline_match_push_test.go:43`
- Test: `internal/domain/dhpricing/service_test.go:52,355`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `inventory.DHInventoryStatusUpdate` — a struct with fields `Status string`, `ListingPriceCents int`, `CertImageURLFront string`, `CertImageURLBack string`. After this task, both `dhlisting.DHInventoryLister.UpdateInventoryStatus` and `dhpricing.DHPriceUpdater.UpdateInventoryStatus` have the signature `(ctx context.Context, inventoryID int, update inventory.DHInventoryStatusUpdate) (int, error)`. Task 2 relies on `internal/domain/dhpricing` no longer importing `internal/domain/dhlisting` in any non-test file.

- [ ] **Step 1: Create the new home for the DTO**

Create `internal/domain/inventory/dh_status_update.go`. A dedicated file, not `types_core.go` — that file is at 562 lines against a 600-line hard fail, and this is exactly the split `CLAUDE.md:163` asks for.

```go
package inventory

// DHInventoryStatusUpdate carries the fields that UpdateInventoryStatus can
// mutate on a DH inventory item. Image URLs are optional; when either is set
// on a transition to "listed", DH uses them instead of doing its own PSA
// lookup, which keeps the listing path functional when PSA is rate-limited
// or authentication is failing.
//
// Lives in inventory rather than dhlisting because both dhlisting and
// dhpricing declare an UpdateInventoryStatus method satisfied by the same
// adapter. Go interface satisfaction is structural on the method signature,
// so the two interfaces must name one identical type — and a sibling may
// only reach it through the inventory hub (the flat sibling rule; see
// scripts/check-imports.sh).
type DHInventoryStatusUpdate struct {
	Status            string
	ListingPriceCents int // 0 means omit
	CertImageURLFront string
	CertImageURLBack  string
}
```

- [ ] **Step 2: Remove the old declaration from dhlisting**

In `internal/domain/dhlisting/types.go`, delete lines 130-140 (the doc comment and the `DHInventoryStatusUpdate` struct) entirely, then update the interface at what was line 150:

```go
type DHInventoryLister interface {
	UpdateInventoryStatus(ctx context.Context, inventoryID int, update inventory.DHInventoryStatusUpdate) (int, error)
	SyncChannels(ctx context.Context, inventoryID int, channels []string) error
}
```

`types.go` does **not** currently import inventory — its import block is only `"context"` and `"errors"` (verified: `grep -n "domain/inventory" internal/domain/dhlisting/types.go` returns nothing, while its sibling `dh_listing_service.go:12` does have it). Add it:

```go
import (
	"context"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)
```

- [ ] **Step 3: Update the two dhlisting call sites**

These are in-package references, so they gain an `inventory.` qualifier. At `internal/domain/dhlisting/dh_listing_service.go:281`:

```go
		dhListingPrice, err := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHInventoryStatusUpdate{
```

At `internal/domain/dhlisting/dh_listing_service.go:349`:

```go
			if _, revertErr := s.lister.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHInventoryStatusUpdate{
```

Leave the struct literal bodies and everything after them unchanged.

- [ ] **Step 4: Update dhpricing to import the hub instead of its sibling**

In `internal/domain/dhpricing/types.go`, delete the `dhlisting` import at line 11 so the block reads:

```go
import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)
```

Update the interface at line 57:

```go
type DHPriceUpdater interface {
	UpdateInventoryStatus(ctx context.Context, inventoryID int, update inventory.DHInventoryStatusUpdate) (int, error)
}
```

The package doc at `types.go:1-4` claims dhpricing "does not import" dhlisting. That claim is currently false and becomes true again with this change — leave the wording as it is.

- [ ] **Step 5: Update dhpricing service**

In `internal/domain/dhpricing/service.go`, delete the `dhlisting` import at line 7 so the block reads:

```go
import (
	"context"
	"time"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

At line 115:

```go
	newDHPrice, err := s.updater.UpdateInventoryStatus(ctx, p.DHInventoryID, inventory.DHInventoryStatusUpdate{
		Status:            status,
		ListingPriceCents: reviewed,
	})
```

Do **not** touch `resolveListingPrice` or `overrideNewer` — those inlined copies stay, and collapsing that duplication is explicitly out of scope.

- [ ] **Step 6: Update the adapter**

At `internal/adapters/clients/dhlisting/adapter.go:175`:

```go
func (a *InventoryAdapter) UpdateInventoryStatus(ctx context.Context, inventoryID int, update inventory.DHInventoryStatusUpdate) (int, error) {
```

The file already imports both `dhlisting` and `inventory`, and keeps importing `dhlisting` for `DHPSAImportItem`, `ErrPSAKeysExhausted`, and the `var _ dhlisting.X = ...` assertions — leave all of those alone.

- [ ] **Step 7: Update the test doubles**

`internal/domain/dhlisting/dh_listing_service_test.go` is `package dhlisting` (internal) and already imports `inventory` (`:12`). Add the `inventory.` qualifier at each of lines 85, 528, 534, 561, 594, 596, 645, 836 — plus the prose reference in the comment at line 592, so the doc matches the code. For example, line 85:

```go
func (m *mockInventoryLister) UpdateInventoryStatus(_ context.Context, _ int, update inventory.DHInventoryStatusUpdate) (int, error) {
```

and line 594:

```go
	var capturedUpdates []inventory.DHInventoryStatusUpdate
```

`internal/domain/dhlisting/inline_match_push_test.go:43` is `package dhlisting_test` and already imports `inventory`:

```go
func (noopLister) UpdateInventoryStatus(_ context.Context, _ int, _ inventory.DHInventoryStatusUpdate) (int, error) {
```

`internal/domain/dhpricing/service_test.go` is `package dhpricing` and already imports both. Change lines 52 and 355, then **delete its now-unused `dhlisting` import at line 9** — Go fails to compile on an unused import, so this is not optional:

```go
func (f *fakeUpdater) UpdateInventoryStatus(_ context.Context, invID int, update inventory.DHInventoryStatusUpdate) (int, error) {
```

```go
func (f *fakeUpdaterFn) UpdateInventoryStatus(ctx context.Context, inventoryID int, update inventory.DHInventoryStatusUpdate) (int, error) {
```

- [ ] **Step 8: Confirm nothing was missed**

Run: `grep -rn "dhlisting.DHInventoryStatusUpdate" --include="*.go" .`
Expected: no output. Any hit is a missed call site.

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 9: Run the tests**

Run: `go test -race ./internal/domain/dhlisting/... ./internal/domain/dhpricing/... ./internal/domain/inventory/... ./internal/adapters/clients/dhlisting/...`
Expected: `ok` for each package, no failures.

Then the full suite: `go test -race -timeout 10m ./...`
Expected: no `FAIL` lines, no `DATA RACE`.

- [ ] **Step 10: Confirm the tree is still green under the OLD checker**

Run: `make check`
Expected: passes. The old checker never looked at `dhpricing`, so this proves the move broke nothing — it does not yet prove the fix. Task 2 supplies that.

- [ ] **Step 11: Commit**

```bash
git add internal/domain/inventory/dh_status_update.go \
        internal/domain/dhlisting/types.go \
        internal/domain/dhlisting/dh_listing_service.go \
        internal/domain/dhlisting/dh_listing_service_test.go \
        internal/domain/dhlisting/inline_match_push_test.go \
        internal/domain/dhpricing/types.go \
        internal/domain/dhpricing/service.go \
        internal/domain/dhpricing/service_test.go \
        internal/adapters/clients/dhlisting/adapter.go
git commit -m "refactor(domain): move DHInventoryStatusUpdate to inventory hub

dhpricing imported its sibling dhlisting for one DTO, violating the flat
sibling rule. Both interfaces are satisfied by the same adapter, so the
type must be shared — inventory is the hub both already depend on.

Refs SLA-12"
```

---

### Task 2: Derive the sibling set and fail closed

**Files:**
- Modify: `scripts/check-imports.sh:7-8` (header comment), `:38-62` (the whole flat-sibling block)

**Interfaces:**
- Consumes: Task 1's removal of the `dhpricing → dhlisting` edge. If Task 1 has not landed, Step 3 below will report that violation and the task cannot complete.
- Produces: a checker whose success message states how many sub-packages and pairs it checked. Task 3 documents the derived rule this establishes.

- [ ] **Step 1: Replace the header comment**

In `scripts/check-imports.sh`, replace invariant 3 (lines 7-8) with:

```bash
#   3. Inventory sub-packages must not import each other (flat sibling rule).
#      Membership is DERIVED, not listed: a sub-package is any directory under
#      internal/domain/ holding a non-test .go file that imports
#      internal/domain/inventory. Sub-packages may depend on internal/domain/
#      inventory (the hub) and on leaf packages, but never on each other.
```

Then add the module constant directly below `set -euo pipefail` (line 9):

```bash
MODULE="github.com/guarzo/slabledger"
```

- [ ] **Step 2: Replace the flat-sibling block**

Replace everything from the `# Flat sibling rule:` comment (line 38) through the final `echo` (line 62) with the following. The first two checks in the file (domain→adapter, storage→clients) are untouched.

```bash
# --- Flat sibling rule -------------------------------------------------------

# Derive the sibling set from the filesystem so a new inventory-importing
# package is covered without editing this script.
#
# find runs in a command substitution assigned to a plain variable, so a
# traversal failure (unreadable directory, bad path) is the assignment's exit
# status and set -e aborts here. Reading it through a process substitution
# instead would discard that status and silently scan a truncated tree.
domain_files=$(find internal/domain -type f -name "*.go" ! -name "*_test.go" | sort)

if [ -z "$domain_files" ]; then
  echo "ERROR: found no non-test .go files under internal/domain/." >&2
  echo "The flat sibling check cannot verify this tree; refusing to pass." >&2
  exit 1
fi

siblings=()
while IFS= read -r file; do
  status=0
  grep -q -- "\"$MODULE/internal/domain/inventory\"" "$file" || status=$?
  if [ "$status" -gt 1 ]; then
    echo "ERROR: grep failed with status $status reading $file" >&2
    echo "The flat sibling check cannot verify this tree; refusing to pass." >&2
    exit 1
  fi
  if [ "$status" -eq 0 ]; then
    dir=${file%/*}
    siblings+=("${dir#internal/domain/}")
  fi
done <<< "$domain_files"

# De-duplicate (a package has many files).
if [ ${#siblings[@]} -gt 0 ]; then
  mapfile -t siblings < <(printf '%s\n' "${siblings[@]}" | sort -u)
fi

sibling_violations=""
pairs_checked=0

for pkg in ${siblings[@]+"${siblings[@]}"}; do
  # -maxdepth 1 keeps nested packages (e.g. pricing/lookup) from being
  # attributed to their parent. Same command-substitution reasoning as above:
  # a failed scan of one package must abort, not silently skip it.
  pkg_files=$(find "internal/domain/$pkg" -maxdepth 1 -type f -name "*.go" ! -name "*_test.go")

  if [ -z "$pkg_files" ]; then
    echo "ERROR: internal/domain/$pkg was derived as a sub-package but now has" >&2
    echo "no non-test .go files. The tree changed under the scan." >&2
    exit 1
  fi

  mapfile -t files <<< "$pkg_files"

  for other in ${siblings[@]+"${siblings[@]}"}; do
    [ "$pkg" = "$other" ] && continue
    pairs_checked=$((pairs_checked + 1))

    status=0
    found=$(grep -n -- "\"$MODULE/internal/domain/$other\"" "${files[@]}") || status=$?
    if [ "$status" -gt 1 ]; then
      echo "ERROR: grep failed with status $status scanning internal/domain/$pkg" >&2
      echo "The flat sibling check cannot verify this tree; refusing to pass." >&2
      exit 1
    fi
    if [ "$status" -eq 0 ]; then
      sibling_violations="${sibling_violations}ERROR: internal/domain/$pkg imports sibling internal/domain/$other\n${found}\n"
    fi
  done
done

# Fail closed: a check that compared nothing — or that compared fewer pairs than
# its own derivation implies — must not report success. Every derived package
# has a non-test .go file by construction, so the count is exactly n*(n-1).
# Asserting equality (not just > 0) catches a scan that lost a package midway.
expected_pairs=$(( ${#siblings[@]} * (${#siblings[@]} - 1) ))
if [ "$pairs_checked" -eq 0 ] || [ "$pairs_checked" -ne "$expected_pairs" ]; then
  if [ "$pairs_checked" -eq 0 ]; then
    echo "ERROR: flat sibling check compared 0 package pairs — it verified nothing."
  else
    echo "ERROR: flat sibling check compared $pairs_checked package pairs, expected $expected_pairs."
  fi
  echo ""
  echo "Derived ${#siblings[@]} sub-package(s) under internal/domain/ importing"
  echo "$MODULE/internal/domain/inventory. At least 2 are required for this"
  echo "check to mean anything, and every derived package must be scanned."
  echo "The derivation is broken (moved tree, renamed module path, a bad"
  echo "pattern, or a truncated traversal) — fix it rather than ignoring this."
  exit 1
fi

if [ -n "$sibling_violations" ]; then
  echo "ERROR: Inventory sub-packages must not import each other (flat sibling rule)."
  echo ""
  printf "%b" "$sibling_violations"
  echo ""
  echo "Sub-packages should depend only on internal/domain/inventory (shared types)."
  exit 1
fi

echo "Flat sibling rule check passed: ${#siblings[@]} sub-packages, $pairs_checked pairs checked, no cross-imports."
```

Four details that are load-bearing, not style:

- `grep -q ... || status=$?` then branching on `0` / `1` / `>1`. A blanket `|| true` (what the old script used at lines 44-45) swallows grep's exit 2, so an unreadable file reads as "no violations" and the scan silently loses coverage. Omitting it entirely kills the script under `set -e` on the *expected* no-match path.
- `${siblings[@]+"${siblings[@]}"}` — expanding an empty array under `set -u` is an unbound-variable error on older bash. This is the safe-expansion idiom.
- Both `find` calls are **command substitutions assigned to a variable**, never process substitutions. `done < <(find ...)` runs `find` in a subshell whose exit status `set -euo pipefail` never observes: a traversal that dies partway prints fewer files, the derivation silently shrinks, and the scan proceeds over a coverage hole. `var=$(find ...)` makes that status the assignment's status, so `set -e` aborts.
- `pairs_checked` compared for **equality against `n*(n-1)`**, not merely `> 0`. Two distinct failures hide behind a positive count: a one-package derivation is non-empty but compares zero pairs, and a nine-of-ten scan compares 81 pairs and passes. Both are the defect class this ticket exists to close. Every derived package has a non-test `.go` file by construction, so the expected count is exact and equality is safe to assert.

- [ ] **Step 3: Verify it passes on the fixed tree**

Run: `bash scripts/check-imports.sh`
Expected: exit 0, final line reads:

```
Flat sibling rule check passed: 10 sub-packages, 90 pairs checked, no cross-imports.
```

If it instead reports `dhpricing imports sibling dhlisting`, Task 1 has not landed — stop and finish Task 1 first.

- [ ] **Step 4: Lint the script**

Run: `shellcheck scripts/check-imports.sh`
Expected: no output, exit 0.

- [ ] **Step 5: Liveness probe — prove it catches the real violation**

The check passing is meaningless unless it can fail. Temporarily restore the exact edge this ticket is about:

```bash
sed -i 's|^\t"github.com/guarzo/slabledger/internal/domain/inventory"$|\t"github.com/guarzo/slabledger/internal/domain/dhlisting"\n\t"github.com/guarzo/slabledger/internal/domain/inventory"|' internal/domain/dhpricing/types.go
bash scripts/check-imports.sh
```

Expected: exit 1, output naming `internal/domain/dhpricing imports sibling internal/domain/dhlisting` and citing `internal/domain/dhpricing/types.go`.

Revert immediately and confirm:

```bash
git checkout internal/domain/dhpricing/types.go
bash scripts/check-imports.sh
```

Expected: exit 0 again.

- [ ] **Step 6: Coverage probe — prove it reaches the packages the old list missed**

Run:

```bash
bash -c 'MODULE="github.com/guarzo/slabledger"; s=(); while IFS= read -r file; do if grep -q -- "\"$MODULE/internal/domain/inventory\"" "$file"; then d=${file%/*}; s+=("${d#internal/domain/}"); fi; done < <(find internal/domain -type f -name "*.go" ! -name "*_test.go" | sort); printf "%s\n" "${s[@]}" | sort -u | tr "\n" " "; echo'
```

Expected, exactly:

```
arbitrage demand dhlisting dhpricing export finance portfolio pricing/lookup psacampaign tuning
```

`demand`, `dhpricing`, `psacampaign`, and `pricing/lookup` are the ones the old six-name list never opened. `pricing/lookup` appearing separately from `pricing` confirms `-maxdepth 1` is not conflating child with parent.

- [ ] **Step 7: Fail-closed probe A — empty derivation**

```bash
cp scripts/check-imports.sh /tmp/probe-a.sh
sed -i 's|^MODULE=.*|MODULE="example.com/nonexistent"|' /tmp/probe-a.sh
bash /tmp/probe-a.sh; echo "EXIT=$?"
rm -f /tmp/probe-a.sh
```

Expected: `EXIT=1` and `compared 0 package pairs — it verified nothing`. A pass here would mean the fix reintroduced the original bug.

- [ ] **Step 8: Fail-closed probe B — single-package derivation**

This is the case a non-empty check would wrongly accept.

```bash
cp scripts/check-imports.sh /tmp/probe-b.sh
sed -i 's|find internal/domain -type f|find internal/domain/finance -type f|' /tmp/probe-b.sh
bash /tmp/probe-b.sh; echo "EXIT=$?"
rm -f /tmp/probe-b.sh
```

Expected: `EXIT=1`, headline `compared 0 package pairs — it verified nothing`, body reporting `Derived 1 sub-package(s)`. One package is non-empty but yields zero pairs; it must still fail.

- [ ] **Step 9: Fail-closed probe C — truncated traversal**

This is the case the *previous* draft of this script would have passed: a `find` that dies partway leaves a smaller-but-plausible derivation. It is only caught because `find` runs in a command substitution whose status `set -e` sees.

```bash
cp scripts/check-imports.sh /tmp/probe-c.sh
sed -i 's#^domain_files=.*#domain_files=$( { find internal/domain -type f -name "*.go" ! -name "*_test.go" | head -20; exit 1; } )#' /tmp/probe-c.sh
grep -n '^domain_files=' /tmp/probe-c.sh
bash /tmp/probe-c.sh; echo "EXIT=$?"
rm -f /tmp/probe-c.sh
```

Expected: the `grep` echoes the rewritten line ending `head -20; exit 1; } )`, then the script prints only the architecture-check line and `EXIT=1` — **no** flat-sibling output at all, because `set -e` aborts at the assignment before any pair is compared. If instead you see a passing line with fewer than 10 sub-packages, the command substitution was rewritten back into a process substitution and the guard is gone.

- [ ] **Step 10: Full quality gate**

Run: `make check`
Expected: lint, imports, file size, and playwright-version checks all pass.

- [ ] **Step 11: Commit**

```bash
git add scripts/check-imports.sh
git commit -m "fix(scripts): derive flat-sibling package set and fail closed

The checker hardcoded six package names and iterated that list rather
than the filesystem, so 4 of the 10 packages under internal/domain/ that
import inventory were never opened — including the one real violation.
It printed a passing message while verifying nothing.

Membership is now derived from the tree, grep exit 2 aborts instead of
reading as 'no match', find runs in a command substitution so a truncated
traversal aborts rather than silently shrinking the scan, and the pair
count is asserted equal to n*(n-1) so a check that compared less than its
own derivation implies fails rather than passes.

Closes SLA-12"
```

---

### Task 3: Restate the rule in the docs

`CLAUDE.md` carries the same stale six-name list the script did. Left alone, it is the seed for the next drift.

**Files:**
- Modify: `CLAUDE.md:94-100`, `CLAUDE.md:167`

**Interfaces:**
- Consumes: the derived rule established in Task 2.
- Produces: nothing downstream.

- [ ] **Step 1: Restate the sub-package section**

Replace `CLAUDE.md:94-100` (the `### Sibling sub-packages` heading and its six bullets) with the following. The per-package descriptions are kept — they are useful orientation — but they are now framed as a dated snapshot rather than the rule:

```markdown
### Sibling sub-packages (flat siblings under `internal/domain/`, no cross-imports between them)

Membership is **derived, not listed**: a sibling is any directory under
`internal/domain/` with a non-test `.go` file importing `internal/domain/inventory`.
`scripts/check-imports.sh` computes this set from the tree on every `make check`,
so adding a new inventory-importing package puts it under the rule automatically —
no list to update here or in the script.

Siblings may depend on `inventory` (the hub) and on leaf packages such as `errors`
and `observability`, but never on each other. The derived set as of 2026-08-08:

- **arbitrage**: Acquisition targets, expected value, Monte Carlo projection
- **demand**: Demand signals
- **dhlisting**: DH listing push pipeline coordination
- **dhpricing**: DH listing price reconciliation
- **export**: Sell sheet generation
- **finance**: Invoices, cashflow forecasting, capital tracking, revocation flags
- **portfolio**: Inventory aging, price signals, portfolio health analysis
- **pricing/lookup**: PriceLookup adapter over PriceProvider
- **psacampaign**: PSA campaign targeting
- **tuning**: Campaign parameter optimization, tuning suggestions and analytics
```

The derivation is the rule; this list is illustrative. Do not reintroduce a list that anything reads programmatically.

Before writing the four new bullets, confirm each description against the package's own doc comment (`head -5 internal/domain/<pkg>/*.go`) and correct it if it does not match. A wrong one-liner here is exactly the drift this task exists to stop.

- [ ] **Step 2: Restate the quality-checks line**

Replace `CLAUDE.md:167` with:

```markdown
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant); also enforces the flat sibling rule against a package set derived from the tree, and fails closed if that derivation yields fewer than two packages
```

- [ ] **Step 3: Verify no stale list survives**

Run: `grep -n "arbitrage, portfolio, tuning, finance, export" CLAUDE.md scripts/check-imports.sh`
Expected: no output.

Run: `grep -rn "SUB_PACKAGES" .`
Expected: no output outside `docs/plans/` and `docs/specs/`.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: restate flat sibling rule as derived, not a fixed list

Refs SLA-12"
```

---

## Definition of Done

From the ticket, verified in order after Task 3:

- [ ] `bash scripts/check-imports.sh` exits 0, reporting 10 sub-packages and 90 pairs checked.
- [ ] The `dhpricing → dhlisting` edge is gone from the source (not allowlisted).
- [ ] A new inventory-importing directory is covered with no script edit (Task 2 Step 6 demonstrates the derivation).
- [ ] `make check` passes.
- [ ] `go test -race -timeout 10m ./...` passes.
- [ ] `web/` untouched — `git diff --name-only main...HEAD -- web/` is empty, so the frontend build/test is not required.

## Out of Scope

- Forbidding all cross-domain imports behind a shared-leaf allowlist (rejected in the spec, §1).
- Asserting `inventory` imports no sibling — true today, beyond the acceptance criteria.
- De-duplicating `resolveListingPrice` / `overrideNewer` in `dhpricing`.
- Broader `CLAUDE.md` drift, owned by SLA-24.
