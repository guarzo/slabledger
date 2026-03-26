#!/usr/bin/env bash
# Fail if any domain package imports an adapter package.
# This is the core hexagonal architecture invariant.
set -euo pipefail

violations=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters' internal/domain/ 2>/dev/null || true)

if [ -n "$violations" ]; then
  echo "ERROR: Domain packages must not import adapter packages."
  echo ""
  echo "Violations found:"
  echo "$violations"
  echo ""
  echo "Domain code should depend only on interfaces defined in internal/domain/."
  exit 1
fi

echo "Architecture check passed: no domain → adapter imports."
