#!/usr/bin/env bash
# Completeness gate validator for the tech-debt audit.
# Usage: validate.sh {scout|findings} <file.json>
# Exit 0 = pass. Exit 1 = fail, reason on stderr.
set -uo pipefail

BASELINE_REV="740976ecf80a4f2ccdaa611d7790ccaa95b48773"
MODE="${1:-}"
FILE="${2:-}"

fail() { echo "GATE FAIL: $*" >&2; exit 1; }

[ -n "$MODE" ] && [ -n "$FILE" ] || fail "usage: validate.sh {scout|findings} <file.json>"
[ -f "$FILE" ] || fail "no such file: $FILE"
jq empty "$FILE" 2>/dev/null || fail "$FILE is not valid JSON"

# Baseline facts. Authoritative; see the spec's baseline table.
declare -A BASELINE=( [go_files]=676 [packages]=53 [migrations]=25
                      [frontend_files]=218 [env_vars]=71 )

if [ "$MODE" = "scout" ]; then
  rev=$(jq -r '.revision // ""' "$FILE")
  [ "$rev" = "$BASELINE_REV" ] || fail "revision is '$rev', expected $BASELINE_REV"

  status=$(jq -r '.status // ""' "$FILE")
  case "$status" in
    complete) ;;
    partial|failed) fail "scout status is '$status' — dependent lenses are blocked" ;;
    *) fail "scout status missing or invalid: '$status'" ;;
  esac

  # Count reconciliation: any declared total that names a baseline key must match.
  while IFS=$'\t' read -r key val; do
    [ -n "${BASELINE[$key]:-}" ] || continue
    [ "$val" = "${BASELINE[$key]}" ] || \
      fail "declared_totals.$key = $val, baseline says ${BASELINE[$key]}"
  done < <(jq -r '.declared_totals | to_entries[] | "\(.key)\t\(.value)"' "$FILE")

  # Provenance: every record must carry the command that produced it.
  missing=$(jq '[.records[] | select((.command // "") == "")] | length' "$FILE")
  [ "$missing" = "0" ] || fail "$missing record(s) missing provenance 'command'"

  n=$(jq '.records | length' "$FILE")
  echo "GATE PASS: scout $(jq -r .scout "$FILE") — $n records, totals reconciled"

elif [ "$MODE" = "findings" ]; then
  bad_rev=$(jq --arg r "$BASELINE_REV" \
    '[.[] | select(.revision != $r)] | length' "$FILE")
  [ "$bad_rev" = "0" ] || fail "$bad_rev finding(s) have the wrong revision"

  # A mechanical claim must answer all nine runtime checks.
  weak=$(jq '[.[] | select(.confidence == "mechanical")
              | select((.runtime_checks // {} | keys | length) < 9)] | length' "$FILE")
  [ "$weak" = "0" ] || \
    fail "$weak mechanical finding(s) answer fewer than 9 runtime checks"

  # Absence must be proven by command output, never by a bare citation.
  noev=$(jq '[.[] | select([.evidence[] | select((.command // "") == "")] | length > 0)] | length' "$FILE")
  [ "$noev" = "0" ] || fail "$noev finding(s) have evidence without a command"

  n=$(jq 'length' "$FILE")
  echo "GATE PASS: findings — $n records, all evidence has provenance"
else
  fail "unknown mode: $MODE"
fi
