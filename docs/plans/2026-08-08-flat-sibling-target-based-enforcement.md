# Flat-Sibling Target-Based Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the source-side escape hatch in `scripts/check-imports.sh` so a package can no longer leave flat-sibling enforcement by dropping its `internal/domain/inventory` import.

**Architecture:** Membership derivation is unchanged (a governed sibling is any directory under `internal/domain/` with a non-test `.go` file importing the hub). Enforcement moves from the source side to the target side: instead of iterating governed packages and grepping their files, iterate *every* non-test `.go` file under `internal/domain/`, resolve its owning package from its path, and flag any import of a governed sibling other than its own. A new self-test script exercises the checker against fixture trees.

**Tech Stack:** Bash (`#!/usr/bin/env bash`, `set -euo pipefail`), GNU coreutils, GNU Make. No new dependencies.

**Spec:** `docs/specs/2026-08-08-flat-sibling-escape-hatch-design.md` (SLA-45)

## Global Constraints

- Bash only. No new dependencies, no Go code, no changes to `internal/domain/` package structure.
- Both scripts must be executable (`chmod +x`) and must keep `#!/usr/bin/env bash`.
- `scripts/check-imports.sh` keeps `set -euo pipefail`. `scripts/check-imports-test.sh` uses `set -uo pipefail` **without** `-e`, because it deliberately captures non-zero exits from the script under test.
- Module path is the literal string `github.com/guarzo/slabledger`, held in `MODULE`.
- The checker **greps**; it never compiles. Fixture files need only the right directory layout and literal import text.
- File-size budget applies (`scripts/check-file-size.sh`: warn 500 lines, fail 600).
- Never hardcode a package list in either script. Everything is derived from the tree.
- Fail closed: an assertion that compared nothing, or fewer things than the derivation implies, must not report success.

## Measurements (current tree, 2026-08-08)

Used by verification steps. Re-derive if the tree changed.

- Governed siblings: **10** — `arbitrage`, `demand`, `dhlisting`, `dhpricing`, `export`, `finance`, `portfolio`, `pricing/lookup`, `psacampaign`, `tuning`
- Non-test `.go` files under `internal/domain/`: **159**
- Of those, owned by a governed sibling: **39**
- Therefore `expected_checks = 159 × 10 − 39 = 1551`
- No `.go` files sit directly at `internal/domain/*.go` (verified) — every file has a package directory.

## File Structure

| File | Responsibility |
|---|---|
| `scripts/check-imports-test.sh` | **Create.** Self-test for the checker. Builds fixture trees in temp dirs, `cd`s into each, invokes the real checker by absolute path, asserts exit status and message content. |
| `scripts/check-imports.sh` | **Modify** lines 80–143. Replace the pair-iteration scan and `n*(n-1)` accounting with a file-iteration scan and the file-oriented invariant. Derivation (lines 43–78) and the hexagonal checks (lines 14–41) are untouched. |
| `Makefile` | **Modify** line 146–149. Run the self-test before the checker — test the tester first. |
| `CLAUDE.md` | **Modify** the flat-sibling section (lines 91–109) and the Quality Checks bullet (line 197) to state the membership/enforcement split. |

---

### Task 1: Self-test harness that demonstrates the bug

Written against the **current** checker. Three cases pass immediately; the escape-hatch case fails. That failure is the bug SLA-45 reports, made executable.

**Files:**
- Create: `scripts/check-imports-test.sh`
- Test: the script *is* the test; it exercises `scripts/check-imports.sh`

**Interfaces:**
- Consumes: `scripts/check-imports.sh` (existing, invoked by absolute path with a temp dir as CWD)
- Produces: `scripts/check-imports-test.sh`, exit 0 when all cases pass, exit 1 otherwise. Task 3 wires this into `make check`.

- [ ] **Step 1: Write the failing test**

Create `scripts/check-imports-test.sh`:

