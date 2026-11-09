# Lint transition

Plan to take the lint gate of gh-get from "every rule with no site is an error, the rest on a debt list" to "every rule an error, no debt list". Written by the lint-gate skill; read cold by an agent that does not have it. Its scripts sit in `docs/lint-scripts/` (a copy: not part of the product, and Python, so no Go linter reads it).

## Target

Every golangci-lint linter an error with no `disable` entry but the noise list; pinned tools; `scripts/check.sh` as the one local command, which CI runs as its only step; every exception inline with a reason (`nolintlint`); counts only go down (`debt.py shrink` against the merge base).

| Knob | End state | Pace | Curve at the end state | Reason |
|---|---|---|---|---|
| Function complexity (gocognit) | 10 | ceiling in steps: 50, 30, 20, 15, 10 | 24 sites over 10; 5 over 20; 1 over 50 | reference value, chosen by the developer |
| Nesting depth | revive `max-control-nesting` 3; nestif 4 | fix then flip | nestif: 2 sites at 4; revive nesting: 0 | reference |
| Arguments | 5 | fix then flip | 2 sites over 5 (`list.go:145` at 7, `freeze_test.go:14` at 6) | reference |
| File size | 500 non-blank lines | phase 5 | 1 file over (`main.go`, 568) | reference |
| Line width | not asked for Go | n/a | gofmt owns layout | reference |
| Clone size | 50 tokens / 5 lines | phase 0 | not measured (jscpd not run; D6) | pending D6 |
| Coverage of the diff | no reference | n/a | not measured — no diff-coverage tool installed | n/a |
| Optional families | none taken | n/a | `depguard` (D4), `paralleltest` (D5) await decisions | n/a |

The end-state `.golangci.yml`:

```yaml
version: "2"
linters:
  default: all
  # Every linter is on; the ones below are noise, off for good with their reason.
  disable:
    # noise: one complexity measure: gocognit
    - cyclop
    # noise: decision first: on with the rules of the layers (D4)
    - depguard
    # noise: jscpd measures clones across the repo
    - dupl
    # noise: deprecated; forces every field in every literal, against zero values
    - exhaustruct
    # noise: successor of exhaustruct, the same rule
    - exhaustruct_v5
    # noise: gocognit finds the real problem with fewer false alarms
    - funlen
    # noise: one complexity measure: gocognit
    - gocyclo
    # noise: comment punctuation
    - godot
    # noise: deprecated; depguard holds the imports
    - gomodguard
    # noise: deprecated; depguard holds the imports
    - gomodguard_v2
    # noise: returning an interface is sometimes the design
    - ireturn
    # noise: gofmt owns layout; the line-width knob is not asked for Go
    - lll
    # noise: one complexity measure: gocognit
    - maintidx
    # noise: every literal becomes a constant with no better name
    - mnd
    # noise: blank-line style only
    - nlreturn
    # noise: idiomatic `if err := f(); err != nil` form; style only, no defect class (decision D1)
    - noinlineerr
    # noise: decision first: whether tests run in parallel (D5)
    - paralleltest
    # noise: an external wire format picks its own casing
    - tagliatelle
    # noise: black-box test packages are a choice
    - testpackage
    # noise: decision first: whether tests run in parallel (D5)
    - tparallel
    # noise: short names in short scopes are idiomatic
    - varnamelen
    # noise: deprecated; blank-line style only
    - wsl
    # noise: blank-line style only
    - wsl_v5
  settings:
    gocognit:
      min-complexity: 10
    nestif:
      min-complexity: 4           # nestif scores nested blocks; the depth knob (3) is revive's max-control-nesting
    errcheck:
      check-type-assertions: true
      check-blank: true
    govet:
      enable-all: true
      disable: [fieldalignment]   # byte order of struct fields: noise outside hot paths
    exhaustive:
      default-signifies-exhaustive: false
    nolintlint:
      require-explanation: true
      require-specific: true
      allow-unused: false
    revive:
      enable-default-rules: true  # a `rules` list alone replaces the defaults
      rules:
        - name: argument-limit
          arguments: [5]
        - name: max-control-nesting
          arguments: [3]
    gosec:
      config:                     # the widest mode each rule accepts; a tool that writes files with these modes is not a finding
        G301: "0755"
        G302: "0755"
        G306: "0644"
    forbidigo:
      forbid:                     # a `forbid` list replaces the default pattern, so keep it
        - pattern: ^(fmt\.Print(|f|ln)|print|println)$
        - pattern: ^os\.Exit$
          msg: exit skips every defer; return from main
        # decision first: one package owns spawns, and a test waits on a condition
        # - pattern: ^exec\.Command$
        # - pattern: ^time\.Sleep$
  exclusions:
    rules:
      - path: _test\.go
        linters: [err113, wrapcheck]
formatters:
  enable: [gofumpt, goimports]
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  uniq-by-line: false             # the default keeps one issue per line and hides the rest
```

