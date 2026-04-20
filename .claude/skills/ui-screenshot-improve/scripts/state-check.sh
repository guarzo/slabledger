#!/usr/bin/env bash
# Check whether $SCREENSHOT_DB_URL / $LOCAL_DB_URL has realistic data for auditing.
# Exit 0 = realistic (audit). Exit 1 = unseeded (halt or db-pull). Exit 2 = cannot connect.
#
# Thresholds (see references/state-check.md):
#   campaigns >= 2, campaign_purchases >= 20, campaign_sales >= 1, invoices >= 1

set -euo pipefail

DB_URL="${SCREENSHOT_DB_URL:-${LOCAL_DB_URL:-postgresql://slabledger:slabledger@postgres:5432/slabledger?sslmode=disable}}"

if ! command -v psql >/dev/null 2>&1; then
  echo "state-check: psql not found" >&2
  exit 2
fi

counts_raw="$(psql "$DB_URL" -tA -c "
  SELECT
    (SELECT COUNT(*) FROM campaigns),
    (SELECT COUNT(*) FROM campaign_purchases),
    (SELECT COUNT(*) FROM campaign_sales),
    (SELECT COUNT(*) FROM invoices);
" 2>/dev/null)" || {
  echo "state-check: could not query $DB_URL" >&2
  exit 2
}

IFS='|' read -r campaigns purchases sales invoices <<<"$counts_raw"
echo "state-check: campaigns=$campaigns purchases=$purchases sales=$sales invoices=$invoices"

fail=0
[ "${campaigns:-0}" -lt 2 ]  && fail=1
[ "${purchases:-0}" -lt 20 ] && fail=1
[ "${sales:-0}" -lt 1 ]      && fail=1
[ "${invoices:-0}" -lt 1 ]   && fail=1

if [ "$fail" -eq 1 ]; then
  if [ "${UI_ALLOW_EMPTY:-0}" = "1" ]; then
    echo "state-check: below threshold, but UI_ALLOW_EMPTY=1 — proceeding" >&2
    exit 0
  fi
  exit 1
fi
exit 0
