#!/usr/bin/env bash
# Deterministic gate for a jarvis checkout or worktree.
#
# Usage: factory/checks.sh [checkout]
#
# The checkout defaults to the current directory. The script does not start the
# voice services or Home Assistant; it only runs static checks, unit tests and
# the dashboard production build.

set -euo pipefail

target="${1:-.}"
if [ ! -d "$target" ]; then
  echo "checks: not a directory: $target" >&2
  exit 2
fi
target="$(cd "$target" && pwd)"

if [ ! -d "$target/client" ] || [ ! -d "$target/dashboard" ]; then
  echo "checks: not a jarvis checkout: $target" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "checks: docker is required for the dashboard checks" >&2
  exit 2
fi

echo "==> gofmt"
unformatted="$(cd "$target/client" && gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "checks: gofmt needed:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> go vet"
(cd "$target/client" && go vet ./...)

echo "==> go test"
(cd "$target/client" && go test ./...)

echo "==> dashboard lint and build"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e npm_config_cache=/tmp/npm-cache \
  -v "$target/dashboard:/app" \
  -w /app \
  node:22-alpine \
  sh -c 'npm ci --no-audit --no-fund && npm run lint && npm run build'

echo "checks: OK"
