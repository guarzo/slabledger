#!/usr/bin/env bash
# Warn at 500 lines, fail at 600 lines for non-test Go source files.
# Test files (*_test.go) are excluded — table-driven tests grow naturally.
set -euo pipefail

WARN_LIMIT=500
FAIL_LIMIT=600
failed=0
warned=0

while IFS= read -r file; do
  lines=$(wc -l < "$file")
  if [ "$lines" -gt "$FAIL_LIMIT" ]; then
    echo "FAIL: $file ($lines lines, limit $FAIL_LIMIT)"
    failed=1
  elif [ "$lines" -gt "$WARN_LIMIT" ]; then
    echo "WARN: $file ($lines lines, guideline $WARN_LIMIT)"
    warned=1
  fi
done < <(find internal/ cmd/ -name '*.go' ! -name '*_test.go' ! -path '*/testutil/*' -type f 2>/dev/null)

if [ "$failed" -eq 1 ]; then
  echo ""
  echo "File size check FAILED. Split files above $FAIL_LIMIT lines."
  exit 1
fi

if [ "$warned" -eq 1 ]; then
  echo ""
  echo "File size check passed with warnings. Consider splitting files above $WARN_LIMIT lines."
else
  echo "File size check passed: all source files under $WARN_LIMIT lines."
fi
