#!/usr/bin/env bash
set -euo pipefail

mkdir -p build
GOOS=windows GOARCH=amd64 go build -o build/os1-windows-amd64.exe ./apps/desktop
GOOS=windows GOARCH=arm64 go build -o build/os1-windows-arm64.exe ./apps/desktop