```bash
#!/usr/bin/env bash
# Self-test for scripts/check-imports.sh.
#
# The checker uses relative paths (internal/domain, internal/adapters) and greps
# for literal import strings — it never compiles. A fixture tree therefore needs
# only the directory layout and files containing the right text.
#
# No `set -e`: this script deliberately captures non-zero exits from the script
# under test.
set -uo pipefail

MODULE="github.com/guarzo/slabledger"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CHECKER="$SCRIPT_DIR/check-imports.sh"

if [ ! -x "$CHECKER" ]; then
  echo "FATAL: $CHECKER not found or not executable" >&2
  exit 1
fi

failures=0

# mkpkg <root> <pkg> [import...]
# Writes internal/domain/<pkg>/<leaf>.go importing each internal/domain/<import>.
mkpkg() {
  local root=$1 pkg=$2
  shift 2
  local leaf=${pkg##*/}
  mkdir -p "$root/internal/domain/$pkg"
  {
    printf 'package %s\n\nimport (\n' "$leaf"
    local imp
    for imp in "$@"; do
      printf '\t"%s/internal/domain/%s"\n' "$MODULE" "$imp"
    done
    printf ')\n'
  } > "$root/internal/domain/$pkg/$leaf.go"
}

# run_case <name> <zero|nonzero> <expected substring, "" to skip> <builder fn>
run_case() {
  local name=$1 expect=$2 needle=$3 builder=$4
  local root out status=0 ok=1

  root=$(mktemp -d)
  "$builder" "$root"

  out=$(cd "$root" && "$CHECKER" 2>&1) || status=$?

  case "$expect" in
    zero)    [ "$status" -eq 0 ] || ok=0 ;;
    nonzero) [ "$status" -ne 0 ] || ok=0 ;;
  esac

  if [ -n "$needle" ] && [[ "$out" != *"$needle"* ]]; then
    ok=0
  fi

  if [ "$ok" -eq 1 ]; then
    echo "PASS: $name"
  else
    echo "FAIL: $name"
    echo "  exit status: $status (expected $expect)"
    [ -n "$needle" ] && echo "  wanted message containing: $needle"
    echo "  --- checker output ---"
    echo "$out" | sed 's/^/  /'
    echo "  ----------------------"
    failures=$((failures + 1))
  fi

  rm -rf "$root"
}

# Case 1 — the escape hatch (SLA-45).
# rogue imports sibling beta but does NOT import the hub, so source-side
# membership never sees it.
fixture_escape_hatch() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkpkg "$root" rogue beta
}

# Case 2 — classic sibling to sibling.
fixture_sibling_to_sibling() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory beta
  mkpkg "$root" beta inventory
}

# Case 3 — clean tree: siblings import only the hub.
fixture_clean() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
}

# Case 4 — fail-closed guard: fewer than two siblings.
fixture_one_sibling() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" leafpkg
}

run_case "escape hatch: non-hub importer of a governed sibling is flagged" \
  nonzero "internal/domain/rogue imports sibling internal/domain/beta" \
  fixture_escape_hatch

run_case "classic: sibling importing a sibling is flagged" \
  nonzero "internal/domain/alpha imports sibling internal/domain/beta" \
  fixture_sibling_to_sibling

run_case "clean tree passes" \
  zero "no cross-imports" \
  fixture_clean

run_case "fail-closed: fewer than two siblings is an error" \
  nonzero "At least 2 are required" \
  fixture_one_sibling

echo ""
if [ "$failures" -ne 0 ]; then
  echo "check-imports self-test: $failures case(s) failed."
  exit 1
fi
echo "check-imports self-test: all cases passed."
```

- [ ] **Step 2: Make it executable**

```bash
chmod +x scripts/check-imports-test.sh
```

- [ ] **Step 3: Run it to confirm exactly one case fails**

Run: `bash scripts/check-imports-test.sh`

Expected: exit 1, with **case 1 FAIL and cases 2–4 PASS**:

