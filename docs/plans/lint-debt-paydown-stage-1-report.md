# Stage 1 — Turn on depguard (D4) and the per-linter probe — Post-stage report

**Backlog items:** D4; helper `scripts/lint-only.sh`
**Commit:** _filled by parent in the End-to-end summary table_
**Plan:** lint-debt-paydown.md

## Files changed
- `scripts/lint-only.sh` — NEW, executable (mode 100755 in the index). Bash, `set -euo pipefail`, `cd` to the repo root, reads the golangci-lint pin from `scripts/check.sh`, runs `go run .../golangci-lint@<pin> run --enable-only <linter> <paths>`; paths default to `./...`.
- `.golangci.yml` — removed the `# noise: decision first: on with the rules of the layers (D4)` comment and the `- depguard` entry from `linters.disable`; added `linters.settings.depguard.rules.main` with `list-mode: strict`, `files: [$all]`, `allow: [$gostd, github.com/paulocorcino/gh-get]`.
- `docs/lint-transition.md` — D4 marked `(taken)` in Decisions; one Progress line added.

## Checks
| Check | Proof run | Result |
|---|---|---|
| C1.1 | `bash scripts/lint-only.sh depguard` — `0 issues.` | pass |
| C1.2 | verify script: no `- depguard` line in `.golangci.yml` (`grep -n "^\s\+- depguard$"` exits 1) | pass |
| C1.3 | `bash scripts/lint-only.sh godox ./internal/...` — `0 issues.` | pass |

## Gate results
- `bash scripts/check.sh`: all rows `ok` (format, go mod tidy, vet, lint, debt list only shrinks, deadcode, build, test) — exit 0.
- `go.mod` / `go.sum`: no diff against HEAD (stdlib-only invariant holds).

## Decisions
- D4 (depguard layer boundaries, from `docs/lint-transition.md`) — taken as the plan recommends: `list-mode: strict`, `files: [$all]`, `allow: [$gostd, github.com/paulocorcino/gh-get]`. Literal shape is the `settings.depguard.rules.main` block in `.golangci.yml`. Alternative rejected: leaving depguard in `disable` — it keeps layer boundaries unenforced at no benefit, since the module has no dependencies to hold off.

## Deviations from plan
None.

## Tried and abandoned
None.

## Noticed, not touched
- `docs/lint-transition.md` Phases section still says "D4: `depguard` on ..." under Phase 0 (line ~197); the stage brief only named the Decisions entry and a Progress line, so that phase-0 bullet was left as is for the parent to reconcile.

## Surprises / notes
- `--enable-only` overrides the `disable` list, so `scripts/lint-only.sh <linter>` works for linters that are still in the debt list.
