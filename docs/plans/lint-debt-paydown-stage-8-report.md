# Stage 8 — Capability rules (gosec) and goconst — Post-stage report

**Backlog items:** D3, D7, D8; gosec G304, G703, G122, G204, G306; goconst
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `.golangci.yml` — `goconst` and its `# debt: 28 sites` comment removed from `disable`; the five gosec ids and the then-empty `gosec.excludes` key removed; the `_test\.go` exclusion rule is now `linters: [err113, wrapcheck, goconst, gosec]`.
- `freeze.go` — `os.ReadFile(absList)` (G304, G703) and `os.WriteFile(absList, ...)` (G703) carry `//nolint:gosec` with a reason: the list file path is the user's own command-line argument.
- `main.go` — `filepath.WalkDir(root, ...)` in `discoverRecursive` (G703) carries `//nolint:gosec`: root is the directory the user named to scan.
- `install.go` — new `const goosWindows = "windows"` (goconst; the 7 sites are one rule, the GOOS value), used at the 4 sites here; `copyFile` opens `filepath.Clean(src)` / `filepath.Clean(dst)` (G304 x2); `addToWindowsUserPath` (G204): the PowerShell script is now a constant that reads `$env:GH_GET_INSTALL_DIR`, set on `cmd.Env`, so the directory is data and never spliced into code (the old `'`-quoting is gone).
- `selfupdate.go` — the 3 `"windows"` sites use `goosWindows`.
- `internal/fetch/write.go` — `publishTree` opens `os.OpenRoot(src)` and reads with `root.ReadFile(rel)` / `root.Readlink(rel)` (G304, G122); the root is closed after the walk and its close error is returned when the walk succeeded. `writeFile`'s `os.WriteFile` (G703) carries `//nolint:gosec`: callers confine `dest` with `safeJoin`; the tainted value is file content, not a path.
- `internal/manifest/manifest.go` — `Load` opens `filepath.Clean(path)` (G304); imports `path/filepath`.
- `internal/meta/meta.go` — `Read` opens `filepath.Clean(filepath.Join(dir, FileName))` (G304).
- `docs/lint-transition.md` — D3 marked taken and delivered; D7 and D8 added; one Progress line.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C8.1 | `bash scripts/lint-only.sh gosec` -> `0 issues.` | pass |
| C8.2 | `bash scripts/lint-only.sh goconst` -> `0 issues.` | pass |
| C8.3 | grep `linters: \[err113, wrapcheck, goconst, gosec\]` in `.golangci.yml` -> 1 match | pass |
| C8.4 | grep `^\s+- (goconst\|G304\|G703\|G122\|G204\|G306)$` in `.golangci.yml` -> 0 matches | pass |
| C8.5 | `nolint:gosec` in production Go files: 4 (max 8) | pass |
| C8.6 | grep `^\s+excludes:` in `.golangci.yml` -> 0 matches | pass |

## Gate results
`bash scripts/check.sh`: ok on every row (format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test). `go build ./...` passes (inside check.sh). `TestPublishTree_CopiesNested` and `TestPublishTree_MergeKeepsNonColliding` seen passing in `go test -v`.

## Decisions
- Root-scoped reads | `root, err := os.OpenRoot(src)`; `root.ReadFile(rel)` / `root.Readlink(rel)` inside the walk; `root.Close()` error returned when the walk succeeded | rejected: `//nolint:gosec` on `os.ReadFile(path)` — G122 names a real symlink TOCTOU class and `os.Root` (Go 1.25+, the module is on 1.26) closes it with no exception to count.
- Data into a spawned shell | constant script reading `$env:GH_GET_INSTALL_DIR`, value set on `cmd.Env` | rejected: `//nolint:gosec` on the built script — the directory would stay spliced into PowerShell source, guarded only by hand-written quoting.

## Deviations from plan
- Measured counts at start differed from the stage block: gosec reported 12 sites, not 23 (G304 6, G703 4, G122 1, G204 1, G306 0; the rest were in tests, now excluded by D8). `main.go` had 1 G703, not 2. goconst had 2 production sites as planned.
- `filepath.Clean` does not satisfy G703 (gosec's taint config lists it as deliberately not a sanitizer), so the G703 sites are exceptions with reasons rather than fixes.

## Tried and abandoned
- `filepath.Clean` at the G703 sites (`freeze.go` x2, `main.go`, `write.go` writeFile): G703 still reported; reverted to the original call plus a reasoned `//nolint:gosec`.

## Noticed, not touched
- `internal/fetch/write.go` still compares `runtime.GOOS != "windows"` as a literal; it is another package, so goconst does not count it with the root package's constant.
- The pre-existing `os.Remove(dest) //nolint:errcheck,gosec` in `writeSymlink` is not counted by the C8.5 pattern (`nolint:gosec`); with it production has 5 gosec exceptions.

## Surprises / notes
- Python `Path.write_text` on Windows wrote CRLF; files were normalized back to LF before the gate.
