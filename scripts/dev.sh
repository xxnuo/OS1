#!/usr/bin/env bash
set -euo pipefail

mkdir -p .tmp/gocache .tmp/gomodcache
GOCACHE="${GOCACHE:-$(pwd)/.tmp/gocache}" GOMODCACHE="${GOMODCACHE:-$(pwd)/.tmp/gomodcache}" go run ./apps/desktop
