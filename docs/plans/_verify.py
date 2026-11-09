"""Verify primitives for staged-plan stages.

Vendored into each repo at <repo>/docs/plans/_verify.py so generated stage
scripts can `from _verify import V` regardless of executor (Claude Code, Codex,
human dev). Python 3 stdlib only - runs on Linux, macOS, Windows native.

Generated script pattern:

    #!/usr/bin/env python3
    import sys
    from pathlib import Path
    sys.path.insert(0, str(Path(__file__).parent))
    from _verify import V

    V.assert_clean_tree()
    V.assert_commit_present(r"^feat: add X")
    V.assert_only_files_touched(["src/x.ts"], base_sha="HEAD~1")
    V.run_gate("bun run build", slug="my-plan", stage=1, gate="build")
    V.run_proof("bun test src/x.test.ts", expect=["rejects empty input"], check="C1.1")
    V.assert_report_exists("docs/plans/<slug>-stage-1-report.md")
    V.assert_no_placeholders("docs/plans/<slug>.md")
    sys.exit(V.summarize())

Run as a script, it serves the parent of a plan:

    python3 -B docs/plans/_verify.py gate docs/plans/<slug>.md
    python3 -B docs/plans/_verify.py brief docs/plans/<slug>.md 3 [--workdir <path>]
    python3 -B docs/plans/_verify.py run-log docs/plans/<slug>.md 3 launched "<detail>"

Design principles (review-driven):
- Guardrails, not summaries: scripts validate contract and preserve evidence;
  agent owns judgment.
- Lossless: full gate output is written to docs/plans/logs/ alongside the tail
  shown in stdout, so a 20-line tail never costs the underlying evidence.
- Cross-platform: stdlib only. No grep, no bash, no shell-out for matching.
"""

from __future__ import annotations

import inspect
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

# Bumped whenever a primitive is added or its behaviour changes. scaffold.py
# compares it with the repo's vendored copy and refreshes an older one.
# Primitives are only ever added, so a refresh never breaks an older plan.
VERSION = 7

# The Windows console code page (cp1252) cannot print every character a
# gate emits; an unencodable one would crash summarize() mid-report.
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")


# Resolve repo root once. Stage scripts live at <repo>/docs/plans/, so the
# repo root is two parents up from this module's location.
_MODULE_DIR = Path(__file__).resolve().parent
_REPO_ROOT = _MODULE_DIR.parent.parent if _MODULE_DIR.name == "plans" else None


def _resolve(path: str | Path) -> Path:
    """Resolve path relative to repo root if known, else cwd."""
    p = Path(path)
    if p.is_absolute():
        return p
    if _REPO_ROOT is not None:
        return (_REPO_ROOT / p).resolve()
    return p.resolve()


def _logs_dir() -> Path:
    """Where full gate logs are written."""
    base = _REPO_ROOT if _REPO_ROOT is not None else Path.cwd()
    d = base / "docs" / "plans" / "logs"
    d.mkdir(parents=True, exist_ok=True)
    return d


@dataclass
class Check:
    name: str
    ok: bool
    detail: str = ""
    log_path: Path | None = None


