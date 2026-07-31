#!/usr/bin/env bash
# Fail if the Playwright base image tag in Dockerfile.harvest does not match the
# @playwright/test version in web/package.json.
#
# Why: the harvester image bakes in a Chromium build that must match the
# npm-installed @playwright/test client. A Dependabot bump to @playwright/test that
# does not also bump the Docker base tag crashes every harvest at chromium.launch()
# ("Executable doesn't exist ... Please update docker image as well"), which took the
# PSA pipeline down for days in 2026-07. This check makes that mismatch fail loudly.
set -euo pipefail

DOCKERFILE="Dockerfile.harvest"
PKG="web/package.json"

# e.g. FROM mcr.microsoft.com/playwright:v1.61.1-noble  ->  1.61.1
image_ver=$(grep -oE 'mcr\.microsoft\.com/playwright:v[0-9]+\.[0-9]+\.[0-9]+' "$DOCKERFILE" \
  | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)

# e.g. "@playwright/test": "^1.61.1"  ->  1.61.1  (strip a leading ^ or ~)
pkg_ver=$(grep -oE '"@playwright/test":[[:space:]]*"[^"]+"' "$PKG" \
  | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)

if [ -z "$image_ver" ]; then
  echo "FAIL: could not parse Playwright base image tag from $DOCKERFILE"
  exit 1
fi
if [ -z "$pkg_ver" ]; then
  echo "FAIL: could not parse @playwright/test version from $PKG"
  exit 1
fi

if [ "$image_ver" != "$pkg_ver" ]; then
  echo "FAIL: Playwright version mismatch — harvester Chromium will not launch."
  echo "  $DOCKERFILE base image: v$image_ver"
  echo "  $PKG @playwright/test:  $pkg_ver"
  echo "Bump the '$DOCKERFILE' base tag (mcr.microsoft.com/playwright:v${pkg_ver}-noble) to match."
  exit 1
fi

echo "Playwright version check passed: $DOCKERFILE and $PKG both at v$image_ver."
