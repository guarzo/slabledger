---
name: polish-all
description: "Full-codebase polish pass — runs /improve + diff-based review with auto-fix across all 12 architecture-aligned segments with progress checkpointing. Use when the user asks for a full codebase polish, quality sweep, or systematic review."
argument-hint: "[--fresh | --resume | --report | --segment N]"
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Edit", "task", "skill"]
---

# Polish-All — Full Codebase Coverage

You are orchestrating a systematic review of the entire codebase. For each architecture-aligned segment, you run two passes: a systemic health check (`/improve` via skill) and a diff-based review with auto-fix (`/polish` pipeline inline). Progress is checkpointed so runs can resume, and all findings accumulate in a single report.

**Arguments**: $ARGUMENTS

---

## Step 0: Parse Arguments

Parse the arguments string:

| Token | Effect |
|-------|--------|
| `--fresh` | Ignore existing checkpoint, start from scratch |
| `--resume` | Explicitly resume from checkpoint (error if none exists) |
| `--report` | Print the cumulative report and stop — do not run any analysis |
| `--segment N` | Run only segment N (1-12). Useful for retrying a failed segment |

Default (no args): resume if a checkpoint exists with pending/failed segments, otherwise start fresh.

---

## Step 1: Initialize

### 1a: Verify git environment

Run `git rev-parse --is-inside-work-tree`. If not in a git repo, print an error and **STOP**.

### 1b: Define segments

Use these **hardcoded** segment definitions:

| # | Name | Paths |
|---|------|-------|
| 1 | `domain/inventory` | `internal/domain/inventory/` |
| 2 | `domain/advisor+social+scoring` | `internal/domain/advisor/`, `internal/domain/social/`, `internal/domain/scoring/` |
| 3 | `domain/favorites+picks+cards+auth+small` | `internal/domain/favorites/`, `internal/domain/picks/`, `internal/domain/cards/`, `internal/domain/auth/`, `internal/domain/errors/`, `internal/domain/constants/`, `internal/domain/observability/`, `internal/domain/storage/`, `internal/domain/intelligence/`, `internal/domain/timeutil/`, `internal/domain/mathutil/`, `internal/domain/pricing/` |
| 4 | `domain/decomposed-siblings` | `internal/domain/arbitrage/`, `internal/domain/portfolio/`, `internal/domain/tuning/`, `internal/domain/finance/`, `internal/domain/export/`, `internal/domain/dhlisting/`, `internal/domain/csvimport/`, `internal/domain/mmutil/` |
| 5 | `adapters/httpserver` | `internal/adapters/httpserver/` |
| 6 | `adapters/storage/sqlite` | `internal/adapters/storage/sqlite/` |
| 7 | `adapters/clients` | `internal/adapters/clients/` |
| 8 | `adapters/scheduler+scoring+advisor` | `internal/adapters/scheduler/`, `internal/adapters/scoring/`, `internal/adapters/advisortool/` |
| 9 | `platform` | `internal/platform/` |
| 10 | `cmd` | `cmd/` |
| 11 | `testutil` | `internal/testutil/` |
| 12 | `web` | `web/` |

### 1c: Load or create checkpoint

The checkpoint file is `docs/polish-progress.json`.

**If `--report` was specified:** Read `docs/polish-report.md` and print it. **STOP** — do not run any analysis.

**If `--fresh` was specified:** Delete existing checkpoint and report files if they exist. Create a fresh checkpoint with all segments set to `pending`.

**If `--resume` was specified:** Read the checkpoint file. If it doesn't exist, print "No checkpoint found. Run without --resume to start fresh." and **STOP**.

**If neither `--fresh` nor `--resume` (default behavior):**
- If checkpoint exists AND all segments are `completed`: Ask "Previous run completed all 12 segments. Start fresh or view report?" Wait for response.
- If checkpoint exists AND some segments are `pending` or `failed`: Print "Resuming from segment N. N segments remaining." and continue.
- If no checkpoint exists: Create a fresh one.

**If `--segment N` was specified:** Load the checkpoint (or create fresh if none exists). Run ONLY segment N, regardless of its current status. After completion, print findings for that segment only.

### 1d: Checkpoint format

When creating a fresh checkpoint, write this structure to `docs/polish-progress.json`:

```json
{
  "base_commit": "<current HEAD commit hash>",
  "started_at": "<ISO 8601 timestamp>",
  "segments": [
    {
      "id": 1,
      "name": "domain/inventory",
      "paths": ["internal/domain/inventory/"],
      "status": "pending"
    },
    ...all 12 segments...
  ]
}
```

