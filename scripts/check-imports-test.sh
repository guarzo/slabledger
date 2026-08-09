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

# mkplatpkg <root> <pkg> <file> [domain import...]
# Writes internal/platform/<pkg>/<file> importing each internal/domain/<import>.
# <file> is named explicitly so a case can build a _test.go file and prove the
# platform pass ignores it.
mkplatpkg() {
  local root=$1 pkg=$2 file=$3
  shift 3
  mkdir -p "$root/internal/platform/$pkg"
  {
    printf 'package %s\n\nimport (\n' "$pkg"
    local imp
    for imp in "$@"; do
      printf '\t"%s/internal/domain/%s"\n' "$MODULE" "$imp"
    done
    printf ')\n'
  } > "$root/internal/platform/$pkg/$file"
}

# The platform pass runs after the flat sibling pass, so any fixture exercising
# it needs a tree that clears the sibling rule first (>=2 siblings, no
# cross-imports).
mkclean_domain() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
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

# Case 3 — clean tree: siblings import only the hub. Includes a nested package
# (the pricing/lookup shape) to prove nesting alone is not a violation.
fixture_clean() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkpkg "$root" nest/deep inventory
}

# Case 4 — fail-closed guard: fewer than two siblings.
fixture_one_sibling() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" leafpkg
}

# Case 5 — nested governed sibling importing a flat sibling.
# Pins owner resolution for multi-segment package paths: the offender must be
# named "nest/deep", not "deep" and not "nest". Membership derivation
# (check-imports.sh:83-86) uses the same ${dir#internal/domain/} formula, so
# this case is what keeps the two in step.
fixture_nested_violation() {
  local root=$1
  mkpkg "$root" inventory
  mkpkg "$root" beta inventory
  mkpkg "$root" nest/deep inventory beta
}

# Case 6 — platform importing a domain package outside the sanctioned leaf set.
fixture_platform_unsanctioned() {
  local root=$1
  mkclean_domain "$root"
  mkplatpkg "$root" cardutil normalize.go constants
  mkplatpkg "$root" rogue rogue.go inventory
}

# Case 7 — platform importing only sanctioned leaves passes.
fixture_platform_sanctioned() {
  local root=$1
  mkclean_domain "$root"
  mkplatpkg "$root" cardutil normalize.go constants
  mkplatpkg "$root" resilience retry.go errors observability
}

# Case 8 — the platform pass is scoped to non-test files, matching the flat
# sibling pass. Pins the real tree's cardutil_test → inventory edge as allowed.
fixture_platform_test_file_ignored() {
  local root=$1
  mkclean_domain "$root"
  mkplatpkg "$root" cardutil normalize.go constants
  mkplatpkg "$root" cardutil chain_test.go inventory
}

# Case 9 — a sanctioned leaf that reaches the hub turns the allowlist into a
# hole, so the checker refuses rather than trusting the set.
fixture_leaf_not_leaf() {
  local root=$1
  mkclean_domain "$root"
  mkpkg "$root" constants inventory
  mkplatpkg "$root" cardutil normalize.go constants
}

# Case 10 — leaf-to-leaf edges inside the sanctioned set stay legal.
fixture_leaf_to_leaf() {
  local root=$1
  mkclean_domain "$root"
  mkpkg "$root" constants errors
  mkpkg "$root" errors
  mkplatpkg "$root" cardutil normalize.go constants
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

run_case "nested sibling importing a flat sibling names the full package path" \
  nonzero "internal/domain/nest/deep imports sibling internal/domain/beta" \
  fixture_nested_violation

run_case "platform importing an unsanctioned domain package is flagged" \
  nonzero "internal/platform/rogue imports internal/domain/inventory" \
  fixture_platform_unsanctioned

run_case "platform importing only sanctioned leaves passes" \
  zero "Platform boundary check passed" \
  fixture_platform_sanctioned

run_case "platform pass ignores _test.go files" \
  zero "Platform boundary check passed" \
  fixture_platform_test_file_ignored

run_case "a sanctioned leaf reaching the hub is flagged" \
  nonzero "sanctioned leaf internal/domain/constants/constants.go imports internal/domain/inventory" \
  fixture_leaf_not_leaf

run_case "leaf-to-leaf edges inside the sanctioned set are allowed" \
  zero "Platform boundary check passed" \
  fixture_leaf_to_leaf

echo ""
if [ "$failures" -ne 0 ]; then
  echo "check-imports self-test: $failures case(s) failed."
  exit 1
fi
echo "check-imports self-test: all cases passed."
