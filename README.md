# gh-get — download a single GitHub folder (no git, no clone)

[![CI](https://github.com/paulocorcino/gh-get/actions/workflows/ci.yml/badge.svg)](https://github.com/paulocorcino/gh-get/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/paulocorcino/gh-get?sort=semver)](https://github.com/paulocorcino/gh-get/releases/latest)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-windows%20%7C%20linux%20%7C%20macos-lightgrey)](#install)

**Grab one folder out of any GitHub repo without cloning the whole thing.** No
`git`, no `gh` CLI, no account required for public repos. The folder remembers
where it came from, so you can re-pull the latest content later with
`gh-get update`.

A single, dependency-free, cross-platform Go binary (Windows, Linux, macOS —
amd64 & arm64). Self-installs onto your `PATH` with `gh-get --install` and
updates itself with `gh-get --self-update`, no admin needed.

```sh
gh-get https://github.com/owner/repo/tree/main/path/to/folder
```

### Why?

`git clone` pulls the entire history of an entire repository when all you wanted
was one directory. `gh-get` fetches **just that folder** — minimally, over the
GitHub API + `raw.githubusercontent.com`, with a tarball fallback for big ones.

**Built for AI coding agents.** Agent runtimes increasingly ship reusable
**skills**, **prompts**, **MCP servers**, and **tool** folders inside larger
monorepos. `gh-get` lets an agent (or you) pull a single skill folder into a
workspace in one command — no clone, no submodules, no token for public repos —
and `gh-get update` re-syncs it when upstream changes. Handy for **Claude Code**,
**Cursor**, **OpenClaw / OpenCode**, and any agent that loads skills or tools
from GitHub.

```sh
# Drop a single "handoff" skill folder into your agent's skills directory
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff
```

## What's new in v0.2.0

- **Far fewer API calls.** Deep URLs resolve their branch/tag with a fixed
  number of requests instead of one per path segment, and requests are reused
  across a run. A list file whose folders are all present now costs **zero**
  requests; `update` on an up-to-date list costs one per repo/ref.
- **Keeps working past the rate limit.** When the anonymous API limit
  (60 requests/hour) is hit, gh-get switches to endpoints that don't count
  against it and finishes the job (see [Rate limits](#rate-limits)).
- **`gh-get --self-update`** downloads the latest release, verifies its
  checksum and replaces itself.
- **`--install` puts gh-get on your `PATH` on Linux and macOS too**, by adding
  a line to your shell profile (`--no-modify-path` to opt out).
- **Large downloads no longer time out** on slow connections: only a transfer
  that stops receiving data for 60s is aborted.

Upgrading from v0.1.x: download v0.2.0 from the
[Releases](https://github.com/paulocorcino/gh-get/releases/latest) page and run
`--install` once; from then on, `gh-get --self-update` keeps it current.

## Usage

```sh
gh-get <github-folder-url> [destination] [--force] [--ref REF] [--token TOKEN]
gh-get -F FILE [--force]
gh-get update [--ref REF]
gh-get update -r | --recursive
gh-get update -F FILE
gh-get freeze [-F FILE]
gh-get --install [--no-modify-path]
gh-get --self-update [--force]
gh-get --version | --help
```

## Install

Download the binary for your platform from the
[Releases](https://github.com/paulocorcino/gh-get/releases) page, then let it
install itself onto your `PATH` — **no admin/root required**:

```sh
# from wherever you unpacked it
./gh-get --install        # Linux/macOS
.\gh-get.exe --install    # Windows (PowerShell)
```

This copies the binary to a per-user directory and ensures it's on your `PATH`:

| Platform | Installed to | PATH |
| --- | --- | --- |
| Windows | `%LOCALAPPDATA%\Programs\gh-get\` | added to your **user** PATH automatically |
| Linux   | `~/.local/bin/` | added to your shell profile automatically |
| macOS   | `~/.local/bin/` | added to your shell profile automatically |

On Linux/macOS the line goes into the profile of your shell (`~/.zshrc`,
`~/.bashrc` — `~/.bash_profile` on macOS —, fish's `conf.d/gh-get.fish`, or
`~/.profile`), marked so re-running `--install` never duplicates it. Prefer to
edit it yourself? Use `gh-get --install --no-modify-path` to only print the line.

Open a new terminal afterwards, then run `gh-get --version` from anywhere.

### Updating gh-get itself

```sh
gh-get --self-update
```

Downloads the latest release for your platform, verifies it against the
release's `checksums.txt`, and replaces the running binary — no admin/root
needed when gh-get was installed with `--install`. It does nothing when you
already have the latest version (`--force` reinstalls it, and is required to
replace a local `dev` build). It does not use the GitHub API, so it works even
when you are rate-limited. Available from v0.2.0.

### Examples

```sh
# Download into ./handoff
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff

# Custom destination, overwrite if it exists
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff ./handoff --force

# Download into the current directory (in place); existing files with the
# same name are overwritten, everything else is left untouched
gh-get https://github.com/OWNER/REPO .

# Later, from inside the folder, re-pull the latest content
cd ./handoff
gh-get update

# Update every gh-get folder under the current directory
cd ./skills
gh-get update -r
```

Recursive update scans the current directory and all of its descendants for
`.gh-get-source` files. It updates every independent installation, continues
when one fails, and prints a summary. Directory symlinks are not followed. If a
managed folder contains another managed folder, the parent is skipped so its
replacement update cannot erase the nested installation. `--recursive` cannot
be combined with `--ref` or `--branch`.

When the destination is `.` (or any path resolving to the current directory),
gh-get writes the files **in place** instead of creating a subfolder, and never
deletes the directory. As a safeguard, `--force` refuses to overwrite a
destination that contains the current working directory (e.g. `..`).

### Downloading from a list file

Like pip's `requirements.txt`, a list file (by convention `gh-get.txt`) declares
several folders at once — one GitHub URL per line, optionally followed by a
destination. Relative destinations resolve against the list file's directory;
`#` starts a comment.

```text
# gh-get.txt
https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff
https://github.com/mattpocock/skills/tree/main/skills/engineering/tdd  skills/tdd
https://github.com/OWNER/REPO/tree/v2.0.0/docs  vendor/docs   # pinned to a tag
```

```sh
# Download every entry that is not there yet (existing ones are left alone)
gh-get -F gh-get.txt

# Update every entry (also downloads entries added to the list since)
gh-get update -F gh-get.txt

# Same, using ./gh-get.txt automatically
gh-get update
```

Plain `gh-get update` uses `./gh-get.txt` only when the current directory is not
itself a gh-get folder. The list is the source of truth for the ref: changing
`tree/main` to `tree/v2.0.0` and running `update` switches that folder. A
destination that already holds something else (a non-gh-get folder or a
different source) fails unless `--force` is given; destinations that overlap
another entry, or contain the list file or the current directory, are refused.
Entries are processed one by one; failures are reported and the command exits
non-zero at the end. `--file` cannot be combined with `--ref` or `--recursive`.

Entries already downloaded are recognized locally from their `.gh-get-source`,
so `gh-get -F` makes no network request for them and `gh-get update` only checks
whether their ref moved. Several entries from the same repo share lookups.

#### Generating the list from what is already downloaded

Like `pip freeze`, `gh-get freeze` scans the current directory and its
subfolders for gh-get folders and creates `gh-get.txt` (or the file given with
`-F`), or updates it if it already exists:

```sh
cd ~/my-skills
gh-get freeze            # writes/updates ./gh-get.txt
```

- Folders not yet listed are appended (`[added]`); the destination is omitted
  when it matches the default name.
- A listed folder whose installed ref differs (e.g. after `update --ref`) has its
  URL rewritten in place (`[changed]`), keeping its destination and comment.
- Comments, order and entries for folders that are not downloaded are kept.
- A gh-get folder that contains another gh-get folder is skipped, as in
  `update -r`.

### Choosing a branch, tag or commit

The ref normally comes from the URL (`tree/<ref>/...`). Use `--ref` (alias
`--branch`) to override it, or to switch an already-downloaded folder:

```sh
# Same folder, but from the dev branch instead of what the URL says
gh-get https://github.com/owner/repo/tree/main/skills/handoff --ref dev

# Switch an installed folder to a tag (rewrites .gh-get-source)
cd ./handoff
gh-get update --ref v2.0.0
```

URLs may use `tree` or `blob`, branch names containing `/` are resolved
automatically, and pointing at the repo root downloads the **whole repo**:

```
https://github.com/OWNER/REPO                       (whole repo, default branch)
https://github.com/OWNER/REPO/tree/BRANCH           (whole repo at BRANCH)
https://github.com/OWNER/REPO/tree/BRANCH/PATH/TO/FOLDER
https://github.com/OWNER/REPO/blob/feature/x/PATH/TO/FOLDER
```

## How it works

- **Ref resolution:** a URL like `tree/feature/x/docs` is split into ref
  (`feature/x`) and folder (`docs`) with a single listing of matching branches
  and tags, however deep the path is.
- **Minimal download (default):** lists the folder via the GitHub Trees API and
  fetches each file from `raw.githubusercontent.com` — only that folder's files.
- **Tarball fallback:** for large folders (> 40 files), truncated trees, or when
  the anonymous rate limit is hit, it downloads the repo tarball in one request
  (from `codeload.github.com` when anonymous) and extracts only the target
  folder.
- **`update`:** stores the commit SHA in `.gh-get-source`; on update it skips the
  download when nothing changed, otherwise overwrites the folder (no merge — the
  folder is treated as installed content).
- **Recursive update:** `gh-get update -r` finds managed folders at or below the
  current directory, updates non-overlapping installations, and reports updated,
  current, skipped, and failed counts.
- **List file:** `gh-get -F gh-get.txt` / `gh-get update` sync every entry of a
  requirements.txt-style list; each folder still gets its own `.gh-get-source`.

## Authentication

Optional. A token raises the rate limit (60 → 5000 req/h) and enables private
repos. Resolution order: `--token` flag → `$GITHUB_TOKEN` → `$GH_TOKEN` →
`gh auth token` (if the GitHub CLI is installed and logged in). The last step
means that if you already ran `gh auth login`, gh-get picks up that token
automatically — no env var needed.

A token found this way is always sent, so a stale `$GITHUB_TOKEN` / `$GH_TOKEN`
makes requests fail (HTTP 401) even for public repos — unset or refresh it.

### Rate limits

Without a token the GitHub API allows 60 requests per hour per IP. gh-get spends
few of them (typically 2–3 per new folder, none for folders already present),
and when the limit is reached it prints one warning and continues without the
API for the rest of the run:

- branches, tags and the default branch are read from the git smart-HTTP
  endpoint (`github.com/OWNER/REPO.git/info/refs`, what `git ls-remote` uses —
  no git needed);
- content comes from the repo tarball on `codeload.github.com`.

Neither counts against the API limit. The trade-off is that the whole repo
tarball is downloaded instead of just the folder's files. Behind a corporate
proxy, set `HTTPS_PROXY` as usual.

## Build

```sh
go build -o gh-get .
# with version stamping:
go build -ldflags "-X main.version=v1.0.0" -o gh-get .
```

## Status

v0.2.0: full CLI with unit tests, list files and `freeze`, recursive update,
self-install with automatic `PATH` on every OS, self-update, a rate-limit-proof
fallback, and a CI cross-compile release workflow publishing binaries for all
six platform/arch targets. Deferred: winget (portable) manifest and a Homebrew
tap. See `CONTEXT.md` for the full design decisions and the
[releases](https://github.com/paulocorcino/gh-get/releases) for the changelog.

## License

MIT — see [LICENSE](LICENSE).

---

<sub>Keywords: download github folder, download single folder from github, github
folder downloader, download subdirectory from github, get folder from github
without git, github sparse checkout alternative, no-clone github download, fetch
github directory, github raw folder download, agent skills, AI agent skill
installer, Claude Code skills, MCP server downloader, cross-platform Go CLI,
Windows Linux macOS.</sub>
