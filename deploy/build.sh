#!/usr/bin/env bash
# Cross-compile budol-api for a server (linux/amd64, static, stripped).
# Run from the repo root:  bash deploy/build.sh
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist-bin
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o dist-bin/budol-api ./cmd/budol-api
ls -la dist-bin/budol-api