```
FAIL: escape hatch: non-hub importer of a governed sibling is flagged
  exit status: 0 (expected nonzero)
PASS: classic: sibling importing a sibling is flagged
PASS: clean tree passes
PASS: fail-closed: fewer than two siblings is an error
```

If any case other than #1 fails, **stop** — the harness is wrong, not the checker. Fix the harness before continuing.

- [ ] **Step 4: Commit**

```bash
git add scripts/check-imports-test.sh
git commit -m "test: add check-imports self-test, demonstrating the SLA-45 escape hatch

Four fixture cases against the real checker. Case 1 fails on the current
script: a package that does not import the hub is never scanned, so its
imports of governed siblings go unenforced."
```

---

### Task 2: Target-based scan and file-oriented accounting

**Files:**
- Modify: `scripts/check-imports.sh:80-143` (replace from `sibling_violations=""` through the final `echo`)
- Test: `scripts/check-imports-test.sh` (from Task 1)

**Interfaces:**
- Consumes: `siblings` array and `domain_files` variable, both already built by `scripts/check-imports.sh:43-78` — unchanged by this task. `MODULE` from line 14.
- Produces: same exit contract as before (0 clean, 1 violation or broken derivation). Violation message format changes from `internal/domain/$pkg imports sibling internal/domain/$other` to the same string with `$owner` in place of `$pkg` — Task 1's assertions already expect this wording.

- [ ] **Step 1: Replace the scan and accounting**

In `scripts/check-imports.sh`, replace everything from line 80 (`sibling_violations=""`) to the end of the file with:

```bash
n_siblings=${#siblings[@]}

# Fail closed before scanning. Below two siblings the rule is vacuous, and a
# derivation that collapsed to 0 or 1 is a broken derivation, not a clean tree.
if [ "$n_siblings" -lt 2 ]; then
  echo "ERROR: derived $n_siblings sub-package(s) under internal/domain/ importing"
  echo "$MODULE/internal/domain/inventory. At least 2 are required for the flat"
  echo "sibling check to mean anything."
  echo "The derivation is broken (moved tree, renamed module path, or a bad"
  echo "pattern) — fix it rather than ignoring this."
  exit 1
fi

is_sibling() {
  local needle=$1 s
  for s in "${siblings[@]}"; do
    [ "$s" = "$needle" ] && return 0
  done
  return 1
}

# Pass 1: compute the expected check count independently of the pass that
# performs the checks. Deriving both in one loop would let a truncated
# traversal lower expectation and actual together — exactly the failure this
# accounting exists to catch.
expected_checks=0
while IFS= read -r file; do
  dir=${file%/*}
  owner=${dir#internal/domain/}
  if is_sibling "$owner"; then
    expected_checks=$(( expected_checks + n_siblings - 1 ))
  else
    expected_checks=$(( expected_checks + n_siblings ))
  fi
done <<< "$domain_files"

# Pass 2: enforce on the TARGET side. Every non-test .go file under
# internal/domain/ is checked against every governed sibling except its own
# package. The importer does not need to import the hub itself — that is the
# escape hatch SLA-45 closes.
#
# Resolving the owning directory per file also retires the old -maxdepth 1
# workaround: nested packages such as pricing/lookup are attributed correctly
# by construction rather than by special case.
sibling_violations=""
checks_performed=0

while IFS= read -r file; do
  dir=${file%/*}
  owner=${dir#internal/domain/}

  for other in "${siblings[@]}"; do
    [ "$owner" = "$other" ] && continue
    checks_performed=$(( checks_performed + 1 ))

    status=0
    found=$(grep -n -- "\"$MODULE/internal/domain/$other\"" "$file") || status=$?
    if [ "$status" -gt 1 ]; then
      echo "ERROR: grep failed with status $status scanning $file" >&2
      echo "The flat sibling check cannot verify this tree; refusing to pass." >&2
      exit 1
    fi
    if [ "$status" -eq 0 ]; then
      sibling_violations="${sibling_violations}ERROR: internal/domain/$owner imports sibling internal/domain/$other\n${found}\n"
    fi
  done
done <<< "$domain_files"

# Fail closed: a scan that performed a different number of checks than its own
# derivation implies has lost or gained files midway. Assert equality, not
# merely > 0, so divergence in either direction is caught.
if [ "$checks_performed" -ne "$expected_checks" ]; then
  echo "ERROR: flat sibling check performed $checks_performed checks, expected $expected_checks."
  echo ""
  echo "Every non-test .go file under internal/domain/ must be checked against"
  echo "each of the $n_siblings governed sibling(s), excluding its own package."
  echo "A mismatch means the traversal was truncated or the tree changed under"
  echo "the scan — fix it rather than ignoring this."
  exit 1
fi

if [ -n "$sibling_violations" ]; then
  echo "ERROR: Packages under internal/domain/ must not import inventory sub-packages (flat sibling rule)."
  echo ""
  printf "%b" "$sibling_violations"
  echo ""
  echo "Sub-packages should depend only on internal/domain/inventory (shared types)."
  exit 1
fi

echo "Flat sibling rule check passed: $n_siblings sub-packages, $checks_performed checks, no cross-imports."
```

