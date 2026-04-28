#!/usr/bin/env bash
set -euo pipefail

skip_full="${1:-}"
export GOCACHE="${PWD}/.gocache"
export XDG_CONFIG_HOME="${PWD}/.xdg-config"
export XDG_CACHE_HOME="${PWD}/.xdg-cache"
mkdir -p "${GOCACHE}" "${XDG_CONFIG_HOME}" "${XDG_CACHE_HOME}"

echo "==> Running focused sandbox suites..."
focused_packages=(
  ./internal/tools
  ./internal/app
  ./internal/agent
  ./internal/sandbox
  ./internal/config
)
for pkg in "${focused_packages[@]}"; do
  if ! go test "${pkg}" -count=1 -timeout 300s; then
    echo "!! Focused sandbox suite failed for ${pkg}. Re-running with -v for diagnostics..."
    if go test "${pkg}" -count=1 -timeout 300s -v; then
      echo "!! Focused sandbox suite for ${pkg} passed on retry (-v). Continuing."
    else
      exit 1
    fi
  fi
done

if [[ "${skip_full}" != "--skip-full" ]]; then
  echo "==> Running full test suite..."
  go test ./... -count=1 -timeout 300s
fi

echo "Sandbox acceptance checks passed."
