# gh-get

Download a folder (or a whole repo) from GitHub. No git, no clone.

[![CI](https://github.com/paulocorcino/gh-get/actions/workflows/ci.yml/badge.svg)](https://github.com/paulocorcino/gh-get/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/paulocorcino/gh-get?sort=semver)](https://github.com/paulocorcino/gh-get/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Platforms](https://img.shields.io/badge/platforms-windows%20%7C%20linux%20%7C%20macos-lightgrey)](#install)

```sh
gh-get https://github.com/owner/repo/tree/main/path/to/folder
```

- One small binary for Windows, Linux and macOS (amd64, arm64).
- No account needed for public repos.
- The folder remembers its source, so `gh-get update` pulls the latest.
- Made for pulling agent skills, prompts and tools out of big repos.

## Install

Download the binary from [Releases](https://github.com/paulocorcino/gh-get/releases),
then run:

```sh
./gh-get --install        # Linux/macOS
.\gh-get.exe --install    # Windows (PowerShell)
```

It copies gh-get to a per-user folder and adds it to your `PATH`. No admin
needed. Open a new terminal and run `gh-get --version`.

| Platform | Installed to |
| --- | --- |
| Windows | `%LOCALAPPDATA%\Programs\gh-get\` |
| Linux, macOS | `~/.local/bin/` |

On Linux and macOS the `PATH` line goes into your shell profile. Use
`--no-modify-path` to only print it.

Upgrade later with `gh-get --upgrade`. It verifies the checksum and replaces
the binary. Use `--force` to reinstall the same version.

## Usage

```sh
gh-get <url> [destination] [--force] [--ref REF] [--token TOKEN]
gh-get -F FILE|URL [--force]
gh-get update [--ref REF]
gh-get update -r | --recursive
gh-get update -F FILE|URL
gh-get freeze [-F FILE]
gh-get --install [--no-modify-path]
gh-get --upgrade [--force]
gh-get --version | --help
```

### Download a folder

```sh
# Into ./handoff
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff

# Into a chosen folder, overwriting it
gh-get https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff ./handoff --force

# A whole repo, into the current directory
gh-get https://github.com/OWNER/REPO .
```

URL formats (`tree` or `blob`, branch names with `/` work):

```text
https://github.com/OWNER/REPO                       whole repo, default branch
https://github.com/OWNER/REPO/tree/BRANCH           whole repo at BRANCH
https://github.com/OWNER/REPO/tree/BRANCH/PATH      a folder
```

With destination `.`, files are written in place and the directory is never
deleted. `--force` refuses a destination that contains the current directory.

### Update

```sh
cd ./handoff && gh-get update    # re-pull this folder
gh-get update -r                 # every gh-get folder at or below here
```

- Only files gh-get installed are replaced or removed. Your other files stay.
- Local edits to installed files are overwritten.
- `-r` skips a folder that contains another gh-get folder, and cannot be used
  with `--ref`.

### Branch, tag or commit

The ref comes from the URL. `--ref` (or `--branch`) overrides it:

```sh
gh-get https://github.com/owner/repo/tree/main/skills/handoff --ref dev
cd ./handoff && gh-get update --ref v2.0.0    # switch an installed folder
```

## List files

A list file (usually `gh-get.txt`) downloads many folders at once. One URL per
line, then an optional destination. `#` starts a comment.

```text
# gh-get.txt
https://github.com/mattpocock/skills/tree/main/skills/productivity/handoff
https://github.com/mattpocock/skills/tree/main/skills/engineering/tdd  skills/tdd
https://github.com/OWNER/REPO/tree/v2.0.0/docs  vendor/docs   # pinned to a tag
```

```sh
gh-get -F gh-get.txt           # download entries that are missing
gh-get update -F gh-get.txt    # also update the existing ones
gh-get update                  # same, using ./gh-get.txt
```

- Relative destinations are relative to the list file's folder.
- Changing `tree/main` to `tree/v2.0.0` and running `update` switches the ref.
- A destination holding something else fails unless you pass `--force`.
- One failed entry does not stop the others. The exit code is non-zero.
- Entries already downloaded cost no network request in `-F`.

### List in a GitHub repo

`-F` also takes the URL of a list file in a repo. The list is read into memory,
never saved. Destinations are relative to the current directory.

```sh
gh-get -F https://github.com/OWNER/REPO/blob/main/lists/tools.txt
```

`freeze` needs a local file, so it does not accept a URL.

### Generate the list

`gh-get freeze` scans the current directory for gh-get folders and creates or
updates `gh-get.txt` (like `pip freeze`).

- New folders are added.
- A changed ref rewrites the URL in place.
- Comments, order and unknown entries are kept.

## How it works

- **Small download:** lists the folder with the Trees API, then fetches each
  file from `raw.githubusercontent.com`.
- **Tarball fallback:** for folders over 40 files, or when rate-limited, it
  downloads the repo tarball once and extracts the folder.
- **Update:** the commit SHA is stored in `.gh-get-source`. If nothing changed,
  nothing is downloaded.

## Authentication

Optional. A token raises the API limit from 60 to 5000 requests per hour and
gives access to private repos. gh-get looks for it in this order:

1. `--token`
2. `$GITHUB_TOKEN`
3. `$GH_TOKEN`
4. `gh auth token` (GitHub CLI)

A stale token makes requests fail with HTTP 401, even for public repos. Unset
or refresh it.

### Rate limits

Without a token, GitHub allows 60 API requests per hour. gh-get uses 2-3 per
new folder. When the limit is hit, it warns once and continues without the API,
using the git ref endpoint and the tarball from `codeload.github.com`. The
whole repo tarball is downloaded instead of just the folder.

Behind a proxy, set `HTTPS_PROXY`.

## Build

```sh
go build -ldflags "-X main.version=v1.0.0" -o gh-get .
```

Design notes are in [CONTEXT.md](CONTEXT.md). Changes are on the
[releases](https://github.com/paulocorcino/gh-get/releases) page.

## License

MIT. See [LICENSE](LICENSE).
