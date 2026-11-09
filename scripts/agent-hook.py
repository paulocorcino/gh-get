#!/usr/bin/env python3
"""Agent hooks for Claude Code (wired in .claude/settings.json). The fast loop of the gate.

  pre-bash   refuse --no-verify, --no-gpg-sign and force pushes
  pre-edit   ask before an edit to a file that defines the gate
  post-edit  after a Go edit: gofmt-clean and golangci-lint on the package; failures go back to the agent

Reads the hook event as JSON on stdin. The full gate stays scripts/check.sh.
"""

import json
import os
import pathlib
import re
import shlex
import subprocess
import sys

ROOT = str(pathlib.Path(__file__).resolve().parent.parent)
GATE_FILES = (
    ".golangci.yml",
    "scripts/check.sh",
    "scripts/lint-only.sh",
    "scripts/agent-hook.py",
    ".claude/settings.json",
    "docs/lint-transition.md",
    "docs/lint-scripts/",
    ".github/workflows/",
)
SEGMENT = re.compile(r"&&|\|\||[;|\n]")
FORCE = ("--force", "--force-with-lease", "-f")


def bypasses(cmd):
    """True when a git invocation in cmd carries a bypass flag. Only real arguments count, not quoted text."""
    for segment in SEGMENT.split(cmd):
        try:
            words = shlex.split(segment)
        except ValueError:
            continue
        while words and "=" in words[0] and not words[0].startswith("-"):
            words = words[1:]  # leading VAR=value
        if not words or words[0] != "git":
            continue
        if "--no-verify" in words or "--no-gpg-sign" in words:
            return True
        if "push" in words and (
            any(w in FORCE or w.startswith("--force-with-lease=") for w in words)
            or any(w.startswith("+") and len(w) > 1 for w in words)
        ):
            return True
    return False


def decide(kind, reason):
    out = {"hookEventName": "PreToolUse", "permissionDecision": kind, "permissionDecisionReason": reason}
    print(json.dumps({"hookSpecificOutput": out}))


def rel(path):
    try:
        return os.path.relpath(path, ROOT).replace("\\", "/")
    except ValueError:
        return path


def pre_bash(event):
    cmd = event.get("tool_input", {}).get("command", "")
    if bypasses(cmd):
        decide("deny", "The gate has no bypass: fix the cause. No --no-verify, --no-gpg-sign or force push.")


def pre_edit(event):
    path = rel(event.get("tool_input", {}).get("file_path", ""))
    if any(path == g or (g.endswith("/") and path.startswith(g)) for g in GATE_FILES):
        decide(
            "ask",
            f"{path} defines the gate: it changes only when the task is about the gate, and a count only goes down.",
        )


def lint_command():
    if os.environ.get("GOLANGCI_LINT"):
        return shlex.split(os.environ["GOLANGCI_LINT"])
    text = (pathlib.Path(ROOT) / "scripts" / "check.sh").read_text(encoding="utf-8")
    version = re.search(r"^GOLANGCI_LINT_VERSION=(\S+)", text, re.MULTILINE).group(1)
    return ["go", "run", f"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{version}"]


def post_edit(event):
    path = event.get("tool_input", {}).get("file_path", "")
    if not path.endswith(".go"):
        return 0
    parent = pathlib.PurePosixPath(rel(path)).parent.as_posix()
    pkg = "./" + ("" if parent == "." else parent)
    gl = lint_command()
    problems = []
    for args in (["fmt", "--diff", rel(path)], ["run", pkg]):
        r = subprocess.run(gl + args, cwd=ROOT, capture_output=True, text=True, check=False)  # noqa: S603 - fixed argv, no shell
        if r.returncode != 0:
            problems.append(f"$ golangci-lint {' '.join(args)}\n{r.stdout}{r.stderr}".strip())
    if problems:
        print(
            "\n\n".join(problems) + "\n\nFix the cause (see AGENTS.md); run scripts/check.sh before you say done.",
            file=sys.stderr,
        )
        return 2
    return 0


def main():
    event = json.load(sys.stdin)
    handlers = {"pre-bash": pre_bash, "pre-edit": pre_edit, "post-edit": post_edit}
    return handlers[sys.argv[1]](event) or 0


if __name__ == "__main__":
    sys.exit(main())
