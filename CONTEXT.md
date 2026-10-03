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
| `gh-get update -r` / `--recursive` | Update all non-overlapping gh-get folders at or below the current dir |
| `gh-get -F FILE` / `--file FILE` | Download every entry of a list file not yet present |
| `gh-get freeze [-F FILE]` | Scan the list file's dir for gh-get folders and create/update the list (default `gh-get.txt`) |
| `gh-get update -F FILE` | Update (and install missing) entries of a list file; plain `update` uses `./gh-get.txt` when the cwd has no `.gh-get-source` |
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
   - Anonymous tarballs come straight from `codeload.github.com`, which does
     not count against the API quota; with a token the API tarball endpoint is
     used (only it serves private repos).
   - **API-free fallback:** once the API rate-limits a run, the client stops
     calling it (one warning). Refs, tags and the default branch come from the
     git smart-HTTP advertisement (`github.com/O/R.git/info/refs?service=
     git-upload-pack`, what `git ls-remote` reads; no git binary needed) and
     content from the codeload tarball. A run therefore keeps working past the
     60/h anonymous limit, at the cost of whole-repo tarballs. Rotating proxies
     to dodge the limit is deliberately not supported (GitHub ToS); a corporate
     proxy works via `HTTPS_PROXY`.

4. **Authentication:** resolution order `--token` flag → `GITHUB_TOKEN` →
   `GH_TOKEN` → `gh auth token` (shells out to the GitHub CLI if present and
   logged in; 3s timeout, failure is silent → anonymous). Raises rate limit to
   5000/h and enables private repos. Fully optional — anonymous by default.

5. **`update` semantics:** store the resolved commit SHA in `.gh-get-source`.
   On update, compare: if unchanged → print "already up to date", no download.
   If changed → overwrite (same destructive behavior as today). **No merge**;
   the downloaded folder is treated as read-only / "installed" content.

5a. **Recursive update:** `update -r` / `--recursive` scans from the current
    directory without following directory symlinks, then updates installations
    sequentially and continues after individual failures. A managed parent that
    contains another managed folder is skipped so its replace operation cannot
    erase the child. The command returns non-zero if any scan or update fails.
    Recursive mode cannot be combined with `--ref` / `--branch`.

5b. **List file (`gh-get.txt`):** requirements.txt-style, one `URL [DEST]` per
    line, `#` comments (full-line or whitespace-preceded), DEST may contain
    spaces and resolves relative to the list file's directory (default: folder
    basename / repo name). All URLs are validated offline before any download.
    Download mode (`-F`) installs absent entries and leaves an existing
    same-source installation alone (`[present]`); update mode also re-pulls it
    when the entry's ref/commit differs, so the list is the source of truth for
    the ref. "Same source" = owner/repo/folder path, ref ignored. A destination
    holding a non-gh-get folder or a different source fails unless `--force`.
    Destinations overlapping another entry, or containing the list's directory
    or the cwd, are refused. Sequential, continue-on-failure, non-zero exit if
    any entry failed. An entry already installed at its destination is
    recognized offline from its marker (as `freeze` matches), so `-F` costs no
    request for it and `update -F` only re-checks the installed ref. The fetch
    client caches ref lookups and tree listings per run, so entries sharing a
    repo reuse them. `--file` excludes `--ref` and `--recursive`. Plain
    `gh-get update` falls back to `./gh-get.txt` only when the cwd is not itself
    managed. Parsing lives in `internal/manifest`; orchestration in `list.go`.

5c. **`freeze`:** pip-freeze analogue. Scans the list file's directory with the
    same discovery as `update -r` (nested parents skipped; the list's own dir is
    never an entry) and merges into the list without network: existing lines,
    comments, order and not-downloaded entries are preserved. Entries are
    matched to folders offline by explicit dest, else by either plausible
    default name (last URL segment or repo). A matched entry whose URL no longer
    describes the installed source has only its URL rewritten (bare repo URLs
    stay bare for whole-repo installs). Unlisted folders are appended with the
    canonical `tree/` URL, with the dest (slash-separated, relative) only when it
    differs from the default name. The file is written only if something changed.

6. **URL parsing:** accept `.../tree/...` and `.../blob/...`, plus the repo root
   (`github.com/OWNER/REPO`) and branch root (`.../tree/BRANCH`) to download the
   **whole repo** (empty folder path). Bare repo URLs resolve the default branch
   via the repos API. When the branch name contains `/` (e.g. `feature/x`),
   disambiguate by querying refs: one `git/matching-refs` listing (heads +
   tags) of refs sharing the first segment, longest match wins; a per-prefix
   probe is only the fallback for an incomplete listing. Keeps the request
   count fixed regardless of URL depth (anonymous limit: 60/h). Fixes the regex
   bug in the original script.

6a. **Destination `.` (in-place):** when the destination resolves to the current
   working directory, download is written **into** it (overwriting only colliding
   paths, never wiping the dir, no `--force` needed). For non-CWD destinations,
   `--force` still wipes+rewrites, but is **refused** when the target contains the
   CWD (e.g. `..`) to avoid deleting the directory the user is standing in.

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

8a. **HTTP timeouts:** no whole-request deadline (it would abort large
    tarballs on slow links). Headers must arrive within 30s and a body that
    delivers no data for 60s is aborted as stalled.

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
  Unix appends a marked, duplicate-guarded line to the profile of `$SHELL`
  (zsh → `$ZDOTDIR/.zshrc`; bash → `~/.bashrc`, `~/.bash_profile` on macOS;
  fish → `conf.d/gh-get.fish`; else `~/.profile`), skipped when the marker is
  already there; `--no-modify-path` only prints the line (rustup-style).
  Idempotent via `os.SameFile`. Tests in `install_test.go` cover the PATH helpers.
- `--self-update [--force]` (`selfupdate.go`): reads the latest tag from the
  `releases/latest` redirect (no REST API, no rate limit), downloads the raw
  `gh-get_<os>_<arch>` asset, verifies it against `checksums.txt`, and swaps it
  in via a sibling temp file + rename. Windows renames the running exe to
  `.old` (removed on a later run). Same version and `dev` builds are replaced
  only with `--force`. Works from the first release that contains it.
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
