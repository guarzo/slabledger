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
# Module path the current fixture writes into its imports. scaffold() sets it so
# a fixture can rename the module and still emit self-consistent imports.
FIXTURE_MODULE="$MODULE"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CHECKER="$SCRIPT_DIR/check-imports.sh"

if [ ! -x "$CHECKER" ]; then
  echo "FATAL: $CHECKER not found or not executable" >&2
  exit 1
fi

failures=0

# scaffold <root> [module path]
# Every fixture needs the things the checker asserts before it will believe a
# silent pass: a go.mod to derive the module path from, a non-empty
# internal/adapters/storage/, a non-empty internal/platform/, and each
# sanctioned domain leaf. Fixtures that exercise a missing path call this and
# then remove the piece under test.
scaffold() {
  local root=$1 module=${2:-$MODULE}
  FIXTURE_MODULE=$module
  printf 'module %s\n\ngo 1.26\n' "$module" > "$root/go.mod"
  mkdir -p "$root/internal/adapters/storage/postgres"
  printf 'package postgres\n' > "$root/internal/adapters/storage/postgres/db.go"
  mkdir -p "$root/internal/platform/base"
  printf 'package base\n' > "$root/internal/platform/base/base.go"
  local leaf
  for leaf in constants errors observability; do
    mkdir -p "$root/internal/domain/$leaf"
    printf 'package %s\n' "$leaf" > "$root/internal/domain/$leaf/$leaf.go"
  done
}

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
      printf '\t"%s/internal/domain/%s"\n' "$FIXTURE_MODULE" "$imp"
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
      printf '\t"%s/internal/domain/%s"\n' "$FIXTURE_MODULE" "$imp"
    done
    printf ')\n'
  } > "$root/internal/platform/$pkg/$file"
}

# mkimport <root> <dir> <pkg> <full import path>
# Writes an arbitrary package importing an arbitrary path — used for the two
# hexagonal fixtures, where the offending import is not a domain package.
mkimport() {
  local root=$1 dir=$2 pkg=$3 path=$4
  mkdir -p "$root/$dir"
  printf 'package %s\n\nimport (\n\t"%s"\n)\n' "$pkg" "$path" > "$root/$dir/$pkg.go"
}

# The platform pass runs after the flat sibling pass, so any fixture exercising
# it needs a tree that clears the sibling rule first (>=2 siblings, no
# cross-imports) — and, since SLA-90, one that clears the fail-closed guards
# ahead of it too, hence the scaffold call.
mkclean_domain() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
}

# run_case <name> <zero|nonzero> <expected substring, "" to skip> <builder fn>
run_case() {
  local name=$1 expect=$2 needle=$3 builder=$4
  local root out status=0 ok=1

  root=$(mktemp -d)
  FIXTURE_MODULE="$MODULE"
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
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkpkg "$root" rogue beta
}

# Case 2 — classic sibling to sibling.
fixture_sibling_to_sibling() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory beta
  mkpkg "$root" beta inventory
}

# Case 3 — clean tree: siblings import only the hub. Includes a nested package
# (the pricing/lookup shape) to prove nesting alone is not a violation.
fixture_clean() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkpkg "$root" nest/deep inventory
}

# Case 4 — fail-closed guard: fewer than two siblings.
fixture_one_sibling() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" leafpkg
}

# Case 5 — nested governed sibling importing a flat sibling.
# Pins owner resolution for multi-segment package paths: the offender must be
# named "nest/deep", not "deep" and not "nest". Membership derivation
# (check-imports.sh:142-143) uses the same ${dir#internal/domain/} formula, so
# this case is what keeps the two in step.
fixture_nested_violation() {
  local root=$1
  scaffold "$root"
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

# Case 11 — hexagonal pass 1: a domain package importing an adapter.
fixture_domain_imports_adapter() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkimport "$root" internal/domain/rogue rogue \
    "$FIXTURE_MODULE/internal/adapters/clients/dh"
}

# Case 12 — hexagonal pass 2: a storage adapter importing a client adapter.
fixture_storage_imports_client() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkimport "$root" internal/adapters/storage/postgres leaky \
    "$FIXTURE_MODULE/internal/adapters/clients/dh"
}