- [ ] **Step 2: Update the header comment**

At `scripts/check-imports.sh:7-11`, replace the rule-3 comment block with:

```bash
#   3. Nothing under internal/domain/ may import an inventory sub-package
#      (flat sibling rule). MEMBERSHIP is DERIVED, not listed: a sub-package is
#      any directory under internal/domain/ holding a non-test .go file that
#      imports internal/domain/inventory. ENFORCEMENT is target-based: any
#      importer of a governed sub-package is flagged, whether or not the
#      importer imports the hub itself. Sub-packages may depend on
#      internal/domain/inventory (the hub) and on leaf packages, but never on
#      each other.
```

- [ ] **Step 3: Run the self-test to verify all four cases pass**

Run: `bash scripts/check-imports-test.sh`

Expected: exit 0.

```
PASS: escape hatch: non-hub importer of a governed sibling is flagged
PASS: classic: sibling importing a sibling is flagged
PASS: clean tree passes
PASS: fail-closed: fewer than two siblings is an error

check-imports self-test: all cases passed.
```

- [ ] **Step 4: Run the checker against the real tree**

Run: `bash scripts/check-imports.sh`

Expected: exit 0, final line exactly:

```
Flat sibling rule check passed: 10 sub-packages, 1551 checks, no cross-imports.
```

`1551 = 159 files × 10 siblings − 39 sibling-owned files`. If the count differs, re-derive the three measurements before assuming a bug — the tree may have changed. If the count is *lower* than the file×sibling arithmetic implies, the traversal truncated and the equality assertion should already have failed.

- [ ] **Step 5: Reproduce the ticket's escape-hatch probe against the real tree**

```bash
mkdir -p internal/domain/zzprobe
printf 'package zzprobe\n\nimport (\n\t"github.com/guarzo/slabledger/internal/domain/dhlisting"\n)\n' \
  > internal/domain/zzprobe/zzprobe.go
bash scripts/check-imports.sh; echo "exit=$?"
rm -rf internal/domain/zzprobe
```

Expected: exit 1, output naming the edge:

```
ERROR: internal/domain/zzprobe imports sibling internal/domain/dhlisting
```

`zzprobe` does not import the hub, so the old script would have exited 0 here. Confirm the probe directory is gone afterwards: `git status --porcelain internal/domain/` must be empty.

- [ ] **Step 6: Commit**

```bash
git add scripts/check-imports.sh
git commit -m "fix: enforce the flat sibling rule on the target side (SLA-45)

The scan derived its governed set from the source of an import edge, so a
package left enforcement by dropping its inventory import, taking its
outgoing sibling edges with it.

Iterate every non-test .go file under internal/domain/ instead, resolve the
owning package from the path, and flag any import of a governed sibling.
Membership derivation is unchanged.

Accounting moves from n*(n-1) pairs to a file-oriented invariant computed in
a separate pass, so a truncated traversal cannot lower expectation and actual
together. Per-file owner resolution also retires the -maxdepth 1 workaround
for nested packages such as pricing/lookup."
```

