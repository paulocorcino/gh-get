# gh-get

Download a **single folder** from a GitHub repository — no `git`, no `gh`, no
account required (for public repos). The folder remembers where it came from, so
you can re-pull it later with `gh-get update`.

Originally a Bash script, now a cross-platform Go binary (Windows, Linux, macOS).

## Usage

```sh
gh-get <github-folder-url> [destination] [--force] [--ref REF] [--token TOKEN]
gh-get update [--ref REF]
gh-get --version | --help
```

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

v1: full CLI with unit tests. Deferred: CI cross-compile release workflow,
winget (portable) manifest, and a LICENSE file. See `CONTEXT.md` for the full
design decisions.
