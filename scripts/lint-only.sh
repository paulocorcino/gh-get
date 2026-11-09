#!/usr/bin/env bash
# Runs one golangci-lint linter with the repo config, also while it sits in the
# debt list of .golangci.yml. Usage: scripts/lint-only.sh <linter> [paths...]
# Paths default to ./... . The pinned version is read from scripts/check.sh.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=$(sed -n 's/^GOLANGCI_LINT_VERSION=//p' scripts/check.sh)
linter=${1:?usage: scripts/lint-only.sh <linter> [paths...]}
if [ $# -ge 2 ]; then
  paths=("${@:2}")
else
  paths=("./...")
fi

exec go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${VERSION}" run --enable-only "$linter" "${paths[@]}"
