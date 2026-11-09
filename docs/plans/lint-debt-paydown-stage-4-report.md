# Stage 4 - Dropped values in internal packages - Post-stage report

**Backlog items:** errcheck, wrapcheck, err113 in ./internal/...; errorlint 2
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `internal/fetch/client.go` - sentinels `errNotFound`, `errUnexpectedStatus`; `(*Client).closeBody` (close on read path, failure goes to `warn`); `isRateLimit` uses `errors.As`; transport type assertion checked; request/Do/ReadAll/Close errors wrapped.
- `internal/fetch/fetch.go` - MkdirTemp/MkdirAll wrapped; temp-dir cleanup failure goes to `warn`; `os.IsNotExist` became `errors.Is(err, fs.ErrNotExist)` (the error is now wrapped).
- `internal/fetch/gitrefs.go`, `ref.go`, `repo.go`, `trees.go`, `tarball.go`, `write.go` - sentinels (`errTruncated`, `errBadPktLength`, `errNoDefaultBranch`, `errTarballRequest`, `errFolderNotFound`, `errUnsafePath`, `errNotFoundBare`), wrapping at each external call, `io.EOF` compared with `errors.Is`.
- `internal/ghurl/url.go` - six sentinels; printed text unchanged.
- `internal/manifest/manifest.go`, `internal/meta/meta.go` - wrapped errors; file close failure now returned (meta parsing split into `parse`).
- `internal/fetch/client_test.go`, `gitrefs_test.go`, `internal/ghurl/url_test.go` - write/close/flush results checked (helpers `write`, `flush`, `closeBody`; `Parse` errors fatal).
- `.golangci.yml` (errorlint removed), `docs/lint-transition.md` (Progress line).

## Checks
| Check | Proof run | Result |
|---|---|---|
| C4.1 | `bash scripts/lint-only.sh errcheck ./internal/...` | pass (0 issues) |
| C4.2 | `bash scripts/lint-only.sh wrapcheck ./internal/...` | pass (0 issues) |
| C4.3 | `bash scripts/lint-only.sh err113 ./internal/...` | pass (0 issues) |
| C4.4 | `bash scripts/lint-only.sh errorlint` | pass (0 issues) |
| C4.5 | verify script (errorlint gone from .golangci.yml) | pass |

## Gate results
`bash scripts/check.sh`: ok on every row (format, tidy, vet, lint, debt, deadcode, build, test). go.mod/go.sum unchanged.

## Decisions
None new. Sentinels follow the plan's door; each keeps the old printed text (sentinel text is a fragment of the message).

## Deviations from plan
- Failures that became visible: `Materialize` temp-dir cleanup and every response-body/gzip `Close` on read paths are reported through `warn` (not returned); `manifest.Load` and `meta.Read` now return a file `Close` failure.
- Wrapping adds context prefixes to some wrapped OS/JSON/IO errors (e.g. `writing <path>: ...`); sentinel messages are unchanged.
- Two nolints: `meta.Read` open error stays unwrapped (`wrapcheck`, root callers use `os.IsNotExist`, which does not unwrap; Stage 5 can revisit); `writeSymlink` `os.Remove(dest)` (`errcheck`, failure surfaces at the following Symlink/WriteFile).

## Tried and abandoned
Named returns with a deferred close-into-error helper: `nonamedreturns` forbids them. Replaced by `closeBody` (warn) and non-deferred close.

## Noticed, not touched
`folderNotFoundError` in fetch.go and `errFolderNotFound` in tarball.go carry the same message; could be unified. Root callers using `os.IsNotExist` on wrapped errors would break if `meta.Read` is wrapped later.

## Surprises / notes
`os.IsNotExist` does not unwrap: any further wrapping of file errors needs `errors.Is(err, fs.ErrNotExist)` at the callers.
