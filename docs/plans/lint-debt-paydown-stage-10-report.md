# Stage 10 — Shape: internal packages, close the debt list — Post-stage report

**Backlog items:** gocognit internal packages and tests; ceiling 35 -> 10; end of the debt list
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `internal/fetch/tarball.go` — `downloadViaTarball` (33) keeps the gzip/tar plumbing and the "folder not found" check; split into `requestTarball` (request + status, closes the body on a refused status), `extractFolder` (the `tar.Next` loop), `extractEntry`, `entryDest` (top-level strip, folder membership, `safeJoin`) and `writeTarEntry` (the switch over the entry type).
- `internal/fetch/gitrefs.go` — `parseAdvertisement` (26) is now the loop; `nextPktLine` does the pkt-line framing (a flush-pkt yields an empty payload, which adds nothing, as before), `addAdvertised` decodes one line, `headSymref` reads the HEAD symref from the capabilities.
- `internal/fetch/fetch.go` — `Materialize` (14): the mode switch and `MkdirAll` move to `prepareDest`.
- `internal/fetch/trees.go` — `downloadViaTrees` (14): fetching and writing one blob moves to `(*Client).writeBlob`.
- `internal/fetch/write.go` — `publishTree` (21): the walk callback becomes a `publisher` struct with `visit` (relative path, skip root) and `copyEntry` (dir / symlink / file). `pruneStale` (16): `staleTarget` (confinement), `removeStale` (delete one file), `removeEmptyParents` (the climb).
- `internal/ghurl/url.go` — `Parse` (12): `trimURL` and `refSegments`. `Resolve` (18): `resolveDefault` (bare repo URL) and `resolveProbed` (the per-prefix probe loop). `resolveListed` (13): `peeled` resolves an annotated tag to its commit. Error texts unchanged.
- `internal/fetch/gitrefs_test.go` — `TestRateLimitedRunAvoidsAPI` (13): the fake server moves to `rateLimitedServer`, the base override to `pointBasesAt` (restores via `t.Cleanup` instead of `defer`). Assertions unchanged.
- `internal/fetch/write_test.go` — `TestPruneStale_RemovesOnlyTrackedStaleFiles` (12): setup uses the existing `mustWrite` helper (same `MkdirAll 0o755` + `WriteFile 0o644`). Assertions unchanged.
- `internal/ghurl/url_test.go` — `TestParse` (18): the anonymous case struct is named `parseCase`; each case runs as subtest `tc.run`. Cases and assertions unchanged.
- `.golangci.yml` — `gocognit.min-complexity: 10`; the `# ceiling in steps` comment removed. No other line changed.
- `AGENTS.md` — the line pointing at the remaining debt removed.
- `docs/lint-transition.md` — final Progress line ending with "debt list empty"; the plan kept as history.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C10.1 | `bash scripts/lint-only.sh gocognit` -> `0 issues.` | pass |
| C10.2 | `.golangci.yml` has `min-complexity: 10` under `gocognit:`; grep `ceiling in steps` -> 0 | pass |
| C10.3 | grep `# debt:` in `.golangci.yml` -> 0 | pass |
| C10.4 | every `disable` entry sits under a `# noise:` line (22 entries) | pass |
| C10.5 | `//nolint:gocognit` in the repo: 0 (max 4) | pass |
| C10.6 | grep `remaining debt` in `AGENTS.md` -> 0 | pass |

Re-measure (Fix a shape step 3), functions above 10 repo-wide, low-limit run:
- before: 12, all internal — `downloadViaTarball` 33, `parseAdvertisement` 26, `publishTree` 21, `Resolve` 18, `TestParse` 18, `pruneStale` 16, `Materialize` 14, `downloadViaTrees` 14, `resolveListed` 13, `TestRateLimitedRunAvoidsAPI` 13, `Parse` 12, `TestPruneStale_RemovesOnlyTrackedStaleFiles` 12.
- after: 0 — top is 10 (`findMarkers`, `TestPlanListEntry`, `TestRateLimitedRunAvoidsAPI`, `(*Client).download`), then 9.

## Gate results
`bash scripts/check.sh`: every row `ok` (format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test with `-race -shuffle=on`). `go build ./...` passes. go.mod untouched; the repo has no go.sum.

## Decisions
None new.

## Deviations from plan
None. No `//nolint:gocognit` was needed.

## Tried and abandoned
- `extractFolder` with `entryDest` and `writeTarEntry` called inline in the loop scored 11; the two calls moved into `extractEntry`.
- `pruneStale` skipping on `err != nil` from `safeJoin` inside the loop, and then `removeStale` with the parent climb ending in `break` + `return nil`, both tripped `nilerr`. The fix: `staleTarget` returns a bool, and the climb is `removeEmptyParents`, which returns nothing. Behavior is the same: an escaping path is skipped and the climb stops at the first directory that is not empty.

## Noticed, not touched
None.

## Surprises / notes
- The scratch configs used for measuring (`docs/plans/logs/s10-g10.yml`, `s10-g1.yml`) are in the gitignored logs directory.
