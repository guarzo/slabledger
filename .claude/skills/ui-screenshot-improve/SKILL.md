---
name: ui-screenshot-improve
description: Visual UI improvement skill for SlabLedger. Captures fresh screenshots of every page via `make screenshots`, analyzes them systematically for UI issues (contrast, hierarchy, status semantics, density, legibility, misleading states), then identifies and fixes the top 3 highest-priority issues with build verification after each fix. Use this whenever the user asks about UI polish, visual bugs, layout problems, or wants to run a visual improvement pass. Designed to work as a self-contained unit inside an overnight improvement loop — one call equals one complete screenshot cycle plus up to 3 verified fixes.
---

# UI Screenshot Improve

A self-contained visual improvement skill: **capture → analyze → rank → fix → verify**.

Each invocation takes fresh screenshots, identifies the top 3 UI issues across all pages, implements targeted code fixes, and verifies each with a build check.

## Step 1: Capture fresh screenshots

```bash
cd /workspace && make screenshots
```

This builds the Go backend, builds the frontend, starts the server with local data (`data/slabledger.db`), runs Playwright, and saves screenshots to `web/screenshots/`. If the command fails (non-zero exit), stop immediately — do not analyze stale screenshots.

Expected output files (desktop):
```
web/screenshots/dashboard.png
web/screenshots/inventory.png
web/screenshots/inventory-expanded.png
web/screenshots/campaigns.png
web/screenshots/campaign-detail.png
web/screenshots/admin-integrations.png
web/screenshots/admin-pricing.png
web/screenshots/admin-stats.png
web/screenshots/admin-users.png
web/screenshots/tools.png
```

## Step 2: Read all desktop screenshots in parallel

Use the Read tool on all desktop screenshots simultaneously in a single message. Read mobile screenshots (`web/screenshots/mobile/`) only if a desktop screenshot reveals a layout concern that likely has a mobile dimension.

## Step 3: Analyze each screenshot

For each screenshot, assess these five dimensions:

**Information hierarchy** — Are the most important data points the most visually prominent? Labels should clearly defer to their values. Counts and numbers should be easy to parse at a glance without hunting for context.

**Contrast and readability** — Column headers, secondary labels, and muted text should be legible without competing with primary content. Low-contrast text that's meant to be read (not decorative) is a bug.

**Status semantics** — Errors and critical operational states must look alarming (red/danger). Expected/neutral states (e.g., "not connected" when no credentials are saved) should look calm and informational, not alarming. Warning states should be visually distinct from both. Mismatched semantics (red error for a non-error state, or a critical metric shown in neutral text) are high-priority findings.

**Density balance** — Layouts that are too crowded are hard to scan. Layouts with large wasted whitespace feel broken and miss an opportunity to show useful context.

**Actionability and self-explanation** — Status badges, chips, and counts should be self-explanatory. A new user glancing at the page should understand what they're seeing without relying on learned knowledge of the codebase.

## Step 4: Rank all findings

After reviewing all screenshots, list every identified issue and rank by:

🔴 **Critical** — confuses users, looks broken, or actively misleads (e.g., red error banner for a non-error state, operational failure metrics shown without visual alarm, critical counts in unreadable low-contrast text)

🟡 **Important** — usability friction, unclear hierarchy, data that should surface but doesn't read prominently enough, misleading neutral state where a colored state is warranted

🟢 **Polish** — aesthetics, minor consistency, density refinements, tooltip copy improvements

Select the **top 3** highest-priority findings. Within a priority tier, prefer issues that can be fixed with the smallest file footprint (1–2 files).

Skip any finding that would require touching more than 3 files or that involves structural layout changes beyond CSS/class tweaks — log it as a finding but mark it out-of-scope.

## Step 5: Fix each issue (one at a time)

For each of the 3 selected issues, working in priority order:

1. **Identify the responsible component file** — trace the screenshot element to its React/TypeScript source. Touch the minimum number of files.
2. **Read the file** before editing — never edit without reading first.
3. **Implement the fix** following the project's existing conventions:
   - Tailwind utility classes
   - CSS variables: `var(--text)`, `var(--text-muted)`, `var(--danger)`, `var(--warning)`, `var(--success)`, `var(--brand-400)`, `var(--surface-1)`, `var(--surface-2)`, etc.
   - No new npm dependencies
   - No new abstractions or helper functions beyond what the change requires
4. **Verify**: `cd /workspace/web && npm run build`
   - Exit 0 → fix stands ✅
   - Non-zero → revert the file to its original content, mark fix as ❌ failed, continue to next issue
5. Proceed to the next issue regardless of success or failure.

## Step 6: Capture after-screenshots

After all fixes are applied (regardless of individual success/failure), run `make screenshots` again to capture the post-fix state:

```bash
cd /workspace && make screenshots
```

This overwrites `web/screenshots/` with the updated renders. These become the baseline for the next invocation and are included in any PR diff for visual review. If this second run fails, note it in the report but do not revert the fixes — the build already passed.

## Step 7: Return a structured results report

```
## UI Screenshot Improve — Results

### All findings (ranked)
1. 🔴 [Title] — [Page] — [one-line description]
2. 🟡 [Title] — [Page] — [one-line description]
3. 🟡 [Title] — [Page] — [one-line description]
4. 🟢 [Title] — [Page] — [one-line description]
(continue for all found)

### Top 3 — outcomes
1. ✅ [Title] — `path/to/file.tsx` — [one-line description of what changed]
2. ✅ [Title] — `path/to/file.tsx` — [one-line description of what changed]
3. ❌ [Title] — build failed, reverted

### Not attempted (out of scope or below top 3)
- [brief list]

### After-screenshots
✅ Re-captured — `web/screenshots/` updated with post-fix renders
❌ Re-capture failed — screenshots may be stale
```

## Conventions and constraints

- **Frontend only** — This skill touches React/TypeScript files in `web/src/`. Go files and backend code are out of scope.
- **Scope discipline** — If a fix requires more than 3 files, it belongs in a dedicated task, not this skill. Log it and move on.
- **Build gate is mandatory** — Never leave a broken build. If the build fails, always revert before moving on.
- **No regressions** — Fixes should be surgical. Prefer changing a class name or a color variable over restructuring a component.
- **Project conventions** — See `/workspace/CLAUDE.md` for project-specific rules. The key ones: Tailwind + CSS vars for styling, `var(--danger/warning/success)` for semantic colors.

## Integration with overnight-improve

This skill is designed as a **self-contained unit of work**: one invocation = one screenshot cycle + up to 3 fixes. It handles its own verification.

To wire into overnight-improve's loop, update overnight-improve to call this as a "do" action rather than a "get findings" action — it does not return unimplemented findings. Overnight-improve's role is to run gates (from `overnight-config.yaml`) and commit after this skill completes.

Suggested `overnight-config.yaml` addition:
```yaml
ui_improve_skill: workspace:ui-screenshot-improve
```
