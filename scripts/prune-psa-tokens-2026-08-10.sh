#!/usr/bin/env bash
# Prune the dead tokens out of PSA_ACCESS_TOKEN (SLA-108, 2026-08-10).
#
# Nine of the eleven tokens in the pool are permanently unapproved: PSA answers
# 403 for them on every request. They are not rate limited and will not recover
# on their own — only PSA re-approving the application changes that. Carrying
# them costs a wasted request per rotation step and hides the two live tokens
# behind noise. The rotation added in SLA-108 skips a 403 token for the rest of
# the process, so this prune is hygiene rather than a fix.
#
# Survey taken 2026-08-10 by probing each token against the PSA cert endpoint:
#
#   pos  token                 HTTP  verdict
#   ---  --------------------  ----  --------------------------------------
#     1  l4SgUC…dCz5yQ         403   unapproved — prune
#     2  9HctMi…gm64mL         403   unapproved — prune
#     3  2lX6T5…xCGZ2i         200   live — KEEP
#     4  JvXeQa…tpB96L         429   live, daily quota spent — KEEP
#     5  9MnT-_…jmVRsi         403   unapproved — prune
#     6  CkHtE3…4pDhAd         403   unapproved — prune
#     7  QwdPuP…YTCPDS         403   unapproved — prune
#     8  ekfCJ9…6nIaMO         403   unapproved — prune
#     9  GUPFA7…JDVPKv         403   unapproved — prune
#    10  2FZaMI…NDcDBS         403   unapproved — prune
#    11  a3NOR8…oQD1V          403   unapproved — prune
#
# 429 is a keeper: the quota is per token and resets daily, so token 4 is a
# working credential that happened to be spent when the survey ran. Only 403
# means "this token will never work".
#
# Reads the pool from the local .env (positions 3 and 4) and writes it to Fly.
# Never prints a token in full. Run from the repository root:
#
#   ./scripts/prune-psa-tokens-2026-08-10.sh          # show what would change
#   ./scripts/prune-psa-tokens-2026-08-10.sh --apply  # actually set the secret
#
# Afterwards, edit .env to match and fix its stale "9 keys for rotation"
# comment — the pool had eleven, and will have two.

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env}"
APP="${FLY_APP:-slabledger}"
KEEP_POSITIONS=(3 4)

if [[ ! -f "$ENV_FILE" ]]; then
  echo "error: $ENV_FILE not found (run from the repository root, or set ENV_FILE)" >&2
  exit 1
fi

raw=$(grep -m1 '^PSA_ACCESS_TOKEN=' "$ENV_FILE" | sed 's/^PSA_ACCESS_TOKEN=//')
if [[ -z "$raw" ]]; then
  echo "error: no PSA_ACCESS_TOKEN in $ENV_FILE" >&2
  exit 1
fi

# The value is quoted in .env and at least one element carries a leading space,
# so trim both from every element rather than trusting the split.
trim() {
  local s=$1
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  s="${s#\"}"
  s="${s%\"}"
  printf '%s' "$s"
}

mask() {
  local t=$1
  if (( ${#t} < 12 )); then printf '<short:%d>' "${#t}"; else printf '%s…%s' "${t:0:6}" "${t: -6}"; fi
}

IFS=',' read -r -a all <<< "$raw"
tokens=()
for t in "${all[@]}"; do
  t=$(trim "$t")
  [[ -n "$t" ]] && tokens+=("$t")
done

echo "Found ${#tokens[@]} token(s) in $ENV_FILE"
if (( ${#tokens[@]} != 11 )); then
  echo "error: expected the 11-token pool this prune was surveyed against." >&2
  echo "       The pool changed — re-survey before pruning by position." >&2
  exit 1
fi

keep=()
for pos in "${KEEP_POSITIONS[@]}"; do
  keep+=("${tokens[pos-1]}")
done

for i in "${!tokens[@]}"; do
  verdict="prune"
  for pos in "${KEEP_POSITIONS[@]}"; do
    (( i + 1 == pos )) && verdict="KEEP"
  done
  printf '  %2d  %s  %s\n' "$((i+1))" "$(mask "${tokens[i]}")" "$verdict"
done

new=$(IFS=','; printf '%s' "${keep[*]}")
echo
echo "New pool: ${#keep[@]} token(s) — $(mask "${keep[0]}"), $(mask "${keep[1]}")"

if [[ "${1:-}" != "--apply" ]]; then
  echo
  echo "Dry run. Re-run with --apply to set the secret on Fly app '$APP'."
  echo "Applying restarts the app."
  exit 0
fi

echo
echo "Setting PSA_ACCESS_TOKEN on Fly app '$APP' (this restarts the app)…"
fly secrets set PSA_ACCESS_TOKEN="$new" -a "$APP"
echo "Done. Now update $ENV_FILE to the same two tokens."
