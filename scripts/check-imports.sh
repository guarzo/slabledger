#!/usr/bin/env bash
# Enforce hexagonal architecture invariants:
#   1. Domain packages must not import adapter packages.
#   2. Storage adapters must not import client adapters. Peers should
#      communicate through domain interfaces; pure utilities (string
#      normalization, encoding, etc.) belong in internal/platform/.
#   3. Inventory sub-packages must not import each other (flat sibling rule).
#      Membership is DERIVED, not listed: a sub-package is any directory under
#      internal/domain/ holding a non-test .go file that imports
#      internal/domain/inventory. Sub-packages may depend on internal/domain/
#      inventory (the hub) and on leaf packages, but never on each other.
set -euo pipefail

MODULE="github.com/guarzo/slabledger"

violations_domain=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters' internal/domain/ 2>/dev/null || true)

if [ -n "$violations_domain" ]; then
  echo "ERROR: Domain packages must not import adapter packages."
  echo ""
  echo "Violations found:"
  echo "$violations_domain"
  echo ""
  echo "Domain code should depend only on interfaces defined in internal/domain/."
  exit 1
fi

violations_storage=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters/clients' internal/adapters/storage/ 2>/dev/null || true)

if [ -n "$violations_storage" ]; then
  echo "ERROR: Storage adapters must not import client adapters."
  echo ""
  echo "Violations found:"
  echo "$violations_storage"
  echo ""
  echo "Adapters should communicate through domain interfaces. Pure utilities"
  echo "(string normalization, encoding, etc.) belong in internal/platform/."
  exit 1
fi

echo "Architecture check passed: no domain → adapter or storage → clients imports."

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
