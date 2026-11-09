# Stage 6 — Output through writers (forbidigo) — Post-stage report

**Backlog items:** D2; forbidigo 79
**Commit:** _filled by parent_
**Plan:** lint-debt-paydown.md

## Files changed
- `main.go` — `main` calls `run(os.Args[1:], os.Stdout, os.Stderr)` and keeps the one `os.Exit(1) //nolint:forbidigo // process exit code; main returns nothing`. `run(args []string, stdout, stderr io.Writer) error` passes the writers down. `runDownload(req downloadRequest, stdout, stderr)` takes a new `downloadRequest{URL, Dest, Force, Token, Ref}`. Also changed: `runUpdate(token, ref, stdout, stderr)`, `runRecursiveUpdate(token, stdout, stderr)` and `updateAt(client, dir, ref, stdout io.Writer, warnf func(string))`, where the `verbose bool` became a writer (the recursive caller passes `io.Discard`). New helpers: `warnTo(w) func(string)` (the fetch client's notices go to `stderr`), plus `outf`/`outln`. The 32 forbidigo sites (31 prints and 1 `os.Exit`) were rewritten with the same text.
- `install.go` — `runInstall(modifyPath, stdout)`, `ensureOnPath(dir, modifyPath, stdout)`; 16 sites.
- `list.go` — `runList(req listRequest, stdout, stderr)`, with a new `listRequest{Path, Token, Refresh, Force}`; the client warns via `warnTo(stderr)`; 12 sites.
- `freeze.go` — `runFreeze(listPath, stdout)`; 16 sites.
- `selfupdate.go` — `runSelfUpdate(force, stdout)`; `selfUpdate(exe, current, asset string, force bool, stdout io.Writer)`. The caller now computes the asset name (`releaseAsset(runtime.GOOS, runtime.GOARCH)`), which keeps the function at 5 arguments. 3 sites.
- `main_test.go`, `freeze_test.go`, `selfupdate_test.go` — call sites pass `new(bytes.Buffer)` (and `releaseAsset("linux", "amd64")` where `goos, goarch` used to go); no assertion changed.
- `install_test.go`, `list_test.go` — no change needed: they call no function whose signature changed.
- `.golangci.yml` — `forbidigo` and its `# debt: 79 sites` comment removed from `disable`.
- `AGENTS.md` — the line: output goes through the writer passed to `run` (`stdout`, `stderr`), never `fmt.Print*`; only `main` exits.
- `docs/lint-transition.md` — D2 marked delivered; Progress line.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C6.1 | `bash scripts/lint-only.sh forbidigo` → `0 issues.` | pass |
| C6.2 | `forbidigo` entry gone from `.golangci.yml` disable list (grep `^\s+- (forbidigo)$` → 0) | pass |
| C6.3 | grep `os\.Exit` in install.go, list.go, freeze.go, selfupdate.go, internal/ → 0 | pass |
| C6.4 | grep `fmt\.Print` in the five production files → 0 | pass |
| C6.5 | grep `writer passed to` in AGENTS.md → 1 | pass |

## Gate results
`bash scripts/check.sh`: ok on format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test (`-race -shuffle=on`). go.mod/go.sum untouched. Smoke test: `go run . --version` prints `gh-get dev`; an unknown option prints `Error: unknown option: --bogus` and exits 1.

## Decisions
- **Terminal output helpers** — shape: `outf(w io.Writer, format string, a ...any)` / `outln(w io.Writer, a ...any)` in `main.go`. Each wraps `fmt.Fprintf`/`fmt.Fprintln` and carries the one `//nolint:errcheck // terminal output, like printing to os.Stdout: ...`. Reason: once the output goes to an injected `io.Writer`, errcheck flags all 79 `fmt.Fprint*` calls. Its default exclusions only cover `fmt.Print*` and writes to `os.Stdout`/`os.Stderr`, so those errors were ignored before this stage. Rejected: (a) returning write errors, because a closed pipe would change exit codes (violates the behavior-preserving invariant); (b) errcheck `exclude-functions` for `fmt.Fprint*`, because it changes `.golangci.yml` lines this stage does not name and would hide every unchecked `Fprint*` in the repo, not only terminal output; (c) `_, _ =` at each site, because `check-blank: true` flags it.
- **Request structs for wide entry points** — shape: `downloadRequest` / `listRequest` value structs, then the writers: `runX(req xRequest, stdout, stderr io.Writer)`. Rejected: adding `stdout, stderr` as two more arguments, which would push `runDownload` to 7 and `runList` to 6. That breaks revive `argument-limit` 5 and adds sites Stage 9 does not plan for (it expects 2). The revive site count is unchanged at 2.

## Deviations from plan
- Order of operations item 2 says "fmt.Fprint*(w, ...) replaces each print". The sites call `outf`/`outln` (thin `fmt.Fprint*` wrappers) instead, for the errcheck reason above. The text written is byte-identical.
- `selfUpdate`'s `goos, goarch` parameters became one `asset` (the caller computes `releaseAsset`). Tests follow the new signature. `TestSelfUpdate_UnknownPlatform` now passes `releaseAsset("plan9", "mips")`, the same value as before.
- `updateAt`'s `verbose bool` became `stdout io.Writer` (`io.Discard` for quiet output). The test passes a buffer and a nil `warnf`: it fails before any download, as it did when it passed a nil warn to `fetch.New`.

## Tried and abandoned
- A Python heredoc to apply the edits in bulk: Git Bash mangled the `\n` escapes and the script's assertion stopped it before any write. I made the edits with the Edit tool instead.
- Plain `fmt.Fprint*(stdout, ...)` at each site (the plan's literal recipe): errcheck failed with 79 issues; replaced by the helpers above.

## Noticed, not touched
- `warn(msg)` still writes to `os.Stderr` directly (now through `warnTo(os.Stderr)`). It is used by the leaf cleanup helpers `closeOrWarn` and `removeOrWarn`, by `latestRelease`'s body close, by `replaceExecutable`'s restore, and by `syncListEntry`'s `Materialize` call. Threading `stderr` there would push `selfUpdate` (already 5) and `syncListEntry` (already 7) past the argument limit. Candidate for Stage 9: once `syncListEntry` takes an options struct, it can carry the `warnf`.
- `main`'s own `fmt.Fprintln(os.Stderr, "Error: ...")` stays (not a forbidigo site; `main` owns the os streams).

## Surprises / notes
- Moving output to injected writers trips errcheck by design (see Decisions). Later stages that add output should use `outf`/`outln`, not `fmt.Fprint*`.
- The `revive` debt comment says 4 sites; the linter reports 2 (`freeze_test.go:15`, `list.go:167`). This was stale before this stage; left as is, since this stage may not change other debt counts.
