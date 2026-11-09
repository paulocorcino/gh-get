#!/usr/bin/env bash
# The gate: the one command that runs every check. CI runs this same script.
# Needs: go, git, python3 (for docs/lint-scripts/debt.py). Prints one row per check.
set -uo pipefail
cd "$(dirname "$0")/.."

GOLANGCI_LINT_VERSION=v2.15.0
DEADCODE_VERSION=v0.52.0 # golang.org/x/tools
GL=${GOLANGCI_LINT:-"go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}"}
PY=$(command -v python3 || command -v python)
BASE=${BASE_REF:-origin/main}

fail=0
step() {
  local name=$1
  shift
  if "$@" >"${TMPDIR:-/tmp}/gate.$$.log" 2>&1; then
    printf 'ok    %s\n' "$name"
  else
    printf 'FAIL  %s\n' "$name"
    cat "${TMPDIR:-/tmp}/gate.$$.log"
    fail=1
  fi
  rm -f "${TMPDIR:-/tmp}/gate.$$.log"
}

unreachable() {
  local out
  out=$(go run "golang.org/x/tools/cmd/deadcode@${DEADCODE_VERSION}" ./...) || return 1
  [ -z "$out" ] || { echo "$out"; return 1; }
}

debt_shrink() {
  local base
  base=$(git merge-base HEAD "$BASE" 2>/dev/null) || return 0 # no base: this change brings the first list
  git show "$base:.golangci.yml" >.golangci.base.yml 2>/dev/null || { rm -f .golangci.base.yml; return 0; }
  "$PY" docs/lint-scripts/debt.py shrink .golangci.base.yml .golangci.yml --plan docs/lint-transition.md
  local rc=$?
  rm -f .golangci.base.yml
  return $rc
}

step "format (golangci-lint fmt)" $GL fmt --diff
step "go mod tidy" go mod tidy -diff
step "vet" go vet ./...
step "lint (golangci-lint)" $GL run ./...
step "debt list only shrinks" debt_shrink
step "unreachable code (deadcode)" unreachable
step "build" go build ./...
step "test" go test -race -count=1 -shuffle=on ./...

exit $fail
