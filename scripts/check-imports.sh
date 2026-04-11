#!/usr/bin/env bash
# Enforce hexagonal architecture invariants:
#   1. Domain packages must not import adapter packages.
#   2. Storage adapters must not import client adapters. Peers should
#      communicate through domain interfaces; pure utilities (string
#      normalization, encoding, etc.) belong in internal/platform/.
set -euo pipefail

violations_domain=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters' internal/domain/ 2>/dev/null || true)

if [ -n "$violations_domain" ]; then
  echo "ERROR: Domain packages must not import adapter packages."
  echo ""
  echo "Violations found:"
  echo "$violations_domain"
  echo ""
  echo "Domain code should depend only on interfaces defined in internal/domain/."
  exit 1
fi

violations_storage=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters/clients' internal/adapters/storage/ 2>/dev/null || true)

if [ -n "$violations_storage" ]; then
  echo "ERROR: Storage adapters must not import client adapters."
  echo ""
  echo "Violations found:"
  echo "$violations_storage"
  echo ""
  echo "Adapters should communicate through domain interfaces. Pure utilities"
  echo "(string normalization, encoding, etc.) belong in internal/platform/."
  exit 1
fi

echo "Architecture check passed: no domain → adapter or storage → clients imports."
