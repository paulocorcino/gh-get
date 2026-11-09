# AGENTS.md

gh-get downloads a folder (or a whole repo) from GitHub. Domain terms: `CONTEXT.md`. Agent-skill docs: `docs/agents/`.

## Lint gate

- Before you call a change done, run `scripts/check.sh`; it passes when every row prints `ok`. CI runs the same script.
- While you edit, `.claude/settings.json` runs the fast loop for you: after each Go edit, the formatter and the linter on that package, and their output comes back to you. It refuses `--no-verify` and force push, and asks before an edit to a file that defines the gate. It is not the gate: run `scripts/check.sh` after each group of edits and read the failing rows.
- When a rule fails, fix the cause:
  - an error that nobody reads goes to its caller or to the log; wrap an error once, at the package boundary;
  - output goes through the writer passed to `run` (`stdout`, `stderr`), never `fmt.Print*`; only `main` exits;
  - a function that is too complex is a god function, and a file that is too long a god file: split each by the responsibilities it already carries, one function or module per responsibility, or rewrite the function with guard clauses or a table of cases;
  - formatting and import order are the formatter's: run `golangci-lint fmt` (the exact command is in `scripts/check.sh`), do not edit by hand.
- An exception is the last resort: `//nolint:<linter> // <why>`, with a reason that says why the rule is wrong at that line. A bare, reasonless or unused `//nolint` fails the gate.
- `.golangci.yml`, `docs/lint-transition.md` and ratchet constants change only when the task is about them, and a count only goes down: a linter leaves the `disable` list when its debt reaches 0, and never joins it.

## Tools

- Before reading a whole source file, to find code by its shape (a call with a given argument, an empty block, an implementation), or to rewrite one shape at many sites, use ast-grep: `docs/ast-grep.md`.