# Case 13 — fail-closed: internal/domain/ renamed out from under pass 1.
# Before SLA-90 the grep simply matched nothing and the run continued.
fixture_domain_renamed() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mv "$root/internal/domain" "$root/internal/domainv2"
}

# Case 14 — fail-closed: internal/adapters/storage/ renamed out from under
# pass 2. This is the hole that reached exit 0 on its own, because nothing
# downstream reads internal/adapters/.
fixture_storage_renamed() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mv "$root/internal/adapters/storage" "$root/internal/adapters/persistence"
}

# Case 15 — the module path is derived from go.mod, not hardcoded: a tree
# renamed end to end still has its hexagonal violation flagged.
fixture_renamed_module() {
  local root=$1
  scaffold "$root" "example.com/acme/newname"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  mkimport "$root" internal/domain/rogue rogue \
    "$FIXTURE_MODULE/internal/adapters/clients/dh"
}

# Case 16 — fail-closed: a grep status above 1 (here, an unreadable file) is a
# scan failure, not "no matches". The unreadable file goes under
# internal/adapters/storage/, which only the storage → clients pass scans — the
# flat-sibling section has guarded grep status since SLA-45, so putting it under
# internal/domain/ would let that older guard answer for pass 2. Root can read
# anything, so this case only means something as an unprivileged user.
fixture_unreadable_file() {
  local root=$1
  scaffold "$root"
  mkpkg "$root" inventory
  mkpkg "$root" alpha inventory
  mkpkg "$root" beta inventory
  chmod 000 "$root/internal/adapters/storage/postgres/db.go"
}

# Case 17 — fail-closed: internal/platform/ renamed out from under the platform
# pass. SLA-91 guarded that pass with `[ -d internal/platform ]`, so a rename
# skipped it entirely and the run still reported success.
fixture_platform_renamed() {
  local root=$1
  mkclean_domain "$root"
  mkplatpkg "$root" cardutil normalize.go constants
  mv "$root/internal/platform" "$root/internal/plat"
}

# Case 18 — fail-closed: a sanctioned leaf that no longer exists is a stale
# allowlist, not a clean tree. The honesty scan used to swallow the missing
# path and check nothing.
fixture_sanctioned_leaf_missing() {
  local root=$1
  mkclean_domain "$root"
  mkplatpkg "$root" cardutil normalize.go constants
  rm -rf "$root/internal/domain/observability"
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

run_case "hexagonal: domain package importing an adapter is flagged" \
  nonzero "internal/domain/rogue/rogue.go" \
  fixture_domain_imports_adapter

run_case "hexagonal: storage adapter importing a client is flagged" \
  nonzero "must not import client adapters" \
  fixture_storage_imports_client

run_case "fail-closed: renamed internal/domain/ is an error, not a silent pass" \
  nonzero "internal/domain/ does not exist" \
  fixture_domain_renamed

run_case "fail-closed: renamed internal/adapters/storage/ is an error, not a silent pass" \
  nonzero "internal/adapters/storage/ does not exist" \
  fixture_storage_renamed

run_case "module path is derived from go.mod: renamed module still flags violations" \
  nonzero "must not import adapter packages" \
  fixture_renamed_module

if [ "$(id -u)" -ne 0 ]; then
  run_case "fail-closed: a grep scan failure is an error, not an empty result" \
    nonzero "grep failed with status" \
    fixture_unreadable_file
else
  echo "SKIP: grep scan failure case (running as root, which can read anything)"
fi

run_case "fail-closed: renamed internal/platform/ is an error, not a skipped pass" \
  nonzero "internal/platform/ does not exist" \
  fixture_platform_renamed

run_case "fail-closed: a missing sanctioned leaf is an error, not an empty scan" \
  nonzero "internal/domain/observability/ does not exist" \
  fixture_sanctioned_leaf_missing

echo ""
if [ "$failures" -ne 0 ]; then
  echo "check-imports self-test: $failures case(s) failed."
  exit 1
fi
echo "check-imports self-test: all cases passed."
