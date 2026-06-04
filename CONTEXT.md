# gh-get — Project Context

> Consolidated design decisions for porting the original Bash script into a
> cross-platform Go executable. This file is the source of truth; update it
> when a decision changes.

## Purpose

Pragmatic CLI to download **a single folder** from a GitHub repository —
without cloning the whole repo and cleaning up afterwards. Primary use case:
installing/updating standalone "skills" that live inside larger repos
(e.g. `mattpocock/skills/tree/main/skills/...`).

The folder remembers its origin (`.gh-get-source`) so it can be re-pulled
later via `gh-get update`.

## Commands (parity with the original Bash tool)

| Command | Action |
|---|---|
| `gh-get <github-folder-url> [dest] [--force]` | Download the folder pointed by a `tree/`/`blob/` URL |
| `gh-get update` | Re-pull the folder in the current dir (reads `.gh-get-source`) |
| `gh-get --version` / `-h`/`--help` | Version / usage |

## Consolidated decisions

1. **Language:** Go. Single static binary, no runtime, trivial cross-compile
   (`GOOS`/`GOARCH`) for windows/linux/mac (amd64 + arm64). Stdlib only.
   Go version: latest installed (no pinned floor yet).

2. **No git / no gh / no account dependency.** Uses GitHub HTTP API + raw
   content. Works anonymously for public repos.

3. **Download strategy — hybrid:**
   - Default: **Trees API** (`git/trees/{ref}?recursive=1`, one light request)
     → filter entries by folder path prefix → fetch each file from
     `raw.githubusercontent.com`. True minimal download.
   - **Fallback to tarball** (`codeload.../tar.gz/{ref}`, 1 request, extract
     only the folder) when the folder has **> ~40 files** OR on **HTTP 403**
     (rate limit). Balances minimal download vs. robustness.

4. **Authentication:** reads `GITHUB_TOKEN` / `GH_TOKEN` from env
   automatically; `--token` flag overrides. Raises rate limit to 5000/h and
   enables private repos. Fully optional — anonymous by default.

5. **`update` semantics:** store the resolved commit SHA in `.gh-get-source`.
   On update, compare: if unchanged → print "already up to date", no download.
   If changed → overwrite (same destructive behavior as today). **No merge**;
   the downloaded folder is treated as read-only / "installed" content.

6. **URL parsing:** accept `.../tree/...` and `.../blob/...`, plus the repo root
   (`github.com/OWNER/REPO`) and branch root (`.../tree/BRANCH`) to download the
   **whole repo** (empty folder path). Bare repo URLs resolve the default branch
   via the repos API. When the branch name contains `/` (e.g. `feature/x`),
   disambiguate by querying refs. Fixes the regex bug in the original script.

6b. **Ref override (`--ref` / `--branch`):** overrides the ref embedded in the
   URL on download (the URL's ref is still resolved to split the folder path).
   On `update`, `--ref` switches the folder to a different branch/tag/commit and
   rewrites `.gh-get-source` (branch + commit). Takes precedence over both the
   URL and the stored marker.

7. **File fidelity (cross-platform, never requires privilege):**
   - Exec bit: Trees mode `100755` → apply `+x` on Linux/Mac, ignored on
     Windows.
   - Symlinks: create if possible; otherwise write as a regular file and warn.
   - Never requires Developer Mode / admin.

8. **CLI messages:** English.

9. **Versioning:** injected at build via `-ldflags "-X main.version=..."`
   from the git tag. Local dev builds report `dev`.

## Repository / distribution

- **Dedicated public repo:** `github.com/paulocorcino/gh-get`.
- **Module path:** `github.com/paulocorcino/gh-get`.
- **Binary name:** `gh-get`.
- **Release tags:** plain SemVer — `vX.Y.Z`.
- **CI:** `ci.yml` builds + tests on push/PR; `release.yml` cross-compiles 6
  targets and publishes a GitHub Release on `v*` tags.
- **Install (later):** winget `portable` type → user scope, **no admin**,
  adds to user PATH. Linux/Mac via release tarball / `go install`.
- **License:** MIT.

## Proposed structure

```
gh-get/
├── go.mod                      # module github.com/paulocorcino/gh-get
├── main.go                     # arg parsing, routing (download | update | help | version)
├── internal/
│   ├── ghurl/url.go            # parse tree/blob URL, resolve branch-with-"/" via API
│   ├── meta/meta.go            # read/write .gh-get-source (incl. commit SHA)
│   └── fetch/
│       ├── fetch.go            # hybrid orchestration: trees → fallback tarball
│       ├── trees.go            # Trees API + raw.githubusercontent
│       └── tarball.go          # codeload tar.gz + extract folder only
└── README.md
```

## v1 scope (current build target)

Full CLI: hybrid download/update, URL parsing, meta with SHA, file-mode
handling, `--version`/`--help`, and unit tests on the pure packages
(`ghurl`, `meta`). Build & run on Windows. **CI cross-compile workflow and
winget manifest are deferred to a later milestone.**

## Done after v1

- Dedicated public repo `paulocorcino/gh-get` + MIT license.
- CI (`ci.yml`): build + vet + test on push/PR.
- Release (`release.yml`): cross-compile 6 targets → GitHub Release on `v*` tags.
  Publishes both versioned archives (`.zip`/`.tar.gz`) and raw,
  directly-downloadable binaries (`gh-get_<os>_<arch>[.exe]`) + `checksums.txt`.
- `--install` (`install.go`): self-copies the running binary into a per-user bin
  dir — `%LOCALAPPDATA%\Programs\gh-get` on Windows, `~/.local/bin` on Unix — and
  puts it on PATH with no admin/root. Windows edits the user PATH via PowerShell
  `[Environment]::SetEnvironmentVariable(...,'User')` (avoids `setx` truncation);
  Unix only prints the `export` line (never edits shell rc files). Idempotent via
  `os.SameFile`. Tests in `install_test.go` cover the pure PATH helpers.
- GitHub discoverability: repo description + topics set; README has badges, an
  AI-agent positioning section, and an SEO keyword footer.

## Release/versioning policy

- Bump the tag for every published change (`v0.1.1`, `v0.2.0`, …). Do **not**
  force-move an already-published tag. v0.1.0 was force-moved once (during
  bring-up, before wide distribution) to fold in `--install` + direct binaries;
  treat that as a one-off, not the pattern going forward.

## Deferred

- winget manifest (portable).
- Homebrew tap / `go install` docs.
- Pinned minimum Go version floor.
