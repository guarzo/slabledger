#!/usr/bin/env bash
# Checkpoint docs/private to its private backup repo.
#
# - No-op if docs/private/.git is missing (fresh clone without private docs).
# - No-op if the working tree is clean.
# - Commit message includes the parent slabledger SHA + subject when available.
# - Push failures are soft (warn, exit 0) so the post-commit hook never blocks commits.

set -u

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo /workspace)"
PRIVATE_DIR="$REPO_ROOT/docs/private"

if [ ! -d "$PRIVATE_DIR/.git" ]; then
  cat >&2 <<EOF
backup-private.sh: $PRIVATE_DIR is not a git working copy.
To set up the private docs backup:
  git clone https://github.com/guarzo/slabledger-private.git $PRIVATE_DIR
EOF
  exit 0
fi

cd "$PRIVATE_DIR" || {
  echo "backup-private.sh: failed to cd into $PRIVATE_DIR" >&2
  exit 1
}

DIRTY=0
[ -n "$(git status --porcelain)" ] && DIRTY=1

UNPUSHED=0
if git rev-parse --verify --quiet '@{u}' >/dev/null 2>&1; then
  [ "$(git rev-list --count '@{u}..HEAD')" -gt 0 ] && UNPUSHED=1
fi

if [ "$DIRTY" -eq 0 ] && [ "$UNPUSHED" -eq 0 ]; then
  exit 0
fi

if [ "$DIRTY" -eq 1 ]; then
  PARENT_SHA="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || true)"
  PARENT_SUBJECT="$(git -C "$REPO_ROOT" log -1 --format=%s 2>/dev/null || true)"

  if [ -n "$PARENT_SHA" ] && [ -n "$PARENT_SUBJECT" ]; then
    MSG="backup: $PARENT_SHA $PARENT_SUBJECT"
  else
    MSG="backup: $(date -u +%Y-%m-%dT%H:%M:%SZ) (manual)"
  fi

  if ! git add -A; then
    echo "backup-private.sh: git add failed" >&2
    exit 1
  fi
  if ! git commit -m "$MSG"; then
    echo "backup-private.sh: git commit failed" >&2
    exit 1
  fi
fi

if ! PUSH_OUTPUT="$(git push 2>&1)"; then
  echo "backup-private.sh: push failed, local commit kept. Error:" >&2
  echo "$PUSH_OUTPUT" >&2
  exit 0
fi

echo "backup-private.sh: pushed $(git rev-parse --short HEAD) — $(git log -1 --format=%s)"
