#!/usr/bin/env bash
# One command for the coverage report the case asks for: Go and frontend lines together, threshold 80%.
# Needs Go, Node and Docker (Go tests start Postgres and Redis in containers). Report goes to coverage/.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
out="$root/coverage"
min="${COVERAGE_MIN:-80}"

rm -rf "$out"
mkdir -p "$out"

echo "==> merger tests"
node --test "$root/scripts/coverage.test.mjs" >/dev/null

echo "==> Go"
make -C "$root/backend" cover COVER_OUT="$out/go.out"

echo "==> frontend"
(cd "$root/frontend" && npx vitest run --coverage \
  --coverage.reporter=lcov --coverage.reporter=html --coverage.reporter=text-summary \
  --coverage.reportsDirectory="$out/frontend")

echo "==> total"
node "$root/scripts/coverage.mjs" "$out/go.out" "$out/frontend/lcov.info" "$out" "$min"
