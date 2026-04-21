#!/usr/bin/env bash
set -euo pipefail

skip_full="${1:-}"
export GOCACHE="${PWD}/.gocache"

echo "==> Running focused sandbox suites..."
go test ./internal/tools ./internal/app ./internal/agent ./internal/sandbox ./internal/config -count=1 -timeout 300s

if [[ "${skip_full}" != "--skip-full" ]]; then
  echo "==> Running full test suite..."
  go test ./... -count=1 -timeout 300s
fi

echo "Sandbox acceptance checks passed."

