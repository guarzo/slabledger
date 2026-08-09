#!/usr/bin/env bash
#
# check-doc-paths.sh — every source path cited by the durable docs must exist.
#
# The failure this exists to stop: a file gets renamed or a subsystem gets deleted,
# the code compiles, CI stays green, and the docs keep pointing at a path that
# resolves to nothing. Every reader after that follows a dead citation. SLA-68
# found five removed subsystems and a dozen wrong filenames accumulated this way.
#
# Scope is the *durable* documentation set — the files a reader is expected to
# trust as current. Historical artifacts are excluded on purpose: a design doc or
# plan records what was true when it was written, so "correcting" its paths would
# falsify the record rather than fix a defect.
#
# Paths inside fenced code blocks are skipped. Recipes like "Adding a new
# scheduler" deliberately name files that do not exist yet
# (internal/adapters/scheduler/myworker.go), and those are illustrations, not claims.

set -euo pipefail

cd "$(dirname "$0")/.."

# Durable docs: the ones that describe the tree as it is now.
mapfile -t DOCS < <(
  git ls-files '*.md' \
    | grep -vE '^docs/(audit|plans|specs|superpowers)/' \
    | grep -vE '^(web/screenshots|\.superpowers|\.claude|\.agents)/' \
    | sort
)

if [ "${#DOCS[@]}" -lt 5 ]; then
  echo "check-doc-paths: only ${#DOCS[@]} durable docs matched — the filter is wrong, failing closed" >&2
  exit 1
fi

TRACKED="$(mktemp)"
trap 'rm -f "$TRACKED"' EXIT
# ls-files, not ls-tree HEAD: a commit that adds a file and documents it in the
# same change must pass.
git ls-files > "$TRACKED"

# Strip fenced code blocks, then emit "line<TAB>path" for each candidate citation.
extract_paths() {
  awk '
    /^[[:space:]]*```/ { fence = !fence; next }
    fence              { next }
    {
      line = $0
      while (match(line, /(internal|cmd|web\/src|scripts)\/[A-Za-z0-9_.\/-]+\.(go|ts|tsx|sh)/)) {
        print FNR "\t" substr(line, RSTART, RLENGTH)
        line = substr(line, RSTART + RLENGTH)
      }
    }
  ' "$1"
}

failed=0
checked=0

for doc in "${DOCS[@]}"; do
  while IFS=$'\t' read -r lineno path; do
    [ -n "$path" ] || continue
    checked=$((checked + 1))
    if ! grep -qxF "$path" "$TRACKED"; then
      echo "$doc:$lineno: cites '$path', which is not a tracked file" >&2
      failed=1
    fi
  done < <(extract_paths "$doc")
done

if [ "$checked" -eq 0 ]; then
  echo "check-doc-paths: extracted zero paths from ${#DOCS[@]} docs — extraction is broken, failing closed" >&2
  exit 1
fi

if [ "$failed" -ne 0 ]; then
  echo "" >&2
  echo "Fix the citation, or move the claim into a fenced code block if it is an illustration." >&2
  exit 1
fi

echo "check-doc-paths: OK (${checked} path citations across ${#DOCS[@]} docs)"
