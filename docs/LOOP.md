# Overnight `/improve` Autonomous Loop

Designed 2026-04-16. Run this before bed to wake up to a branch of auto-generated fixes on the SlabLedger codebase. Each iteration runs `/improve`, picks the top ranked finding not already attempted, fixes it, runs verification gates, and either commits or rolls back. Everything accumulates on one `overnight-improvements-YYYY-MM-DD` branch.

## Launching the session

The global default permission mode is `plan` (see `~/.claude/settings.json` → `defaultMode`). Plan mode blocks Edit/Write and prompts on every Bash call, so an unattended loop stalls at the first tool call. Launch the overnight session with permissions bypassed:

```bash
claude --permission-mode bypassPermissions
# equivalent: claude --dangerously-skip-permissions
```

Inside this session every tool runs without prompting — use it for the overnight loop only and exit it when the run finishes, so normal interactive sessions stay in the safer plan-mode default.

## Pre-flight checklist

Run these before kicking off the loop:

```bash
# 0. Confirm the session was launched with --permission-mode bypassPermissions
#    (check the startup banner or run /status). If not, exit and relaunch —
#    plan mode will block every iteration.

# 1. main is clean and up to date
git status                                    # must be clean
git fetch origin && git log HEAD..origin/main # must be empty

# 2. Create the overnight branch
git checkout -b overnight-improvements-$(date +%Y-%m-%d)

# 3. Seed the state file (optional — the prompt creates it if missing)
mkdir -p .claude
cat > .claude/overnight-run-state.md <<EOF
---
started: $(date -u +%Y-%m-%dT%H:%M:%SZ)
branch: overnight-improvements-$(date +%Y-%m-%d)
---

== Attempted findings ==
(none yet)
EOF

# 4. Verify gates pass on the baseline BEFORE starting the loop
make check
go test -race -timeout 10m ./...
( cd web && npm run lint:strict && npm run typecheck && npm test )
# If any gate fails on baseline, fix or abort — don't start the loop on broken main
```

## Dry run first (strongly recommended)

Don't commit a whole night without a 2-iteration test:

```bash
/ralph-loop --max-iterations 2 --completion-promise 'NO_FINDINGS_WORTH_FIXING' '<prompt below>'
```

Verify all 4:

1. Iteration 1 produced a commit that passes gates.
2. The state file got updated correctly (one `succeeded: <sha>` entry).
3. Iteration 2 saw iteration 1's fix in the tree (didn't re-flag the same finding).
4. `git status` ended clean after each iteration.

If those check out, run the real loop.

## The full invocation

```bash
/ralph-loop:ralph-loop --max-iterations 15 --completion-promise 'NO_FINDINGS_WORTH_FIXING' 'You are running one iteration of an autonomous overnight improvement loop on the SlabLedger codebase. The same prompt will fire again after you finish this iteration, until max-iterations is hit or you output the completion promise.

== OPERATING RULES (pre-authorized for this loop only) ==

- You are working on branch `overnight-improvements-<today>`. Verify with `git branch --show-current` first. If you are on any other branch, output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` immediately — DO NOT switch branches autonomously.
- You MAY `git reset --hard HEAD` to discard uncommitted changes on this branch to roll back a failed attempt. This overrides the "never reset without asking" rule for THIS branch only.
- You may NOT: force-push, push at all, switch/merge/delete branches, run `git clean -fd`, run any `--no-verify` variant, touch any other branch.
- You may run any read-only command (tests, lint, typecheck, build, `go test`, `make check`).
- Commits must include the `Co-Authored-By: Claude` trailer and a clear subject `improve: <finding title>`.

== STATE TRACKING ==

Read `.claude/overnight-run-state.md`. If it does not exist, create it with:

---
started: <ISO timestamp>
branch: overnight-improvements-<today>
---

== Attempted findings ==
(none yet)

Each entry under "Attempted findings" has the form:
`- <file:line> [<category>] <title> — status: <succeeded|failed-once|failed-twice|skipped-design>` with commit SHA when succeeded.

Use `file:line + category` as the identity key (titles shift between /improve runs; file + category is stable).

== WORK FOR THIS ITERATION ==

1. Run /improve (no arguments — full codebase scope). Capture the top 10 findings.

