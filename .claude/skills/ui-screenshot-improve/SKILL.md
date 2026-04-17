---
name: ui-screenshot-improve
description: Visual UI improvement skill for SlabLedger, focused on finding and removing points of user friction — places where a real user would get confused, stuck, or blocked. Captures fresh screenshots of every page via `make screenshots`, walks canonical user journeys, audits each page through a friction-first lens (unclear next step, hidden actions, ambiguous state, broken-looking empty states, illegible info), then identifies and fixes the top 3 highest-friction issues with build verification and a before/after regression check. Use whenever the user asks about UI polish, UX quality, visual bugs, usability, layout problems, or wants to run a visual improvement pass. Self-contained unit for overnight improvement loops — one call equals one screenshot cycle plus up to 3 verified fixes, with a persistent friction log that lets successive iterations converge instead of re-litigating the same findings.
---

# UI Screenshot Improve

A self-contained visual improvement skill:
**load log → capture → journey check → per-page audit → rank → fix → recapture → regress check → report & log**.

Each invocation reads what prior runs already fixed, takes fresh screenshots, walks the primary user journeys, finds the top 3 highest-friction issues, implements targeted code fixes with build verification, then confirms the fixes didn't introduce new friction before reporting.

## Known blind spots

This skill audits static screenshots. It cannot see hover/focus states, keyboard paths, transitions, loading/toast states, or touch-target hit areas. Friction in those dimensions must come from the interactive probe in Step 4.75 or a user report — do not infer them from stills, and don't claim "no issues" for behavior the skill physically can't observe.

## Step 1: Load the friction log

```bash
cat web/screenshots/friction-log.md 2>/dev/null || echo "No prior log — this is iteration 1."
```

The friction log is a per-cycle record of what was found, what was fixed, and what was deferred. You'll append to it at the end of this run (Step 10). Reading it first prevents:

