# Lint debt paydown (phases 1-5) - Staged Execution Plan

<!-- scaffolded 2026-11-08 via staged-plan/lib/scaffold.py -->

## Execution model (READ FIRST)
Staged subagent execution over a dependency graph: parallel work, serial
integration, gate checks at every integration. Do NOT run as one linear task.
Run it as written — each stage in its own fresh subagent, the reviewer gate
as configured — whoever the parent is (a person's session or an agent running
it for its caller). Whether staging pays off was settled when the plan was
approved; a track that looks small still runs this way.

0. **Pre-execution gate** (mandatory, before launching any stage). Run:
   ```
   python3 -B docs/plans/_verify.py gate docs/plans/lint-debt-paydown.md
   ```
   (Use `python` where `python3` is absent, common on Windows.) It checks
   placeholders, the stage graph, one proof per check, stage sizes, that the
   parent's part and every brief fit one read, and that no line passes 160
   characters; it ends with the stage index. If non-zero, abort and surface the offending lines. Fix or delete
   the flagged blocks; do NOT bypass. `[WARN]` lines do not block.
1. **Parent** reads the plan down to `## Stages` in one read, plus the stage
   index the gate prints in step 0 — enough to schedule the graph. It reads a
   stage's block when it launches or integrates that stage. **Subagents** read
   one document: the brief their prompt names (step 3), never the plan.
   **Run log:** `docs/plans/logs/lint-debt-paydown-run.md` is the parent's memory of the
   run. Append one line per event:
   ```
   python3 -B docs/plans/_verify.py run-log docs/plans/lint-debt-paydown.md <N> <event> "<detail>"
   ```
   Events: `queued` (ready, waiting for a free slot), `launched` (tree, branch,
   brief path, and the executor's handle for the subagent — its id or output
   file), `returned`, `verifying` (integration gates started), `integrated`
   (SHA, once every gate is green), `red`, `stalled` (launched, then silent past
   the window), `retry`, `halted`. After a compaction, a resume, or any doubt
   about what is in flight, read the run log and `git worktree list` before
   the next action; a conversation summary is not a source.
2. Run Stage 0 (Pre-flight) yourself — it is mechanical and needs no
   subagent. If any gate is red on the baseline, abort.
3. **Launch the frontier.** A stage is *ready* when every stage its
   `Depends on:` names is integrated green. Launch ready stages longest
   `chain` first (the gate's stage index), then in plan order, never more in flight than `Concurrency` (`## Execution policy`), each as a
   fresh subagent (see `## Executor adapter`), and log `launched` (a ready
   stage with no free slot logs `queued`):
   - prompt: the one line printed by
     `python3 -B docs/plans/_verify.py brief docs/plans/lint-debt-paydown.md <N>`, passed
     verbatim. The command writes the stage's brief (`## Hand-off prompt`
     filled, the stage block, the plan sections the stage works under) to
     `docs/plans/logs/lint-debt-paydown-stage-<N>-brief.md` (a retry gets `-brief-2.md`, and
     so on: one record per attempt); the line tells the subagent to read it
     in one read
   - description: the stage title
   - `model` selected per Tier/Effort (see `## Executor adapter` mapping table)
   - **in place** (foreground, the main working tree) when it is the only
     stage in flight at launch; otherwise **isolated**: background, in its own
     worktree of the current `HEAD` on a branch of its own
     (`staged/lint-debt-paydown/stage-N` when you create it; whatever name the
     executor's isolation option reports otherwise — record it), under
     `## Execution policy` § Worktree root. Run `## Global conventions`
     § Worktree setup there first, and write the brief with
     `--workdir <worktree path>`.
   Before the first isolated launch, add a Worktree root inside the repo to
   `.git/info/exclude` unless `.gitignore` already ignores it, so the
   clean-tree checks keep meaning something.
   The gate in step 0 guarantees two ready stages never declare the same file:
   a shared file forces a dependency between them.
4. On each return, **integrate** — one stage at a time, in plan order among
   the stages that have returned:
   - isolated: `git cherry-pick <stage SHA>` onto the main working tree. A conflict means
     the stage touched a file outside its list: `git cherry-pick --abort` and
     treat it as a scope violation.
   - then log `verifying` and verify **independently** at the integrated `HEAD` — the subagent's
     reported results are a claim, not evidence. Re-run the stage's proofs
     yourself — its verify script, or every `Proof:` of its `**Checks:**` list
     run as its own command, never folded into one file-level run — and
     confirm each named test appears in the output as run — a filter
     matching nothing exits 0 on several runners. Then: build + every gate of
     `## Global conventions` clean (this is what catches two stages that edit
     different files but break each other), commit present in `git log`,
     post-stage report written, scope respected (only declared files touched).
     The bar holds: the test count is at least the previous integration's, and
     every limit and ratchet is where the plan set it. A drop or a raise is a
     ruling — yours, with its cause in the run log — never the stage's.
   - a **pilot** (`## Procedures` names its first stage): before launching the
     stages that depend on it, fold what it learned into the recipe and the
     gates — the later stages read the corrected recipe in their briefs.
   - red after the stage reported green: `git reset --keep HEAD~1` drops the
     pick, then the retry rule applies, from the current `HEAD`.
   - green: log `integrated`, copy `docs/plans/logs/` (and the stage report,
     when the report policy is `gitignored`) from the worktree into the main
     tree, then `git worktree remove --force <dir>` and
     `git branch -D <stage branch>` (`-d` refuses: the cherry-picked commit
     has a different SHA).
   - **stalled:** a stage is running only while its record shows activity —
     the executor's handle (transcript or output file) or its tree
     (`git log`, `git status`) changed within the `Stall window` of
     `## Execution policy`. Check both before reporting a stage as running.
     A parent that ends its turn to wait keeps a wake-up armed — a background
     command that ends after the window (`sleep 1200`, or the executor's
     timer), re-armed at each wake — so a silent stage still wakes it. Once no
     stage is in flight, stop the wake-up: left running, it holds the session
     open until the window ends.
     Past the window with no return, the stage is stalled: log `stalled` with
     its tree's new commits and `git diff --stat`, stop the subagent if the
     executor can, and treat the stage as red. Its uncommitted edits go with
     its worktree. When its commits pass the stage's gates at their tip, the
     retry starts from that tip; otherwise from the current `HEAD`. The retry
     prompt carries the partial report and the diff stat as
     `Tried and abandoned`.
5. Green -> Mode handling:
   - autonomous: recompute the ready stages and launch them (step 3).
   - semi-autonomous: Concurrency is 1. Post the post-stage summary +
     `Resume? [y / edit / abort]` and wait. `y` -> launch the next ready
     stage; `edit` -> user adjusts the next stage's block (its brief is written from it at launch) then `y`; `abort` -> stop
     (committed work is preserved).
   Red -> log `red`, then apply the `## Execution policy` retry rule (log
   `retry` at each relaunch, `halted` when execution stops for a human). A red stage holds only
   the stages that descend from it through `Depends on:`; the others keep
   launching.
6. After the final stage, stop any wake-up still armed, run `## End-to-end verification`, run the
   `## Reviewer gate` if not `none`, and fill `## End-to-end summary` (the
   `Commit SHA` column is the integrated SHA). Commit the filled plan and the
   reviewer output together as the closing commit
   `docs(plans): close lint-debt-paydown with end-to-end summary`, so the tree ends
   clean. Then list any externally-blocked items still open (with reopen criteria) and,
   if the working-tree policy was `stash-authorized`, remind the user to
   `git stash pop` (name the stash ref).

Parent responsibilities (not delegable): launching ready stages, integrating
them one at a time, verifying green at every integration, running end-to-end
verification, running the reviewer gate if configured, producing the summary.
The parent alone writes to the main working tree while isolated stages run.

Resuming after a red stage or a lost session: read the run log first. Each
hand-off prompt only assumes prior commits exist in `git log`, not that they
came from subagents. If Stage K was fixed manually, its dependents become
ready unchanged. Never re-run integrated stages; remove the leftover worktrees
and branches of unintegrated stages (`git worktree list`) before relaunching.

### Resource selection vocabulary (read before launching each stage)

Each stage declares `Tier:` (cognitive load) and `Effort:` (reasoning budget).
The executor at runtime maps these to the cheapest viable resource on its
platform that meets BOTH dimensions. The plan does NOT name models — that is
the executor's responsibility (it knows its own lineup and pricing).

**Tier:**
- `mechanical` — literal execution of a well-specified hand-off (rename, move,
  apply pattern from list). Smallest model that can follow the instruction.
- `standard` — typical coding within the declared file list, light judgment.
- `judgment` — scope decisions, semantic synthesis, non-obvious refactors.
- `critical` — security, public contract, data migration, irreversible changes.

**Effort:**
- `minimal` — no extended reasoning; cheapest setting.
- `standard` — default reasoning budget.
- `extended` — maximum reasoning budget the executor offers.

**Selection rule:** pick the cheapest model × reasoning combo on your platform
that meets or exceeds the declared Tier and Effort. Do NOT auto-promote on
retry — if a `mechanical` stage fails twice, the classification was wrong;
STOP and replan rather than silently escalating to a bigger model.

**Role defaults** (apply when not overridden by a stage block):
- Parent / orchestrator: `standard / standard`
- Stage 0 (pre-flight gates): `mechanical / minimal`
- Reviewer gate: `critical / extended`
- Stage N >= 1: declared per stage; absence defaults to `standard / standard`

## Execution policy (fixed defaults unless user overrode)
- Mode: autonomous
- Concurrency: 1 — one stage at a time, in place.
- Commit authorization: per-stage-direct (an isolated stage commits on its worktree branch; the
  parent cherry-picks it)
- On red: auto-retry-up-to-2 — cap of 2 retries; each retry passes the prior failure excerpt plus
  the subagent's `Tried and abandoned` list, and narrows the instruction to the same file list; an
  isolated retry starts from a fresh worktree of the current `HEAD`. NEVER retry on scope
  violations, pre-commit hook rejections, test-weakening, or hook bypass attempts (escalate
  immediately). Never promote the stage's Tier on retry: a `mechanical` stage failing twice means
  the classification was wrong — stop and replan. On exhaustion: stop launching the red stage's
  descendants, keep running and integrating the stages that do not descend from it, and surface once
  nothing else can run.
- Stall window: 20 min — longer than the longest single command an executor runs in the foreground;
  a stage whose record and tree stay unchanged past it is stalled (`## Execution model` step 4).
- Working-tree policy: clean-required — per-state behavior is described inline in `## Stage 0`.
- Reviewer: light  # 10 stages
- Human-interaction stops: an escalation (scope violation, hook bypass, dirty tree, reviewer
  `fail`/`blocked`) launches nothing new — the stages in flight return and integrate, then execution
  halts; retry exhaustion halts once nothing else can run (see On red). Whenever execution halts
  awaiting a human decision (reviewer verdict `fail`/`blocked` after fix round; retry exhausted;
  scope violation / hook-bypass escalation; dirty-tree mid-run), the executor MUST send a push
  notification BEFORE yielding control (Claude Code: `PushNotification`; otherwise the executor's
  equivalent). Message format: `"Plan <slug>: <reason> — see <report-or-file path>"`, under ~120
  chars. With no notification tool, print the same message inside a `<<HUMAN_REQUIRED>>` ...
  `<<END>>` block so an outer harness can grep for it; an autonomous parent (an agent running this
  plan for its own caller) ends its final message with that block, so the caller sees it.
  `pass-with-pending` is not a stop: the run completes, and the `human-required` items travel in the
  final message. Do NOT notify on normal between-stage transitions or successful completion.

## Plan landing commit (mandatory before Phase 2)
Before launching Stage 1, the planner (NOT a subagent) makes a single commit
that lands this plan and its support artifacts. This is plan setup, not
feature work — isolating it here keeps Stage 0 and Stage 1+ scope-clean.

**Pre-check (mandatory):** before staging anything, inspect `C:\Dev\gh-get/.gitignore`.
The Plan landing commit assumes `docs/plans/` is **trackable**. Two cases:

- If `.gitignore` ignores `docs/plans/` wholesale (e.g. a `docs/plans/` line),
  **narrow the rule to ignore only generated artifacts**: replace that line
  with `docs/plans/logs/` and `docs/plans/__pycache__/`. The plan file,
  `_verify.py`, and verify scripts MUST be versioned; only gate logs and the
  bytecode cache Python writes when a script imports `_verify` are excluded.
  Do NOT use `git add -f` to bypass — the rule itself needs fixing.
- If `.gitignore` does not ignore `docs/plans/`, append `docs/plans/logs/`
  and `docs/plans/__pycache__/` where not already covered — the cache would
  otherwise dirty the tree on the first verify run and fail `clean-required`.
- If `.gitignore` ignores report files (e.g. `docs/plans/*-report.md`) while
  this plan's `Report policy` is `committed`, STOP: re-scaffold with
  `--report-policy gitignored`, or remove the pattern if reports should be
  versioned. A mismatch fails `clean-required` after the first stage.

The landing commit MUST contain:
1. `C:\Dev\gh-get/docs/plans/lint-debt-paydown.md` — this plan file.
2. `C:\Dev\gh-get/docs/plans/_verify.py` — vendored verify primitives, written by
   `scaffold.py` (a missing or older copy is replaced; stage it even when the
   only change is the refresh). Stage scripts import it via
   `sys.path.insert(0, 'docs/plans'); from _verify import V`.
3. `C:\Dev\gh-get/docs/plans/_report-template.md` — scaffolded alongside the plan;
   subagents copy it as the starting structure for post-stage reports.
4. Any `C:\Dev\gh-get/docs/plans/lint-debt-paydown-verify-stage-N.py` and
   `C:\Dev\gh-get/docs/plans/lint-debt-paydown-verify-e2e.py` scripts the plan declares.
5. `C:\Dev\gh-get/.gitignore` with the narrowed/added rule from the pre-check above
   (plus the report-ignoring pattern when report-policy = `gitignored`).

Suggested subject:
`chore(plans): land lint-debt-paydown staged plan + verify scripts`

After this commit, working tree is clean and Phase 2 starts.

## Logs policy
Gate execution logs are written to `C:\Dev\gh-get/docs/plans/logs/<prefix>-<ts>.log`
on every `run_gate()` call. They are **local evidence artifacts, not
versioned**: `docs/plans/logs/` is gitignored via the Plan landing commit.
Reports (committed alongside each stage) capture the deviations and
judgments needed for PR review; raw logs are kept locally for forensics.

## Executor adapter

Each stage runs in a fresh context window via whatever delegated-agent
mechanism the executor provides. Claude Code: `Agent` tool with
`subagent_type: general-purpose`; an isolated stage runs in the background —
launch every ready stage in one message so they run concurrently. When the
Worktree root is `.claude/worktrees/`, where `isolation: "worktree"` places its
trees, add that option and take the path and branch from the tool's result;
under any other root, create the tree as below and launch without it. Codex /
others: the equivalent fresh-window mechanism plus
`git worktree add <Worktree root>/<slug>-s<N> -b staged/<slug>/stage-N HEAD`. Record the worktree
path, branch, and the subagent's handle (Claude Code: the agent id and its
output file) in the run log at launch. An executor without
background delegation or without worktrees runs at Concurrency 1: in place,
one stage at a time, inline in a clean session if no delegate mechanism
exists.

