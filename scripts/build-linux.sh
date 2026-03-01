#!/usr/bin/env bash
set -euo pipefail

mkdir -p build
GOOS=linux GOARCH=amd64 go build -o build/os1-linux-amd64 ./apps/desktop
GOOS=linux GOARCH=arm64 go build -o build/os1-linux-arm64 ./apps/desktop
