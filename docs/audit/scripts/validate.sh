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

# Closed per-scout contract: the exact declared_totals keys each scout must
# report. Value is a space-separated list of baseline keys (empty = none).
# An unrecognized .scout value gets no entry here and must fail.
declare -A SCOUT_KEYS=( [go-reference-map]="go_files packages"
                        [frontend-reference-map]="frontend_files"
                        [db-map]="migrations"
                        [config-map]="env_vars"
                        [docs-map]="" )

if [ "$MODE" = "scout" ]; then
  rev=$(jq -r '.revision // ""' "$FILE")
  [ "$rev" = "$BASELINE_REV" ] || fail "revision is '$rev', expected $BASELINE_REV"

  status=$(jq -r '.status // ""' "$FILE")
  case "$status" in
    complete) ;;
    partial|failed) fail "scout status is '$status' — dependent lenses are blocked" ;;
    *) fail "scout status missing or invalid: '$status'" ;;
  esac

  scout_name=$(jq -r '.scout // ""' "$FILE")
  [ -n "${SCOUT_KEYS[$scout_name]+isset}" ] || \
    fail "unrecognized scout: '$scout_name'"

  # Every key this scout's contract requires must be present in declared_totals.
  # Absence is exactly the gap that let a silently-truncated report through.
  for key in ${SCOUT_KEYS[$scout_name]}; do
    present=$(jq --arg k "$key" '(.declared_totals // {}) | has($k)' "$FILE")
    [ "$present" = "true" ] || \
      fail "declared_totals missing required key '$key' for scout '$scout_name'"
  done

  # Count reconciliation: every key present in declared_totals must be a
  # recognized baseline key (an unrecognized key is a loud failure, not a
  # silent skip), and its value must match the baseline exactly.
  while IFS=$'\t' read -r key val; do
    [ -n "${BASELINE[$key]:-}" ] || \
      fail "declared_totals has unrecognized key: '$key'"
    [ "$val" = "${BASELINE[$key]}" ] || \
      fail "declared_totals.$key = $val, baseline says ${BASELINE[$key]}"
  done < <(jq -r '(.declared_totals // {}) | to_entries[] | "\(.key)\t\(.value)"' "$FILE")

  # Payload cross-check: declared_totals is self-reported and proves nothing
  # on its own. records_count must be present, numeric, and match the actual
  # records payload — and a "complete" scout can never carry zero records.
  rc_present=$(jq 'has("records_count")' "$FILE")
  [ "$rc_present" = "true" ] || fail "records_count is missing"
  rc_type=$(jq -r '.records_count | type' "$FILE")
  [ "$rc_type" = "number" ] || fail "records_count is not a number"
  rc=$(jq '.records_count' "$FILE")
  n=$(jq '.records | length' "$FILE")
  [ "$rc" = "$n" ] || fail "records_count=$rc but records has $n entries"
  if [ "$status" = "complete" ] && [ "$n" = "0" ]; then
    fail "status is complete but records is empty"
  fi

  # Provenance: every record must carry the command that produced it.
  missing=$(jq '[.records[] | select((.command // "") == "")] | length' "$FILE")
  [ "$missing" = "0" ] || fail "$missing record(s) missing provenance 'command'"

  echo "GATE PASS: scout $scout_name — $n records, totals reconciled"

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
