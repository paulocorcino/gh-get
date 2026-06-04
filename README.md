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
amd64 & arm64). Self-installs onto your `PATH` with `gh-get --install`, no admin
needed.

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

## Usage

```sh
gh-get <github-folder-url> [destination] [--force] [--ref REF] [--token TOKEN]
gh-get update [--ref REF]
gh-get --install
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
| Linux   | `~/.local/bin/` | already on PATH on most distros |
| macOS   | `~/.local/bin/` | added manually if not present (instructions printed) |

Open a new terminal afterwards, then run `gh-get --version` from anywhere.

### Examples

```sh
# Download into ./handoff
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff

# Custom destination, overwrite if it exists
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff ./handoff --force

# Later, from inside the folder, re-pull the latest content
cd ./handoff
gh-get update
```

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

- **Minimal download (default):** lists the folder via the GitHub Trees API and
  fetches each file from `raw.githubusercontent.com` — only that folder's files.
- **Tarball fallback:** for large folders (> 40 files), truncated trees, or when
  the anonymous rate limit is hit, it downloads the repo tarball in one request
  and extracts only the target folder.
- **`update`:** stores the commit SHA in `.gh-get-source`; on update it skips the
  download when nothing changed, otherwise overwrites the folder (no merge — the
  folder is treated as installed content).

## Authentication

Optional. A token raises the rate limit (60 → 5000 req/h) and enables private
repos. Resolution order: `--token` flag → `$GITHUB_TOKEN` → `$GH_TOKEN`.

## Build

```sh
go build -o gh-get .
# with version stamping:
go build -ldflags "-X main.version=v1.0.0" -o gh-get .
```

## Status

v0.1.0: full CLI with unit tests, self-install, and a CI cross-compile release
workflow publishing binaries for all six platform/arch targets. Deferred: winget
(portable) manifest and a Homebrew tap. See `CONTEXT.md` for the full design
decisions.

## License

MIT — see [LICENSE](LICENSE).

---

<sub>Keywords: download github folder, download single folder from github, github
folder downloader, download subdirectory from github, get folder from github
without git, github sparse checkout alternative, no-clone github download, fetch
github directory, github raw folder download, agent skills, AI agent skill
installer, Claude Code skills, MCP server downloader, cross-platform Go CLI,
Windows Linux macOS.</sub>
