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
max_iterations: 15
iteration: 0
max_wrap_iterations: 3
---

== Attempted findings ==
(none yet)

== Wrap-up ==
(pending)
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
2. The state file got updated correctly (one `succeeded: <sha>` entry, `iteration: 2` after the second fire).
3. Iteration 2 saw iteration 1's fix in the tree (didn't re-flag the same finding).
4. `git status` ended clean after each iteration.

If those check out, run the real loop.

**Dry-run wrap-up note.** PHASE 2 fires when `iteration == max_iterations` *as written in the state file* (15), not the `--max-iterations` flag value. So a 2-iter dry-run normally won't push or open a PR. The exception is the "no eligible findings" path — if both dry-run iterations exhaust the eligible top-ranked findings, PHASE 2 triggers and you'll get a real PR. To dry-run without that risk, either delete the seeded state file before each dry-run or comment out the `gh pr create` block in the prompt while you're testing.

## The full invocation

```bash
/ralph-loop:ralph-loop --max-iterations 15 --completion-promise 'NO_FINDINGS_WORTH_FIXING' 'You are running one iteration of an autonomous overnight improvement loop on the SlabLedger codebase. The same prompt will fire again after you finish this iteration, until max-iterations is hit or you output the completion promise.

== OPERATING RULES (pre-authorized for this loop only) ==

- You are working on branch `overnight-improvements-<today>`. Verify with `git branch --show-current` first. If you are on any other branch, output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` immediately — DO NOT switch branches autonomously.
- You MAY `git reset --hard HEAD` to discard uncommitted changes on this branch to roll back a failed attempt. This overrides the "never reset without asking" rule for THIS branch only.
- You may NOT: force-push, switch/merge/delete branches, run `git clean -fd`, run any `--no-verify` variant, touch any other branch.
- During PHASE 2 (wrap-up) ONLY, you MAY `git push -u origin HEAD`, run `gh pr create`, run additional `git push` commands for follow-up commits, and invoke the `coderabbit:autofix` skill. These permissions do NOT apply during normal iteration work — only inside the wrap-up phase below.
- You may run any read-only command (tests, lint, typecheck, build, `go test`, `make check`).
- Commits must include the `Co-Authored-By: Claude` trailer and a clear subject `improve: <finding title>`.

== STATE TRACKING ==

Read `.claude/overnight-run-state.md`. If it does not exist, create it with:

---
started: <ISO timestamp>
branch: overnight-improvements-<today>
max_iterations: 15
iteration: 0
max_wrap_iterations: 3
---

== Attempted findings ==
(none yet)

== Wrap-up ==
(pending)

Each entry under "Attempted findings" has the form:
`- <file:line> [<category>] <title> — status: <succeeded|failed-once|failed-twice|skipped-design>` with commit SHA when succeeded.

Use `file:line + category` as the identity key (titles shift between /improve runs; file + category is stable).

The frontmatter `iteration` counter tracks how many times this prompt has fired. Bump it at step 0 of every iteration (see below). When `iteration == max_iterations`, this is the LAST fire and PHASE 2 wrap-up MUST run before the iteration ends.

== WORK FOR THIS ITERATION ==

0. Bump the iteration counter. Read `iteration` from the frontmatter, add 1, write it back. Note the new value — call it `N`. If the frontmatter is missing the field (older state file), add `iteration: 1`, `max_iterations: 15`, and `max_wrap_iterations: 3`.

1. Run /improve (no arguments — full codebase scope). Capture the top 10 findings.

2. Pick the finding to attempt. From the returned list, pick the highest-ranked finding where the state file does NOT show the same file:line + category already marked `succeeded`, `failed-twice`, or `skipped-design`. If the finding is marked `failed-once`, you may retry it. If no eligible finding remains, jump to PHASE 2 (wrap-up) below — do NOT emit the completion promise yet.

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

8. Wrap-up trigger. If `N == max_iterations` (this was the last fire), jump to PHASE 2 below before ending the iteration. Otherwise end the iteration normally and let ralph-loop fire the next one.

== PHASE 2: WRAP-UP (PR + CodeRabbit) ==

Trigger conditions (either fires the wrap-up):
- Step 2 found no eligible findings (all top-ranked are already `succeeded`, `failed-twice`, or `skipped-design`).
- Step 8 detected `N == max_iterations`.

Skip wrap-up entirely (and emit the completion promise immediately) if the wrong-branch safety check from the rules above tripped — never push or open PRs from an unexpected branch.

Permissions for this phase only: `git push -u origin HEAD`, `gh pr create`, follow-up `git push`, and the `coderabbit:autofix` skill are allowed. Force-push, branch switching, merging, and `--no-verify` remain forbidden.

W1. Verify there is something to push. Run `git log --oneline main..HEAD` — if zero commits, log `no commits, nothing to PR` under `== Wrap-up ==` in the state file, output the completion promise, and stop.