## Measured

Commit `e6aeaa3` (main), 2026-11-08. golangci-lint 2.15.0 (go install, go1.26.4 local; CI pinned to go 1.26.10), deadcode v0.52.0, govulncheck v1.9.0 (could not run again locally: the antivirus quarantines the build; the 15 findings come from its one successful run). Files read: 31 of 31 tracked `.go` files (`go list` sums to 31).

```
golangci-lint run -c <draft>.golangci.yml --output.json.path=gl.json ./...   # draft = go.md target, relative-path-mode: wd
python docs/lint-scripts/tally.py golangci gl.json --json gl-t.json --scope-depth 2
python docs/lint-scripts/tally.py curve golangci low.json --rule gocognit      # low.json: min-complexity 1, argument-limit 0
python docs/lint-scripts/tally.py sizes --ext go --lines 500 $(git ls-files '*.go')
python docs/lint-scripts/debt.py golangci --sites gl-t.json --noise noise.json --by-id gosec,gocritic --plan docs/lint-transition.md
```

519 sites in 28 files (94 of them `noinlineerr`, turned off as noise D1): 425 in the debt list. Debt table, largest first (`wrapcheck` and `err113` skip tests by config, so their test column is a floor):

| Rule | Class | Production | Test | Scopes | Strategy | Phase |
|---|---|---|---|---|---|---|
| `noinlineerr` | Surface | 45 | 49 | . 49, internal/fetch 36, internal/meta 5, internal/ghurl 2, internal/manifest 2 | noise D1 | — |
| `forbidigo` | Capability | 79 | 0 | . 79 | Fix then flip | 3 |
| `wrapcheck` | Dropped value | 77 | 0 | . 45, internal/fetch 27, internal/meta 3, internal/manifest 2 | Fix then flip | 2 |
| `errcheck` | Dropped value | 23 | 33 | . 24, internal/fetch 19, internal/ghurl 11, internal/manifest 1, internal/meta 1 | Fix then flip | 2 |
| `err113` | Dropped value | 56 | 0 | . 37, internal/fetch 13, internal/ghurl 6 | Fix then flip | 2 |
| `goconst` | Surface | 2 | 26 | . 15, internal/fetch 8, internal/ghurl 5 | Fix then flip | 4 |
| `gocognit` | Shape | 20 | 4 | . 12, internal/fetch 8, internal/ghurl 4 | Ceiling in steps | 5 |
| `gosec:G104` | Dropped value | 8 | 9 | . 10, internal/fetch 7 | Fix then flip | 2 |
| `perfsprint` | Surface | 14 | 1 | . 13, internal/fetch 1, internal/ghurl 1 | Fix by command | 1 |
| `gosec:G304` | Capability | 6 | 9 | . 11, internal/fetch 2, internal/manifest 1, internal/meta 1 | Decision first (D3) | 4 |
| `govet` | Surface | 6 | 0 | . 5, internal/fetch 1 | Fix then flip | 4 |
| `nonamedreturns` | Surface | 5 | 0 | internal/fetch 5 | Fix then flip | 1 |
| `gosec:G703` | Capability | 5 | 0 | . 4, internal/fetch 1 | Decision first (D3) | 4 |
| `gochecknoglobals` | Surface | 5 | 0 | internal/fetch 4, . 1 | Fix then flip | 4 |
| `modernize` | Surface | 3 | 1 | internal/fetch 3, internal/meta 1 | Fix by command | 1 |
| `noctx` | Capability | 3 | 0 | . 3 | Fix then flip | 4 |
| `gosec:G602` | Escape hatch | 3 | 0 | . 3 | Fix then flip | 4 |
| `forcetypeassert` | Escape hatch | 1 | 2 | internal/fetch 3 | Fix then flip | 4 |
| `exhaustive` | Escape hatch | 3 | 0 | . 2, internal/fetch 1 | Fix then flip | 4 |
| `usetesting` | Surface | 0 | 2 | . 2 | Fix by command | 1 |
| `thelper` | Surface | 0 | 2 | internal/ghurl 2 | Fix then flip | 1 |
| `revive:unused-parameter` | Surface | 0 | 2 | internal/fetch 1, internal/ghurl 1 | Fix then flip | 1 |
| `revive:argument-limit` | Shape | 1 | 1 | . 2 | Fix then flip | 5 |
| `nestif` | Shape | 2 | 0 | . 2 | Fix then flip | 5 |
| `errorlint` | Dropped value | 2 | 0 | internal/fetch 2 | Fix then flip | 2 |
| `intrange` | Surface | 0 | 1 | internal/fetch 1 | Fix by command | 1 |
| `gosec:G306` | Capability | 0 | 1 | . 1 | Decision first (D3) | 4 |
| `gosec:G204` | Capability | 1 | 0 | . 1 | Decision first (D3) | 4 |
| `gosec:G122` | Capability | 1 | 0 | internal/fetch 1 | Decision first (D3) | 4 |
| `gocritic:badCall` | Escape hatch | 0 | 1 | . 1 | Fix then flip | 4 |
| `errname` | Surface | 1 | 0 | internal/fetch 1 | Fix then flip | 1 |
| `embeddedstructfieldcheck` | Surface | 1 | 0 | internal/fetch 1 | Fix then flip | 1 |
| `containedctx` | Shape | 1 | 0 | internal/fetch 1 | Fix then flip | 4 |
| `canonicalheader` | Surface | 1 | 0 | internal/fetch 1 | Fix by command | 1 |

