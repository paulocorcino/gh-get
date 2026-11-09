# Stage 3 — Surface fixes by hand — Post-stage report

**Backlog items:** nonamedreturns 5, thelper 2, revive:unused-parameter 2, errname 1, embeddedstructfieldcheck 1, containedctx 1
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- internal/fetch/membership.go: unnamed results in folderMember.
- internal/fetch/trees.go: unnamed results in listFolder, explicit local `entries`.
- internal/fetch/fetch.go: errFolderNotFound renamed folderNotFoundError (only uses were in this file).
- internal/fetch/client.go: stallBody: blank line after embedded field; ctx field replaced by `cause func() error` closure over context.Cause(ctx) (Read has a fixed signature, so ctx cannot be a parameter; newStallBody callers unchanged).
- internal/fetch/client_test.go: unused `r` renamed `_`.
- internal/ghurl/url_test.go: t.Helper() first in noDefault and noCheck; unused `prefix` renamed `_`.
- .golangci.yml: five linters and their debt comments removed; revive left listed.
- docs/lint-transition.md: six Progress lines.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C3.1-C3.5 | `bash scripts/lint-only.sh <linter>` for each | pass (0 issues) |
| C3.6 | `bash scripts/lint-only.sh revive ./internal/...` | pass |
| C3.7 | `python docs/plans/lint-debt-paydown-verify-stage-3.py` | pass |

## Gate results
`bash scripts/check.sh`: ok on every row (format, tidy, vet, lint, debt shrink, deadcode, build, test).

## Decisions
None.

## Deviations from plan
containedctx fixed with a closure field rather than a ctx parameter, because the method is io.Reader.Read.

## Tried and abandoned
None.

## Noticed, not touched
The revive debt comment still says 4 sites; 2 remain (argument-limit). Left per "do not touch other counts".

## Surprises / notes
None.
