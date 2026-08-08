#!/usr/bin/env bash
# Completeness gate validator for the tech-debt audit.
# Usage: validate.sh {scout|findings|verdicts} <file.json>
# Exit 0 = pass. Exit 1 = fail, reason on stderr.
set -uo pipefail

BASELINE_REV="740976ecf80a4f2ccdaa611d7790ccaa95b48773"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE="${1:-}"
FILE="${2:-}"

fail() { echo "GATE FAIL: $*" >&2; exit 1; }

[ -n "$MODE" ] && [ -n "$FILE" ] || \
  fail "usage: validate.sh {scout|findings|verdicts} <file.json>"
[ -f "$FILE" ] || fail "no such file: $FILE"
jq empty "$FILE" 2>/dev/null || fail "$FILE is not valid JSON"

# Structural validation against the JSON Schemas in ../schema/. `jq empty`
# only proves the bytes parse — it would accept a findings file with no ids
# at all. A real validator is not vendored, so use one when the environment
# has it and say so loudly when it does not: a schema nothing ever runs is
# documentation, and the checks below are the actual enforced contract.
schema_check() {
  local schema="$HERE/../schema/$1" filter="$2"
  [ -f "$schema" ] || return 0
  if command -v check-jsonschema >/dev/null 2>&1; then
    jq "$filter" "$FILE" > "$FILE.tmp.$$" || { rm -f "$FILE.tmp.$$"; return 0; }
    check-jsonschema --schemafile "$schema" "$FILE.tmp.$$" >/dev/null 2>&1
    local rc=$?
    rm -f "$FILE.tmp.$$"
    [ "$rc" -eq 0 ] || fail "$FILE does not satisfy schema/$1"
    echo "  schema/$1: OK" >&2
  elif python3 -c 'import jsonschema' 2>/dev/null; then
    python3 - "$schema" "$FILE" "$filter" <<'PY' || fail "$FILE does not satisfy schema"
import json, subprocess, sys
import jsonschema
schema = json.load(open(sys.argv[1]))
docs = json.loads(subprocess.run(["jq", sys.argv[3], sys.argv[2]],
                                 capture_output=True, text=True, check=True).stdout)
for d in (docs if isinstance(docs, list) else [docs]):
    jsonschema.validate(d, schema)
PY
    echo "  schema/$1: OK" >&2
  else
    echo "NOTE: no JSON Schema validator on PATH (check-jsonschema or python3" \
         "jsonschema) — schema/$1 was NOT enforced; only the checks below ran." >&2
  fi
}

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
  schema_check scout-report.schema.json .
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

  # Count reconciliation: every key present in declared_totals must belong to
  # THIS scout's contract (a key that is merely a recognized baseline key is
  # not enough — go-reference-map declaring `migrations` is a report that
  # counted something it never scanned), and its value must match the
  # baseline exactly. An unrecognized key is a loud failure, not a silent skip.
  while IFS=$'\t' read -r key val; do
    case " ${SCOUT_KEYS[$scout_name]} " in
      *" $key "*) ;;
      *) fail "declared_totals key '$key' is not in scout '$scout_name'" \
              "contract (${SCOUT_KEYS[$scout_name]:-none})" ;;
    esac
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
  schema_check finding.schema.json '.[]'
  bad_rev=$(jq --arg r "$BASELINE_REV" \
    '[.[] | select(.revision != $r)] | length' "$FILE")
  [ "$bad_rev" = "0" ] || fail "$bad_rev finding(s) have the wrong revision"

  # A mechanical claim must answer all nine runtime checks — by NAME. Counting
  # to nine passes a finding that answered `embedding` nine times under nine
  # invented keys, which is exactly the shape a rushed agent produces.
  missing_rc=$(jq -r --argjson want '["interface_satisfaction","functional_options",
      "embedding","serialization","go_embed","build_tags","registration",
      "init_side_effects","reflection_or_string"]' '
    [.[] | select(.confidence == "mechanical")
     | {id, miss: ($want - (.runtime_checks // {} | keys)),
            extra: ((.runtime_checks // {} | keys) - $want)}
     | select((.miss | length) > 0 or (.extra | length) > 0)
     | "\(.id): missing=\(.miss) unknown=\(.extra)"] | .[]' "$FILE")
  [ -z "$missing_rc" ] || \
    fail "mechanical finding(s) do not answer the nine named runtime checks: $missing_rc"

  # Absence must be proven by command output, never by a bare citation.
  noev=$(jq '[.[] | select([.evidence[] | select((.command // "") == "")] | length > 0)] | length' "$FILE")
  [ "$noev" = "0" ] || fail "$noev finding(s) have evidence without a command"

  n=$(jq 'length' "$FILE")
  echo "GATE PASS: findings — $n records, all evidence has provenance"

elif [ "$MODE" = "verdicts" ]; then
  schema_check verdict.schema.json '.[]'

  # corrected_severity is rendered verbatim into REPORT.md's severity line, so
  # it must be a bare token. A verifier that explains its re-grounding there
  # instead of in what_the_finding_got_wrong puts a paragraph where a word goes.
  prose=$(jq -r '[.[] | select(.corrected_severity
                   | IN("high", "medium", "low", "") | not)
                  | "\(.id)=\(.corrected_severity[0:40])"] | join(", ")' "$FILE") \
    || fail "jq failed while checking corrected_severity in $FILE"
  [ -z "$prose" ] || fail "corrected_severity must be a bare severity token: $prose"

  # A downgrade that names no new severity silently keeps the filed one.
  nosev=$(jq -r '[.[] | select(.verdict == "confirmed_lower_severity"
                   and (.corrected_severity == "")) | .id] | join(", ")' "$FILE")
  [ -z "$nosev" ] || fail "confirmed_lower_severity with no corrected_severity: $nosev"

  # A refuted finding that stays ticketable files a ticket for a non-issue.
  tick=$(jq -r '[.[] | select(.verdict == "refuted" and .ticketable != false)
                 | .id] | join(", ")' "$FILE")
  [ -z "$tick" ] || fail "refuted but still ticketable: $tick"

  n=$(jq 'length' "$FILE")
  echo "GATE PASS: verdicts — $n records, severities are bare tokens"
else
  fail "unknown mode: $MODE"
fi