Get the base commit by running `git rev-parse HEAD`.

### 1e: Create report file

If `docs/polish-report.md` doesn't exist (or `--fresh`), create it with:

```markdown
# Polish-All Report
Base commit: <hash> | Started: <date>
```

---

## Step 2: Execute Per-Segment Pipeline

For each segment in the checkpoint that has status `pending` or `failed` (or for the single segment if `--segment N`):

### 2a: Mark in-progress

Update the segment's status to `in_progress` in the checkpoint file. Save the checkpoint.

Print: `\n═══════════════════════════════════════════════════════════════`
Print: `Segment <N>/12: <segment-name>`
Print: `Paths: <comma-separated paths>`
Print: `═══════════════════════════════════════════════════════════════\n`

### 2b: Phase A — Improve (systemic health check)

Load the `improve` skill using the `skill` tool.

Execute the improve workflow with these modifications:
- **Scope**: Pass the segment name as the scope argument. For domain packages, use the package name (e.g., `inventory`, `arbitrage`). For multi-package segments, run improve once per package in the segment and merge findings.
- **Skip interactive follow-up**: Do NOT execute Step 6 (interactive follow-up) of the improve skill. Capture findings only.
- **Skip memory file**: Do NOT save to `improve_findings.md`. Findings go to the cumulative report only.

Capture the top-10 findings from the improve output. Store the count as `improve_findings` on the segment.

If improve fails for this segment, log the error and set `improve_findings` to 0. Do NOT stop — proceed to Phase B.

### 2c: Phase B — Polish (diff-based review + auto-fix)

Execute the `/polish` pipeline inline for this segment. The full pipeline is defined below, adapted from the `/polish` command with segment scoping.

**Base commit**: Use the `base_commit` from the checkpoint file.

**Scope**: Only analyze files whose paths start with any of the segment's paths.

#### Phase B.0: Validate

Run `git diff --stat <base_commit>..HEAD -- <path1> <path2> ...` for all segment paths. If no changes in this segment, print "No changes in this segment since base commit" and skip to 2d with `polish_fixed: 0, polish_needs_review: 0`.

#### Phase B.1: Detect stack & conventions

Scan for language markers. Load language rules from `~/.config/opencode/skills/code-simplifier/rules/` based on file extensions in the diff:
- `.go` → `rules/go.md`
- `.ts`, `.tsx`, `.js`, `.jsx` → `rules/typescript.md`
- `.py` → `rules/python.md`
- `.rb` → `rules/ruby.md`
- `.ex`, `.exs` → `rules/elixir.md`
- `.rs` → `rules/rust.md`
- `.java` → `rules/java.md`

Load `CLAUDE.md` at project root for conventions.

#### Phase B.2: Gather the diff

1. Run `git diff <base_commit>..HEAD -- <paths>` for segment paths.
2. Run `git diff --name-status <base_commit>..HEAD -- <paths>` for the file list.
3. Skip generated files: `*.lock`, `*.min.*`, `dist/`, `build/`, `vendor/`, `node_modules/`, `*.generated.*`, `*.pb.go`, `*_generated.ts`, `migrations/`, `__snapshots__/`, `*.snap`.
4. For each added/modified file, read the full file (not just hunks).
5. Apply large diff tiers:
   - Under 30 files: full analysis
   - 30-80 files: full diff, read full contents for top 20 by lines changed
   - Over 80 files: analyze top 40 by change size

#### Phase B.3: Analyze

Run all 9 sub-analyses from the `/polish` command. Each finding is classified as HIGH or MEDIUM confidence, and `auto-fix` or `report` action type.

**3a: Bugs & Security** — off-by-one, null deref, race conditions, injection, missing validation. All findings: action `report`.

**3b: Idiomatic Code** — check against loaded language rules. HIGH confidence idiom fixes: `auto-fix`. MEDIUM: `report`. Naming suggestions: always `report`.

**3c: Codebase Pattern Adherence** — compare against surrounding unchanged files. All: `report`.

**3d: Duplication Detection** — search for existing utilities, standard library replacements. Standard library at HIGH confidence: `auto-fix`. Others: `report`.

**3e: Over-Engineering Detection** — unnecessary interfaces, premature generalization, deep abstraction stacks. All: `report`.

**3f: Comment Quality** — tautological comments at HIGH confidence: `auto-fix`. Stale/noise/journal: `report`. Preserve why-comments, warnings, legal headers.

**3g: Dead Code & Dead Abstractions** — use Grep/Glob to verify liveness across full codebase (not just segment). Dead exports/parameters/branches/orphaned abstractions. Most: `report`. Dead branches at HIGH: `auto-fix`.