W2. Push the branch.

```
git push -u origin HEAD
```

If push fails, log the error under `== Wrap-up ==`, output the completion promise, and stop.

W3. Open or find the PR (target `main`, ready for review — not draft).

```
gh pr create --base main --head "$(git branch --show-current)" \
  --title "improve: overnight loop $(date +%Y-%m-%d)" \
  --body "$(cat <<EOF
## Summary
Autonomous overnight /improve loop on $(date +%Y-%m-%d). Per-finding log: \`.claude/overnight-run-state.md\`.

## Commits
$(git log --oneline main..HEAD)

## Verification
All commits passed: \`make check\`, \`go test -race ./...\`, web \`lint:strict\` + \`typecheck\` + \`test\`.

## Test plan
- [ ] Skim per-finding log
- [ ] Confirm no scope creep beyond findings
- [ ] Address any CodeRabbit follow-ups
EOF
)"
```

If `gh pr create` fails because a PR already exists, fall back to `gh pr view --json number -q .number` to get the existing PR number. Capture the PR number — call it `PR`. Log `pr: #<PR>` under `== Wrap-up ==`.

W4. Address CodeRabbit in rounds. CodeRabbit (`coderabbitai[bot]`) auto-reviews on PR open AND re-reviews each new commit. One autofix pass usually does not clear every comment, so loop up to `max_wrap_iterations` times (default 3). Each round: wait for the review of the *current HEAD*, apply autofix, gate, push.

```
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
WRAP_MAX=$(awk '/^max_wrap_iterations:/ {print $2}' .claude/overnight-run-state.md)

for round in $(seq 1 "$WRAP_MAX"); do
  HEAD_SHA=$(git rev-parse HEAD)

  # Wait up to 15 min for CodeRabbit to review THIS commit (commit_id match)
  REVIEW_ID=""
  for poll in $(seq 1 10); do
    REVIEW_ID=$(gh api "repos/$REPO/pulls/$PR/reviews" \
      --jq "[.[] | select(.user.login | startswith(\"coderabbit\")) | select(.commit_id == \"$HEAD_SHA\")] | last | .id // empty")
    [ -n "$REVIEW_ID" ] && break
    sleep 90
  done

  if [ -z "$REVIEW_ID" ]; then
    echo "round $round: coderabbit timeout for $HEAD_SHA"
    # log under == Wrap-up == and break
    break
  fi

  # If CodeRabbit posted "Actionable comments posted: 0", we're done
  REVIEW_BODY=$(gh api "repos/$REPO/pulls/$PR/reviews/$REVIEW_ID" --jq .body)
  if printf '%s' "$REVIEW_BODY" | grep -q "Actionable comments posted: 0"; then
    echo "round $round: 0 actionable comments — clean"
    # log under == Wrap-up == and break
    break
  fi

  # Apply autofix (Skill tool: coderabbit:autofix, batch mode, on PR $PR)
  # ... invoke skill ...

  if [ -z "$(git status --porcelain)" ]; then
    echo "round $round: autofix made no file changes — stopping"
    # log and break
    break
  fi

  # Re-run gates
  make check && \
    go test -race -timeout 10m ./... && \
    ( cd web && npm run lint:strict ) && \
    ( cd web && npm run typecheck ) && \
    ( cd web && npm test )

  if [ $? -ne 0 ]; then
    git reset --hard HEAD
    echo "round $round: autofix broke gates — rolled back"
    # log which gate failed and break (do NOT push a broken commit)
    break
  fi

  # Commit + push (no -A; stage only the files autofix touched)
  git add <modified files>
  git commit -m "improve: address coderabbit review (round $round)"  # + Co-Authored-By trailer
  git push
  NEW_SHA=$(git rev-parse --short HEAD)
  echo "round $round: pushed $NEW_SHA"
  # log "round $round: pushed $NEW_SHA, addressed N comments" and continue

done
```

For each round, append one line to `== Wrap-up ==` in the state file: `round <N>: <outcome>` where outcome is one of `pushed <sha> (cleared M comments)`, `0 actionable comments`, `autofix no-op`, `autofix broke <gate>, rolled back`, or `coderabbit timeout`.

Stop conditions (any breaks the loop):
- CodeRabbit's latest review on the current HEAD reports `Actionable comments posted: 0`.
- `coderabbit:autofix` produces no file changes.
- Gates fail after autofix (rolled back, do not push).
- Poll for the current HEAD's review times out (15 min).
- `round` reaches `max_wrap_iterations` (loop exit). Log `wrap-iter cap reached, leaving remaining comments for human`.

W5. Verify the PR reflects the latest state. Run `gh pr view "$PR" --json headRefOid -q .headRefOid` and confirm it matches `git rev-parse HEAD`. Log a mismatch under `== Wrap-up ==`.