---

### Task 3: Wire into `make check` and update the documented rule

**Files:**
- Modify: `Makefile:146-149`
- Modify: `CLAUDE.md:91-109` and `CLAUDE.md:197`

**Interfaces:**
- Consumes: `scripts/check-imports-test.sh` (Task 1), the corrected `scripts/check-imports.sh` (Task 2)
- Produces: nothing downstream — this is the last task.

- [ ] **Step 1: Run the self-test before the checker in `make check`**

In `Makefile`, replace the `check` recipe body:

```make
check: lint
	./scripts/check-imports-test.sh
	./scripts/check-imports.sh
	./scripts/check-file-size.sh
	./scripts/check-playwright-version.sh
```

The self-test runs first deliberately: a broken checker should be reported as a broken checker, not as a clean tree.

- [ ] **Step 2: Update the flat-sibling section in `CLAUDE.md`**

Replace `CLAUDE.md:93-97` (the "Membership is **derived, not listed**" paragraph) with:

```markdown
Membership and enforcement are separate, and the split matters:

- **Membership is derived, not listed**: a sibling is any directory under
  `internal/domain/` with a non-test `.go` file importing `internal/domain/inventory`.
  Adding a new inventory-importing package puts it under the rule automatically —
  no list to update here or in the script.
- **Enforcement is target-based**: `scripts/check-imports.sh` flags *any* importer of
  a governed sibling, anywhere under `internal/domain/`, whether or not that importer
  imports the hub itself. A package cannot leave enforcement by dropping its own hub
  import (SLA-45).

The checker computes both from the tree on every `make check`, and
`scripts/check-imports-test.sh` tests the checker.
```

- [ ] **Step 3: Update the Quality Checks bullet in `CLAUDE.md`**

Replace `CLAUDE.md:197` with:

```markdown
- `scripts/check-imports.sh` — fails if domain packages import adapter packages (hexagonal invariant); also enforces the flat sibling rule, deriving membership from the tree and enforcing on the target side, and fails closed if fewer than two siblings are derived or if the scan performs an unexpected number of checks
- `scripts/check-imports-test.sh` — self-test for the above; four fixture cases, run first by `make check`
```

- [ ] **Step 4: Verify the whole gate**

```bash
make check
```

Expected: exit 0. The self-test's four PASS lines appear before the checker's output.

- [ ] **Step 5: Verify nothing else broke**

```bash
go test -race -timeout 10m ./...
```

Expected: PASS. No Go code changed, so this is a regression guard, not a target.

- [ ] **Step 6: Commit**

```bash
git add Makefile CLAUDE.md
git commit -m "chore: run the check-imports self-test in make check, document the rule split

Membership is derived from importing the hub; enforcement applies to any
importer of a governed sibling. The self-test runs first — a broken checker
should report as broken, not as a clean tree."
```

---

## Verification (against the spec's Verification section)

| Spec requirement | Covered by |
|---|---|
| `bash scripts/check-imports-test.sh` — all four cases pass | Task 2, Step 3 |
| `make check` passes | Task 3, Step 4 |
| `go test -race ./...` passes | Task 3, Step 5 |
| Probe reproduced against old script (exit 0, the bug) | Task 1, Step 3 — case 1 fails on the unmodified checker |
| Probe reproduced against new script (exit ≠ 0, fixed) | Task 2, Steps 3 and 5 |

## Out of scope

Per the spec: the leaf/non-leaf taxonomy (SLA-48), the four existing non-inventory edges (`advisor → ai`, `advisor → scoring`, `dhlisting → dhevents`, `pricing/lookup → pricing`), and any change to `internal/domain/` package structure.

The residual limitation stands and is not addressed here: a governed sibling can still escape by dropping its *own* hub import, de-governing itself as an import target. See the spec's "Residual limitation" section.