@dataclass
class _Verifier:
    checks: list[Check] = field(default_factory=list)

    def _record(self, name: str, ok: bool, detail: str = "", log_path: Path | None = None) -> bool:
        self.checks.append(Check(name=name, ok=ok, detail=detail, log_path=log_path))
        marker = "PASS" if ok else "FAIL"
        line = f"[{marker}] {name}"
        if detail and not ok:
            line += f"\n        {detail}"
        if log_path is not None:
            line += f"\n        full log: {log_path}"
        print(line)
        return ok

    # ------------------------------------------------------------------ gates

    _CALLER_RX = re.compile(r"^(?P<slug>.+?)-verify-stage-(?P<stage>\d+)\.py$")
    _CALLER_E2E_RX = re.compile(r"^(?P<slug>.+?)-verify-e2e\.py$")

    def _derive_log_prefix(
        self, cmd: str, slug: str | None, stage: int | None, gate: str | None
    ) -> tuple[str, str]:
        """Return (prefix, gate_label) for the log filename.

        Auto-derives slug+stage from the calling script's filename when caller
        is `<slug>-verify-stage-<N>.py` or `<slug>-verify-e2e.py`. Falls back
        to a generic prefix when nothing can be inferred. Logging is always
        on; explicit kwargs override auto-derivation.
        """
        # Walk the stack to find the first frame outside this module.
        caller_name = ""
        for frame_info in inspect.stack()[1:]:
            if Path(frame_info.filename).resolve() != Path(__file__).resolve():
                caller_name = Path(frame_info.filename).name
                break

        derived_slug, derived_stage = None, None
        m = self._CALLER_RX.match(caller_name)
        if m:
            derived_slug = m.group("slug")
            derived_stage = int(m.group("stage"))
        else:
            m = self._CALLER_E2E_RX.match(caller_name)
            if m:
                derived_slug = m.group("slug")
                derived_stage = "e2e"

        eff_slug = slug or derived_slug or "gate"
        eff_stage = stage if stage is not None else (derived_stage if derived_stage is not None else "x")
        if gate:
            label = re.sub(r"[^A-Za-z0-9._-]+", "_", gate)
        else:
            # Derive a stable label from the command's first token
            # (e.g. "bun", "pytest", "go"). Falls back to "cmd" if cmd is empty.
            tokens = cmd.strip().split()
            first_token = tokens[0] if tokens else ""
            label = re.sub(r"[^A-Za-z0-9._-]+", "_", (first_token or "cmd"))[:24] or "cmd"

        prefix = f"{eff_slug}-stage-{eff_stage}-{label}"
        return prefix, label

    def run_gate(
        self,
        cmd: str,
        *,
        cwd: str | None = None,
        timeout: int = 600,
        slug: str | None = None,
        stage: int | None = None,
        gate: str | None = None,
    ) -> bool:
        """Execute a shell gate command. Pass = exit 0.

        Logging is ALWAYS ON. Full stdout+stderr is written to
        docs/plans/logs/<prefix>-<timestamp>.log. The timestamp prevents retry
        runs from clobbering the previous attempt's evidence. Slug, stage, and
        gate are auto-derived from the calling script's filename when not
        passed explicitly. The tail (last 20 lines) is shown in the on-screen
        FAIL detail; the log file always has the complete output. Partial
        output is preserved on timeout.
        """
        # Use a sub-second timestamp so retries fired in the same second get unique names.
        ts = datetime.now().strftime("%Y%m%d-%H%M%S-%f")[:-3]
        prefix, gate_label = self._derive_log_prefix(cmd, slug, stage, gate)
        log_path = _logs_dir() / f"{prefix}-{ts}.log"

        stdout, stderr, returncode, timed_out = "", "", -1, False
        try:
            proc = subprocess.run(
                cmd,
                shell=True,
                cwd=cwd,
                capture_output=True,
                text=True,
                encoding="utf-8",
                errors="replace",
                timeout=timeout,
            )
            stdout, stderr, returncode = proc.stdout or "", proc.stderr or "", proc.returncode
        except subprocess.TimeoutExpired as exc:
            timed_out = True
            stdout = exc.stdout.decode("utf-8", errors="replace") if isinstance(exc.stdout, bytes) else (exc.stdout or "")
            stderr = exc.stderr.decode("utf-8", errors="replace") if isinstance(exc.stderr, bytes) else (exc.stderr or "")

        header = (
            f"$ {cmd}\n"
            f"# cwd={cwd or os.getcwd()} timeout={timeout}s timed_out={timed_out} "
            f"returncode={returncode} ts={ts} gate_label={gate_label}\n\n"
        )
        log_path.write_text(
            header + "--- STDOUT ---\n" + stdout + "\n--- STDERR ---\n" + stderr,
            encoding="utf-8",
        )

        if timed_out:
            return self._record(
                f"gate: {cmd}",
                False,
                f"timeout after {timeout}s (partial output preserved)",
                log_path,
            )

        ok = returncode == 0
        detail = ""
        if not ok:
            tail = (stdout + stderr).strip().splitlines()[-20:]
            detail = "\n        ".join(tail)
        return self._record(f"gate: {cmd}", ok, detail, log_path)

    def run_proof(
        self,
        cmd: str,
        *,
        expect: list[str],
        check: str | None = None,
        cwd: str | None = None,
        timeout: int = 600,
    ) -> bool:
        """Run a check's proof and confirm every named test actually ran.

        Pass = exit 0 AND each string in `expect` (a test name or id) appears
        in the runner output. A filter that matches nothing exits 0 on several
        runners (`passWithNoTests` and equivalents) — a green check with no
        test behind it; requiring the name in the output closes that hole.
        Use a runner flag that prints per-test names (`pytest -v`,
        `cargo test -- --nocapture`, `jest --verbose`, `go test -v`).
        """
        label = check or (expect[0] if expect else "proof")
        if not self.run_gate(cmd, cwd=cwd, timeout=timeout, gate=f"proof-{label}"):
            return False
        log_path = self.checks[-1].log_path
        output = log_path.read_text(encoding="utf-8", errors="replace") if log_path else ""
        # Search only what the runner printed: the log header echoes `cmd`,
        # and a `-k <name>` filter would otherwise match itself.
        output = output.partition("--- STDOUT ---\n")[2]
        missing = [name for name in expect if name not in output]
        ok = not missing
        detail = "" if ok else f"exit 0 but not seen in output: {', '.join(missing)}"
        return self._record(f"proof ran: {label}", ok, detail, log_path)

    # ------------------------------------------------------------------ git

    def assert_clean_tree(self, *, cwd: str | None = None) -> bool:
        """Working tree clean: git status --porcelain returns empty."""
        proc = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=cwd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        clean = proc.returncode == 0 and proc.stdout.strip() == ""
        detail = proc.stdout.strip() if not clean else ""
        return self._record("working tree clean", clean, detail)

    def assert_commit_present(
        self, subject_pattern: str, *, depth: int = 20, cwd: str | None = None
    ) -> bool:
        """Last `depth` commits contain a subject matching `subject_pattern` (regex).

        The pattern is searched in the subject alone, so `^feat: ...` anchors
        at the subject's start; the matched line reported keeps the hash.
        """
        proc = subprocess.run(
            ["git", "log", f"-{depth}", "--pretty=format:%h %s"],
            cwd=cwd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        if proc.returncode != 0:
            return self._record(
                f"commit present: /{subject_pattern}/", False, proc.stderr.strip()
            )
        rx = re.compile(subject_pattern)
        for line in proc.stdout.splitlines():
            if rx.search(line.partition(" ")[2]):
                return self._record(
                    f"commit present: /{subject_pattern}/", True, line[:80]
                )
        return self._record(
            f"commit present: /{subject_pattern}/",
            False,
            f"no match in last {depth} commits",
        )

    def assert_only_files_touched(
        self,
        allowlist: list[str],
        *,
        base_sha: str = "HEAD~1",
        head_sha: str = "HEAD",
        subject: str | None = None,
        depth: int = 50,
        cwd: str | None = None,
    ) -> bool:
        """Files changed between base_sha..head_sha are a subset of allowlist.

        With `subject` (a regex on the commit subject), the range is that one
        commit, found among the last `depth`: the check keeps holding after
        later stages land on top of it. For the standard pattern (one delivery
        commit per stage with the post-stage report committed alongside the
        code), pass the report path in the allowlist as well.
        """
        if subject is not None:
            log = subprocess.run(
                ["git", "log", f"-{depth}", "--pretty=format:%H %s"],
                cwd=cwd, capture_output=True, text=True, encoding="utf-8", errors="replace",
            )
            rx = re.compile(subject)
            sha = next(
                (ln.split(" ", 1)[0] for ln in log.stdout.splitlines()
                 if rx.search(ln.partition(" ")[2])),
                None,
            )
            if sha is None:
                return self._record(
                    "scope: only declared files touched", False,
                    f"no commit subject matches /{subject}/ in last {depth}",
                )
            base_sha, head_sha = f"{sha}~1", sha
        proc = subprocess.run(
            ["git", "diff", "--name-only", f"{base_sha}..{head_sha}"],
            cwd=cwd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        if proc.returncode != 0:
            return self._record(
                "scope: only declared files touched", False, proc.stderr.strip()
            )
        touched = {p.strip() for p in proc.stdout.splitlines() if p.strip()}
        allowed = set(allowlist)
        unexpected = sorted(touched - allowed)
        ok = not unexpected
        detail = "" if ok else f"unexpected: {', '.join(unexpected)}"
        return self._record("scope: only declared files touched", ok, detail)

    # --------------------------------------------------- file content (no grep)

    def _walk_files(self, paths: list[str]) -> list[Path]:
        """Expand a list of files/dirs into a flat list of files.

        Pure Python; no shell-out to grep/find. Skips common noise (.git,
        node_modules, dist, build, .venv, __pycache__, target).
        """
        skip_dirs = {".git", "node_modules", "dist", "build", ".venv", "venv", "__pycache__", "target", ".next"}
        result: list[Path] = []
        for raw in paths:
            p = _resolve(raw)
            if p.is_file():
                result.append(p)
            elif p.is_dir():
                for root, dirs, files in os.walk(p):
                    dirs[:] = [d for d in dirs if d not in skip_dirs]
                    for f in files:
                        result.append(Path(root) / f)
        return result

    def _scan(self, pattern: str, paths: list[str], max_hits: int = 5) -> list[str]:
        rx = re.compile(pattern)
        hits: list[str] = []
        for f in self._walk_files(paths):
            try:
                with f.open("r", encoding="utf-8", errors="replace") as fh:
                    for lineno, line in enumerate(fh, start=1):
                        if rx.search(line):
                            hits.append(f"{f}:{lineno}: {line.rstrip()}")
                            if len(hits) >= max_hits:
                                return hits
            except OSError:
                continue
        return hits

    def assert_grep_zero(self, pattern: str, paths: list[str]) -> bool:
        """Invariant: pattern must NOT appear in any file under `paths`."""
        hits = self._scan(pattern, paths)
        ok = not hits
        detail = "" if ok else "\n        ".join(hits)
        return self._record(f"invariant absent: /{pattern}/", ok, detail)

    def assert_grep_nonzero(self, pattern: str, paths: list[str]) -> bool:
        """Invariant: pattern MUST appear at least once in `paths`."""
        hits = self._scan(pattern, paths, max_hits=1)
        ok = bool(hits)
        detail = "" if ok else "no match"
        return self._record(f"invariant present: /{pattern}/", ok, detail)

    # ---------------------------------------------------------------- artifacts

    def assert_report_exists(self, path: str) -> bool:
        """Post-stage report file exists and is non-empty.

        Path is resolved against repo root when relative, so this works whether
        the script is invoked from repo root, from docs/plans/, or elsewhere.
        """
        p = _resolve(path)
        ok = p.is_file() and p.stat().st_size > 0
        detail = "" if ok else f"missing or empty: {p}"
        return self._record(f"report exists: {path}", ok, detail)

    def assert_file_exists(self, path: str) -> bool:
        p = _resolve(path)
        ok = p.exists()
        return self._record(f"file exists: {path}", ok, "" if ok else f"missing: {p}")

    def assert_file_absent(self, path: str) -> bool:
        p = _resolve(path)
        ok = not p.exists()
        return self._record(f"file absent: {path}", ok, "" if ok else f"still exists: {p}")

    # ----------------------------------------------------------- plan validation

    # Catches scaffold-generated <FILL: ...> and <FILL-OR-DELETE: ...> markers
    # plus the bare placeholders from the SKILL.md Plan structure prose
    # (in case someone hand-builds a plan from the skill template instead of
    # invoking scaffold.py). Avoids generic <[a-z]+> to prevent false positives
    # on legitimate angle-bracket content (HTML, generics, SQL types).
    # `<FILL` matches on the opening token alone, so a marker whose closing
    # `>` sits on a later line is still caught.
    _PLACEHOLDER_RX = re.compile(
        r"<FILL"
        r"|<repo>"
        r"|<plan-slug>"
        r"|<plan title>"
        r"|<absolute path>"
        r"|<absolute plan path>"
        r"|<branch>"
        r"|<os>"
        r"|<cmd>"
        r"|<cmds>"
        r"|<path>"
        r"|<paths>"
        r"|<title>"
        r"|<intent>"
        r"|<atomic IDs>"
        r"|<one sentence>"
        r"|<list>"
        r"|<table of file -> stages that touch it>"
        r"|<commands \+ manual smoke[^>]*>"
        r"|<FILL or \"none\">"
    )

    def assert_no_placeholders(self, plan_path: str) -> bool:
        """Plan file contains no unfilled scaffold placeholders.

        Run this BEFORE Phase 2 launches Stage 1. A plan with surviving
        <FILL>, <FILL-OR-DELETE>, or other scaffold tokens is incomplete and
        will cause subagents to follow boilerplate instead of real instructions.
        """
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(f"plan filled: {plan_path}", False, f"missing: {p}")
        text = p.read_text(encoding="utf-8", errors="replace")
        hits: list[str] = []
        for lineno, line in enumerate(text.splitlines(), start=1):
            if self._PLACEHOLDER_RX.search(line):
                hits.append(f"{p.name}:{lineno}: {line.strip()[:120]}")
                if len(hits) >= 10:
                    break
        ok = not hits
        detail = "" if ok else "\n        ".join(hits)
        return self._record(f"plan filled: {plan_path}", ok, detail)

    _CHECK_RX = re.compile(r"^- (C\d+\.\d+)\b.*?Proof:\s*(.+)$", re.MULTILINE)

    def assert_distinct_proofs(self, plan_path: str) -> bool:
        """No two checks share a proof command.

        A command that settles several claims settles none of them alone: a
        whole test file, a whole suite, or one verify script named for many
        checks. Commands are compared by the span from the first to the last
        backtick, so a proof whose regex itself holds backticks stays whole.
        """
        name = f"one proof per check: {plan_path}"
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(name, False, f"missing: {p}")
        text = p.read_text(encoding="utf-8", errors="replace")
        owners: dict[str, list[str]] = {}
        for m in self._CHECK_RX.finditer(text):
            check, proof = m.group(1), m.group(2)
            if "<FILL" in proof:
                continue
            first, last = proof.find("`"), proof.rfind("`")
            span = proof[first + 1:last] if last > first else proof
            key = " ".join(span.split())
            owners.setdefault(key, []).append(check)
        shared = [
            f"{', '.join(checks)} share `{cmd[:100]}`"
            for cmd, checks in owners.items()
            if len(checks) > 1
        ]
        return self._record(name, not shared, "\n        ".join(shared))

    # Anchored at line start: each hand-off quotes its own markers mid-line.
    _STAGE_BLOCK_RX = re.compile(
        r"^<!-- BEGIN STAGE (\d+) -->(.*?)^<!-- END STAGE \1 -->",
        re.DOTALL | re.MULTILINE,
    )
    _DEPENDS_RX = re.compile(r"^\*\*Depends on:\*\*(.*)$", re.MULTILINE)
    _FILE_ITEM_RX = re.compile(r"^- `([^`]+)`(.*)$")

    def assert_stage_graph(self, plan_path: str) -> bool:
        """Stage `Depends on:` lines form a sound graph over the `Files:` lists.

        Every dependency points to an existing earlier stage, so plan order is
        a valid topological order and no cycle can exist. Two stages that
        declare the same file must be ordered by a dependency path: the later
        one builds on what the earlier one wrote. A directory or glob entry
        (trailing `/`, or `*`) names where new files go and never conflicts.
        With `- Concurrency:` above 1, a file three or more stages declare is a
        hotspot that chains them all, and is rejected unless one listing says
        `hotspot accepted: <extension point cost> / <wiring stage cost>` —
        both single owners weighed and rejected, so the chain is chosen, not
        inherited. On green, prints the
        waves (stages whose dependencies are all in earlier waves) and the
        widest.
        """
        name = f"stage graph: {plan_path}"
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(name, False, f"missing: {p}")
        text = p.read_text(encoding="utf-8", errors="replace")
        stages: dict[int, tuple[set[int], set[str]]] = {}
        accepted: set[str] = set()
        problems: list[str] = []
        for m in self._STAGE_BLOCK_RX.finditer(text):
            n, body = int(m.group(1)), m.group(2)
            dep_m = self._DEPENDS_RX.search(body)
            raw = dep_m.group(1).strip() if dep_m else ""
            if "<FILL" in raw:
                problems.append(f"Stage {n}: `Depends on:` is unfilled")
                stages[n] = (set(), set())
                continue
            deps = {int(d) for d in re.findall(r"Stage\s+(\d+)", raw)}
            if not deps and raw.lower() != "none":
                problems.append(f"Stage {n}: `Depends on:` is neither stage refs nor `none`: {raw!r}")
            for d in sorted(deps):
                if d >= n:
                    problems.append(f"Stage {n}: depends on Stage {d}, which is not earlier")
            files: set[str] = set()
            in_files = False
            for line in body.splitlines():
                if line.startswith("**Files:**"):
                    in_files = True
                    continue
                if in_files:
                    fm = self._FILE_ITEM_RX.match(line)
                    if fm:
                        files.add(fm.group(1).strip())
                        # Both owners must be weighed: a reason that rejects
                        # only one leaves the other unexamined.
                        note = fm.group(2).partition("hotspot accepted:")[2]
                        if " / " in note:
                            accepted.add(fm.group(1).strip())
                    elif line.strip():
                        break
            stages[n] = (deps, files)
        if not stages:
            return self._record(name, False, "no `<!-- BEGIN STAGE n -->` blocks found")
        for n, (deps, _) in stages.items():
            for d in sorted(deps):
                if d < n and d not in stages:
                    problems.append(f"Stage {n}: depends on Stage {d}, which does not exist")
        if problems:
            return self._record(name, False, "\n        ".join(problems))

        # Plan order is topological, so one forward pass closes ancestry.
        ancestors: dict[int, set[int]] = {}
        wave: dict[int, int] = {}
        for n in sorted(stages):
            deps = stages[n][0]
            ancestors[n] = set(deps).union(*(ancestors[d] for d in deps))
            wave[n] = 1 + max((wave[d] for d in deps), default=0)
        ordered = sorted(stages)
        shared = {
            n: {f for f in files if not f.endswith("/") and "*" not in f}
            for n, (_, files) in stages.items()
        }
        conc_m = re.search(r"^- Concurrency:\s*(\d+)", text, re.MULTILINE)
        if conc_m and int(conc_m.group(1)) > 1:
            owners: dict[str, list[int]] = {}
            for n in ordered:
                for f in shared[n]:
                    owners.setdefault(f, []).append(n)
            for f in sorted(owners):
                if len(owners[f]) >= 3 and f not in accepted:
                    listed = ", ".join(f"Stage {n}" for n in owners[f])
                    problems.append(
                        f"hotspot `{f}` is in {listed}: give it one owner "
                        "(an extension point or a closing wiring stage), or, when one "
                        "owner would weaken the design, mark a listing `hotspot accepted: "
                        "<extension point cost> / <wiring stage cost>`; "
                        "Concurrency stays as the user set it"
                    )
        for i, a in enumerate(ordered):
            for b in ordered[i + 1:]:
                for f in sorted(shared[a] & shared[b]):
                    if a not in ancestors[b]:
                        problems.append(
                            f"`{f}` is in Stage {a} and Stage {b}, but Stage {b} does not depend on Stage {a}"
                        )
        ok = not problems
        result = self._record(name, ok, "\n        ".join(problems))
        if ok:
            waves: dict[int, list[int]] = {}
            for n in ordered:
                waves.setdefault(wave[n], []).append(n)
            for w in sorted(waves):
                print(f"        wave {w}: " + ", ".join(f"Stage {n}" for n in waves[w]))
            print(f"        widest wave: {max(len(v) for v in waves.values())}")
        return result

    # ------------------------------------------------------------ plan shape

    # Thousands of tokens of reading (`**Size:**`). Past the split bound,
    # iteration (test output, fixes, retries) pushes a stage subagent toward a
    # window it cannot finish in.
    SIZE_TARGET_K = 60
    SIZE_SPLIT_K = 100
    # One parent holds the whole run; past these, the plan is several plans.
    PLAN_STAGES_WARN = 12
    PLAN_ITEMS_WARN = 3

    _SIZE_RX = re.compile(r"^\*\*Size:\*\*(.*)$", re.MULTILINE)
    _SIZE_K_RX = re.compile(r"~?\s*(\d+(?:\.\d+)?)\s*k\b")
    _ITEMS_RX = re.compile(r"^\*\*Items:\*\*(.*)$", re.MULTILINE)
    # Issue numbers and tracker keys; a decision record is not a backlog item.
    _ITEM_ID_RX = re.compile(r"#\d+|\b(?!ADR-|RFC-)[A-Z][A-Z0-9]+-\d+\b")

    def assert_plan_size(self, plan_path: str) -> bool:
        """Each stage fits its subagent's window, and the plan fits one parent.

        Fails when a stage's `**Size:**` names more than SIZE_SPLIT_K thousand
        tokens, or no `~Nk` at all. Warns, without failing, when a stage passes
        SIZE_TARGET_K, when the plan holds more than PLAN_STAGES_WARN stages,
        or when its `**Items:**` lines name more than PLAN_ITEMS_WARN distinct
        backlog ids — the shape of several plans sharing one parent.
        """
        name = f"plan size: {plan_path}"
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(name, False, f"missing: {p}")
        text = p.read_text(encoding="utf-8", errors="replace")
        problems: list[str] = []
        warnings: list[str] = []
        items: set[str] = set()
        blocks = list(self._STAGE_BLOCK_RX.finditer(text))
        for m in blocks:
            n, body = int(m.group(1)), m.group(2)
            size_m = self._SIZE_RX.search(body)
            raw = size_m.group(1).strip() if size_m else ""
            k_m = self._SIZE_K_RX.search(raw)
            if "<FILL" in raw:
                continue  # assert_no_placeholders reports it
            if not k_m:
                problems.append(f"Stage {n}: `Size:` names no `~Nk tokens`: {raw!r}")
            else:
                k = float(k_m.group(1))
                if k > self.SIZE_SPLIT_K:
                    problems.append(
                        f"Stage {n}: ~{k:g}k tokens of reading is past ~{self.SIZE_SPLIT_K}k: "
                        "split it where the code surface changes"
                    )
                elif k > self.SIZE_TARGET_K:
                    warnings.append(f"Stage {n}: ~{k:g}k tokens, past the ~{self.SIZE_TARGET_K}k target")
            for im in self._ITEMS_RX.finditer(body):
                items.update(self._ITEM_ID_RX.findall(im.group(1)))
        if not blocks:
            return self._record(name, False, "no `<!-- BEGIN STAGE n -->` blocks found")
        if len(blocks) > self.PLAN_STAGES_WARN:
            warnings.append(
                f"{len(blocks)} stages, past {self.PLAN_STAGES_WARN}: "
                "consider phase plans under one program entry plan"
            )
        if len(items) > self.PLAN_ITEMS_WARN:
            warnings.append(
                f"{len(items)} backlog items ({', '.join(sorted(items))}), past {self.PLAN_ITEMS_WARN}: "
                "consider phase plans under one program entry plan"
            )
        for w in warnings:
            print(f"[WARN] {w}")
        return self._record(name, not problems, "\n        ".join(problems))

    # -------------------------------------------------------------- hand-off

    _HANDOFF_TEMPLATE_RX = re.compile(
        r"^<!-- BEGIN HANDOFF TEMPLATE -->\n```[a-z]*\n(.*?)\n```\n<!-- END HANDOFF TEMPLATE -->",
        re.DOTALL | re.MULTILINE,
    )
    _STAGE_TITLE_RX = re.compile(r"^## Stage \d+ - (.+)$", re.MULTILINE)

    def handoff(self, plan_path: str, n: int, workdir: str | None = None) -> str:
        """The hand-off prompt of Stage `n`, rendered from the plan's template.

        Fills `{{N}}`, `{{TITLE}}` and `{{ITEMS}}` from the stage block and,
        for an isolated stage, appends the `Working directory:` line. Raises
        ValueError when the plan has no template or no such stage.
        """
        p = _resolve(plan_path)
        text = p.read_text(encoding="utf-8", errors="replace")
        tm = self._HANDOFF_TEMPLATE_RX.search(text)
        if not tm:
            raise ValueError(f"{p.name}: no `<!-- BEGIN HANDOFF TEMPLATE -->` block")
        block = next(
            (m.group(2) for m in self._STAGE_BLOCK_RX.finditer(text) if int(m.group(1)) == n),
            None,
        )
        if block is None:
            raise ValueError(f"{p.name}: no Stage {n} block")
        title_m = self._STAGE_TITLE_RX.search(block)
        items_m = self._ITEMS_RX.search(block)
        prompt = (
            tm.group(1)
            .replace("{{N}}", str(n))
            .replace("{{TITLE}}", title_m.group(1).strip() if title_m else f"Stage {n}")
            .replace("{{ITEMS}}", items_m.group(1).strip() if items_m else "none")
        )
        if workdir:
            prompt += f"\n\nWorking directory: {workdir}"
        return prompt

    # ------------------------------------------------------------ one read

    # Measured on 2026-10-30 with Claude Code's Read tool: one call returns at
    # most 25,000 tokens, and Rust, TypeScript and plan markdown all ran at
    # ~2.4-2.6 bytes per token. 2.5 keeps the estimate on the safe side.
    BYTES_PER_TOKEN = 2.5
    READ_LIMIT_K = 25
    READ_TARGET_K = 20

    # Sections of the plan a stage works under, in the order its brief shows
    # them: rules first, reference last. The rest of the plan is the parent's.
    BRIEF_SECTIONS = (
        "Hand-off conventions",
        "Global conventions",
        "Procedures",
        "One-way doors",
        "Critical files",
    )
    _STAGES_RX = re.compile(r"^## Stages\s*$", re.MULTILINE)
    _ANCHOR_LINE_RX = re.compile(r"^<!-- (?:BEGIN |END )?STAGE \d+(?::[^>]*)? -->\n?", re.MULTILINE)
    _TIER_RX = re.compile(r"^\*\*Tier:\*\*\s*(\w+)", re.MULTILINE)
    _EFFORT_RX = re.compile(r"^\*\*Effort:\*\*\s*(\w+)", re.MULTILINE)

    def _tokens_k(self, text: str) -> float:
        return len(text.encode("utf-8")) / self.BYTES_PER_TOKEN / 1000

    def _head(self, text: str) -> str:
        """The parent's part: everything above `## Stages` (or the first stage block)."""
        m = self._STAGES_RX.search(text) or self._STAGE_BLOCK_RX.search(text)
        return text[: m.start()] if m else text

    def _section(self, head: str, name: str) -> str:
        m = re.search(rf"^## {re.escape(name)}\b.*?(?=^## |\Z)", head, re.DOTALL | re.MULTILINE)
        return m.group(0).rstrip() + "\n" if m else ""

    def _named_recipes(self, section: str, block: str) -> str:
        """The `### <name>` recipes of `## Procedures` that the stage block names.

        A brief carries the recipes its stage runs, not the whole catalogue,
        and the recipes those recipes name in turn.
        Empty when the stage names none.
        """
        heading, _, body = section.partition("\n")
        recipes = [r for r in re.split(r"(?m)^(?=### )", body) if r.startswith("### ")]

        def name(recipe: str) -> str:
            return recipe[4:].splitlines()[0].strip().strip("`")

        kept: list[str] = []
        reach = block
        while True:
            new = [r for r in recipes if r not in kept and name(r) in reach]
            if not new:
                break
            kept += new
            reach = "".join(new)
        kept.sort(key=recipes.index)
        return heading + "\n" + "".join(kept).rstrip() + "\n" if kept else ""

    def brief_text(self, plan_path: str, n: int, workdir: str | None = None) -> str:
        """Stage `n`'s whole hand-off as one document.

        The rendered prompt, then the stage block, then the plan sections the
        stage works under (BRIEF_SECTIONS) — so the subagent reads one document
        in one read and none of the parent's text. Raises ValueError like
        `handoff`.
        """
        text = _resolve(plan_path).read_text(encoding="utf-8", errors="replace")
        prompt = self.handoff(plan_path, n, workdir)
        block = next(m.group(0) for m in self._STAGE_BLOCK_RX.finditer(text) if int(m.group(1)) == n)
        # The anchors serve the planner's edits; the subagent reads past them.
        block = re.sub(r"\n{3,}", "\n\n", self._ANCHOR_LINE_RX.sub("", block)).strip() + "\n"
        head = self._head(text)
        shared = []
        for name in self.BRIEF_SECTIONS:
            section = self._section(head, name)
            if name == "Procedures" and section:
                section = self._named_recipes(section, block)
            if section:
                shared.append(section)
        title_m = self._STAGE_TITLE_RX.search(block)
        title = title_m.group(1).strip() if title_m else f"Stage {n}"
        parts = [f"# Brief: Stage {n} - {title}\n", prompt.strip() + "\n", "---\n", block, "---\n", *shared]
        return "\n".join(parts)

    def brief(self, plan_path: str, n: int, workdir: str | None = None) -> tuple[Path, float]:
        """Write Stage `n`'s brief to `logs/<slug>-stage-<n>-brief.md`.

        Returns the path and its size in thousands of tokens. One file per
        launch: a retry gets `-brief-2.md`, `-brief-3.md`, ..., so each file
        stays the record of what that attempt was handed.
        """
        text = self.brief_text(plan_path, n, workdir)
        stem = f"{Path(plan_path).stem}-stage-{n}-brief"
        path = _logs_dir() / f"{stem}.md"
        attempt = 1
        while path.exists():
            attempt += 1
            path = _logs_dir() / f"{stem}-{attempt}.md"
        with path.open("w", encoding="utf-8", newline="\n") as fh:
            fh.write(text)
        return path, self._tokens_k(text)

    def assert_read_budget(self, plan_path: str) -> bool:
        """The parent's part and every stage's brief each fit one read.

        Fails past READ_LIMIT_K thousand tokens — the most one read returns —
        and warns past READ_TARGET_K. A plan without a hand-off template
        (scaffolded before briefs existed) has only its parent's part checked.
        """
        name = f"one read each: {plan_path}"
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(name, False, f"missing: {p}")
        text = p.read_text(encoding="utf-8", errors="replace")
        problems: list[str] = []
        sizes = [(
            "the parent's part (down to `## Stages`)",
            self._tokens_k(self._head(text)),
            "move stage-specific detail into stage blocks, or split the plan",
        )]
        if self._HANDOFF_TEMPLATE_RX.search(text):
            for m in self._STAGE_BLOCK_RX.finditer(text):
                n = int(m.group(1))
                sizes.append((
                    f"Stage {n}'s brief",
                    self._tokens_k(self.brief_text(plan_path, n)),
                    "move detail only some stages use out of the shared sections, or split the stage",
                ))
        else:
            print("        no hand-off template: briefs not measured")
        for what, k, fix in sizes:
            if k > self.READ_LIMIT_K:
                problems.append(f"{what}: ~{k:.1f}k tokens, past the ~{self.READ_LIMIT_K}k one read returns: {fix}")
            elif k > self.READ_TARGET_K:
                print(f"[WARN] {what}: ~{k:.1f}k tokens, past the ~{self.READ_TARGET_K}k target")
        line = f"        parent's part ~{sizes[0][1]:.1f}k tokens"
        if len(sizes) > 1:
            what, k, _ = max(sizes[1:], key=lambda s: s[1])
            line += f"; largest brief ~{k:.1f}k ({what})"
        print(line)
        return self._record(name, not problems, "\n        ".join(problems))

    # ------------------------------------------------------------ program

    _PHASES_RX = re.compile(r"^## Phases\s*$", re.MULTILINE)
    _PHASE_ROW_RX = re.compile(r"^\|\s*([A-Za-z0-9_-]+)\s*\|\s*`([^`]+)`\s*\|([^|]*)\|([^|]*)\|\s*$")

    def is_program(self, plan_path: str) -> bool:
        """A program is an entry plan whose work lives in phase plans (`## Phases`)."""
        text = _resolve(plan_path).read_text(encoding="utf-8", errors="replace")
        return bool(self._PHASES_RX.search(text))

    def phases(self, plan_path: str) -> list[tuple[str, Path, str, list[str]]]:
        """The `## Phases` rows: (id, phase plan path, items, the phase ids it depends on).

        A phase plan's path is read relative to the program file's folder.
        """
        p = _resolve(plan_path)
        text = p.read_text(encoding="utf-8", errors="replace")
        m = self._PHASES_RX.search(text)
        if not m:
            return []
        body = text[m.end():]
        end = re.search(r"^## ", body, re.MULTILINE)
        rows = []
        for line in (body[: end.start()] if end else body).splitlines():
            rm = self._PHASE_ROW_RX.match(line)
            if rm:
                raw = rm.group(4).strip()
                deps = [] if raw.lower() in ("none", "") else [d.strip() for d in raw.split(",")]
                rows.append((rm.group(1), p.parent / rm.group(2), rm.group(3).strip(), deps))
        return rows

    def assert_program(self, plan_path: str) -> bool:
        """`## Phases` names existing phase plans and a sound phase graph.

        Every phase plan exists; ids are unique; each dependency names an
        earlier phase, so table order is a valid order. Prints the phase waves.
        """
        name = f"program phases: {plan_path}"
        rows = self.phases(plan_path)
        if not rows:
            return self._record(name, False, "no `| <id> | `<plan>.md` | <items> | <depends on> |` rows under `## Phases`")
        problems: list[str] = []
        seen: dict[str, int] = {}
        wave: dict[str, int] = {}
        for pid, path, _items, deps in rows:
            if pid in seen:
                problems.append(f"phase {pid}: listed twice")
            if not path.is_file():
                problems.append(f"phase {pid}: no plan at {path}")
            for d in deps:
                if d not in seen:
                    problems.append(f"phase {pid}: depends on `{d}`, which is not an earlier phase")
            seen[pid] = len(seen)
            wave[pid] = 1 + max((wave.get(d, 0) for d in deps), default=0)
        ok = not problems
        result = self._record(name, ok, "\n        ".join(problems))
        if ok:
            for w in sorted(set(wave.values())):
                print(f"        phase wave {w}: " + ", ".join(pid for pid in wave if wave[pid] == w))
        return result

    LINE_LIMIT = 160

    def assert_line_width(self, plan_path: str) -> bool:
        """No line of the plan is longer than LINE_LIMIT characters.

        Every line, tables and fences included: a row too long to read at a
        glance is a list item waiting to happen, and a proof command too long
        for one line belongs in the stage's verify script.
        """
        name = f"lines within {self.LINE_LIMIT} characters: {plan_path}"
        p = _resolve(plan_path)
        if not p.is_file():
            return self._record(name, False, f"missing: {p}")
        lines = p.read_text(encoding="utf-8", errors="replace").splitlines()
        long = [(i, len(line)) for i, line in enumerate(lines, start=1) if len(line) > self.LINE_LIMIT]
        detail = "\n        ".join(f"{p.name}:{i}: {n} characters" for i, n in long[:10])
        if len(long) > 10:
            detail += f"\n        ... and {len(long) - 10} more"
        return self._record(name, not long, detail)

    def print_stage_index(self, plan_path: str) -> None:
        """One line per stage: what the parent schedules from, without the blocks."""
        text = _resolve(plan_path).read_text(encoding="utf-8", errors="replace")

        def pick(rx: re.Pattern[str], body: str) -> str:
            m = rx.search(body)
            return m.group(1).strip() if m else "?"

        blocks = [(int(m.group(1)), m.group(2)) for m in self._STAGE_BLOCK_RX.finditer(text)]
        # chain: the stage plus the longest run of stages waiting on it. The
        # parent launches the ready stage with the longest chain first.
        dependents: dict[int, list[int]] = {n: [] for n, _ in blocks}
        for n, body in blocks:
            for d in re.findall(r"Stage\s+(\d+)", pick(self._DEPENDS_RX, body)):
                dependents.setdefault(int(d), []).append(n)
        chain: dict[int, int] = {}

        def chain_of(n: int, seen: frozenset[int] = frozenset()) -> int:
            if n not in chain:
                kids = [k for k in dependents.get(n, []) if k not in seen]
                chain[n] = 1 + max((chain_of(k, seen | {n}) for k in kids), default=0)
            return chain[n]

        print("stage index:")
        for n, body in blocks:
            files: list[str] = []
            in_files = False
            for line in body.splitlines():
                if line.startswith("**Files:**"):
                    in_files = True
                    continue
                if in_files:
                    fm = self._FILE_ITEM_RX.match(line)
                    if fm:
                        files.append(fm.group(1).strip())
                    elif line.strip():
                        break
            size = self._SIZE_K_RX.search(pick(self._SIZE_RX, body))
            print(
                f"  Stage {n} | {pick(self._STAGE_TITLE_RX, body)}"
                f" | {pick(self._TIER_RX, body)}/{pick(self._EFFORT_RX, body)}"
                f" | {('~' + size.group(1) + 'k') if size else '?'}"
                f" | depends: {pick(self._DEPENDS_RX, body)} | chain: {chain_of(n)}"
                f" | files: {', '.join(files) or '-'}"
            )

    # --------------------------------------------------------------- run log

    def record_run(self, plan_path: str, stage: str, event: str, detail: str = "") -> Path:
        """Append one event to the plan's run log, `logs/<slug>-run.md`.

        The run log is the parent's memory of what is in flight: it survives a
        compaction or a crash of the conversation that drives the run.
        """
        path = _logs_dir() / f"{Path(plan_path).stem}-run.md"
        ts = datetime.now().astimezone().isoformat(timespec="seconds")
        s = str(stage)
        who = "plan" if s in ("", "-") else (f"Stage {s}" if s.isdigit() else f"Phase {s}")
        line = f"- {ts} | {who} | {event}" + (f" | {detail}" if detail else "")
        with path.open("a", encoding="utf-8", newline="\n") as fh:
            fh.write(line + "\n")
        return path

    # ---------------------------------------------------------------- summary

    def summarize(self) -> int:
        """Print a summary table; return 0 if all green, else 1."""
        passed = sum(1 for c in self.checks if c.ok)
        total = len(self.checks)
        print()
        print(f"=== {passed}/{total} checks passed ===")
        if passed != total:
            print("Failures:")
            for c in self.checks:
                if not c.ok:
                    print(f"  - {c.name}")
                    if c.detail:
                        for line in c.detail.splitlines():
                            print(f"      {line}")
                    if c.log_path is not None:
                        print(f"      full log: {c.log_path}")
        return 0 if passed == total else 1


V = _Verifier()


def _main(argv: list[str]) -> int:
    import argparse

    ap = argparse.ArgumentParser(prog="_verify.py", description="Parent-side commands of a staged plan.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("gate", help="pre-execution gate: placeholders, stage graph, one proof per check, sizes, one read each, line width; then the stage index")
    g.add_argument("plan")
    h = sub.add_parser("brief", help="write one stage's brief and print the prompt that points to it")
    h.add_argument("plan")
    h.add_argument("stage", type=int)
    h.add_argument("--workdir", help="worktree of an isolated stage")
    r = sub.add_parser("run-log", help="append one event to the plan's run log")
    r.add_argument("plan")
    r.add_argument("stage", help="stage number, a phase id in a program, or - for a plan-wide event")
    r.add_argument(
        "event",
        help="queued | launched | returned | verifying | integrated | red | stalled | retry | halted; "
        "phase-started | phase-closed",
    )
    r.add_argument("detail", nargs="?", default="")
    args = ap.parse_args(argv)

    if args.cmd == "gate":
        plans = [args.plan]
        if V.is_program(args.plan):
            # The entry plan itself, then every phase plan's own gate.
            V.assert_no_placeholders(args.plan)
            V.assert_program(args.plan)
            V.assert_line_width(args.plan)
            k = V._tokens_k(_resolve(args.plan).read_text(encoding="utf-8", errors="replace"))
            V._record(f"program fits one read (~{k:.1f}k tokens): {args.plan}", k <= V.READ_LIMIT_K)
            plans = [str(path) for _pid, path, _i, _d in V.phases(args.plan) if path.is_file()]
        for plan in plans:
            V.assert_no_placeholders(plan)
            V.assert_stage_graph(plan)
            V.assert_distinct_proofs(plan)
            V.assert_plan_size(plan)
            V.assert_read_budget(plan)
            V.assert_line_width(plan)
        code = V.summarize()
        for plan in plans:
            print()
            print(f"{Path(plan).name}:")
            V.print_stage_index(plan)
        return code
    if args.cmd == "brief":
        try:
            path, k = V.brief(args.plan, args.stage, args.workdir)
            print(f"brief: {path.as_posix()} (~{k:.1f}k tokens)")
            print(f"prompt: Read {path.as_posix()} in one read: it is your whole hand-off for this stage. Then execute it.")
        except ValueError as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 2
        return 0
    print(V.record_run(args.plan, args.stage, args.event, args.detail))
    return 0


if __name__ == "__main__":
    sys.exit(_main(sys.argv[1:]))
