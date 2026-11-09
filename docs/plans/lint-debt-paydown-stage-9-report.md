# Stage 9 — Shape: root package complexity — Post-stage report

**Backlog items:** gocognit root package (ceiling 35, root at 10), nestif 2, revive:argument-limit 2, main.go size
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `main.go` — now entry, usage, sentinels, `run` and argument parsing only (417 non-blank lines, was 568+). `run` (62) splits into `parseInstallArgs`, `parseSelfUpdateArgs`, `parseCommandLine` (a `commandLine` struct with `valueOf`/`apply`/`applyInline`, `missingValue`), one check per command (`freezeListFile`, `checkUpdate`, `listRequest`, `downloadRequest`), and `dispatch` / `runUpdateCommand`. New `const optToken = "--token"` (goconst). `displayPath` moved next to the output helpers.
- `cmd_download.go` — NEW: `downloadRequest`, `runDownload` (17 -> split into `resolveDownload`, `defaultDownloadName`, `planDownload`, `printDownloadHeader`), `metaFrom`.
- `cmd_update.go` — NEW: `runUpdate`, `updateAt` (13; marker reading moved to `readUpdateMarker`), recursive discovery (`discoverRecursive` 22 -> `findMarkers`, `sortDeepestFirst`, `containsAnother`), `pathDepth`, `isDescendant`, `updateAll`, `runRecursiveUpdate`.
- `freeze.go` — `mergeFreeze` (16) -> `matchEntry`, `freezeLine`; `runFreeze` (26) -> `readFreezeList` (+ `freezeList` type), `scanInstallations`, `reportFreeze` (+ `freezeTally`), `checkMarker` (also used by `readUpdateMarker`).
- `list.go` — `syncListEntry` takes a `listSync` options struct (`Client, BaseDir, Cwd, Refresh, Force`) plus entry and claimed (argument-limit); split into `resolveListEntry`, `checkListDest`. `runList` (13) tallies through `listTally.record` / `summarize`.
- `install.go` — `ensureOnPath` (14, nestif) -> `ensureOnWindowsPath`, `ensureInProfile`.
- `selfupdate.go` — `replaceExecutable` (14, nestif) -> `stageBinary`, `swapRunningBinary`; `selfUpdate` (12) -> `fetchVerifiedAsset`.
- `freeze_test.go` — `inst(base, rel string, m meta.Meta)` (argument-limit); `TestRunFreeze_CreatesThenKeepsListFile` (11) uses the existing `writeMeta` and `mustRead` helpers. No assertion changed.
- `list_test.go` — one call follows the new `syncListEntry` signature (see Deviations).
- `.golangci.yml` — `gocognit`, `revive`, `nestif` and their `# debt:` comments removed from `disable`; `gocognit.min-complexity: 35` with the comment line `# ceiling in steps: 35, then 10 (docs/lint-transition.md, Phase 5)`.
- `docs/lint-transition.md` — one Progress line.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C9.1 | `bash scripts/lint-only.sh gocognit .` -> `0 issues.` | pass |
| C9.2 | `bash scripts/lint-only.sh nestif` -> `0 issues.` | pass |
| C9.3 | `bash scripts/lint-only.sh revive` -> `0 issues.` | pass |
| C9.4 | gocognit on `.` with `min-complexity: 1`: highest root function is 10 (`findMarkers`, `TestPlanListEntry`), none above 10 | pass |
| C9.5 | `grep -c . main.go` -> 417 (max 500) | pass |
| C9.6 | `.golangci.yml` has no `- gocognit`, `- nestif`, `- revive` line | pass |

Re-measure (Fix a shape step 3), root package functions above 10, low-limit run:
- before: 12 — `run` 62, `runFreeze` 26, `discoverRecursive` 22, `syncListEntry` 20, `runDownload` 17, `mergeFreeze` 16, `replaceExecutable` 14, `ensureOnPath` 14, `updateAt` 13, `runList` 13, `selfUpdate` 12, `TestRunFreeze_CreatesThenKeepsListFile` 11.
- after: 0 — top is 10 (`findMarkers`, `TestPlanListEntry`), then 9 (`updateAt`, `selfUpdate`, `runRecursiveUpdate`, `runInstall`, `installedSource`, `dispatch`, `addToProfile`).

## Gate results
`bash scripts/check.sh`: every row `ok` (format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test with `-race -shuffle=on`). go.mod/go.sum untouched.

## Decisions
None new. `cmd_<command>.go` follows the plan's Cmd files door; only download and update got cmd files, since freeze, list, install and self-update already had their own files (`freeze.go`, `list.go`, `install.go`, `selfupdate.go`). All argument parsing for every command stays in `main.go`, apart from execution.

## Deviations from plan
- `list_test.go` is outside the declared file list but changed by one line: the plan's options struct for `syncListEntry` (step 4) changes its signature, and `TestSyncListEntry_PresentEntryNeedsNoNetwork` calls it. The call now passes `listSync{Client: fetch.New("", nil), BaseDir: base, Cwd: t.TempDir()}` with the same values; nothing in the test's assertions changed. The Global conventions allow this ("a test may change only to follow a signature"). The file list just did not include it.
- The ceiling comment line sits above `gocognit:` instead of between `gocognit:` and `min-complexity:`. The verify script rewrites the value with the regex `gocognit:\s*\n\s*min-complexity:`, which would not match with a comment line between them, so C9.4 would have run at 35.
- The brief counted nestif at 2; the measure found 3 (`install.go`, `main.go`, `selfupdate.go`). All 3 are fixed. `selfupdate.go` was in the file list.

## Tried and abandoned
- A first `readInstallation(dir)` helper in `freeze.go` returned `meta.Read`'s error unwrapped (wrapcheck). Wrapping it would have changed the printed `[failed]` line, so it became `checkMarker(m)` with `meta.Read` called inline.

## Noticed, not touched
- `internal/fetch/tarball.go:22` (gocognit 33) and the other internal sites above 10 are why the ceiling is 35. They belong to Stage 10.

## Surprises / notes
- `listTally` holds `Refresh` so `record` stays at 5 arguments under revive's argument-limit.
