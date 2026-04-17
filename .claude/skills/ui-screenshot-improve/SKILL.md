---
name: ui-screenshot-improve
description: Visual UI improvement skill for SlabLedger. Captures fresh screenshots of every page via `make screenshots`, analyzes them systematically for UI issues (layout, spacing, hierarchy, user flow, empty states, affordances, contrast, status semantics), then identifies and fixes the top 3 highest-priority issues with build verification after each fix. Use this whenever the user asks about UI polish, visual bugs, layout problems, UX quality, or wants to run a visual improvement pass. Designed to work as a self-contained unit inside an overnight improvement loop — one call equals one complete screenshot cycle plus up to 3 verified fixes.
---

# UI Screenshot Improve

A self-contained visual improvement skill: **capture → analyze → rank → fix → verify**.

Each invocation takes fresh screenshots, finds the top 3 impactful UI issues across all pages, implements targeted code fixes, and verifies each with a build check.

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

## Step 3: Analyze each screenshot across eight dimensions

Look at each page with fresh eyes, as if you've never seen this app before. Ask: *what would confuse or frustrate a real user here?* Work through each dimension:

**1. Layout and spatial composition**
Alignment, breathing room, and grid coherence. Are elements sitting at awkward sizes relative to each other? Is whitespace distributed intentionally — generous where it guides the eye, tight where it creates grouping? Look for: orphaned elements, uneven column widths, cards that are way too tall/short for their content, items that wrap in ugly ways, sections that feel unanchored or disconnected from the rest of the page.

**2. Typography hierarchy**
Do heading sizes, weights, and colors create an unmistakable reading order? Primary values (dollar amounts, counts, status) should dominate their labels. Secondary metadata should recede. Look for: body text that's the same size as headings, labels visually competing with their values, numbers that are hard to parse because font weight doesn't differentiate them, line lengths that are too wide to read comfortably.

**3. Information hierarchy**
Are the most important data points the most visually prominent? A new user should be able to name the 3 most important facts on a page within 2 seconds. Look for: critical metrics buried in a sea of equal-weight items, actionable states hidden in muted text, key numbers that require hunting to find.

**4. Empty states, zero states, and loading states**
Pages or sections that have no data (zero campaigns, no results, nothing pending) should look intentional and helpful — not broken. Look for: blank sections with no explanation, raw "0" values with no context, tables with headers but no rows that look like a rendering failure.

**5. User flow and primary actions**
The most important action on each page should be the most visually prominent. Navigation between related pages should feel obvious. Look for: primary buttons buried at the bottom of a long form, CTAs that blend into the background, pages where it's unclear what the user is supposed to do next, related data split across tabs in a way that forces unnecessary clicks.

**6. Interaction clarity and affordances**
Clickable elements should look clickable. Expandable rows, sortable columns, and hoverable chips should have visual cues. Look for: rows that expand on click but have no visual indicator, buttons with insufficient padding that are hard to click, chips or badges that look interactive but aren't, or vice versa.

**7. Contrast and readability**
Muted/secondary text should still be legible at normal reading distance. Low-contrast text that serves a functional purpose (not just decorative) is a bug. Look for: timestamps, labels, or secondary values that are effectively invisible against the background.

**8. Status semantics**
Errors and critical states must look alarming. Neutral states (not yet configured, nothing pending) should look calm. Warnings should be visually distinct from both. Look for: danger red on non-error states, neutral white on genuinely alarming values, warning/success colors applied unconditionally regardless of the actual value.

## Step 4: Rank all findings

After reviewing all screenshots, list every identified issue and rank by:

🔴 **Critical** — actively confuses or misleads users, looks broken, or hides important operational state
🟡 **Important** — usability friction, unclear hierarchy, data that's hard to scan or act on
🟢 **Polish** — minor aesthetics, consistency, density refinements

**Select the top 3 highest-priority findings by impact, not ease of fix.**

Before finalizing the top 3, apply this diversity constraint: **at most 1 of the 3 selected fixes may be a semantic color change** (adding/changing a color class or CSS variable on an existing element). If your top 3 are all color tweaks, bump the most impactful non-color finding into the top 3 and drop the weakest color fix.

Skip any finding that would require touching more than 3 files — log it as out-of-scope. Layout changes within a single component (spacing, sizing, element order, adding/removing a wrapper div) are fair game.

## Step 5: Fix each issue (one at a time)

For each of the 3 selected issues, in priority order:

1. **Trace it to source** — identify the responsible React/TypeScript file in `web/src/`. Read the file before editing.
2. **Implement the fix** following project conventions:
   - Tailwind utility classes for all styling: spacing (`p-`, `m-`, `gap-`, `space-y-`), layout (`flex`, `grid`, `items-`, `justify-`), sizing (`w-`, `h-`, `max-w-`), typography (`text-`, `font-`, `leading-`)
   - CSS variables for semantic colors: `var(--text)`, `var(--text-muted)`, `var(--danger)`, `var(--warning)`, `var(--success)`, `var(--brand-400)`, `var(--surface-1)`, `var(--surface-2)`
   - No new npm dependencies, no new abstractions beyond what the fix requires
3. **Verify**: `cd /workspace/web && npm run build`
   - Exit 0 → fix stands ✅
   - Non-zero → revert to original, mark ❌, continue

## Step 6: Capture after-screenshots

```bash
cd /workspace && make screenshots
```

This overwrites `web/screenshots/` with post-fix renders. If the run fails, note it but don't revert — the build already passed.

## Step 7: Return a structured results report

```
## UI Screenshot Improve — Results

### All findings (ranked)
1. 🔴 [Title] — [Page] — [one-line description]
2. 🟡 [Title] — [Page] — [one-line description]
...

### Top 3 — outcomes
1. ✅ [Title] — `path/to/file.tsx` — [what changed]
2. ✅ [Title] — `path/to/file.tsx` — [what changed]
3. ❌ [Title] — build failed, reverted

### Not attempted (out of scope or below top 3)
- [brief list]

### After-screenshots
✅ Re-captured — `web/screenshots/` updated
```

## Constraints

- **Frontend only** — `web/src/` files only. Go files are out of scope.
- **Scope discipline** — More than 3 files = out of scope for this skill.
- **Build gate is mandatory** — Always revert on build failure.
- **No color-only runs** — The diversity constraint in Step 4 is not optional. If every finding you see is a color fix, look harder at layout, spacing, and flow until you find a real structural issue.
- **Project conventions** — See `/workspace/CLAUDE.md`.
