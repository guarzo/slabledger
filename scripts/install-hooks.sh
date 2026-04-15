#!/usr/bin/env bash
# Install slabledger git hooks by symlinking scripts/hooks/* into .git/hooks/.
# Run once after cloning. Safe to re-run — idempotent.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC_DIR="$ROOT/scripts/hooks"
DEST_DIR="$ROOT/.git/hooks"

if [ ! -d "$SRC_DIR" ]; then
  echo "install-hooks.sh: $SRC_DIR not found" >&2
  exit 1
fi

for src in "$SRC_DIR"/*; do
  [ -f "$src" ] || continue
  name="$(basename "$src")"
  dest="$DEST_DIR/$name"
  expected_target="../../scripts/hooks/$name"

  chmod +x "$src"

  if [ -L "$dest" ] && [ "$(readlink "$dest")" = "$expected_target" ]; then
    echo "install-hooks.sh: $name already installed"
    continue
  fi

  if [ -e "$dest" ] && [ ! -L "$dest" ]; then
    echo "install-hooks.sh: $dest exists and is not our symlink — refusing to overwrite" >&2
    echo "  Remove it manually if you want to install the tracked hook." >&2
    exit 1
  fi

  ln -sf "$expected_target" "$dest"
  echo "install-hooks.sh: linked $name"
done