Existing suppressions: `//nolint` 0, `//go:build ignore` 0, `_ =` / `_, _` on a call 3 (read each when `errcheck` is turned on). `n/a`: `exhaustruct`, `funlen`, `lll`, `cyclop`, `gocyclo`, `maintidx`, `mnd` are in the noise list with their reasons in `.golangci.yml`.

## Decisions for the human

- **D1 (taken)** `noinlineerr` off as noise: it bans the idiomatic `if err := f(); err != nil`; style only.
- **D2 (taken)** Terminal output goes through an injected `io.Writer`; only `main` calls `os.Exit`. Holds phase 3.
- **D3 (open)** Which package owns file writes with their modes, process spawns (`gosec` G204/G306/G304/G703/G122) and waiting in tests (`time.Sleep`). Options: one `internal/` package owns each capability and carries the one `//nolint` per call (recommended); or each call site carries its own reason (more exceptions to count). Holds the capability rows of phase 4.
- **D4 (taken)** Layer boundaries for `depguard` (`strict` list: `$gostd` and the module path; the module has no dependencies). Recommended: turn it on with that list in phase 0. Costs nothing today.
- **D5 (open)** Whether tests run in parallel (`paralleltest`, `tparallel`). Recommended: no for now; keep them off with this decision as the reason until the test suite is shared-state free.
- **D6 (open)** jscpd for duplicate blocks (needs Node or a pinned binary with checksum): recommended, at 50 tokens / 5 lines, `--fail-on-new-clones 0` plus a count ratchet. Not measured in this run.

## Phases

Order: cheap mechanical fixes first (they shrink the noise), then dropped values (touch most files), capability (needs D3), escape hatches, shape last because splitting moves code the other phases edit. Phase 0 is independent. Phases touching the same files run in sequence.

### Phase 0: the gate itself

- **Rules**: none. **Slices**: one commit per item.
- Add `govulncheck` to `scripts/check.sh` (`go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`): 15 standard-library findings at go1.26.4; CI is already pinned at go 1.26.10. Re-measure in CI before adding; if findings remain, pin the first fixing release.
- Pins: a test or step that compares the CI `go-version`, `GOLANGCI_LINT_VERSION` and `DEADCODE_VERSION` in `scripts/check.sh` with the versions this plan names; `go run pkg@version` is verified by the Go checksum database. A scheduled job that opens an issue when a pinned tool has a newer release (`drift.py age`).
- A per-edit agent hook (fast: `golangci-lint run` on the changed package and `golangci-lint fmt --diff`) that also refuses `--no-verify`, force push, and unasked edits to `.golangci.yml`; then replace the "no per-edit hook yet" line in `AGENTS.md`.
- Protected `main`: pull requests only, `Gate` required, no force push (GitHub setting). Pin actions by SHA, set a Dependabot cooldown; add actionlint and zizmor to the gate.
- Add gitleaks, typos, shellcheck/shfmt on `scripts/check.sh`, markdownlint/yamllint/lychee for docs. Each at 0 findings before it joins the gate; otherwise it gets its own debt row here.
- D4: `depguard` on with `list-mode: strict` and `allow: [$gostd, github.com/paulocorcino/gh-get]` (no site expected). D6: jscpd, with `docs/lint-scripts/` and the plan's code blocks in its ignore list (not the repo's code).
- GOOS matrix: the release builds other platforms; run the lint with `GOOS=windows` and `GOOS=linux` (a runner per OS), so code under build constraints is read.
- **Done when**: `scripts/check.sh` has a row per item above and CI is green.