2. Pick the finding to attempt. From the returned list, pick the highest-ranked finding where the state file does NOT show the same file:line + category already marked `succeeded`, `failed-twice`, or `skipped-design`. If the finding is marked `failed-once`, you may retry it. If no eligible finding remains, output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` and stop.

3. Log as in-progress in the state file: `- <file:line> [<category>] <title> — status: in-progress (iteration <N>)`.

4. Implement the fix. Respect existing architecture (hexagonal: domain packages never import adapters; flat-sibling rule between inventory sub-packages). Stay inside the scope of the finding — do not rename, restructure, or add features beyond what the finding names. If during implementation you realize the finding genuinely requires a design decision you cannot make alone (e.g. "split this service into which pieces?"), mark it `skipped-design` in the state file, `git reset --hard HEAD`, and return to step 2 to pick the next finding.

5. Run verification gates. All must exit 0. Capture the exit code of each:

   make check
   go test -race -timeout 10m ./...
   cd web && npm run lint:strict
   cd web && npm run typecheck
   cd web && npm test

6. Commit or roll back.
   - All gates pass: `git add` the files you modified (do NOT use `git add -A` — avoid staging unrelated artifacts) → `git commit` with subject `improve: <finding title>` and body containing: the category, severity, and effort of the finding, and a 2-3 line description of what was changed and why. Update state file entry to `succeeded: <short SHA>`.
   - Any gate fails: `git reset --hard HEAD`. If this was the first attempt at this finding, update state entry to `failed-once: <gate that failed>, <brief error>`. If this was the second attempt, update to `failed-twice`. Do NOT output the completion promise just because a fix failed — let the loop try the next finding.

7. Sanity check before ending the iteration. Run `git status` — working tree must be clean. If it is not clean, you have a bug; `git reset --hard HEAD` and log the anomaly in the state file.

== COMPLETION PROMISE RULES ==

Output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` ONLY when at least one of:
- /improve returned 0 findings.
- Every top-5 finding in the current /improve output is already marked `succeeded`, `failed-twice`, or `skipped-design` in the state file.
- You are on the wrong branch (safety check from the rules above).

Do NOT output the promise to escape the loop because "this is hard" or "I am stuck". The loop is designed to continue through failed attempts — that is what `failed-once`/`failed-twice` tracking is for.

== PROJECT-SPECIFIC DO NOTs ==

- Do not mock the database in integration tests.
- Do not split `cmd/slabledger/main.go` (accepted tech debt).
- Do not parse card numbers from titles — only from cert lookup.
- Do not touch files outside the scope of the finding. Scope discipline over cleanup.'
```

## Morning review checklist

```bash
git log --oneline main..HEAD         # what got committed
cat .claude/overnight-run-state.md   # every attempt, including failures
git diff main..HEAD --stat           # scope of changes
```

For each commit decide: keep, `git revert`, or `git rebase -i` to drop. Or open a PR with the whole branch and use the GitHub UI to cherry-pick.

Exit the bypass-mode session before doing interactive review work, so subsequent sessions return to the plan-mode default.

## Design notes

- **Why one branch instead of branch-per-fix?** Each iteration runs `/improve` against the branch HEAD. A branch-per-fix layout would either force PR chaining or leave each iteration blind to prior fixes. Accumulating commits on one branch solves both.
- **Why `file:line + category` as the dedupe key?** Finding titles shift between `/improve` runs as code changes around them. File + category is stable.
- **Why allow Large-effort findings?** Guardrails are the verification gates plus the single-branch rollback, not a size cap. Failed fix attempts roll back cleanly.
- **Runtime budget.** 15 iterations × ~20-40 min = 5-10 hours. Large-effort iterations can run 90+ minutes. If you want a tighter ceiling, drop to `--max-iterations 12`.

## Critical files referenced

- `/workspace/.claude/skills/improve/SKILL.md` — the `/improve` skill (output format, memory file location)
- `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md` — `/improve`s cross-run memory (preserved by the skill itself; no loop action needed)
- `/workspace/.claude/overnight-run-state.md` — this loop's per-night state (gitignored via existing `.claude/` entry)
- `/workspace/CLAUDE.md` — project conventions the loop must respect
- `/home/vscode/.claude/CLAUDE.md` — global preferences (the `git reset` rule is the one explicitly overridden)
