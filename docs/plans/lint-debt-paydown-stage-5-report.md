# Stage 5 — Dropped values in the root package, flip — Post-stage report

**Backlog items:** errcheck, wrapcheck, err113, gosec G104
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `main.go` - `Err*` sentinels for usage errors (text unchanged), context wraps at the boundary, `meta.Read` callers use `errors.Is(err, fs.ErrNotExist)`, `filepath.WalkDir` error recorded in the scan errors.
- `list.go`, `freeze.go` - sentinels (`ErrUseForce`, `ErrDestOverlap`, `ErrFoldersUnread`, ...), wraps, `errors.Is` for not-exist.
- `install.go` - wraps; `copyFile` now reports a failed `Close` of the written file; `closeOrWarn` helper logs a close failure on read paths; `addToProfile` reports `Close` failure of the profile.
- `selfupdate.go` - sentinels (`ErrDevBuild`, `ErrChecksumMismatch`, ...), wraps, `removeOrWarn` helper, rollback rename failure is logged, response body close is logged.
- `freeze_test.go`, `install_test.go`, `selfupdate_test.go`, `main_test.go` - `mustRead` / `writeBody` helpers and checked `ReadDir`.
- `internal/meta/meta.go` - open error wrapped (`opening marker: %w`); the `nolint:wrapcheck` is gone.
- `.golangci.yml` - `errcheck`, `wrapcheck`, `err113` and `G104` left the debt lists (`gosec.excludes` still holds the other ids).
- `AGENTS.md` - "wrap an error once, at the package boundary" in the Dropped-value bullet.
- `docs/lint-transition.md` - Progress line.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C5.1 | `bash scripts/lint-only.sh errcheck` (via check.sh lint, linter now on) | pass |
| C5.2 | `bash scripts/lint-only.sh wrapcheck` | pass |
| C5.3 | `bash scripts/lint-only.sh err113` | pass |
| C5.4 | gosec has no G104 (`docs/plans/lint-debt-paydown-verify-stage-5.py`) | pass |
| C5.5 | verify script: entries gone from `.golangci.yml` | pass |
| C5.6 | verify script: grep "wrap an error once" in AGENTS.md | pass |

## Gate results
`bash scripts/check.sh`: ok on every row (format, tidy, vet, lint, debt shrink, deadcode, build, test). `go.mod`/`go.sum` unchanged.

## Decisions
None new. Sentinels follow the plan's door (`ErrX`, `%w`); the text printed is identical, the wrap prefix added where the error crosses a package.

## Deviations from plan
- Files outside the list, both forced: `internal/meta/meta_test.go` (its `os.IsNotExist` assertion cannot hold once `meta.Read` wraps; now `errors.Is(err, fs.ErrNotExist)`, same condition) and `internal/fetch/write.go` (`nolint:errcheck` gained `,gosec`: G104 no longer excluded and gosec ignores `nolint:errcheck`).
- One `//nolint:err113` in `main.go` on `errors.New(plan.Reason)`: the message belongs to `internal/dest`, no condition to name.
- Two `//nolint:errcheck,gosec` (`os.Remove(old)` best-effort in `removeReplacedBinary`, and the one in write.go).

## Tried and abandoned
None.

## Noticed, not touched
None.

## Surprises / notes
Failures now visible: `copyFile` close of the destination (returned), `addToProfile` profile close (returned, was already), selfupdate: failed rollback rename, failed staged-file removal and response-body close (logged as `warning:`), `latestRelease` tag unescape (wrapped), `discoverRecursive` WalkDir error (added to scan errors; cannot occur today since the callback never returns one), tests: `ReadFile`, `ReadDir`, `ResponseWriter.Write` failures now fail the test. Printed error text gains context prefixes where errors cross a package ("parsing URL: ...", "downloading: ...", "reading the source marker: ..."); usage messages are byte-identical.
