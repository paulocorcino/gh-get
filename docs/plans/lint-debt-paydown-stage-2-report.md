# Stage 2 — Mechanical fixes by command — Post-stage report

**Backlog items:** phase 1 linters: perfsprint 15, modernize 4, intrange 1, canonicalheader 1, usetesting 2
**Commit:** _filled by parent in the End-to-end summary table_
**Plan:** lint-debt-paydown.md

## Files changed
- `install.go` — perfsprint (1): `fmt.Errorf` without verbs to `errors.New` (LOCALAPPDATA); `errors` import added.
- `list.go` — perfsprint (3): three `fmt.Errorf` without verbs to `errors.New`; `errors` import added.
- `main.go` — perfsprint (9): nine `fmt.Errorf` without verbs to `errors.New`; `errors` import added.
- `main_test.go` — usetesting (2): `TestRun_RecursiveNoInstallationsSucceeds` uses `t.Chdir(t.TempDir())` instead of `os.Getwd` + `os.Chdir` + cleanup.
- `internal/fetch/gitrefs.go` — perfsprint (1): "truncated pkt-line" to `errors.New`; modernize (1): `strings.Fields` range to `strings.FieldsSeq`; `errors` import added.
- `internal/fetch/client.go` — canonicalheader (1): `h.Set("X-Github-Api-Version", ...)`.
- `internal/fetch/client_test.go` — modernize (1) and intrange (1), same site: `for i := 0; i < 5; i++` to `for range 5`.
- `internal/fetch/membership.go` — modernize (1): `HasPrefix` + `TrimPrefix` to `strings.CutPrefix`.
- `internal/meta/meta.go` — modernize (1): `IndexByte` + slicing to `strings.Cut`.
- `internal/ghurl/url_test.go` — perfsprint (1): the `join` helper builds with a `strings.Builder` (body rewritten by hand, see Deviations); `strings` import added.
- `.golangci.yml` — flipped `perfsprint` (15), `modernize` (4), `intrange` (1), `usetesting` (2), `canonicalheader` (1): entries and their `# debt:` lines deleted. Nothing else changed.
- `docs/lint-transition.md` — five `## Progress` lines, one per flipped linter.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C2.1 | `bash scripts/lint-only.sh perfsprint` — `0 issues.` | pass |
| C2.2 | `bash scripts/lint-only.sh modernize` — `0 issues.` | pass |
| C2.3 | `bash scripts/lint-only.sh intrange` — `0 issues.` | pass |
| C2.4 | `bash scripts/lint-only.sh canonicalheader` — `0 issues.` | pass |
| C2.5 | `bash scripts/lint-only.sh usetesting` — `0 issues.` | pass |
| C2.6 | grep of `.golangci.yml` for `^\s+- (perfsprint\|modernize\|intrange\|canonicalheader\|usetesting)$` — 0 matches | pass |

The verify script `docs/plans/lint-debt-paydown-verify-stage-2.py` needs the stage commit to exist; it was not run before the commit. See Surprises.

## Gate results
- `bash scripts/check.sh`: every row `ok` (format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test with `-race -shuffle=on`).
- `go build ./...` and `go vet ./...`: pass.
- `go test -race ./internal/fetch/` and `TestRun_RecursiveNoInstallationsSucceeds`: pass.
- `git diff -- go.mod go.sum`: empty (stdlib only).

## Decisions
None. No one-way door was closed in this stage. `errors.New` for constant messages and `t.Chdir` are the standard forms the linters name.

## Deviations from plan
- The `--fix` runs produced no import changes. `errors` (in `internal/fetch/gitrefs.go`, `install.go`, `list.go`, `main.go`) and `strings` (in `internal/ghurl/url_test.go`) were added by hand; the build failed until they were.
- `usetesting` has no safe autofix: `--fix` reported the two sites and exited 1. Fixed by hand with `t.Chdir`.
- `intrange` had 0 sites left after the `modernize` run (`rangeint` rewrote the same `for i := 0; i < 5` loop in `client_test.go`). Its `--fix` was a no-op.
- The `join` helper in `url_test.go`: the tool's output kept a `out += builder.String()` concatenation and a generated variable name (`outSb267`). Rewritten by hand as a single `strings.Builder`, same output.
- The `membership.go` variable is named `found` rather than the tool's `ok0`.
- The Flip procedure's `check.sh` step ran once after all five flips (the five flips were config-only edits, so one gate run covers them), not once after each flip.

## Tried and abandoned
- An `Edit` on `client.go` failed on indentation; the retry matched on the call text without leading whitespace. No code change was dropped.
- `canonicalheader`: before fixing, confirmed with a scratch program outside the repo that `http.Header.Set` canonicalizes both spellings to `X-Github-Api-Version`. The request is unchanged.

## Noticed, not touched
- `internal/ghurl/url_test.go` `join` re-implements `strings.Join(s, "/")`. Candidate for a cleanup; not changed here.

## Surprises / notes
- Future stages that use `golangci-lint run --fix` (the `perfsprint` form, in particular) do not fix imports. Run `go build ./...` straight after `--fix` before reading the diff.
- The verify script for this stage asserts a commit with subject `^lint\(stage 2\)`; it runs after the parent takes the commit.
