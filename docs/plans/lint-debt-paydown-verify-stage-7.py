#!/usr/bin/env python3
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from _verify import V

GOLANGCI = "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0"


def sh(cmd):
    p = subprocess.run(cmd, shell=True, capture_output=True, text=True, encoding="utf-8", errors="replace")
    return p.returncode, p.stdout + p.stderr


def prod_files():
    return [p for p in Path(".").rglob("*.go") if not p.name.endswith("_test.go") and ".git" not in p.parts]


def count_matches(pattern, scope):
    files = prod_files() if scope == "prod" else list(Path(".").rglob("*.go"))
    return sum(len(re.findall(pattern, f.read_text(encoding="utf-8", errors="replace"))) for f in files)


def absent(check, cmd, rule):
    code, out = sh(cmd)
    V._record(f"proof: {check} {rule} absent from gosec output", rule not in out, "" if rule not in out else f"{rule} still reported")


def count_max(check, pattern, limit, scope):
    n = count_matches(pattern, scope)
    V._record(f"proof: {check} count of /{pattern}/ = {n} (max {limit})", n <= limit, "" if n <= limit else f"{n} > {limit}")


def lines_max(check, path, limit):
    n = sum(1 for ln in Path(path).read_text(encoding="utf-8").splitlines() if ln.strip())
    V._record(f"proof: {check} {path} has {n} non-blank lines (max {limit})", n <= limit, "" if n <= limit else f"{n} > {limit}")


def gocognit_root(check):
    cfg = Path(".golangci.yml").read_text(encoding="utf-8")
    cfg = re.sub(r"(gocognit:\s*\n\s*min-complexity:) \d+", r"\1 10", cfg)
    scratch = Path("docs/plans/logs/gocognit-root.yml")
    scratch.parent.mkdir(parents=True, exist_ok=True)
    scratch.write_text(cfg, encoding="utf-8")
    code, out = sh(f"{GOLANGCI} run -c {scratch} --enable-only gocognit .")
    V._record(f"proof: {check} root package at gocognit 10", code == 0, out[-400:] if code else "")


def noise_only(check):
    lines = Path(".golangci.yml").read_text(encoding="utf-8").splitlines()
    in_disable, bad = False, []
    for i, ln in enumerate(lines):
        if re.match(r"^  disable:", ln):
            in_disable = True
            continue
        if in_disable and re.match(r"^  \S", ln):
            in_disable = False
        if in_disable and re.match(r"^    - ", ln) and not lines[i - 1].strip().startswith("# noise:"):
            bad.append(ln.strip())
    V._record(f"proof: {check} every disable entry is noise", not bad, ", ".join(bad))


V.assert_clean_tree()
V.assert_commit_present(r"^lint\(stage 7\)")
V.run_gate("bash scripts/check.sh")
V.run_proof('bash scripts/lint-only.sh forcetypeassert', expect=['0 issues'], check="C7.1")
V.run_proof('bash scripts/lint-only.sh exhaustive', expect=['0 issues'], check="C7.2")
V.run_proof('bash scripts/lint-only.sh noctx', expect=['0 issues'], check="C7.3")
V.run_proof('bash scripts/lint-only.sh govet', expect=['0 issues'], check="C7.4")
V.run_proof('bash scripts/lint-only.sh gochecknoglobals', expect=['0 issues'], check="C7.5")
V.run_proof('bash scripts/lint-only.sh gocritic', expect=['0 issues'], check="C7.6")
absent("C7.7", 'bash scripts/lint-only.sh gosec', 'G602')
V.assert_grep_zero('^\\s+- (forcetypeassert|exhaustive|noctx|govet|gochecknoglobals|badCall|G602)$', ['.golangci.yml'])
V.assert_report_exists("docs/plans/lint-debt-paydown-stage-7-report.md")
sys.exit(V.summarize())