**3h: Structural Simplification** — early returns, guard clauses. HIGH confidence without defer/finally: `auto-fix`. With cleanup logic: `report`.

**3i: Dispatch Subagents** — dispatch these in parallel using the `task` tool:

1. **code-reviewer** (`subagent_type: "code-reviewer"`) — quality/fragility review
2. **silent-failure-hunter** (`subagent_type: "silent-failure-hunter"`) — swallowed errors
3. **comment-analyzer** (`subagent_type: "comment-analyzer"`) — cross-reference comment accuracy (skip if <5 comment lines in diff)
4. **type-design-analyzer** (`subagent_type: "type-design-analyzer"`) — type/interface design (skip if no type definitions in diff)

Each subagent receives: changed file list, full diff, base commit, stack info, conventions.

Deduplicate: merge subagent findings into main list. Same file + overlapping lines (within 5) + same issue = duplicate → keep Phase 3 finding. If subagent adds context, append it. When in doubt, keep both.

Timeout: 90 seconds per subagent. Log timeouts and proceed.

#### Phase B.4: Apply auto-fixes

Apply all `auto-fix` classified findings using the Edit tool. Process files one at a time, top-to-bottom within each file.

After all fixes, do an **import cleanup pass**: scan each modified file for imports orphaned by this phase's changes. Grep the file for references before removing any import.

Log every change: `Fixing: file:line — description`

#### Phase B.5: Generate segment report

Count: `polish_fixed` (number of auto-fixes applied), `polish_needs_review` (number of report items).

**Do NOT run Phase 6 (interactive follow-up).** All findings go to the cumulative report.

### 2d: Commit auto-fixes

If any auto-fixes were applied in Phase B.4, commit them:

```bash
git add -A
git commit -m "polish-all: auto-fix segment <N> (<segment-name>)"
```

If no fixes were applied, skip the commit.

### 2e: Update checkpoint

Update the segment in `docs/polish-progress.json`:

```json
{
  "id": N,
  "name": "<name>",
  "paths": [...],
  "status": "completed",
  "improve_findings": <count>,
  "polish_fixed": <count>,
  "polish_needs_review": <count>,
  "completed_at": "<ISO 8601 timestamp>"
}
```

If either phase had an error, set status to `failed` and add `"error": "<description>"`.

Save the checkpoint file.

### 2f: Append to cumulative report

Append the following to `docs/polish-report.md`:

```markdown
---
## Segment N: <segment-name> (<file-count> files)

### Improve Findings (<count>)
<numbered list of improve findings>

### Polish — Fixed (<count>)
<list of auto-fixes applied, one per line: ✓ file:line — description>

### Polish — Needs Review (<count>)
<numbered list of report items, grouped by category>
<format: N. ⚠ [Severity|Confidence] file:line — Description>
```

If a phase was skipped (no changes), note it: "No changes in this segment since base commit."
If a phase failed, note it: "Phase A/B failed: <error description>"

### 2g: Continue to next segment

Move to the next pending/failed segment. Repeat from 2a.

---

## Step 3: End-of-Run Summary

After all segments are processed, append a summary table to `docs/polish-report.md`:

```markdown
---
## Summary

| Segment | Improve | Fixed | Needs Review | Status |
|---------|---------|-------|-------------|--------|
| domain/inventory | 7 | 4 | 12 | ✓ |
| domain/advisor+social+scoring | 5 | 2 | 8 | ✓ |
| ... | ... | ... | ... | ... |
| **Total** | **N** | **N** | **N** | **N/12 done** |
```

Status column: `✓` for completed, `✗` for failed, `—` for skipped, `○` for pending.

Print the summary table to the console as well.

Print: `\nPolish-all complete. Report saved to docs/polish-report.md`
Print: `Checkpoint saved to docs/polish-progress.json`

If any segments failed: `\nN segment(s) failed. Retry with: /polish-all --segment N`

---

## Important Notes

- **Behavior preservation is paramount.** All auto-fixes must be behavior-preserving. When in doubt, classify as `report`.
- **Ground every finding in evidence.** Use Grep and Glob to search the codebase. Never report findings based on guesses.
- **One segment at a time.** Do not parallelize segments — sequential execution prevents commit conflicts.
- **Do not prompt between segments.** The entire run is non-interactive. Review the cumulative report afterward.
- **Context management.** Each segment involves substantial analysis. Compress completed segment context before starting the next segment to maintain a clean context window.
- **Import cleanup.** After auto-fixing, always check for orphaned imports before committing.
