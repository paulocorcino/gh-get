#!/usr/bin/env bash
# The gate: the one command that runs every check. CI runs this same script.
# Needs: go, git, python3 (for docs/lint-scripts/debt.py). Prints one row per check.
# shellcheck disable=SC2329 # the check functions are called through `step`
set -uo pipefail
cd "$(dirname "$0")/.." || exit

GOLANGCI_LINT_VERSION=v2.15.0
DEADCODE_VERSION=v0.52.0 # golang.org/x/tools
GOVULNCHECK_VERSION=v1.9.0
ACTIONLINT_VERSION=v1.7.12
GITLEAKS_VERSION=v8.30.1
SHFMT_VERSION=v3.13.1
ZIZMOR_VERSION=1.32.0          # PyPI
TYPOS_VERSION=1.52.0           # PyPI
SHELLCHECK_PY_VERSION=0.11.0.1 # PyPI
YAMLLINT_VERSION=1.38.0        # PyPI
MARKDOWNLINT_VERSION=0.18.1    # npm markdownlint-cli2
JSCPD_VERSION=4.0.5            # npm
RUFF_VERSION=0.17.4            # PyPI
read -ra GL <<<"${GOLANGCI_LINT:-go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}}"
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
  [ -z "$out" ] || {
    echo "$out"
    return 1
  }
}

vulns() {
  go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./...
}

# The PyPI tools (zizmor, typos, shellcheck) live in one throwaway venv, pinned.
# The release builds windows, linux and darwin: vet reads the code under each GOOS.
cross_vet() {
  local os
  for os in windows linux darwin; do
    GOOS=$os go vet ./... || {
      echo "GOOS=$os"
      return 1
    }
  done
}

PYTOOLS=
pytools() {
  [ -n "$PYTOOLS" ] && return 0
  PYTOOLS=$(mktemp -d)
  "$PY" -m venv "$PYTOOLS" || return 1
  local bin="$PYTOOLS/bin"
  [ -d "$PYTOOLS/Scripts" ] && bin="$PYTOOLS/Scripts"
  "$bin/pip" install -q "zizmor==${ZIZMOR_VERSION}" "typos==${TYPOS_VERSION}" "shellcheck-py==${SHELLCHECK_PY_VERSION}" "yamllint==${YAMLLINT_VERSION}" "ruff==${RUFF_VERSION}" || return 1
}
pybin() {
  local tool=$1
  shift
  pytools || return 1
  if [ -d "$PYTOOLS/Scripts" ]; then "$PYTOOLS/Scripts/$tool" "$@"; else "$PYTOOLS/bin/$tool" "$@"; fi
}

workflows() {
  go run "github.com/rhysd/actionlint/cmd/actionlint@${ACTIONLINT_VERSION}" -color=false || return 1
  pybin zizmor --no-progress --offline .
}

shell_scripts() {
  pybin shellcheck scripts/*.sh || return 1
  go run "mvdan.cc/sh/v3/cmd/shfmt@${SHFMT_VERSION}" -i 2 -d scripts/*.sh
}

spelling() {
  pybin typos .
}

docs() {
  pybin yamllint -c .yamllint.yml . || return 1
  npx --yes "markdownlint-cli2@${MARKDOWNLINT_VERSION}"
}

# Links between the repo's own files and their anchors. External URLs are not checked: a site's
# downtime would turn the gate red with no change in the code. The binary is pinned by checksum.
links() {
  local dir bin
  dir=$(mktemp -d)
  bin=$("$PY" scripts/fetch-lychee.py "$dir") || {
    rm -rf "$dir"
    return 1
  }
  "$bin" --offline --no-progress --include-fragments --exclude-path docs/plans --exclude-path docs/lint-scripts '**/*.md'
  local rc=$?
  rm -rf "$dir"
  return "$rc"
}

clones() {
  npx --yes "jscpd@${JSCPD_VERSION}" .
}

python_scripts() {
  pybin ruff check scripts/ || return 1
  pybin ruff format --check scripts/
}

secrets() {
  go run "github.com/zricethezav/gitleaks/v8@${GITLEAKS_VERSION}" git --no-banner --redact
}

debt_shrink() {
  local base
  base=$(git merge-base HEAD "$BASE" 2>/dev/null) || return 0 # no base: this change brings the first list
  git show "$base:.golangci.yml" >.golangci.base.yml 2>/dev/null || {
    rm -f .golangci.base.yml
    return 0
  }
  "$PY" docs/lint-scripts/debt.py shrink .golangci.base.yml .golangci.yml --plan docs/lint-transition.md
  local rc=$?
  rm -f .golangci.base.yml
  return "$rc"
}

step "format (golangci-lint fmt)" "${GL[@]}" fmt --diff
step "go mod tidy" go mod tidy -diff
step "vet" go vet ./...
step "vet (windows, linux, darwin)" cross_vet
step "lint (golangci-lint)" "${GL[@]}" run ./...
step "debt list only shrinks" debt_shrink
step "unreachable code (deadcode)" unreachable
step "vulnerabilities (govulncheck)" vulns
step "workflows (actionlint, zizmor)" workflows
step "shell scripts (shellcheck, shfmt)" shell_scripts
step "spelling (typos)" spelling
step "docs (yamllint, markdownlint)" docs
step "links (lychee, offline)" links
step "python scripts (ruff)" python_scripts
step "duplicate code (jscpd)" clones
step "secrets (gitleaks)" secrets
step "build" go build ./...
step "test" go test -race -count=1 -shuffle=on ./...

[ -n "$PYTOOLS" ] && rm -rf "$PYTOOLS"
exit "$fail"