**The parent's own context:** the parent holds the run for hours of background
stages. Where the executor compacts an idle conversation on its own, turn that
off for the run (Claude Code: `"idleCompaction": false` in `settings.json`).
After any compaction, the run log comes before the next action
(`## Execution model` step 1).

**Model & effort selection:** the plan declares `Tier:` and `Effort:` per stage
(see `## Execution model` § Resource selection vocabulary). The executor maps
those to its own model lineup, picking the cheapest viable combo. The plan
itself names no model — only the executor knows what's available and what it
costs.

**Mapping for Claude Code** (pass as `model` argument to the `Agent` tool):

| Tier / Effort | `model` |
|---|---|
| mechanical / minimal | `haiku` |
| mechanical / standard | `haiku` |
| standard / minimal | `sonnet` |
| standard / standard | `sonnet` |
| standard / extended | `sonnet` or `opus` |
| judgment / standard | `opus` |
| judgment / extended | `opus` |
| critical / * | `opus` |

Do NOT omit `model` (omission inherits the parent's model and defeats the
cost-tiering — every stage would silently run on the parent's model).

**Any other executor:** the same shape over its own lineup — `mechanical` on
its smallest model, `standard` on its middle one, `judgment` and `critical` on
its largest; `Effort` maps to its reasoning-effort setting where it has one.
An executor with a single model runs every stage on it and says so in the run
log at the first launch.

Roles when no stage-level override is present:
- Parent / orchestrator: `standard / standard`
- Stage 0: `mechanical / minimal`
- Reviewer gate: `critical / extended`
- Stage N >= 1: as declared; default `standard / standard`

## Hand-off conventions (apply to every stage)

**Authorization:**
- MAY commit directly after all verifications pass — on the worktree's
  branch when the prompt names a `Working directory:`.
- MAY NOT push.
- MAY NOT write outside your working tree — the worktree, when the prompt
  names one; the main working tree belongs to the parent while stages run in
  parallel.
- MAY NOT modify files outside the stage's declared file list.
- MAY NOT touch pre-existing unrelated working-tree edits.
- MAY NOT skip gates or use --no-verify / bypass hooks.
- MAY NOT spawn nested subagents (no Agent calls inside this stage).
- MAY NOT weaken an assertion, delete a test, skip one, or raise a limit
  (a ratchet's max, a count a check allows) to turn a gate green. The stage's
  `**Checks:**` are the bar you build under, fixed for the whole stage: if a
  check, a limit, or an existing test is genuinely wrong, STOP and report.
  A paragraph that justifies a workaround is that STOP, written as a comment.
- MAY NOT change the machine outside the tree — install or update a
  toolchain, edit global config, start or stop a service: report what you
  need, and the parent provides it. A tool call denied by a permission check
  is the same: stop that line of work and report the exact call and why.

**Checks and proofs:**
- Tests assert what the check says, never what the code happens to do —
  write each new proof from the check, not by reading the implementation.
- A proof that is a new test: write it first and watch it fail for the right
  reason (red), then implement until it passes.
- Every `Proof:` must be seen to run: the named test appears in the runner
  output as executed and passed, not just a green suite.
- Re-run a command only after the code changed since its last green run; a
  repeat on unchanged code adds no information.
- Wait for a long command with one foreground run whose timeout covers it, or
  with its completion notification. The parent reads a record silent past the
  stall window of the run (20 min by default) as a dead stage.

**Scope discipline:**
- If the stage appears to require files outside the declared list, STOP and
  report. Do NOT silently expand scope.
- If pre-existing test/build failure is unrelated to this stage, STOP and
  report. Do NOT fix it.
- Improvements noticed outside the stage: list them under `Noticed, not
  touched` in the report and leave the code as it is.

**One-way doors found mid-stage:** a hard-to-reverse choice the plan's
`## One-way doors` does not cover (persisted schema, a contract someone else
consumes, a new dependency, a pattern the codebase does not have yet) is
yours to decide: record it under `Decisions` in the report — literal shape +
the alternative rejected and the property that disqualified it — **before**
writing the code that closes it. A choice that contradicts a door the plan
already fixed is a STOP.

**Failure protocol:**
- Gate fails within declared scope -> fix within scope and re-run the gate.
- Any STOP condition above -> return to parent with a clear reason.
- Context compacted mid-stage -> re-read your stage block and `git diff`
  before continuing.

**Return to parent:**
- Per-file summary with actual grep-found locations.
- Check results: check id -> proof run -> pass/fail.
- Gate results (pass/fail + snippets).
- Commit SHA + subject.
- Deviations from the plan, if any.
- Tried and abandoned: approaches discarded and why (mandatory on a red
  return — the parent passes it into the retry so the next attempt starts
  past the dead ends).
- Path to the post-stage report written to disk.

## Context
Why: `docs/lint-transition.md` lists 425 lint sites as debt (`.golangci.yml` `disable` list, gosec/gocritic ids) after the lint gate landed
(PR #2). This track pays Phases 1-5 of that plan so the end state holds: every linter an error, no `debt:` comment left.

Constraints: every fix keeps behavior and the text the CLI prints; the module stays stdlib-only (`go.mod` and `go.sum` unchanged); a
linter leaves `disable` only in the stage that brings its sites to 0; `debt.py shrink` (inside `scripts/check.sh`) never allows the list to grow.

In scope: the five phases of the transition plan, plus decisions D4 (depguard on), D3/D7/D8 (capability rules, tests excluded from goconst
and gosec). D1 and D2 were taken before this track.

Out of scope (needs tools or settings this machine cannot run): govulncheck in the gate (Defender quarantines the build locally), gitleaks,
typos, actionlint/zizmor, jscpd (D6), the per-edit agent hook, branch protection, and pinning actions by SHA. D5 stays: `paralleltest` and
`tparallel` remain noise, off.

Concurrency is 1 on purpose: the stages form a chain through `.golangci.yml`, `main.go` and the other root files (the lint sites
overlap), so a worktree per stage would only add merge work. Fresh context per stage and the green-to-green gate still apply.

Size: the stages' summed `Size` is above the ~30k threshold; the track is staged because ~425 sites across 28 files do not fit one session
of context. Branch: `lint-phases`; open one pull request at the end.

## Swept (requirements the source did not state)
Each theme resolves to the check ids that cover it, to `existing: <where the
code already enforces it>`, or to `not in scope — <why>`. Every theme gets an
answer; none is passed over in silence.
- validation: not in scope — behavior-preserving; only Stage 8 touches path handling and keeps the accepted inputs
- failure modes: C4.1, C4.2, C5.1, C5.2 — ignored and unwrapped errors reach the caller
- idempotency / retry: not in scope — no retry or write semantics change
- authorization: not in scope — the tool has no accounts; the token handling is untouched
- concurrency / ordering: existing: `go test -race -shuffle=on` in `scripts/check.sh` runs at every stage
- data lifecycle: not in scope — nothing new is persisted; Stage 8 keeps file modes as they are or stricter
- external-dependency failure: C5.1, C5.2 — HTTP and file errors are no longer dropped; `noctx` (C7.3) adds the context to requests
- state transitions: not in scope — no state machine is touched
- observability: C6.1 — all output goes through the writers `main` passes in, so tests can read it

## One-way doors
Choices whose reversal costs more than a refactor: a persisted schema, a
contract someone else consumes, a new dependency, a data backfill, a pattern
the codebase does not have yet. Each row: the literal shape the next person
will copy, and the alternative rejected — named with the property that
disqualified it ("cannot express the next state", never "cleaner"). Stages
build to these rows; a stage that finds a new door records it in its report.

- **Sentinel errors** — shape: package-level `var ErrX = errors.New("...")` in the package that owns the condition, wrapped with
  `fmt.Errorf("...: %w", ErrX)`, matched with `errors.Is`. Rejected: a custom error type per condition — more code for conditions no caller
  inspects beyond identity; the codebase has neither convention yet.
- **Writers in signatures** — shape: `func run(args []string, stdout, stderr io.Writer) error`, `main` passes `os.Stdout`/`os.Stderr`.
  Rejected: a package-level `var out io.Writer` — `gochecknoglobals` forbids it and tests would share state.
- **Test exclusions** — shape: `exclusions.rules` entry `path: _test\.go`, `linters: [err113, wrapcheck, goconst, gosec]`.
  Rejected: fixing 26 goconst and 12 gosec findings in tests — repeated literals and temp-file reads there are readable, not defects.
- **Exceptions** — shape: `//nolint:<linter> // <why>`, enforced by `nolintlint`. Rejected: path-based exclusions in production code, which
  carry no reason at the site.
- **Cmd files** — shape: `cmd_<command>.go` in the root package, one per subcommand. Rejected: a `cmd/` or `internal/cli` package — it would
  change import paths and the release build for a size problem.

## Alternatives considered
One stage per linter (about 40 stages): rejected, the per-stage reading overhead exceeds the work. One stage per scope (root, then each package):
rejected, a linter can only leave `disable` when every scope is at 0, so every stage would edit the same lines of `.golangci.yml`.

## Global conventions
- Build gate: `go build ./...`
- Lint/test gates: `bash scripts/check.sh` (format, tidy, vet, lint, debt-list shrink, deadcode, build, `go test -race -shuffle=on`), plus the
  stage's own proofs. It must print `ok` on every row at the end of every stage.
- Invariants: behavior-preserving (CLI flags, exit codes and every printed message stay as they are; a test may change only to follow a signature);
  stdlib only (`git diff <landing>..HEAD -- go.mod go.sum` is empty); English only; a test is never weakened, skipped or deleted; an exception is
  `//nolint:<linter> // <why>` and never bare; `.golangci.yml` changes only the lines the stage names, and the debt list only shrinks.
- Stage commit subject: `lint(stage N): <short title>`, with the stage report in the same commit.
- Flipping order inside `.golangci.yml`: after editing, run `bash scripts/check.sh`; `debt.py shrink` compares with the merge base.
- Commit style: ONE commit per stage that includes BOTH the code changes AND
  the post-stage report file. The report is staged alongside code; there is
  no separate "report commit". Trailer:
  `Co-Authored-By: $EXECUTOR_NAME $EXECUTOR_EMAIL` — the agent and model
  that wrote the stage, as the executor names itself (its own attribution
  rule wins); an executor that cannot name itself leaves the trailer out.
- Report content: do NOT include the stage's own commit SHA in the report
  body (impossible: the file is part of the commit). The parent emits the
  canonical stage->SHA mapping in the End-to-end summary table.
- Staging: only files the stage declares PLUS the stage's own
  `lint-debt-paydown-stage-{N}-report.md`, by explicit path; never `git add -A`.

## Procedures
### Flip a linter
Pilot: Stage 2
1. In `.golangci.yml` delete the linter's entry and the `# debt: N site(s)` comment on the line above it. For a check id (gosec `excludes`,
   gocritic `disabled-checks`) delete the id and its comment; when the list is then empty, delete the key too.
2. `bash scripts/lint-only.sh <linter>` prints `0 issues.` (for a check id: the whole linter's output does not name the id).
3. Append one line to `## Progress` of `docs/lint-transition.md`: date, stage, what was flipped.
4. `bash scripts/check.sh` is green. Do not touch any other debt entry or its count.

### Fix a dropped value
1. Error with no reader (`errcheck`): handle it. Production code returns it to the caller or logs it; a deferred `Close` on a read path goes
   through a helper that returns or logs the error; tests check with `if err != nil { t.Fatal(err) }`.
2. Dynamic error (`err113`): a sentinel `var ErrX = errors.New(...)` in the package that owns the condition, used as
   `fmt.Errorf("context: %w", ErrX)`; the printed text stays identical.
3. Unwrapped error from another package (`wrapcheck`): `fmt.Errorf("doing X: %w", err)` once, at the package boundary, not at every level.
4. Every site where a failure became visible is named in the stage report.

### Fix a shape
1. Complexity: guard clauses and early returns, a table of cases or a `switch` over a tuple, or a split along a responsibility the code
   already has (parse arguments / run the command / print the result). A helper with no responsibility of its own is not a fix.
2. Keep the exported behavior; tests stay as they are, except where the split moves a function they call.
3. Re-measure with the low-limit config from the stage and put the before/after count in the report.

## Hand-off prompt
One brief serves every stage. At launch the parent runs
`python3 -B docs/plans/_verify.py brief docs/plans/lint-debt-paydown.md <N>` (plus
`--workdir` and the worktree path for an isolated stage). It writes the stage's
brief to `docs/plans/logs/` — this prompt with `{{N}}`, `{{TITLE}}` and
`{{ITEMS}}` filled, then the stage block, then the plan sections the stage works
under — and prints the one-line prompt the parent passes to the subagent.

<!-- BEGIN HANDOFF TEMPLATE -->
```text
You are executing Stage {{N}} of Lint debt paydown (phases 1-5).
This brief is your whole hand-off, in one document: this prompt, then your stage
block (your authoritative spec), then the sections of the plan
C:/Dev/gh-get/docs/plans/lint-debt-paydown.md you work under, copied when you were launched. The plan
file itself is the parent's; everything you need from it is below. Then read the
repo's agent instruction files for repo-wide rules, whichever exist:
`C:\Dev\gh-get/AGENTS.md`,
`C:\Dev\gh-get/CLAUDE.md`,
or your executor's equivalent.

Repo root: C:/Dev/gh-get
Branch: lint-phases
Your working tree is `git rev-parse --show-toplevel` from where you start. When it
differs from the repo root above (or this prompt ends with a `Working directory:`
line), you are in an isolated worktree: that path replaces the repo root everywhere,
and you commit on the branch checked out there.
Platform: Windows 11  (Windows: use bash syntax, forward slashes)

Status: every stage your block's `Depends on:` names, and their own prerequisites,
is integrated green at your starting `HEAD` (`none`: you build on the plan landing
commit). Other stages may be integrated or still running elsewhere: build only on what
your dependencies wrote. Prior work is in `git log --oneline` and
`git show --stat <sha>`, in `Critical files` below, and, for a specific surprise or
deviation, in `docs/plans/lint-debt-paydown-stage-K-report.md`. Git is the source of truth.
Line-number hints in the plan may be stale; grep for symbols.

Your scope: Stage {{N}} only - {{TITLE}}. Items: {{ITEMS}}.

Run your block's Order of operations; a step naming a procedure runs that recipe from
`Procedures` below on the input the step gives. Then every proof of its Checks and
every gate of `Global conventions` passes, and the commit step below closes the stage.
Authorization, scope, failure and return-to-parent: `Hand-off conventions` below.

Commit step: copy `docs/plans/_report-template.md` to
`docs/plans/lint-debt-paydown-stage-{{N}}-report.md` and fill it (leave the `Commit:` slot as
`_filled by parent_`), then stage the code files AND the report together by explicit
path and commit with the `Co-Authored-By: $EXECUTOR_NAME $EXECUTOR_EMAIL` trailer.
One commit per stage.

Begin now.
```
<!-- END HANDOFF TEMPLATE -->

## Stage 0 - Pre-flight (mandatory, no feature work, no commit, no versioned report)
**Tier:** mechanical
**Effort:** minimal
Purpose: record baseline state and apply the working-tree policy so later
failures cannot be blamed on prior repo state. Plan support artifacts
(`_verify.py`, verify scripts, the plan file) are already committed via the
Plan landing commit before Phase 2 began.

**No versioned report:** Stage 0 must NOT write `lint-debt-paydown-stage-0-report.md`
under `docs/plans/` — that would leave the working tree dirty and conflict
with `clean-required`. Baseline evidence goes to the gitignored logs dir;
the human-readable summary is returned to the parent.

1. Capture `git status` and the current HEAD SHA. Write them to
   `C:\Dev\gh-get/docs/plans/logs/lint-debt-paydown-stage-0-baseline.log` (gitignored) and
   return the same summary to the parent.
2. Apply the working-tree policy from `## Execution policy`:
   - clean-required: tree must be clean; if not, abort.
   - stash-authorized: `git stash push -u -m "staged-plan-lint-debt-paydown-pre"`; record stash ref in the log + parent summary.
   - integrate-existing: leave changes in place; list them in the log + parent summary.
   - abort-until-clean: abort the plan; user resolves manually.
3. Run every gate (build, lint, tests, etc.) on the resulting baseline.
   `run_gate()` already writes its own per-command log under `docs/plans/logs/`.
4. Red -> abort. Green -> working tree must still be clean (or match the
   integrate-existing manifest); proceed to Stage 1.

## Reviewer gate (only if Reviewer != none)
**Tier:** critical
**Effort:** extended
After the final stage commits green, the **parent** dispatches a fresh
reviewer subagent — never a stage subagent, never a child of one — over the
whole plan: diff range `<plan landing commit>..HEAD` and **every** check of
every stage block, whoever wrote them.
- reviewer: light -> accounts for every check: the proof ran green at `HEAD`
  and the assertion that settles it is located (`file:line` + the assertion
  expression, targeting the value the check names). A check without a located
  assertion counts as not proven. Plus scope, diff vs. plan, post-stage
  reports, `## Swept` / `## One-way doors` honoured, and obvious risk.
- reviewer: deep -> same plus a security/perf/maintainability lens for
  stack-relevant best practices, plus **fault injection**: in a scratch
  worktree (`git worktree add <scratch> HEAD`; never `git stash`), inject one
  behaviour-level fault per distinct assertion surface (flip a condition,
  change a returned value, shift a bound by one; cap 5), run the narrowest
  covering proof, confirm it FAILS, discard the scratch. A surviving mutant is
  a finding.

Reviewer returns a verdict in
`pass | pass-with-notes | fail | blocked`
plus a check table (check id, proof run, `file:line` evidence, result) and a
findings list (each: `file:line`, severity, description).
Reviewer never edits code and never replans.
If a `reviewer` skill is available in the executor, prefer it; otherwise use
an inline QA prompt that takes the plan + diff range as input.

### Arbiter (run only if findings list is non-empty)
**Tier:** critical / **Effort:** standard
Reads the reviewer verdict + findings + diff range. For each finding,
applies this decision tree verbatim:

1. Is this a real defect? (correctness, security, contract, data integrity)
   - No  -> classify `nice-to-have`.
   - Yes -> step 2.
2. Is the fix mechanical (one obvious right answer, no design choice)
   AND fully inside the plan's declared file list?
   - Yes -> classify `must-fix`.
   - No  -> classify `open`.

Arbiter records both answers per finding in the output md (auditable).
Arbiter does NOT edit code and does NOT replan.

### Fix round (run only if any `must-fix` exists; HARD MAX = 1 round)
**Tier:** standard / **Effort:** standard
Fix-subagent receives only the `must-fix` items + their target files.
- Applies fixes within those files only.
- For each item: writes a `fix note` -- what changed, line(s), and one
  sentence linking the change to the original finding.
- If a `must-fix` proves to require out-of-scope work or a design choice,
  reclassify it as `open` and skip it. Do NOT block sibling fixes.
After the round: re-run every declared gate (build/lint/test/etc.). Gate
failure does NOT trigger another fix round -- the failure goes into the
pending list and the verdict degrades.

### Re-review (conditional)
Run a second reviewer pass (same level: light/deep) only if EITHER:
- The plan's Tier was `critical`, OR
- The fix round modified files outside the declared scope of the
  originating `must-fix` finding (scope-creep signal).

Any *new* finding from re-review is resolved as below, in the next sequence
file. Re-review does NOT trigger another fix round.

### Resolve before escalating (parent)
Every `open` finding, and every question the reviewer leaves open, is the
parent's to answer first — the code, the plan, the repo's docs and git
history usually hold the answer. Record each answer with its evidence under
`## Resolved by the parent`; an answer that turns into a mechanical in-scope
fix is applied and re-gated like a fix-round item.
A finding reaches a human only when acting on it risks one of:
- a regression for someone who depends on today's behaviour;
- loss or corruption of data;
- a high-risk or irreversible change (security, public contract, migration);
- context nothing in the repo or plan provides.
Those become `human-required`, each naming its criterion. When unsure, the
cost of a wrong guess decides: one of the four above goes to the human,
anything cheaper to reverse stays with the parent.

### Output file (always written when this gate runs)
Path: `docs/plans/reports/lint-debt-paydown_reviewer_<seq>.md`
- `<seq>` is a zero-padded 3-digit counter starting at `001`, incremented
  for each run of this gate against the same plan.
- Each sequence file is **immutable** once written. Re-runs produce a
  new file, never overwrite.

Top of file: final verdict in
`pass | pass-with-notes | pass-with-fixes | pass-with-pending | fail | blocked`

Body sections (in order):
- `## Reviewer verdict` -- raw reviewer output, verbatim.
- `## Arbiter classification` -- table per finding: `file:line`, severity,
  class (must-fix / nice-to-have / open), decision-tree answers
  (defect? yes/no -- mechanical+in-scope? yes/no), 1-line reason.
- `## Fixes applied` -- one entry per `must-fix` corrected, with the fix
  note (what changed, lines, link to finding).
- `## Resolved by the parent` -- every `open` finding the parent answered:
  the question, the answer, its evidence (`file:line`, command + output),
  and what changed if anything.
- `## Pending` -- every `human-required` item. Each entry:
  - `file:line`
  - reviewer's original finding (short quote)
  - the criterion that needs a human (regression / data loss / high-risk /
    missing context) and what the parent already established
  - the decision the human is asked to make

Verdict mapping:
- `pass` / `pass-with-notes` -- reviewer's original verdict, no findings
  needed fixing.
- `pass-with-fixes` -- all `must-fix` corrected, no pending items.
- `pass-with-pending` -- corrections completed (or none needed), but
  `Pending` holds `human-required` items.
- `fail` / `blocked` -- reviewer's verdict was `fail`/`blocked`, OR gates
  failed after the fix round. Parent stops the plan and surfaces (to the
  user, or to its caller when it runs autonomously) the md
  file path to the user.

## Critical files (cross-stage index)
| File | Stages |
|---|---|
| `.golangci.yml` | 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 |
| `AGENTS.md` | 5, 6, 10 |
| `cmd_*.go` | 9 |
| `docs/lint-transition.md` | 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 |
| `freeze.go` | 5, 6, 7, 8, 9 |
| `freeze_test.go` | 5, 6, 9 |
| `install.go` | 2, 5, 6, 7, 8, 9 |
| `install_test.go` | 5, 6, 7 |
| `internal/fetch/client.go` | 2, 3, 4, 7 |
| `internal/fetch/client_test.go` | 2, 3, 4, 7 |
| `internal/fetch/fetch.go` | 3, 4, 7, 10 |
| `internal/fetch/gitrefs.go` | 2, 4, 10 |
| `internal/fetch/gitrefs_test.go` | 4, 10 |
| `internal/fetch/membership.go` | 2, 3 |
| `internal/fetch/ref.go` | 4 |
| `internal/fetch/repo.go` | 4 |
| `internal/fetch/tarball.go` | 4, 10 |
| `internal/fetch/trees.go` | 3, 4, 10 |
| `internal/fetch/write.go` | 4, 8, 10 |
| `internal/fetch/write_test.go` | 10 |
| `internal/ghurl/url.go` | 4, 10 |
| `internal/ghurl/url_test.go` | 2, 3, 4, 10 |
| `internal/manifest/manifest.go` | 4, 8 |
| `internal/meta/meta.go` | 2, 4, 8 |
| `list.go` | 2, 5, 6, 7, 9 |
| `list_test.go` | 6 |
| `main.go` | 2, 5, 6, 7, 8, 9 |
| `main_test.go` | 2, 5, 6 |
| `scripts/lint-only.sh` | 1 |
| `selfupdate.go` | 5, 6, 7, 8, 9 |
| `selfupdate_test.go` | 5, 6 |

The chain is serial (Concurrency 1), so shared files need no ownership split.

## End-to-end verification (after final stage)
Run `python3 -B docs/plans/lint-debt-paydown-verify-e2e.py`: the whole gate passes, a plain `golangci-lint run ./...` reports 0 issues,
no `# debt:` comment is left, `go.mod` and `go.sum` are unchanged since the landing commit, and every `disable` entry is noise.
Manual smoke: `go run . --help` prints the usage text, and `go run . https://github.com/paulocorcino/gh-get/tree/main/docs/agents <tmpdir>`
downloads the folder (needs network).

## End-to-end summary (parent fills after final stage)
| Stage | Title | Tier | Effort | Model used | Checks proven | Commit SHA | Status | Report |
|-------|-------|------|--------|------------|---------------|------------|--------|--------|
<!-- one row per stage. `Model used` is what the executor actually selected
on its platform for the declared Tier/Effort (the executor fills this — the
plan never prescribes model names). Used post-hoc to audit whether the
platform mapping is well-calibrated. `Checks proven` is `proven/total` from
the parent's independent re-run of the stage's proofs. If <40% of rows are mechanical/standard,
the decomposition is suspect — too many stages classified as judgment/critical
defeats the cost-savings purpose. -->

## Stages
<!-- STAGES: the parent's one-pass read ends above. Each block below is read
when its stage is launched or integrated, and copied into that stage's brief. -->

<!-- BEGIN STAGE 1 -->
## Stage 1 - Turn on depguard (D4) and the per-linter probe
<!-- STAGE 1: tier-effort -->
**Tier:** mechanical
**Effort:** minimal
<!-- STAGE 1: tier-rationale -->
**Tier rationale:** Config plus a 10-line helper script; depguard has 0 sites by measure, nothing to judge.
<!-- STAGE 1: size -->
**Size:** ~16k tokens
**Depends on:** none
<!-- STAGE 1: items -->
**Items:** D4; helper `scripts/lint-only.sh`
<!-- STAGE 1: scope -->
**Scope:** `depguard` runs at error with a strict allow list (stdlib and this module), and `scripts/lint-only.sh <linter> [paths]` exists.

<!-- STAGE 1: files -->
**Files:**
- `scripts/lint-only.sh` - NEW, executable (`git update-index --chmod=+x`): runs one linter with the repo config, also while it sits in the debt list
- `.golangci.yml` - delete `depguard` from `disable` with its noise comment; add the strict `depguard` settings block
- `docs/lint-transition.md` - mark D4 taken in Decisions; add a Progress line

<!-- STAGE 1: order -->
**Order of operations:**
1. Create `scripts/lint-only.sh`: bash, `set -euo pipefail`, `cd` to the repo root, read the pin with `sed -n 's/^GOLANGCI_LINT_VERSION=//p'
   scripts/check.sh`, then `exec go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$VERSION run --enable-only "$1" "${@:2}"`, defaulting the
   paths to `./...` when none is given. Run `git update-index --chmod=+x scripts/lint-only.sh` after `git add`.
2. In `.golangci.yml` delete the `# noise: decision first: on with the rules of the layers (D4)` line and the `- depguard` line below it.
3. Under `linters.settings` add `depguard` with `rules.main`: `list-mode: strict`, `files: [$all]`, `allow: [$gostd, github.com/paulocorcino/gh-get]`.
4. In `docs/lint-transition.md` Decisions, change D4 to `(taken)`; add one Progress line.

<!-- STAGE 1: checks -->
**Checks:**
- C1.1 — `depguard` reports 0 issues · Proof: `bash scripts/lint-only.sh depguard`
- C1.2 — the stage's debt entries left `.golangci.yml` · Proof: verify script: depguard (+0 more) gone from .golangci.yml
- C1.3 — the helper takes a linter and paths: `godox` on `./internal/...` reports 0 issues · Proof: `bash scripts/lint-only.sh godox ./internal/...`
Verify script: docs/plans/lint-debt-paydown-verify-stage-1.py

<!-- STAGE 1: manual -->
**Manual verification (if any):** none
<!-- END STAGE 1 -->
---

<!-- BEGIN STAGE 2 -->
## Stage 2 - Mechanical fixes by command
<!-- STAGE 2: tier-effort -->
**Tier:** mechanical
**Effort:** minimal
<!-- STAGE 2: tier-rationale -->
**Tier rationale:** Tool-applied safe fixes on a list the linter prints; the only judgment is reading the diff.
<!-- STAGE 2: size -->
**Size:** ~42k tokens
**Depends on:** Stage 1
<!-- STAGE 2: items -->
**Items:** phase 1 linters: perfsprint 15, modernize 4, intrange 1, canonicalheader 1, usetesting 2
<!-- STAGE 2: scope -->
**Scope:** Five Surface linters reach 0 sites and leave the debt list.

<!-- STAGE 2: files -->
**Files:**
- `main.go` - perfsprint (9 sites)
- `install.go` - perfsprint (1)
- `list.go` - perfsprint (3)
- `main_test.go` - usetesting (2)
- `internal/fetch/gitrefs.go` - perfsprint (1), modernize (1)
- `internal/fetch/client.go` - canonicalheader (1)
- `internal/fetch/client_test.go` - modernize (1), intrange (1)
- `internal/fetch/membership.go` - modernize (1)
- `internal/meta/meta.go` - modernize (1)
- `internal/ghurl/url_test.go` - perfsprint (1)
- `.golangci.yml` - the Flip a linter procedure, five times
- `docs/lint-transition.md` - Progress lines

<!-- STAGE 2: order -->
**Order of operations:**
1. For each linter in `perfsprint, modernize, intrange, usetesting`: measure with `bash scripts/lint-only.sh <linter>`, apply `go run
   github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run --fix --enable-only <linter>` (safe fixes), read the diff, then run the procedure Flip
   a linter. One local commit is not needed: the stage commits once, but keep the order so a failure is traceable.
2. `canonicalheader`: fix by hand (the header key spelled in canonical form); confirm the request sent is unchanged.
3. Procedure Flip a linter for each of the five.

<!-- STAGE 2: checks -->
**Checks:**
- C2.1 — `perfsprint` reports 0 issues · Proof: `bash scripts/lint-only.sh perfsprint`
- C2.2 — `modernize` reports 0 issues · Proof: `bash scripts/lint-only.sh modernize`
- C2.3 — `intrange` reports 0 issues · Proof: `bash scripts/lint-only.sh intrange`
- C2.4 — `canonicalheader` reports 0 issues · Proof: `bash scripts/lint-only.sh canonicalheader`
- C2.5 — `usetesting` reports 0 issues · Proof: `bash scripts/lint-only.sh usetesting`
- C2.6 — the stage's debt entries left `.golangci.yml` · Proof: verify script: perfsprint (+4 more) gone from .golangci.yml
Verify script: docs/plans/lint-debt-paydown-verify-stage-2.py

<!-- STAGE 2: manual -->
**Manual verification (if any):** none
<!-- END STAGE 2 -->
---

<!-- BEGIN STAGE 3 -->
## Stage 3 - Surface fixes by hand
<!-- STAGE 3: tier-effort -->
**Tier:** standard
**Effort:** standard
<!-- STAGE 3: tier-rationale -->
**Tier rationale:** Small hand edits in 6 files; containedctx changes a signature, so callers need a grep.
<!-- STAGE 3: size -->
**Size:** ~25k tokens
**Depends on:** Stage 2
<!-- STAGE 3: items -->
**Items:** nonamedreturns 5, thelper 2, revive:unused-parameter 2, errname 1, embeddedstructfieldcheck 1, containedctx 1
<!-- STAGE 3: scope -->
**Scope:** Six Surface/Shape linters reach 0 sites; `revive` stays listed only for argument-limit (2 root sites, Stage 9).

<!-- STAGE 3: files -->
**Files:**
- `internal/fetch/membership.go` - nonamedreturns (2): explicit locals instead of named results
- `internal/fetch/trees.go` - nonamedreturns (3)
- `internal/fetch/fetch.go` - errname (1): error type renamed `...Error`; grep every use
- `internal/fetch/client.go` - embeddedstructfieldcheck (1), containedctx (1): pass ctx as a parameter, grep every caller
- `internal/fetch/client_test.go` - revive:unused-parameter (1): name the parameter `_`
- `internal/ghurl/url_test.go` - thelper (2): `t.Helper()` first; unused-parameter (1)
- `.golangci.yml` - Flip a linter, five times
- `docs/lint-transition.md` - Progress lines

<!-- STAGE 3: order -->
**Order of operations:**
1. For each linter in `nonamedreturns, thelper, errname, embeddedstructfieldcheck, containedctx`: list the sites with `bash scripts/lint-only.sh <linter>`
   and fix them by the Surface recipe (rename or move; no behavior change). A named result a `defer` writes to becomes an explicit local, not an exception.
2. `revive` unused-parameter: fix the 2 sites in `./internal/...` (the other revive rule, argument-limit, stays for Stage 9). Do NOT remove `revive` from
   `disable`.
3. Procedure Flip a linter for each of the five.

<!-- STAGE 3: checks -->
**Checks:**
- C3.1 — `nonamedreturns` reports 0 issues · Proof: `bash scripts/lint-only.sh nonamedreturns`
- C3.2 — `thelper` reports 0 issues · Proof: `bash scripts/lint-only.sh thelper`
- C3.3 — `errname` reports 0 issues · Proof: `bash scripts/lint-only.sh errname`
- C3.4 — `embeddedstructfieldcheck` reports 0 issues · Proof: `bash scripts/lint-only.sh embeddedstructfieldcheck`
- C3.5 — `containedctx` reports 0 issues · Proof: `bash scripts/lint-only.sh containedctx`
- C3.6 — `revive` reports 0 issues in ./internal/... · Proof: `bash scripts/lint-only.sh revive ./internal/...`
- C3.7 — the stage's debt entries left `.golangci.yml` · Proof: verify script: nonamedreturns (+4 more) gone from .golangci.yml
Verify script: docs/plans/lint-debt-paydown-verify-stage-3.py

<!-- STAGE 3: manual -->
**Manual verification (if any):** none
<!-- END STAGE 3 -->
---

<!-- BEGIN STAGE 4 -->
## Stage 4 - Dropped values in internal packages
<!-- STAGE 4: tier-effort -->
**Tier:** standard
**Effort:** standard
<!-- STAGE 4: tier-rationale -->
**Tier rationale:** Mechanical in kind, but wrapping and sentinel errors touch 14 files and must keep every printed message.
<!-- STAGE 4: size -->
**Size:** ~39k tokens
**Depends on:** Stage 2, Stage 3
<!-- STAGE 4: items -->
**Items:** errcheck, wrapcheck, err113 in ./internal/...; errorlint 2
<!-- STAGE 4: scope -->
**Scope:** `internal/**` has 0 sites of errcheck, wrapcheck, err113; errorlint is 0 repo-wide and leaves the debt list.

<!-- STAGE 4: files -->
**Files:**
- `internal/fetch/client.go` - errcheck 2, wrapcheck 4, err113 2, errorlint 1
- `internal/fetch/client_test.go` - errcheck 6
- `internal/fetch/fetch.go` - errcheck 1, wrapcheck 2
- `internal/fetch/gitrefs.go` - errcheck 1, wrapcheck 1, err113 5
- `internal/fetch/gitrefs_test.go` - errcheck 5
- `internal/fetch/ref.go` - errcheck 1, wrapcheck 2, err113 1
- `internal/fetch/repo.go` - wrapcheck 1, err113 2
- `internal/fetch/tarball.go` - errcheck 2, wrapcheck 3, err113 2, errorlint 1
- `internal/fetch/trees.go` - wrapcheck 1
- `internal/fetch/write.go` - errcheck 1, wrapcheck 13, err113 1
- `internal/ghurl/url.go` - err113 6
- `internal/ghurl/url_test.go` - errcheck 11
- `internal/manifest/manifest.go` - errcheck 1, wrapcheck 2
- `internal/meta/meta.go` - errcheck 1, wrapcheck 3
- `.golangci.yml` - Flip a linter for errorlint only
- `docs/lint-transition.md` - Progress line

<!-- STAGE 4: order -->
**Order of operations:**
1. Measure: `bash scripts/lint-only.sh errcheck ./internal/...`, the same for `wrapcheck` and `err113`.
2. Procedure Fix a dropped value over those three lists, package by package (`internal/fetch`, then `ghurl`, `manifest`, `meta`).
3. `errorlint`: compare errors with `errors.Is`/`errors.As`, format wrapped errors with `%w`.
4. Procedure Flip a linter for `errorlint` only: errcheck, wrapcheck and err113 still have root-package sites (Stage 5).

<!-- STAGE 4: checks -->
**Checks:**
- C4.1 — `errcheck` reports 0 issues in ./internal/... · Proof: `bash scripts/lint-only.sh errcheck ./internal/...`
- C4.2 — `wrapcheck` reports 0 issues in ./internal/... · Proof: `bash scripts/lint-only.sh wrapcheck ./internal/...`
- C4.3 — `err113` reports 0 issues in ./internal/... · Proof: `bash scripts/lint-only.sh err113 ./internal/...`
- C4.4 — `errorlint` reports 0 issues · Proof: `bash scripts/lint-only.sh errorlint`
- C4.5 — the stage's debt entries left `.golangci.yml` · Proof: verify script: errorlint (+0 more) gone from .golangci.yml
Verify script: docs/plans/lint-debt-paydown-verify-stage-4.py

<!-- STAGE 4: manual -->
**Manual verification (if any):** none
<!-- END STAGE 4 -->
---

<!-- BEGIN STAGE 5 -->
## Stage 5 - Dropped values in the root package, flip
<!-- STAGE 5: tier-effort -->
**Tier:** standard
**Effort:** standard
<!-- STAGE 5: tier-rationale -->
**Tier rationale:** Same recipe as Stage 4 over the larger root package (~90 sites); no design decision left.
<!-- STAGE 5: size -->
**Size:** ~43k tokens
**Depends on:** Stage 2, Stage 4
<!-- STAGE 5: items -->
**Items:** errcheck, wrapcheck, err113, gosec G104 (root package and tests)
<!-- STAGE 5: scope -->
**Scope:** errcheck, wrapcheck, err113 and G104 reach 0 sites repo-wide and leave the debt list.

<!-- STAGE 5: files -->
**Files:**
- `main.go` - errcheck 1, wrapcheck 11, err113 21
- `install.go` - errcheck 3, wrapcheck 13, err113 1, G104 1
- `list.go` - wrapcheck 9, err113 7
- `freeze.go` - wrapcheck 3, err113 2
- `selfupdate.go` - errcheck 9, wrapcheck 9, err113 6, G104 7
- `freeze_test.go` - errcheck 1
- `install_test.go` - errcheck 2
- `main_test.go` - errcheck 1
- `selfupdate_test.go` - errcheck 7, G104 2
- `.golangci.yml` - Flip a linter for errcheck, wrapcheck, err113 and the G104 entry of `gosec.excludes`
- `AGENTS.md` - Dropped-value recipe gains: wrap an error once, at the package boundary
- `docs/lint-transition.md` - Progress lines

<!-- STAGE 5: order -->
**Order of operations:**
1. Measure: `bash scripts/lint-only.sh errcheck`, then wrapcheck, err113.
2. Procedure Fix a dropped value over the three lists, file by file. Reuse the sentinels Stage 4 defined where the root package matches the same condition.
3. Procedure Flip a linter for errcheck, wrapcheck, err113, and for G104 (the entry in `settings.gosec.excludes`; if the list empties, delete the key).
4. AGENTS.md: in the Dropped-value bullet of the lint gate block add: wrap an error once, at the package boundary.

<!-- STAGE 5: checks -->
**Checks:**
- C5.1 — `errcheck` reports 0 issues · Proof: `bash scripts/lint-only.sh errcheck`
- C5.2 — `wrapcheck` reports 0 issues · Proof: `bash scripts/lint-only.sh wrapcheck`
- C5.3 — `err113` reports 0 issues · Proof: `bash scripts/lint-only.sh err113`
- C5.4 — `gosec` output has no G104 finding · Proof: bash scripts/lint-only.sh gosec (the verify script asserts G104 is absent)
- C5.5 — the stage's debt entries left `.golangci.yml` · Proof: verify script: errcheck (+3 more) gone from .golangci.yml
- C5.6 — AGENTS.md carries the wrap-once line · Proof: V.assert_grep_nonzero(r"wrap an error once", ["AGENTS.md"])
Verify script: docs/plans/lint-debt-paydown-verify-stage-5.py

<!-- STAGE 5: manual -->
**Manual verification (if any):** none
<!-- END STAGE 5 -->
---

<!-- BEGIN STAGE 6 -->
## Stage 6 - Output through writers (forbidigo)
<!-- STAGE 6: tier-effort -->
**Tier:** judgment
**Effort:** extended
<!-- STAGE 6: tier-rationale -->
**Tier rationale:** Decision D2 changes function signatures across the whole root package and its tests; message text must stay identical.
<!-- STAGE 6: size -->
**Size:** ~44k tokens
**Depends on:** Stage 2, Stage 5
<!-- STAGE 6: items -->
**Items:** D2; forbidigo 79
<!-- STAGE 6: scope -->
**Scope:** All terminal output goes through writers passed in; only `main` calls `os.Exit`; `forbidigo` leaves the debt list.

<!-- STAGE 6: files -->
**Files:**
- `main.go` - forbidigo 32: run, runDownload and runUpdate take stdout and stderr writers; main passes the os streams and keeps the one exit
- `install.go` - forbidigo 16: writers passed down
- `list.go` - forbidigo 12
- `freeze.go` - forbidigo 16
- `selfupdate.go` - forbidigo 3
- `main_test.go` - pass buffers where signatures changed
- `install_test.go` - same
- `list_test.go` - same
- `freeze_test.go` - same
- `selfupdate_test.go` - same
- `.golangci.yml` - Flip a linter for forbidigo
- `AGENTS.md` - add: output goes through the writer passed to `run`; only `main` exits
- `docs/lint-transition.md` - mark D2 delivered; Progress line

<!-- STAGE 6: order -->
**Order of operations:**
1. Measure: `bash scripts/lint-only.sh forbidigo` lists the 79 sites (`fmt.Print*`, `print`, `os.Exit`).
2. Thread `stdout, stderr io.Writer` from `main` down the call chain, bottom-up (leaf functions first) so each edit compiles; `fmt.Fprint*(w, ...)` replaces
   each print with the same text. Update tests to pass `bytes.Buffer`.
3. `os.Exit` stays only in `main()`, as `//nolint:forbidigo // process exit code; main returns nothing`.
4. Procedure Flip a linter for forbidigo; then the AGENTS.md line.

<!-- STAGE 6: checks -->
**Checks:**
- C6.1 — `forbidigo` reports 0 issues · Proof: `bash scripts/lint-only.sh forbidigo`
- C6.2 — the stage's debt entries left `.golangci.yml` · Proof: verify script: forbidigo (+0 more) gone from .golangci.yml
- C6.3 — `os.Exit` only in main.go · Proof: V.assert_grep_zero(r"os\.Exit", ["install.go", "list.go", "freeze.go", "selfupdate.go", "internal/"])
- C6.4 — no `fmt.Print` in production code · Proof: V.assert_grep_zero(r"fmt\.Print", ["main.go", "install.go", "list.go", "freeze.go", "selfupdate.go"])
- C6.5 — AGENTS.md carries the writer line · Proof: V.assert_grep_nonzero(r"writer passed to", ["AGENTS.md"])
Verify script: docs/plans/lint-debt-paydown-verify-stage-6.py

<!-- STAGE 6: manual -->
**Manual verification (if any):** none
<!-- END STAGE 6 -->
---

<!-- BEGIN STAGE 7 -->
## Stage 7 - Escape hatches and local rules
<!-- STAGE 7: tier-effort -->
**Tier:** standard
**Effort:** standard
<!-- STAGE 7: tier-rationale -->
**Tier rationale:** Local fixes, each needs reading the call site (type assertion, slice index, request context).
<!-- STAGE 7: size -->
**Size:** ~41k tokens
**Depends on:** Stage 2, Stage 6
<!-- STAGE 7: items -->
**Items:** forcetypeassert 3, exhaustive 3, noctx 3, govet 6, gochecknoglobals 5, gocritic:badCall 1, gosec G602 3
<!-- STAGE 7: scope -->
**Scope:** Seven linters/check ids reach 0 sites and leave the debt list.

<!-- STAGE 7: files -->
**Files:**
- `internal/fetch/client.go` - forcetypeassert 1, gochecknoglobals 4
- `internal/fetch/client_test.go` - forcetypeassert 2
- `internal/fetch/fetch.go` - exhaustive 1, govet 1
- `freeze.go` - exhaustive 1
- `main.go` - exhaustive 1, G602 3, govet 2
- `install.go` - noctx 1, govet 2
- `selfupdate.go` - noctx 2, gochecknoglobals 1
- `list.go` - govet 1
- `install_test.go` - gocritic:badCall 1
- `.golangci.yml` - Flip a linter for the seven entries
- `docs/lint-transition.md` - Progress lines

<!-- STAGE 7: order -->
**Order of operations:**
1. Measure each with `bash scripts/lint-only.sh <linter>`; for `govet` read each message (shadow and others) before editing.
2. Fix by the Escape-hatch recipe: type assertions check `ok` and return an error; indexes check the length once; `noctx` uses `http.NewRequestWithContext`
   with the caller's context; globals become constants or are passed in; exhaustive switches list every case (a `default` that hides a case is not a fix).
3. Procedure Flip a linter for forcetypeassert, exhaustive, noctx, govet, gochecknoglobals, gocritic:badCall (`disabled-checks`) and G602
   (`gosec.excludes`).

<!-- STAGE 7: checks -->
**Checks:**
- C7.1 — `forcetypeassert` reports 0 issues · Proof: `bash scripts/lint-only.sh forcetypeassert`
- C7.2 — `exhaustive` reports 0 issues · Proof: `bash scripts/lint-only.sh exhaustive`
- C7.3 — `noctx` reports 0 issues · Proof: `bash scripts/lint-only.sh noctx`
- C7.4 — `govet` reports 0 issues · Proof: `bash scripts/lint-only.sh govet`
- C7.5 — `gochecknoglobals` reports 0 issues · Proof: `bash scripts/lint-only.sh gochecknoglobals`
- C7.6 — `gocritic` reports 0 issues · Proof: `bash scripts/lint-only.sh gocritic`
- C7.7 — `gosec` output has no G602 finding · Proof: bash scripts/lint-only.sh gosec (the verify script asserts G602 is absent)
- C7.8 — the stage's debt entries left `.golangci.yml` · Proof: verify script: forcetypeassert (+6 more) gone from .golangci.yml
Verify script: docs/plans/lint-debt-paydown-verify-stage-7.py

<!-- STAGE 7: manual -->
**Manual verification (if any):** none
<!-- END STAGE 7 -->
---

<!-- BEGIN STAGE 8 -->
## Stage 8 - Capability rules (gosec) and goconst
<!-- STAGE 8: tier-effort -->
**Tier:** judgment
**Effort:** extended
<!-- STAGE 8: tier-rationale -->
**Tier rationale:** D3: where a file write or process spawn is allowed; each remaining call needs a written reason.
<!-- STAGE 8: size -->
**Size:** ~35k tokens
**Depends on:** Stage 2, Stage 7
<!-- STAGE 8: items -->
**Items:** D3, D7, D8; gosec G304 15, G703 5, G122 1, G204 1, G306 1; goconst 28
<!-- STAGE 8: scope -->
**Scope:** gosec and goconst run with 0 sites; tests are excluded from both by a recorded decision; production exceptions are few and reasoned.

<!-- STAGE 8: files -->
**Files:**
- `freeze.go` - G304 1, G703 2
- `install.go` - G304 2, G204 1, goconst 1
- `main.go` - G703 2
- `selfupdate.go` - goconst 1
- `internal/fetch/write.go` - G304 1, G703 1, G122 1
- `internal/manifest/manifest.go` - G304 1
- `internal/meta/meta.go` - G304 1
- `.golangci.yml` - add `goconst` and `gosec` to the `_test\.go` exclusion rule; Flip a linter for goconst and the five gosec ids
- `docs/lint-transition.md` - record D3 (taken), D7 (goconst not checked in tests), D8 (gosec not checked in tests); Progress lines

<!-- STAGE 8: order -->
**Order of operations:**
1. D3 taken: file writes keep going through `internal/fetch/write.go` and `internal/dest`; no new package. A remaining call whose path or command comes from
   the user's own command line by design carries `//nolint:gosec // <rule>: <why the input is trusted here>`. Prefer fixing: validate the path with
   `filepath.Clean`/`filepath.IsLocal`, write with mode 0o644/0o755 where the rule asks.
2. D7/D8 taken: in `exclusions.rules` change the `_test\.go` rule to `linters: [err113, wrapcheck, goconst, gosec]`; repeated literals and temp-file reads
   in tests are readable, not defects. Record both decisions in the plan.
3. `goconst`: the 2 production sites (`install.go`, `selfupdate.go`) become a named constant only when the strings are one rule.
4. Procedure Flip a linter for goconst and for the gosec ids G304, G703, G122, G204, G306; delete the empty `excludes` key.
5. Count `//nolint:gosec` in production files: at most 8; if more, fix more sites instead.

<!-- STAGE 8: checks -->
**Checks:**
- C8.1 — `gosec` reports 0 issues · Proof: `bash scripts/lint-only.sh gosec`
- C8.2 — `goconst` reports 0 issues · Proof: `bash scripts/lint-only.sh goconst`
- C8.3 — test exclusion rule lists goconst, gosec · Proof: V.assert_grep_nonzero(r"linters: \[err113, wrapcheck, goconst, gosec\]", [".golangci.yml"])
- C8.4 — the stage's debt entries left `.golangci.yml` · Proof: verify script: goconst (+5 more) gone from .golangci.yml
- C8.5 — at most 8 `//nolint:gosec` in production Go files · Proof: counted by the verify script: `//nolint:gosec` in production files
- C8.6 — `gosec.excludes` key is gone · Proof: V.assert_grep_zero(r"^\s+excludes:", [".golangci.yml"])
Verify script: docs/plans/lint-debt-paydown-verify-stage-8.py

<!-- STAGE 8: manual -->
**Manual verification (if any):** none
<!-- END STAGE 8 -->
---

<!-- BEGIN STAGE 9 -->
## Stage 9 - Shape: root package complexity
<!-- STAGE 9: tier-effort -->
**Tier:** judgment
**Effort:** extended
<!-- STAGE 9: tier-rationale -->
**Tier rationale:** Splits god functions and god file main.go by responsibility; needs design judgment and must keep behavior.
<!-- STAGE 9: size -->
**Size:** ~36k tokens
**Depends on:** Stage 2, Stage 8
<!-- STAGE 9: items -->
**Items:** gocognit root package (ceiling 35, root at 10), nestif 2, revive:argument-limit 2, main.go size
<!-- STAGE 9: scope -->
**Scope:** Root-package functions are at complexity <= 10, nestif and revive leave the debt list, main.go is under 500 lines, gocognit is on at ceiling 35.

<!-- STAGE 9: files -->
**Files:**
- `main.go` - gocognit 4 (run: 62, one at 21), nestif 1; split into one file per command (`cmd_*.go`)
- `cmd_*.go` - NEW: files carved out of main.go by responsibility
- `install.go` - gocognit 1, nestif 1
- `list.go` - gocognit 2, argument-limit 1 (an options struct)
- `freeze.go` - gocognit 2 (one at 26)
- `selfupdate.go` - gocognit 2
- `freeze_test.go` - gocognit 1, argument-limit 1
- `.golangci.yml` - Flip a linter for nestif and revive; take gocognit out of `disable` and set `min-complexity: 35` with a ceiling comment
- `docs/lint-transition.md` - Progress lines

<!-- STAGE 9: order -->
**Order of operations:**
1. Measure with a low-limit run: copy `.golangci.yml` to a scratch path with `min-complexity: 1` and run `go run
   github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run -c <scratch> --enable-only gocognit .` (root package only).
2. Procedure Fix a shape over the root-package sites, worst first: `main.go` `run` (62), `freeze.go:162` (26), `main.go:447` (21), then the rest. Split
   `run` by subcommand: one function per command, argument parsing apart from execution, guard clauses.
3. Carve `main.go` into `main.go` (entry, usage, `run` dispatch) and `cmd_<command>.go` files, so `main.go` is under 500 non-blank lines.
4. `argument-limit`: `list.go:145` takes an options struct; `freeze_test.go` helper likewise.
5. In `.golangci.yml` remove `gocognit`, `nestif` and `revive` from `disable` (with their debt comments), and set `gocognit.min-complexity: 35` with the
   comment line `# ceiling in steps: 35, then 10 (docs/lint-transition.md, Phase 5)`.

<!-- STAGE 9: checks -->
**Checks:**
- C9.1 — `gocognit` reports 0 issues in . · Proof: `bash scripts/lint-only.sh gocognit .`
- C9.2 — `nestif` reports 0 issues · Proof: `bash scripts/lint-only.sh nestif`
- C9.3 — `revive` reports 0 issues · Proof: `bash scripts/lint-only.sh revive`
- C9.4 — root functions have complexity <= 10 · Proof: verify script: gocognit on `.` with `min-complexity: 10`
- C9.5 — main.go has at most 500 non-blank lines · Proof: counted by the verify script: non-blank lines of main.go
- C9.6 — the stage's debt entries left `.golangci.yml` · Proof: verify script: gocognit (+2 more) gone from .golangci.yml
Verify script: docs/plans/lint-debt-paydown-verify-stage-9.py

<!-- STAGE 9: manual -->
**Manual verification (if any):** none
<!-- END STAGE 9 -->
---

<!-- BEGIN STAGE 10 -->
## Stage 10 - Shape: internal packages, close the debt list
<!-- STAGE 10: tier-effort -->
**Tier:** judgment
**Effort:** extended
<!-- STAGE 10: tier-rationale -->
**Tier rationale:** Splits internal functions up to complexity 33 and closes the track: the config must end with noise entries only.
<!-- STAGE 10: size -->
**Size:** ~32k tokens
**Depends on:** Stage 2, Stage 9
<!-- STAGE 10: items -->
**Items:** gocognit internal packages and tests; ceiling 35 -> 10; end of the debt list
<!-- STAGE 10: scope -->
**Scope:** `gocognit` runs at `min-complexity: 10` repo-wide; `.golangci.yml` has no `debt:` comment left.

<!-- STAGE 10: files -->
**Files:**
- `internal/fetch/tarball.go` - gocognit 1 (33)
- `internal/fetch/gitrefs.go` - gocognit 1 (26)
- `internal/fetch/fetch.go` - gocognit 1
- `internal/fetch/trees.go` - gocognit 1
- `internal/fetch/write.go` - gocognit 2
- `internal/ghurl/url.go` - gocognit 3
- `internal/fetch/gitrefs_test.go` - gocognit 1
- `internal/fetch/write_test.go` - gocognit 1
- `internal/ghurl/url_test.go` - gocognit 1
- `.golangci.yml` - `gocognit.min-complexity: 10`, ceiling comment removed
- `AGENTS.md` - delete the line pointing at the remaining debt
- `docs/lint-transition.md` - Progress: debt list empty; keep the plan as history

<!-- STAGE 10: order -->
**Order of operations:**
1. Measure as in Stage 9 over `./internal/...`.
2. Procedure Fix a shape over the sites, worst first (`tarball.go` 33, `gitrefs.go` 26). Split by responsibility; for table-driven tests prefer subtests per
   case to nesting.
3. A test function that cannot reach 10 may carry `//nolint:gocognit // <reason>`; at most 4 in the repo.
4. Set `min-complexity: 10`, delete the ceiling comment. Check that every `disable` entry left in `.golangci.yml` sits under a `# noise:` comment and that
   no `# debt:` comment remains.
5. Delete the AGENTS.md line `The remaining debt, its order and its fix recipes are in docs/lint-transition.md.`; add a final Progress line `debt list
   empty`.

<!-- STAGE 10: checks -->
**Checks:**
- C10.1 — `gocognit` reports 0 issues · Proof: `bash scripts/lint-only.sh gocognit`
- C10.2 — `.golangci.yml` sets `min-complexity: 10` for gocognit and names no ceiling · Proof: V.assert_grep_zero(r"ceiling in steps", [".golangci.yml"])
- C10.3 — no `# debt:` comment is left in `.golangci.yml` · Proof: V.assert_grep_zero(r"# debt:", [".golangci.yml"])
- C10.4 — every `disable` entry has a `# noise:` comment · Proof: the verify script reads the file
- C10.5 — at most 4 `//nolint:gocognit` in the repo · Proof: counted by the verify script: `//nolint:gocognit` in the repo
- C10.6 — AGENTS.md no longer points at remaining debt · Proof: V.assert_grep_zero(r"remaining debt", ["AGENTS.md"])
Verify script: docs/plans/lint-debt-paydown-verify-stage-10.py

<!-- STAGE 10: manual -->
**Manual verification (if any):** none
<!-- END STAGE 10 -->
---