W6. Output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` and stop. The branch is pushed, the PR is open, and the wrap-up rounds either resolved CodeRabbit cleanly or left a logged audit trail of what didn't apply.

== COMPLETION PROMISE RULES ==

Output `<promise>NO_FINDINGS_WORTH_FIXING</promise>` ONLY when at least one of:
- PHASE 2 wrap-up has finished (cleanly or with a logged failure).
- You are on the wrong branch (safety check from the rules above) — skip wrap-up in this case.

Do NOT output the promise to escape the loop because "this is hard" or "I am stuck". The loop is designed to continue through failed attempts — that is what `failed-once`/`failed-twice` tracking is for. The two normal exit paths both go through PHASE 2 first: no-eligible-findings (step 2) and max-iterations-reached (step 8).

== PROJECT-SPECIFIC DO NOTs ==

- Do not mock the database in integration tests.
- Do not split `cmd/slabledger/main.go` (accepted tech debt).
- Do not parse card numbers from titles — only from cert lookup.
- Do not touch files outside the scope of the finding. Scope discipline over cleanup.'
```

## Morning review checklist

```bash
git log --oneline main..HEAD         # what got committed
cat .claude/overnight-run-state.md   # every attempt + the == Wrap-up == section
git diff main..HEAD --stat           # scope of changes
gh pr view                           # PHASE 2 should have opened a PR; check CodeRabbit status
```

The loop's PHASE 2 pushes the branch, opens a PR against `main`, then runs up to 3 rounds of CodeRabbit address-and-push: each round waits up to 15 min for CodeRabbit to review the *current HEAD* (matched by `commit_id`), runs `coderabbit:autofix`, re-runs gates, and pushes the fixes as an additional commit. Reading the `== Wrap-up ==` section shows what each round did — pushed a fix, hit zero actionable comments, autofix no-op, gates rolled back, or timeout.

For each commit decide: keep, `git revert`, or `git rebase -i` to drop. The PR is already open, so cherry-picking via the GitHub UI works directly.

Exit the bypass-mode session before doing interactive review work, so subsequent sessions return to the plan-mode default.

## Design notes

- **Why one branch instead of branch-per-fix?** Each iteration runs `/improve` against the branch HEAD. A branch-per-fix layout would either force PR chaining or leave each iteration blind to prior fixes. Accumulating commits on one branch solves both.
- **Why `file:line + category` as the dedupe key?** Finding titles shift between `/improve` runs as code changes around them. File + category is stable.
- **Why allow Large-effort findings?** Guardrails are the verification gates plus the single-branch rollback, not a size cap. Failed fix attempts roll back cleanly.
- **Why a state-file `iteration` counter?** Ralph-loop fires the prompt N times but doesn't tell the prompt which fire it's on. Each iteration may produce 0–1 attempt entries (failed-once gets retried under the same entry), so counting attempts is not the same as counting fires. The frontmatter counter is the cheapest way for the prompt to know "this is the last fire" so PHASE 2 can run before the loop ends.
- **Why is wrap-up inside the loop instead of a separate post-step?** It needs to fire on both exit paths (no eligible findings AND max-iterations reached) without the user being awake. Folding it into the prompt — gated by trigger conditions — guarantees a PR + CodeRabbit pass on either exit, with no wrapper script needed.
- **Why 15 min per CodeRabbit poll?** CodeRabbit normally posts within 2–5 min on a typical PR; 15 min absorbs queue delays without burning the night. If it times out, the PR is still open and you handle it in the morning.
- **Why match reviews by `commit_id`?** CodeRabbit posts a fresh review for every push. Without the SHA match, round 2 of the wrap loop would immediately "see" round 1's stale review and try to autofix already-applied comments. Matching `commit_id == git rev-parse HEAD` forces each round to wait for *its own* review.
- **Why `max_wrap_iterations: 3`?** Empirically (e.g. PR #181), the first autofix round dropped 12 actionable comments to 4, so a single pass leaves work on the table. Three rounds covers most cases without unbounded looping; raise via the state-file field if you want more.
- **Runtime budget.** 15 iterations × ~20-40 min = 5-10 hours, plus up to ~75 min for PHASE 2 (3 wrap rounds × ~25 min worst-case each). Large-effort iterations can run 90+ minutes. If you want a tighter ceiling, drop `--max-iterations` or `max_wrap_iterations`.

## Critical files referenced

- `/workspace/.claude/skills/improve/SKILL.md` — the `/improve` skill (output format, memory file location)
- `/home/vscode/.claude/projects/-workspace/memory/improve_findings.md` — `/improve`s cross-run memory (preserved by the skill itself; no loop action needed)
- `/workspace/.claude/overnight-run-state.md` — this loop's per-night state (gitignored via existing `.claude/` entry)
- `/workspace/CLAUDE.md` — project conventions the loop must respect
- `/home/vscode/.claude/CLAUDE.md` — global preferences (the `git reset` rule is the one explicitly overridden)