### Phase 1: mechanical and surface (Fix by command, then Fix then flip)

- **Rules**: `perfsprint` 15, `modernize` 4, `intrange` 1, `canonicalheader` 1, `usetesting` 2 (command: `golangci-lint run --fix --enable-only <linter>`, one commit per rule with the command in its message; read the diff for `usetesting`); then by hand: `nonamedreturns` 5, `thelper` 2, `revive:unused-parameter` 2, `errname` 1, `embeddedstructfieldcheck` 1.
- **Measure first**: `golangci-lint run --enable-only <linter> ./...` for each, with the linter removed from `disable` in a scratch copy of `.golangci.yml`.
- **Slices**: root package, then `internal/fetch`, `internal/ghurl`, `internal/meta`.
- **How a site is fixed**: Surface recipe (tool command, or rename/move with no behavior change). A named return that a `defer` writes to becomes an explicit local, not an exception.
- **Ratchet**: delete each linter's `disable` line and its `# debt:` comment in the commit that reaches 0. **Agent docs**: none.
- **Done when**: the linter lines are gone from `.golangci.yml`, the re-measure shows 0, `scripts/check.sh` is green.

### Phase 2: dropped values

- **Rules**: `errcheck` 56 (`check-blank: true` also flags `_ =` writes), `gosec:G104` 17 (the same sites as `errcheck`: `G104` leaves `excludes` with the `errcheck` slice), `wrapcheck` 77, `err113` 56, `errorlint` 2.
- **Measure first**: as phase 1.
- **Slices**: the root package (`main.go`, `install.go`, `list.go`, `freeze.go`, `selfupdate.go`: 24 `errcheck`, 45 `wrapcheck`, 37 `err113`); `internal/fetch`; `internal/ghurl`, `internal/manifest`, `internal/meta`.
- **How a site is fixed**: Dropped value recipe. `err113`: sentinel errors (`var ErrX = errors.New(...)`) defined where the caller matches on them, or `fmt.Errorf("...: %w", ErrX)`. `wrapcheck`: wrap with context at the package boundary only (`fmt.Errorf("fetching %s: %w", ref, err)`). A deferred `Close` on a read path is wrapped in a helper that logs. Each commit body names every site where a failure became visible.
- **Agent docs**: add "wrap an error once, at the package boundary".
- **Done when**: no `errcheck`, `wrapcheck`, `err113`, `errorlint` line in `disable`, `G104` out of `excludes`, re-measure 0.

### Phase 3: output through a writer (D2)

- **Rules**: `forbidigo` 79 (all in the root package: `fmt.Print*`, `print`, `os.Exit`).
- **How a site is fixed**: Capability recipe: `run` receives `stdout, stderr io.Writer`; `fmt.Fprint*` to them; `main` is the only `os.Exit`, with the one `//nolint:forbidigo // exit code of the process; main returns nothing`. Tests pass buffers.
- **Slices**: `main.go`; then `install.go`, `list.go`, `freeze.go`, `selfupdate.go`.
- **Agent docs**: add "output goes through the writer passed to `run`; only `main` exits".
- **Done when**: `forbidigo` is out of `disable`, and the re-measure shows only the `main` exception.

### Phase 4: escape hatches, capability and the rest (the file and spawn rows wait on D3)

