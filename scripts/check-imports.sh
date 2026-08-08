#!/usr/bin/env bash
# Enforce hexagonal architecture invariants:
#   1. Domain packages must not import adapter packages.
#   2. Storage adapters must not import client adapters. Peers should
#      communicate through domain interfaces; pure utilities (string
#      normalization, encoding, etc.) belong in internal/platform/.
#   3. Nothing under internal/domain/ may import an inventory sub-package
#      (flat sibling rule). MEMBERSHIP is DERIVED, not listed: a sub-package is
#      any directory under internal/domain/ holding a non-test .go file that
#      imports internal/domain/inventory. ENFORCEMENT is target-based: any
#      importer of a governed sub-package is flagged, whether or not the
#      importer imports the hub itself. Sub-packages may depend on
#      internal/domain/inventory (the hub) and on leaf packages, but never on
#      each other.
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
# performs the checks. Deriving both in one loop would let a scan that dropped
# files midway lower expectation and actual together — exactly the failure this
# accounting exists to catch.
#
# Scope: both passes read the same $domain_files, so this catches divergence
# BETWEEN the passes, not a truncated `find`. Truncation upstream of
# $domain_files is covered above — the find runs in a plain command
# substitution so set -e aborts on traversal failure, and an empty result is
# rejected outright.
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

# Fail closed: a scan that performed a different number of checks than pass 1
# implies has lost or gained files midway. Assert equality, not merely > 0, so
# divergence in either direction is caught.
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
