#!/usr/bin/env bash
set -euo pipefail

mkdir -p build
GOOS=darwin GOARCH=amd64 go build -o build/os1-macos-amd64 ./apps/desktop
GOOS=darwin GOARCH=arm64 go build -o build/os1-macos-arm64 ./apps/desktop