- **Rules**: `forcetypeassert` 3, `exhaustive` 3, `gosec:G602` 3, `gocritic:badCall` 1, `noctx` 3, `govet` 6, `gochecknoglobals` 5, `goconst` 28, `containedctx` 1; and after **D3**: `gosec:G304` 15, `G703` 5, `G122`, `G204`, `G306` 1 each.
- **How a site is fixed**: type assertions: check `ok` and return an error; slice index: check the length once; `noctx`: `http.NewRequestWithContext`; globals: constants, or passed in; `goconst`: a named constant only when the strings are one rule (not two that look alike); `containedctx`: pass the context as a parameter. File and spawn calls: Capability recipe through the package D3 names, with the one exception and its reason there. A linter off whole (`govet`, `revive`) turns its other checks off for the window; its slice removes its sites first.
- **Slices**: root, `internal/fetch`, the rest.
- **Done when**: each linter or `excludes` entry is gone; re-measure 0.

### Phase 5: shape

- **Rules**: `gocognit` 24 (Ceiling in steps), `nestif` 2, `revive:argument-limit` 2, and the size of `main.go` (568 lines, limit 500).
- **Measure first**: a run with `min-complexity: 1`, then `tally.py curve golangci <out> --rule gocognit`.
- **Strategy**: `gocognit` goes on at the ceiling 50 first (1 site over: `main.go:109` at 62): `min-complexity: 50` with the linter out of `disable`; each step lowers the number (30: 2 sites, 20: 5, 15: 12, 10: 24) in the commit that fixes the sites above the new value. The last step is `min-complexity: 10`; whatever cannot reach it carries `//nolint:gocognit // <reason>`.
- **How a site is fixed**: Shape recipe. First `main.go:109` (`run`, 62), `internal/fetch/tarball.go:22` (33), `internal/fetch/gitrefs.go:96` (26), `freeze.go:162` (26), `main.go:447` (21): split by responsibility (argument parsing, one function per subcommand), never into helpers without one. `main.go` is split into one file per command.
- **Done when**: `gocognit`, `nestif`, `revive` are out of `disable`, the ceiling is 10, `main.go` is under 500 lines, `disable` holds only noise, and no `# debt:` comment is left.

## One-way doors