- Re-raising findings that were already fixed in a prior cycle (they'd show up on fresh screenshots as "fine" but might still read as imperfect).
- Re-raising findings that were deferred for a principled reason (out of scope, intentional design choice noted by the user).
- Spinning on the same 🔴 across iterations when the first fix didn't fully resolve it — if something keeps recurring, the note in the log tells you to dig deeper or escalate to out-of-scope instead of trying the same fix again.

If the log exists, scan the "Deferred / intentional", "Recurring", and "Design conventions ratified" sections carefully. The first two are a suppression list for findings; the third is a suppression list for proposals that would undo a locked design decision.

## Step 2: Capture fresh screenshots

```bash
cd /workspace && make screenshots
```

This builds the Go backend, builds the frontend, starts the server with local data (`data/slabledger.db`), runs Playwright, and saves screenshots to `web/screenshots/`. If the command fails (non-zero exit), stop immediately — do not analyze stale screenshots.

Expected output files (desktop):
```
web/screenshots/login.png
web/screenshots/dashboard.png
web/screenshots/insights.png
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

Mobile equivalents land under `web/screenshots/mobile/`.

## Step 3: Read screenshots in parallel

Use the Read tool on these screenshots simultaneously in a single message:

**Always read mobile AND desktop for the high-traffic pages** (the user actually uses these on a phone):
- `dashboard`, `insights`, `inventory`, `inventory-expanded`, `campaigns`, `campaign-detail`

**Desktop only by default** for the lower-traffic pages:
- `admin-*`, `tools`

Pull the mobile versions of the lower-traffic pages only if their desktop screenshot surfaces a layout concern that likely has a mobile dimension (wide tables, multi-column forms, sticky headers, dense cards).

## Step 4: Journey sanity check

Before drilling into individual pages, walk the screenshots in the order a real user would hit them. The goal is to catch **cross-page friction** — friction that's invisible in any single screenshot but obvious across a sequence.

Walk these three canonical journeys and ask, at each transition: *"Would the user know what to do next? Where their data went? Whether their last action succeeded?"*

**Journey A — New intake to listed**
`dashboard → inventory → inventory-expanded → (hypothetical push action) → campaign-detail`
*"I just received a slab. How do I get it listed for sale?"*

**Journey B — Campaign health check**
`dashboard → campaigns → campaign-detail → inventory (filtered)`
*"Is campaign X profitable? What's aging? What should I liquidate?"*

**Journey C — "Something's wrong, find and fix it"**
`dashboard (notice a stale price / red status / empty stat) → admin-integrations (diagnose) → admin-pricing (re-check) → dashboard (confirm resolved)`
*"I just saw something that looked wrong. Can I diagnose it, fix it, and confirm the fix from the dashboard?"*
This journey surfaces the handoff between operational signals and recovery surfaces — friction that's invisible when admin pages are walked in isolation.

For each journey, note: broken handoffs, disappearing state, unclear success/failure signals, unnecessary navigation, dead-ends. Journey-level findings are first-class and frequently outrank per-page findings.

## Step 4.5: Design coherence check

Look at all screenshots together as a set — not as individual pages. Answer three questions, one short sentence each:

1. *What is the aesthetic direction?* (data-dense terminal, editorial, brutalist utilitarian, glassy modern, etc.) If you can't name it in one phrase, the product doesn't yet have a point of view — log `Design POV undefined` for the next cycle but do not count it toward the top-3.
2. *Is the direction consistent across pages?* Flag any single page that reads as if it belongs to a different product (admin pages with a different type ramp, card shell, or spacing rhythm than inventory; modals styled differently from their parent pages).
3. *What is the one memorable thing?* If every page's memorable element is "the data," the UI is doing no design work beyond rendering — that's a legitimate 🟡 finding on a dashboard product.

Coherence findings feed into Step 6's ranking but rarely outrank blocking friction.

## Step 4.75: Interactive probe (one page, ≤3 min)

Static screenshots hide hover, focus, keyboard paths, loading, and touch-targets. Pick the single page flagged as highest-risk by the journey check and probe it live:

```bash
cd /workspace/web && npm run dev
# Vite dev server + backend proxy — see CLAUDE.md "Frontend-Backend Integration"
```

In the browser:
- Tab through focusable elements once — is the focus ring visible on every stop? Does tab order match visual order?
- Hover the primary CTA and any row/chip that looks clickable — does the affordance change (cursor, background, outline)?
- Trigger an empty/loading/error state if reachable (e.g. slow network throttle, empty filter).
- On the mobile viewport (Chrome devtools → iPhone 14), confirm the primary action is reachable without horizontal scroll and touch targets feel ≥44px.

Log anything surprising into the audit — these findings are first-class and commonly outrank static ones. If nothing surprised you, say so explicitly. This step is bounded: one page, three minutes. It is a sanity check, not a full accessibility audit.

## Step 5: Friction-first audit (per page)

Now look at each page with fresh eyes, as if you've just logged in for the first time. The primary question on every screenshot is:

> *Where on this page would a real user hesitate, misread, or get stuck?*

We are hunting for **user friction** — moments where the UI fails the person trying to use it. Aesthetic issues matter only when they cause friction. Work the two tiers in order: find Tier A signals first, then use Tier B lenses to explain *why* the friction happens and where the fix lives.

### Tier A — Friction signals (what we're hunting)

**Action friction** — *"What can I do here, and how?"*
Is the primary action on this page obvious, reachable, and labeled for its outcome (not its mechanism)? Do clickable elements look clickable? Is there a clear next step, or does the page dead-end? Look for: primary CTAs buried below the fold or blended into the background, rows/chips that expand or navigate on click but have no visual affordance, forms with no visible submit, pages that show data but offer no action against it, destructive actions placed where a misclick would hurt.

**Comprehension friction** — *"What am I looking at?"*
A user should be able to name the 3 most important facts on a page in about 2 seconds. Numbers should be scannable at a glance. Labels should match the user's mental model, not the database column. Look for: critical metrics buried in a sea of equal-weight items, values that compete with their own labels for attention, numbers that are hard to parse because weight/size/alignment don't differentiate them, jargon or internal-ese where plain words would do, line lengths too wide to read comfortably.

**Flow friction** — *"What happens next? How do I get back?"*
Related actions should live together; navigation between related pages should feel obvious. Look for: related data or flows split across tabs or pages in a way that forces unnecessary back-and-forth, pages with no breadcrumb or obvious "back" path, sequences where the user has to remember state the UI could carry for them, multi-step flows where the current step is unclear.

**Trust friction** — *"Is this broken, or is it just empty?"*
Zero-states, loading states, and error states should all look *intentional*. A user should never have to wonder whether the page is broken. Look for: blank sections with no explanation, raw `0` values with no context (is that a count of zero, or did it fail to load?), tables with headers but no rows that read as a rendering failure, genuine errors shown as calm grey text, or neutral states shown in alarming red. Good zero-states also *teach* — "no campaigns yet, here's how to make one" removes more friction than a polite blank canvas.

**Mobile-shape friction** — *"Can this be used one-handed?"* (apply when reviewing a mobile screenshot)
Are touch targets roughly ≥44px square? Is the primary action reachable without scrolling past dense data or through a closed disclosure? Wide tables handled with horizontal-scroll-with-fade, stacked cards, or responsive reflow (not silently clipped)? Sticky headers or bottom bars covering the first/last data row? Modals using the full viewport, or rendered as a desktop-sized box with cramped margins?

### Tier B — Supporting craft lenses (use to diagnose Tier A findings)

These are the levers for fixing friction. Don't raise them as standalone findings — raise the friction issue from Tier A, then use Tier B to identify the mechanism.

- **Layout & spacing** — poor alignment, uneven grids, awkward wrapping, orphaned elements, or sections that feel disconnected. Common cause of comprehension and flow friction.
- **Typography hierarchy & voice** — size/weight ramp between page title → section label → body → meta (if H1 and H2 look nearly identical, reading order is guesswork); any visible pairing (display + text, or weight contrast) vs everything rendering in a single roman weight; leading/tracking appropriate for size (dense metadata tolerates tighter leading; long-form does not); tabular-nums on columns that must line up. Common cause of comprehension friction.
- **Contrast & readability** — functional text (timestamps, labels, secondary values) that's effectively invisible. Common cause of comprehension and trust friction.
- **Status semantics** — colors misrepresenting severity (danger red on neutral states, neutral grey on genuine alarms, success/warning applied unconditionally). Common cause of trust friction.
- **Data density & number craft** — for pages dominated by currency, counts, or tables. Numeric columns right-aligned with tabular numerals? Hero metrics with a distinct weight/size ramp vs their labels so the eye lands on the value first? Deltas carrying both direction (↑/↓) and semantic color, not color alone (a11y risk)? Zebra rows or row grouping used only when they actually aid scanning? Common cause of comprehension friction on dashboards, campaign-detail, inventory, and admin-stats.

## Step 6: Rank all findings by friction severity

After reviewing all screenshots and journeys, list every identified issue and rank by the friction it creates for a real user:

🔴 **Blocking friction** — a user cannot complete a task, misreads critical state, or reasonably concludes the page is broken. This is what we came for.
🟡 **Slowing friction** — a user completes the task but with measurable hesitation, backtracking, hunting, or rework.
🟢 **Polish** — no functional friction; aesthetic or consistency only.

### Every 🔴 finding must carry a user story

Before a finding counts as 🔴, write it as one sentence in this shape:

> *"A user trying to **[goal]** ends up **[wrong outcome]** because **[mechanism]**."*

Example: *"A user trying to find out whether a slab was listed ends up bouncing between inventory and campaign-detail because neither page shows the listing status after the push."*

If you can't write the sentence cleanly, it isn't 🔴 — downgrade to 🟡 or 🟢. This rule exists to prevent taste-based "spacing feels off" complaints from masquerading as blocking friction, and to make the log in Step 10 auditable.

### Selection rules

**Select the top 3 highest-friction findings by user impact, not ease of fix.**

- If every finding in your top 3 is 🟢 polish, you haven't looked hard enough — go back through the Tier A signals and find at least one real friction issue, even if the fix is small. A run that ships only polish fixes is a failed run.
- **Suppression**: skip any finding already logged as "Deferred / intentional" in the friction log unless you have new evidence it matters.
- **Diversity constraint**: at most 1 of the 3 selected fixes may be a semantic color change (adding/changing a color class or CSS variable on an existing element). Color tweaks rarely remove friction by themselves.
- **Scope cap**: skip any finding that would require touching more than 3 files — log it as out-of-scope. Layout changes within a single component (spacing, sizing, element order, adding/removing a wrapper div, surfacing a hidden action) are fair game.

## Step 7: Fix each issue (one at a time)

For each of the 3 selected issues, in priority order:

1. **Trace it to source** — identify the responsible React/TypeScript file in `web/src/`. Read the file before editing.
2. **Implement the fix** following project conventions:
   - Tailwind utility classes for all styling: spacing (`p-`, `m-`, `gap-`, `space-y-`), layout (`flex`, `grid`, `items-`, `justify-`), sizing (`w-`, `h-`, `max-w-`), typography (`text-`, `font-`, `leading-`)
   - CSS variables for semantic colors: `var(--text)`, `var(--text-muted)`, `var(--danger)`, `var(--warning)`, `var(--success)`, `var(--brand-400)`, `var(--surface-1)`, `var(--surface-2)`
   - No new npm dependencies, no new abstractions beyond what the fix requires
3. **Verify**: `cd /workspace/web && npm run build`
   - Exit 0 → fix stands ✅
   - Non-zero → revert to original, mark ❌, continue

## Step 8: Capture after-screenshots

```bash
cd /workspace && make screenshots
```

This overwrites `web/screenshots/` with post-fix renders. If the run fails, note it but don't revert — the build already passed.

## Step 9: Regression check

Build success does not mean UX success. For each page whose files you edited:

1. Re-read the post-fix screenshot (both desktop and mobile where you pulled both earlier).
2. Confirm the specific friction you targeted is actually resolved, not just papered over.
3. Scan the rest of the page for **new** friction the fix may have introduced — shifted layout, newly-misaligned elements, unintended contrast changes, broken empty states.

Record the outcome per fix:
- ✅ **Clean** — target friction resolved, no new friction observed.
- ⚠️ **Resolved with side effects** — target friction resolved but a new minor issue appeared; log it for the next cycle.
- ❌ **Regressed** — new friction is worse than what was fixed. Revert this specific fix and mark the finding unresolved.

## Step 10: Return the report and append to the friction log

Emit this structured report back to the caller:

```
## UI Screenshot Improve — Results

### Journey findings
- [Journey A/B/C] — [one-line cross-page friction, or "none"]

### All findings (ranked)
1. 🔴 [Title] — [Page or Journey] — user story: "A user trying to X ends up Y because Z"
2. 🟡 [Title] — [Page] — [one-line description]
(Cap at top 3 🔴 and top 3 🟡 inline. Roll all 🟢 into a single count — "+N 🟢 polish items, see friction log" — so the report stays scannable. The log keeps the exhaustive list.)

### Top 3 — outcomes
1. ✅ [Title] — `path/to/file.tsx` — [what changed] — regression: clean
2. ⚠️ [Title] — `path/to/file.tsx` — [what changed] — regression: [side effect]
3. ❌ [Title] — build failed (or regressed), reverted

### Not attempted (out of scope or below top 3)
- [brief list]

### Completion signal
[Emit `<promise>UI_CLEAN</promise>` only if: zero 🔴 findings this cycle AND no 🔴 in the prior cycle's log. Otherwise omit.]

### After-screenshots
✅ Re-captured — `web/screenshots/` updated
```

Then append to `web/screenshots/friction-log.md` (create it if missing):

```markdown
## Iteration <N> — <YYYY-MM-DD HH:MM>

### Fixed this cycle
- ✅ [Title] — `path/to/file.tsx` — user story — regression: clean|side-effect

### Deferred / intentional
- [Title] — reason (out-of-scope, intentional design, awaiting input)

### Recurring (raised but not fully resolved across 2+ cycles)
- [Title] — last attempt: [summary] — next move: [dig deeper / escalate out-of-scope]

### Design conventions ratified this cycle
- [One-line convention — e.g. "currency uses `font-variant-numeric: tabular-nums` and right-aligns in tables", "status chips use colored border + neutral fill, not colored fill"]

### Outstanding 🔴 (blocking friction not yet resolved)
- [Title] — user story
```

The log is append-only. Keep entries terse — this file must stay under 500 lines to remain useful in Step 1 of the next cycle. If it grows beyond that, roll the oldest iterations into a collapsed summary line per iteration.

## Completion signaling (for ralph-loop usage)

If the skill is being driven by a ralph loop, the loop should stop when the UI is genuinely clean. Use a two-cycle quiescence rule, not a single-cycle one:

> Emit `<promise>UI_CLEAN</promise>` only when this cycle produces **zero 🔴 findings** AND the prior iteration's friction log also recorded zero 🔴 findings.

A single quiet cycle is not enough — the skill might have missed something. Two in a row means the surface is genuinely stable.

## Running in a ralph loop

To run this skill unattended (e.g., overnight), wrap it in the `ralph-loop` plugin. The skill's own two-cycle quiescence rule (see "Completion signaling" above) handles exit:

```
/ralph-loop "Run the ui-screenshot-improve skill per its SKILL.md. The skill itself defines the UI_CLEAN completion rule — emit <promise>UI_CLEAN</promise> only when its two-cycle quiescence condition is met." --max-iterations 20 --completion-promise "UI_CLEAN"
```

- `--max-iterations 20` is the real safety net (~60 verified fixes over an overnight run). Adjust to taste.
- The promise string must match `--completion-promise` exactly; ralph is instructed not to fake it.
- `/cancel-ralph` stops the loop mid-run.
- Each iteration sees prior commits, screenshots, and `friction-log.md`, so the loop converges instead of re-litigating.

## Constraints

- **Frontend only** — `web/src/` files only. Go files are out of scope.
- **Scope discipline** — More than 3 files = out of scope for this skill.
- **Build gate is mandatory** — Always revert on build failure.
- **Regression gate is mandatory** — Revert fixes that cause worse new friction than they resolve.
- **User-story rule** — Every 🔴 finding needs the "user trying to X ends up Y because Z" sentence; no exceptions.
- **No color-only runs** — The diversity constraint in Step 6 is not optional. If every finding you see is a color fix, look harder at layout, spacing, and flow until you find a real structural issue.
- **Friction log is source of truth** — Read it in Step 1, respect its suppression list, append to it in Step 10.
- **Project conventions** — See `/workspace/CLAUDE.md`.
