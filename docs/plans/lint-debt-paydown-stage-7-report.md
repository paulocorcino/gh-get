# Stage 7 — Escape hatches and local rules — Post-stage report

**Backlog items:** forcetypeassert 3, exhaustive 3, noctx 3, govet 6, gochecknoglobals 5, gocritic:badCall 1, gosec G602 3
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `freeze.go`, `internal/fetch/fetch.go`, `main.go`: exhaustive switches list every case (`freezeKept`, `Merge`, `destpolicy.Replace`; `mode` in `runDownload` is now set per case).
- `install.go`, `list.go`, `main.go`, `selfupdate.go`, `internal/fetch/fetch.go`: shadowed `err` renamed (7 sites).
- `install.go`: `exec.CommandContext(context.Background(), ...)`. `selfupdate.go`: `latestRelease` and `fetchReleaseFile` take a context and use `http.NewRequestWithContext` + `Do`; `selfUpdate` creates `context.Background()`.
- `main.go`: new `optionValue(args, i)` checks the length once; replaces the three `args[i]` reads (G602).
- `install_test.go`: `filepath.Join(x)` of one argument -> `x` (badCall).
- `internal/fetch/client.go`, `selfupdate.go`: the 5 endpoint globals carry `//nolint:gochecknoglobals // test seam ...`.
- `.golangci.yml`: removed govet, gochecknoglobals, exhaustive, forcetypeassert, noctx, gocritic `badCall` (and the empty `gocritic` key), gosec `G602`.
- `docs/lint-transition.md`: Progress line.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C7.1-C7.6 | `bash scripts/lint-only.sh <linter>` for forcetypeassert, exhaustive, noctx, govet, gochecknoglobals, gocritic: `0 issues.` | pass |
| C7.7 | `bash scripts/check.sh` lint row (G602 no longer excluded, 0 findings) | pass |
| C7.8 | the seven entries are gone from `.golangci.yml` | pass |

## Gate results
`bash scripts/check.sh`: ok on every row (format, tidy, vet, lint, debt shrink, deadcode, build, test).

## Decisions
None new door. Globals: kept as annotated exceptions (see Deviations).

## Deviations from plan
- The 5 endpoint globals were not turned into constants/parameters: `internal/fetch/gitrefs_test.go` and `selfupdate_test.go` reassign them, and neither test file is in this stage's list. Used `//nolint:gochecknoglobals` with a reason instead (the Exceptions door).
- forcetypeassert, gocritic and gosec showed 0 under `lint-only.sh` before any edit (forcetypeassert had no remaining sites; the others were hidden by the config's exclusions). Flipping G602/badCall exposed 3+1 real sites, fixed.
- `noctx` has no caller context available, so `context.Background()` is created at the entry point; signatures of tested functions unchanged.

## Tried and abandoned
None.

## Noticed, not touched
- Making the endpoints Client fields (and `releasesURL` a parameter) would remove the 5 nolints; needs edits to `gitrefs_test.go` and `selfupdate_test.go`.

## Surprises / notes
- `lint-only.sh gosec` does not show excluded check ids; flip first to see them.