- The debt list is the `disable` list of `.golangci.yml`, each entry under a `# debt: N sites` comment on its own line, held by `debt.py shrink`; rejected: a baseline file (golangci-lint has no per-rule baseline, and the list already satisfies "a count only goes down").
- `gosec` and `gocritic` stay on whole and list only their check ids in `settings.*.excludes` / `disabled-checks`; rejected: disabling them whole, which turns off their other checks.
- Exceptions are `//nolint:<linter> // <why>`; rejected: `exclusions.rules` paths, which carry no reason at the site.
- The gate is `scripts/check.sh` (bash) and CI runs it as its single step; rejected: a Makefile or `just` (not installed on the developer's Windows machine) and a CI that lists the steps again, which could drift.
- Tools run by `go run pkg@version`; rejected: a `tools/go.mod`, which adds a `go.sum` to a stdlib-only module (the Go checksum database verifies the download).

## Fix discipline

- A fix keeps the behavior. A behavior change is its own commit, outside the lint work.
- Each rule has a class, and the class gives the fix:
  - **Dropped value** (an ignored error, a value never read): the value reaches its caller or a log line. The commit body names each site where a failure became visible.
  - **Fail fast** (a fallback that hides a failure): the fallback goes; the failure surfaces at the boundary that can act on it.
  - **Escape hatch** (a cast, an unchecked assertion, a bare ignore): parse, don't validate: narrow the value once, at the boundary, into a type the checker reads.
  - **Capability** (`print`, a spawn, `exit`, a file write outside the module that owns it): the call goes through the owning module, which carries the one exception with its reason.
  - **Shape** (complexity, depth, arguments, size): let the data structure the program: guard clauses and early returns, a table of cases, a `switch` over a tuple, or a split along a responsibility the code already has. A file over the size limit is a god file, a function over the complexity limit a god function: split each by the responsibilities it already carries, one module or function per responsibility. Don't patch bad code, rewrite it: helpers that follow no responsibility are not a fix. A clone: one rule written twice becomes one function or constant named for the rule; two rules that only look alike stay two, and the clone carries the ignore marker with the reason they change for different reasons.
  - **Surface** (format, naming, imports): the tool's own command, in a commit of its own.
- An exception is the last resort: inline, the rule named, and a reason that names where the rule's approximation is wrong here, or the design fact that makes compliance impossible. "To pass the check" is not a reason. Each exception is counted by a ratchet.
- A test is never weakened, skipped or deleted to turn the gate green, and a hook is never bypassed.
- A baseline or a ratchet constant only goes down, in the commit that removed the sites.
- Each slice leaves the whole gate green.

## Coverage

| Row | Verdict |
|---|---|
| repo: One local command | on — scripts/check.sh |
| repo: CI mirrors it | on — CI runs scripts/check.sh itself; a test of the pairing is unneeded while CI has one step |
| repo: No warn level | on — golangci-lint v2 has no warn level; every enabled linter fails the run |
| repo: Pinned tools | phase 0 |
| repo: New releases of the pinned tools | phase 0 |
| repo: Locked dependencies | n/a — stdlib-only module, no go.sum |
| repo: No new duplicated block | phase 0 — D6 |
| repo: The existing duplicated blocks only go down | phase 0 — D6 |
| repo: Function complexity at most 10 | sites 24 — gocognit, phase 5 |
| repo: File size at most N lines | phase 5 — main.go 568 lines; limit 500 |
| repo: Inline test block at most N lines | n/a — Go keeps tests in _test.go files |
| repo: Direction of dependencies | decision — D4 |
| repo: Ratchets as tests | n/a — no ratchet count exists yet; the debt list is held by debt.py shrink |
| repo: No automatic retry | n/a — go test has no retry |
| repo: Doctests run | n/a — no Example functions to run beyond go test |
| repo: Coverage of the diff at least N% | not measured — needs a diff-coverage tool the toolchain does not ship |
| repo: The test catches the real error | phase 0 — gremlins on the diff, optional |
| repo: No silent snapshot update | n/a — no snapshot tests |
| repo: No focused or skipped test left behind | n/a — Go has no focus marker; no t.Skip in the tests |
| repo: A secret in the code | phase 0 |
| repo: A vulnerable dependency | phase 0 — govulncheck, 15 stdlib findings at go1.26.4 |
| repo: Static analysis | phase 0 — CodeQL via GitHub setting |
| repo: An unsafe workflow | phase 0 — actionlint, zizmor, SHA pins |
| repo: Dependency updates | phase 0 |
| repo: A typo in code or docs | phase 0 |
| repo: Shell scripts | phase 0 — shellcheck and shfmt on scripts/check.sh |
| repo: PowerShell scripts | n/a — no tracked .ps1 |
| repo: HTML and CSS | n/a — none |
| repo: TOML, YAML, Markdown | phase 0 |
| repo: A broken link in the docs | phase 0 |
| repo: Line ends and encoding | on — .gitattributes sets eol=lf |
| repo: A generated file out of date | n/a — no generated files |
| repo: Commit message format | n/a — conventional commits by habit; no check wanted yet |
| repo: Changelog | n/a — releases are tags |
| repo: Internal words in user-facing text | n/a — no such convention found |
| repo: Protected main branch | phase 0 |
| repo: An agent hook | phase 0 |
| repo: Shift left | phase 0 |
| repo: Pre-commit hook (optional) | n/a — optional; CI and the gate are the check |
| repo: A warn level | closed by golangci-lint v2 (no warn level) |
| repo: A bypass (--no-verify, a force push, a merge with a red check) | open — needs the agent hook and branch protection (phase 0) |
| repo: An unasked edit to a lint config, a debt list, an ignore list or a ratchet constant | closed by debt.py shrink (list); AGENTS.md rule; the agent hook is phase 0 |
| repo: A suppression with no reason, or one that suppresses nothing | closed by nolintlint |
| repo: A blanket or file-level suppression (@ts-nocheck, a file-level disable, a crate-root allow) | closed by nolintlint require-specific |
| repo: A path no check reads (a new directory outside every glob) | closed by golangci-lint run ./... and go list file count |
| repo: A skipped or deleted test | open — mutation testing is phase 0 and optional |
| repo: A weakened test (an assertion that checks nothing) | open — mutation testing is phase 0 and optional |
| repo: Code the gate does not build (#[cfg] of another platform, a feature, a build tag) | open — release builds other GOOS targets; the gate lints the host only (phase 0: GOOS matrix) |
| repo: A split into helpers with no responsibility, to pass a complexity limit | open — a human reads the split |
| repo: Two rules merged into one helper to remove a clone | open — a human reads the merge |
| repo: A clone hidden by an ignore marker (jscpd:ignore-start) | n/a — no jscpd yet (D6) |
| repo: A row of a reference that the lint gate run skipped | closed by rows.py check |
| repo: An ast-grep-ignore with no rule id, or on the first line of a file | n/a — no ast-grep rule exists |
| repo: A force push to the default branch that drops the base commit: shrink against an empty base passes | open — branch protection (phase 0) |
| repo: The skill's own artifacts in the repo (the copied scripts, the plan's code blocks) counted or linted as the repo's code | closed by golangci-lint (Go files only); excluded from jscpd when it joins (D6) |
| repo: A new noise: entry in a debt list: a debt rule off "for good" | closed by debt.py shrink (reason must cite a decision id) |
| repo: A fake reason on an exception | open — a human reads each new exception |
| go: Format | on — golangci-lint fmt (gofumpt, goimports) |
| go: Lint | on — .golangci.yml, 425 sites in the debt list |
| go: Module file tidy | on — go mod tidy -diff |
| go: Vulnerable dependency | phase 0 |
| go: Unreachable code | on — deadcode, 0 findings |
| go: Leftovers | on — godox, 0 sites |
| go: Tests | on — go test -race -count=1 -shuffle=on |
| go: Coverage of the diff | not measured — no diff-coverage tool installed |
| go: Mutation | phase 0 |
| go: Go floor and tool pins | phase 0 — CI pins go 1.26.10 and golangci-lint/deadcode by version; checksum check and a pin test remain |
| go: exhaustruct | out — deprecated, against zero values |
| go: funlen, lll | out — length is not the defect |
| go: cyclop, gocyclo, maintidx with gocognit | out — one measure: gocognit |
| go: mnd | out — constants with no better name |

## Progress

- 2026-11-08, step 5: every rule with 0 sites is an error; 425 sites listed as debt, 94 `noinlineerr` sites as noise.
- 2026-11-08, lint(stage 1): `depguard` on with `list-mode: strict`, allow `$gostd` and the module path (D4 taken, 0 sites); `scripts/lint-only.sh <linter> [paths]` runs one linter while it is in the debt list.
- 2026-11-08, lint(stage 2): `perfsprint` flipped (15 sites: `fmt.Errorf` without verbs to `errors.New`, one `strings.Builder` in a test helper).
- 2026-11-08, lint(stage 2): `modernize` flipped (4 sites: `range` over int, `FieldsSeq`, `CutPrefix`, `strings.Cut`).
- 2026-11-08, lint(stage 2): `intrange` flipped (1 site, fixed by the same `range 5` edit as `modernize`).
- 2026-11-08, lint(stage 2): `usetesting` flipped (2 sites: `t.Chdir` in `TestRun_RecursiveNoInstallationsSucceeds`).
- 2026-11-08, lint(stage 2): `canonicalheader` flipped (1 site: `X-Github-Api-Version`; `Header.Set` canonicalizes both spellings to the same key, so the request is unchanged).
- 2026-11-08, lint(stage 3): `nonamedreturns` flipped (5 sites: explicit result types and a local `entries` in `listFolder`/`folderMember`).
- 2026-11-08, lint(stage 3): `thelper` flipped (2 sites: `t.Helper()` first in `noDefault` and `noCheck`).
- 2026-11-08, lint(stage 3): `errname` flipped (1 site: `errFolderNotFound` renamed `folderNotFoundError`).
- 2026-11-08, lint(stage 3): `embeddedstructfieldcheck` flipped (1 site: blank line after the embedded `io.ReadCloser` in `stallBody`).
- 2026-11-08, lint(stage 3): `containedctx` flipped (1 site: `stallBody` keeps a `cause func() error` closure instead of a `context.Context`).
- 2026-11-08, lint(stage 3): `revive` unused-parameter fixed at 2 sites (`_`); `revive` stays listed for argument-limit (Stage 9).
- 2026-11-08, lint(stage 4): `errorlint` flipped (2 sites: `errors.As` in `isRateLimit`, `errors.Is` for `io.EOF` in the tarball reader); errcheck, wrapcheck and err113 are 0 in `./internal/...` and stay listed for the root package (Stage 5).
- 2026-11-08, lint(stage 5): `errcheck`, `wrapcheck`, `err113` and `gosec:G104` flipped (root package: sentinels `Err*` for usage and entry errors, context wraps at the package boundary, `meta.Read` now wraps and its callers use `errors.Is(err, fs.ErrNotExist)`).
