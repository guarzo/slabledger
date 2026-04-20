#!/usr/bin/env bash
# Append a well-formed iteration entry to web/screenshots/friction-log.md.
#
# Usage:   ./append-friction-log.sh < entry.json
# Depends: jq
#
# JSON schema (all fields required unless marked optional):
#   {
#     "iteration": 7,
#     "date":      "2026-04-23 10:15",
#     "fixed_this_cycle": [
#       {"title": "...", "file": "web/src/...", "story": "user trying to ...", "kind": "structural|cosmetic|tier-c", "regression": "clean|side-effect"}
#     ],
#     "structural_attempt":    "Fix 1 (...) was structural.",   // required string; if all-cosmetic, name that explicitly
#     "deferred":              [ {"title": "...", "reason": "...", "first_raised": 7} ],     // optional
#     "reexamined":            [ {"title": "...", "outcome": "re-raised|promoted|retired", "note": "..."} ], // optional
#     "epics":                 [ {"title": "...", "sketch": "...", "last_touched": 7} ],    // optional
#     "retired":               [ {"title": "...", "justification": "..."} ],                 // optional
#     "recurring":             [ {"title": "...", "last_attempt": "...", "next_move": "..."} ], // optional
#     "conventions":           [ "..." ],                                                     // optional
#     "outstanding_red":       [ {"title": "...", "story": "..."} ]                           // optional
#   }

set -euo pipefail

LOG_PATH="${LOG_PATH:-web/screenshots/friction-log.md}"

if ! command -v jq >/dev/null 2>&1; then
  echo "append-friction-log.sh: jq is required" >&2
  exit 1
fi

log_dir="$(dirname "$LOG_PATH")"
if ! mkdir -p "$log_dir"; then
  echo "append-friction-log.sh: could not create log directory: $log_dir" >&2
  exit 1
fi

payload="$(cat)"

require() {
  local field="$1"
  if ! jq -e ".${field}" <<<"$payload" >/dev/null; then
    echo "append-friction-log.sh: missing required field: ${field}" >&2
    exit 1
  fi
}

require iteration
require date
require fixed_this_cycle
require structural_attempt

{
  echo ""
  echo "## Iteration $(jq -r '.iteration' <<<"$payload") — $(jq -r '.date' <<<"$payload")"
  echo ""

  echo "### Fixed this cycle"
  jq -r '.fixed_this_cycle[] | "- ✅ \(.title) — `\(.file)` — \(.story) — regression: \(.regression) — kind: \(.kind)"' <<<"$payload"
  echo ""

  echo "### Structural attempt this cycle"
  echo "- $(jq -r '.structural_attempt' <<<"$payload")"
  echo ""

  if jq -e '.deferred and (.deferred | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Deferred / intentional"
    jq -r '.deferred[] | "- \(.title) — \(.reason) — first raised: iter \(.first_raised)"' <<<"$payload"
    echo ""
  fi

  if jq -e '.reexamined and (.reexamined | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Re-examined deferred items"
    jq -r '.reexamined[] | "- \(.title) — outcome: \(.outcome) — \(.note)"' <<<"$payload"
    echo ""
  fi

  if jq -e '.epics and (.epics | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Epics"
    jq -r '.epics[] | "- \(.title) — \(.sketch) — last touched: iter \(.last_touched)"' <<<"$payload"
    echo ""
  fi

  if jq -e '.retired and (.retired | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Retired"
    jq -r '.retired[] | "- \(.title) — \(.justification)"' <<<"$payload"
    echo ""
  fi

  if jq -e '.recurring and (.recurring | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Recurring"
    jq -r '.recurring[] | "- \(.title) — last attempt: \(.last_attempt) — next move: \(.next_move)"' <<<"$payload"
    echo ""
  fi

  if jq -e '.conventions and (.conventions | length > 0)' <<<"$payload" >/dev/null; then
    echo "### Design conventions ratified this cycle"
    jq -r '.conventions[] | "- \(.)"' <<<"$payload"
    echo ""
  fi

  echo "### Outstanding 🔴"
  if jq -e '.outstanding_red and (.outstanding_red | length > 0)' <<<"$payload" >/dev/null; then
    jq -r '.outstanding_red[] | "- \(.title) — \(.story)"' <<<"$payload"
  else
    echo "- (none)"
  fi
  echo ""
  echo "---"
} >> "$LOG_PATH"

lines="$(wc -l < "$LOG_PATH")"
if [ "$lines" -gt 500 ]; then
  echo "append-friction-log.sh: warning — ${LOG_PATH} is now ${lines} lines; roll up oldest iterations per references/friction-log-template.md" >&2
fi
