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
#
#      A LEAF is a package under internal/domain/ whose transitive import
#      closure excludes internal/domain/inventory; non-leaf is the hub plus
#      everything depending on it (SLA-48; see internal/README.md).
#      No separate leaf check exists because none is needed: the derived
#      governed set (direct hub importers) and the non-leaf set (transitive)
#      differ only for a package reaching the hub via another non-leaf, and
#      that package imports a governed sub-package, which pass 2 already
#      flags. On any passing tree the two sets coincide, so the scan below IS
#      the leaf rule. Corollary: making membership transitive would be a
#      no-op — closure can only add a package that is already in violation.
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

# --- Platform -> domain boundary (SLA-91) ------------------------------------

# internal/README.md draws dependencies as flowing inward only, which would make
# every platform -> domain edge illegal. The tree has always had a few, and they
# all target the same dependency-free bottom layer. Rather than pretend the
# arrow is true, the rule sanctions that layer by name and enforces the
# boundary, so a platform package reaching for something heavier than a leaf
# fails here instead of depending on reviewer memory.
SANCTIONED_DOMAIN_LEAVES=(constants errors observability)

is_sanctioned() {
  local needle=$1 s
  for s in "${SANCTIONED_DOMAIN_LEAVES[@]}"; do
    [ "$s" = "$needle" ] && return 0
  done
  return 1
}

# Scoped to non-test files, matching the flat sibling pass above. An external
# _test package (internal/platform/cardutil's cardutil_test) pins normalization
# behavior end to end against the hub; that edge is test-only and absent from
# the shipped import graph, so it is out of scope rather than sanctioned.
if [ -d internal/platform ]; then
  platform_files=$(find internal/platform -type f -name "*.go" ! -name "*_test.go" | sort)

  if [ -z "$platform_files" ]; then
    echo "ERROR: internal/platform/ exists but holds no non-test .go files." >&2
    echo "The platform boundary check cannot verify this tree; refusing to pass." >&2
    exit 1
  fi

  # Keep the allowlist honest. A sanctioned package that itself reached the hub
  # would turn this rule into a hole, so assert each one still depends on
  # nothing outside the sanctioned set. Leaf-to-leaf edges stay legal.
  #
  # Greps per file rather than recursively: the extraction below strips to the
  # first "internal/domain/" in the line, so a recursive grep's path prefix
  # would be parsed as the import target.
  leaf_files=$(find "${SANCTIONED_DOMAIN_LEAVES[@]/#/internal/domain/}" \
    -type f -name "*.go" ! -name "*_test.go" 2>/dev/null | sort || true)

  while IFS= read -r file; do
    [ -n "$file" ] || continue
    status=0
    found=$(grep -n -- "\"$MODULE/internal/domain/" "$file") || status=$?
    if [ "$status" -gt 1 ]; then
      echo "ERROR: grep failed with status $status scanning $file" >&2
      echo "The platform boundary check cannot verify this tree; refusing to pass." >&2
      exit 1
    fi
    [ "$status" -eq 0 ] || continue

    while IFS= read -r line; do
      target=${line#*internal/domain/}
      target=${target%%\"*}
      if ! is_sanctioned "$target"; then
        echo "ERROR: sanctioned leaf $file imports internal/domain/$target."
        echo ""
        echo "The platform -> domain allowlist assumes these packages are a"
        echo "dependency-free bottom layer. This edge breaks that assumption and"
        echo "would silently widen the boundary — fix it or revisit the rule."
        exit 1
      fi
    done <<< "$found"
  done <<< "$leaf_files"

  platform_violations=""
  while IFS= read -r file; do
    status=0
    found=$(grep -n -- "\"$MODULE/internal/domain/" "$file") || status=$?
    if [ "$status" -gt 1 ]; then
      echo "ERROR: grep failed with status $status scanning $file" >&2
      echo "The platform boundary check cannot verify this tree; refusing to pass." >&2
      exit 1
    fi
    [ "$status" -eq 0 ] || continue

    dir=${file%/*}
    owner=${dir#internal/platform/}

    while IFS= read -r line; do
      target=${line#*internal/domain/}
      target=${target%%\"*}
      if ! is_sanctioned "$target"; then
        platform_violations="${platform_violations}ERROR: internal/platform/$owner imports internal/domain/$target\n${file}:${line%%:*}\n"
      fi
    done <<< "$found"
  done <<< "$platform_files"

  if [ -n "$platform_violations" ]; then
    echo "ERROR: Packages under internal/platform/ may import only the sanctioned domain leaves."
    echo ""
    printf "%b" "$platform_violations"
    echo ""
    echo "Sanctioned set: ${SANCTIONED_DOMAIN_LEAVES[*]}"
    echo "See internal/README.md (Dependency Rule). Platform is the bottom layer;"
    echo "anything richer belongs behind an interface the domain owns."
    exit 1
  fi

  echo "Platform boundary check passed: only ${SANCTIONED_DOMAIN_LEAVES[*]} imported from internal/domain/."
fi
