#!/usr/bin/env bash
set -euo pipefail

mkdir -p .tmp/gocache .tmp/gomodcache
GOCACHE="${GOCACHE:-$(pwd)/.tmp/gocache}" GOMODCACHE="${GOMODCACHE:-$(pwd)/.tmp/gomodcache}" go run ./apps/desktop &
backend_pid=$!
trap 'kill $backend_pid' EXIT
pnpm --filter @os1/ui dev
