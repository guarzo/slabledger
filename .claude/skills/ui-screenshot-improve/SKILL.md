---
name: ui-screenshot-improve
description: Visual UI improvement skill for SlabLedger, focused on finding and removing points of user friction — places where a real user would get confused, stuck, or blocked. Captures fresh screenshots of every page via `make screenshots`, walks canonical user journeys, audits each page through a friction-first lens (unclear next step, hidden actions, ambiguous state, broken-looking empty states, illegible info), then identifies and fixes the top 3 highest-friction issues with build verification and a before/after regression check. Also surfaces product-level gaps (missing first-run flow, dead-end navigation, pages that feel like a different product) and can scaffold a new route under the single-fix budget. Use whenever the user asks about UI polish, UX quality, visual bugs, usability, layout problems, or wants to run a visual improvement pass. Self-contained unit for overnight improvement loops — one call equals one screenshot cycle plus up to 3 verified fixes, with a persistent friction log that lets successive iterations converge instead of re-litigating the same findings.
---

# UI Screenshot Improve

A self-contained visual improvement skill:
**load log → state check → capture → journey check → per-page audit → rank → select → fix → recapture → regress check → report & log**.

Each invocation reads what prior runs already fixed, verifies the DB has realistic data, takes fresh screenshots, walks the primary user journeys, finds the top 3 highest-friction issues, implements targeted code fixes with build verification, then confirms the fixes didn't introduce new friction before reporting.

## Known blind spots

This skill audits static screenshots. It cannot see hover/focus states, keyboard paths, transitions, loading/toast states, or touch-target hit areas. Friction in those dimensions must come from the interactive probe (Step 4.75) or a user report — do not infer them from stills, and don't claim "no issues" for behavior the skill physically can't observe.

The skill audits whatever DB `make screenshots` was configured to read. If that DB is unseeded or has rolled over, the audit will be dominated by empty states and miss real product friction. Step 1.5 guards against this — don't skip it.

## Step 1: Load the friction log

```bash
cat web/screenshots/friction-log.md 2>/dev/null || echo "No prior log — this is iteration 1."
```

The friction log is a per-cycle record of what was found, fixed, and deferred. The full format lives in `references/friction-log-template.md`. Reading the log first prevents:

- Re-raising findings already fixed in a prior cycle.
- Re-raising findings deferred for a principled reason.
- Spinning on the same 🔴 across iterations when the first fix didn't fully resolve it.

Scan the **Deferred / intentional**, **Recurring**, **Epics**, and **Design conventions ratified** sections. The first two are a suppression list for findings; Epics are structural work too large for a single cycle that deserves visibility; conventions suppress proposals that would undo a locked design decision.

### Re-examination rule

Any item on the deferred list for **≥3 cycles** (count the `first raised: iter N` tag; add the tag if prior runs didn't) must be re-examined this cycle. Three outcomes are acceptable:

1. **Re-raise** — pull back into this cycle's ranking. Most items that were out of scope for a 3-file fix become in-scope under the raised scope cap on fix 1.
2. **Promote to epic** — write a one-paragraph epic entry pointing at the files/route work needed; continue deferring. Keeps the issue visible and actionable.
3. **Retire** — move to the log's Retired section with a one-line justification (user confirmed intentional, page removed, convention ratified). Retired items are out of the re-examination loop permanently.

Do not carry an item under "Deferred / intentional" for a fourth cycle. If none of the three outcomes fit, the skill is dodging the work — force one.

## Step 1.5: DB state precondition

Before capturing screenshots, verify the local DB has realistic data. An unseeded DB produces empty-state-dominated screenshots that surface polish-class findings instead of real product friction — the least interesting kind of audit.

```bash
/workspace/.claude/skills/ui-screenshot-improve/scripts/state-check.sh
```

Exit behavior:

- **0 (healthy)** — counts meet the threshold (see `references/state-check.md`). Proceed to Step 2.
- **1 (unseeded)** — counts below threshold. If `$PROD_DB_URL` (or `$SUPABASE_URL`) is set, run `YES=1 make db-pull` and re-check. If still below threshold, halt with guidance. If `$PROD_DB_URL` is not set, halt immediately.
- **2 (cannot connect)** — DB not reachable. Halt with the psql error.

Override with `UI_ALLOW_EMPTY=1` only when the intent is to audit empty-state copy specifically.

```bash
# Auto-recovery pattern
if ! /workspace/.claude/skills/ui-screenshot-improve/scripts/state-check.sh; then
  if [ -n "${PROD_DB_URL:-${SUPABASE_URL:-}}" ]; then
    echo "DB unseeded — pulling prod..."
    cd /workspace && YES=1 make db-pull
    /workspace/.claude/skills/ui-screenshot-improve/scripts/state-check.sh || { echo "still unseeded after pull — halt"; exit 1; }
  else
    echo "DB unseeded and PROD_DB_URL not set — halt"
    exit 1
  fi
fi
```

## Step 2: Capture fresh screenshots

```bash
cd /workspace && make screenshots
```

Builds the Go backend, builds the frontend, starts the server against the devcontainer Postgres via `DATABASE_URL`, runs Playwright, and saves screenshots to `web/screenshots/`. If the command fails, stop immediately — do not analyze stale screenshots.

Expected output (desktop):
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

**Mobile AND desktop** for high-traffic pages: `dashboard`, `insights`, `inventory`, `inventory-expanded`, `campaigns`, `campaign-detail`.

**Desktop only by default** for: `admin-*`, `tools`. Pull mobile of these only if their desktop reading surfaces a layout concern with a likely mobile dimension.

## Step 4: Journey sanity check

Walk the screenshots in the order a real user would hit them. Catch **cross-page friction** that's invisible in any single screenshot.

At each transition: *"Would the user know what to do next? Where their data went? Whether their last action succeeded?"*

**Journey A — New intake to listed**
`dashboard → inventory → inventory-expanded → (hypothetical push action) → campaign-detail`
*"I just received a slab. How do I get it listed for sale?"*

**Journey B — Campaign health check**
`dashboard → campaigns → campaign-detail → inventory (filtered)`
*"Is campaign X profitable? What's aging? What should I liquidate?"*

**Journey C — "Something's wrong, find and fix it"**
`dashboard (notice a stale price / red status / empty stat) → admin-integrations (diagnose) → admin-pricing (re-check) → dashboard (confirm resolved)`
*"I just saw something wrong. Can I diagnose, fix, and confirm from the dashboard?"*

Journey-level findings are first-class and frequently outrank per-page findings.

## Step 4.5: Design coherence check

Look at all screenshots together. Answer three questions, one sentence each:

1. *What is the aesthetic direction?* (data-dense terminal, editorial, brutalist utilitarian, glassy modern, etc.) If you can't name it, log `Design POV undefined` — don't count toward top-3.
2. *Is the direction consistent across pages?* Flag any page that reads as a different product.
3. *What is the one memorable thing?* If every page's memorable element is "the data," the UI is doing no design work — that's a legitimate 🟡 on a dashboard product.

Coherence findings rarely outrank blocking friction on their own but feed Tier C.

## Step 4.75: Interactive probe

Static screenshots hide hover, focus, keyboard, loading, and touch-targets. Pick the highest-risk page from the journey check and probe it live:

```bash
cd /workspace/web && npm run dev
```

In the browser:
- Tab through focusable elements once — is the focus ring visible on every stop? Does tab order match visual order?
- Hover the primary CTA and any row/chip that looks clickable — does the affordance change?
- Trigger an empty/loading/error state if reachable (slow network throttle, empty filter).
- On mobile viewport (Chrome devtools → iPhone 14), confirm the primary action is reachable without horizontal scroll and touch targets feel ≥44px.

**Escalation when statics are clean.** If the per-page static audit (Step 5) is on track to produce zero 🔴, expand this probe to **3 pages × 5 minutes each** before calling the cycle clean. Pick pages by a different criterion each cycle (highest-traffic in Journey A, most interactive elements, lowest score on Tier C) to avoid always probing the same surface. Findings here are first-class and commonly outrank static ones.

Log anything surprising — or explicitly say nothing surprised you. This is bounded: sanity check, not full a11y audit.

## Step 5: Friction-first audit (per page)

Primary question on every screenshot:

> *Where on this page would a real user hesitate, misread, or get stuck?*

Work the three tiers in order: Tier A signals first, then Tier B lenses to diagnose *why*, then Tier C once across the whole product.

### Tier A — Friction signals (what we're hunting)

**Action friction** — *"What can I do here, and how?"* Primary action obvious, reachable, labeled for outcome (not mechanism)? Clickable elements look clickable? A clear next step, or does the page dead-end?

**Comprehension friction** — *"What am I looking at?"* A user should name the 3 most important facts in ~2 seconds. Numbers scannable. Labels matching the user's mental model, not the database column.

**Flow friction** — *"What happens next? How do I get back?"* Related actions living together; clear navigation between related pages; breadcrumbs; current step visible in multi-step flows.

**Trust friction** — *"Is this broken, or is it just empty?"* Zero/loading/error states looking *intentional*. No raw `0` with no context; no tables-with-headers-and-no-rows that read as rendering failures; no calm grey on genuine errors; no alarming red on neutral states. Good zero-states *teach*.

**Mobile-shape friction** — *"Can this be used one-handed?"* Touch targets ≥44px. Primary action reachable without scrolling past dense data or closed disclosures. Wide tables handled with horizontal-scroll-with-fade, stacked cards, or responsive reflow. Sticky headers/bottom bars not covering the first/last row. Modals full-viewport on mobile.

### Tier B — Supporting craft lenses (diagnose Tier A findings)

Don't raise as standalone findings — raise Tier A, then use Tier B to identify the mechanism.

- **Layout & spacing** — poor alignment, uneven grids, awkward wrapping, orphaned elements. Common cause of comprehension + flow friction.
- **Typography hierarchy & voice** — size/weight ramp between page title → section label → body → meta; visible pairing (display + text, or weight contrast); leading/tracking appropriate; tabular-nums on columns that must line up.
- **Contrast & readability** — functional text (timestamps, labels, secondary values) that's effectively invisible.
- **Status semantics** — colors misrepresenting severity; danger red on neutral states; neutral grey on genuine alarms.
- **Data density & number craft** — numeric columns right-aligned with tabular numerals; hero metrics with a distinct weight/size ramp vs labels; deltas carrying both direction (↑/↓) and color; zebra rows only when they actually aid scanning.

### Tier C — Systemic / product-level lens

Tier A and B are per-page. Tier C is one pass over the whole product per cycle. Findings here don't need the per-user-story sentence — they use the structural sentence (see Step 6's user-story gate). They compete for the top-3 slot and can outrank per-page friction when the systemic gap is large.

Pick 3–5 questions per cycle from `references/tier-c-questions.md`, rotating across pillars (Nav & IA / Onboarding / Missing Destinations / Product Coherence / System-Level Surfaces). Don't answer the same set twice in a row.

If Tier C produces no findings two cycles in a row, don't coast — rotate the questions rather than accept silence as truth.

## Step 6: Rank all findings by friction severity

List every identified issue and rank by the friction it creates for a real user:

🔴 **Blocking friction** — user cannot complete a task, misreads critical state, or reasonably concludes the page is broken. This is what we came for.
🟡 **Slowing friction** — user completes the task but with measurable hesitation, backtracking, hunting, or rework.
🟢 **Polish** — no functional friction; aesthetic or consistency only.

### Every 🔴 must carry a "why it hurts" sentence

- **Tier A/B 🔴** uses *"A user trying to [goal] ends up [wrong outcome] because [mechanism]."*
  Example: *"A user trying to find out whether a slab was listed ends up bouncing between inventory and campaign-detail because neither page shows the listing status after the push."*
- **Tier C 🔴** uses the structural sentence *"The product does [X]; it should do [Y]; because [Z]."*
  Example: *"The product shows a '2 unpaid invoices' chip on the dashboard but no route to invoices exists; it should open an /invoices page; because every drill-in callout creates a contract the product must honor."*

Same discipline, different shape — both forms exist to prevent taste-based complaints from masquerading as blocking friction and to keep the log auditable.

If you can't write one of these sentences cleanly, the finding isn't 🔴 — downgrade to 🟡 or 🟢.

Cap at top 3 🔴 and top 3 🟡 in the inline report. Roll all 🟢 into a single count ("+N 🟢 polish items, see friction log") so the report stays scannable.

## Step 7: Select what to work on this cycle

Select the **top 3 highest-friction findings by user impact, not ease of fix.**

A run where all three are 🟢 is a failed run — go back through Tier A/C and find at least one real friction issue.

### Suppression

Skip any finding already on the deferred list **unless** its re-examination threshold (3 cycles) has been hit (Step 1) or you have new evidence it matters.

### Diversity constraint

At most **2 of the 3** selected fixes may be *cosmetic* — label/copy, spacing/sizing, variant/color swap, adding or removing a single element. At least one fix must be *structural* — layout change affecting how users scan the page (grid/flex restructure, element reorder, surfacing a hidden action, collapsing a split flow) — or a Tier C finding. A cycle of three cosmetic fixes is a failed run even if each fix is green.

See `references/structural-vs-cosmetic.md` for worked examples of the boundary.

### Scope cap

- **Fix 1 (the biggest finding)** may touch up to **6 files** *or* scaffold a new route/page (router entry + one new page component + supporting imports), provided the report in Step 11 lists the files budgeted and why. Layout restructures across a feature shell + one or two child components are fair game.
- **Fixes 2 and 3** cap at 3 files each.
- Findings larger than fix-1's budget get **promoted to an epic** (a log entry naming files, flow, and a rough sketch of the destination) rather than silently deferred.

## Step 8: Fix each issue (one at a time)

For each of the 3 selected, in priority order:

1. **Trace to source** — identify the responsible React/TypeScript file in `web/src/`. Read the file before editing.
2. **Implement the fix** following project conventions:
   - Tailwind utility classes for styling: spacing, layout, sizing, typography.
   - CSS variables for semantic colors: `var(--text)`, `var(--text-muted)`, `var(--danger)`, `var(--warning)`, `var(--success)`, `var(--brand-400)`, `var(--surface-1)`, `var(--surface-2)`.
   - No new npm deps, no new abstractions beyond what the fix requires.
3. **Verify**: `cd /workspace/web && npm run build`
   - Exit 0 → fix stands ✅
   - Non-zero → revert, mark ❌, continue.

## Step 9: Capture after-screenshots

```bash
cd /workspace && make screenshots
```

Overwrites `web/screenshots/` with post-fix renders. If the run fails, note it but don't revert — the build already passed.

## Step 10: Regression check

Build success ≠ UX success. For each page you edited:

1. Re-read the post-fix screenshot (desktop and mobile where you pulled both).
2. Confirm the targeted friction is actually resolved, not papered over.
3. Scan the rest of the page for **new** friction introduced — shifted layout, newly-misaligned elements, unintended contrast changes, broken empty states.

Record per fix:
- ✅ **Clean** — resolved, no new friction.
- ⚠️ **Resolved with side effects** — resolved but a minor new issue appeared; log for next cycle.
- ❌ **Regressed** — new friction worse than what was fixed. Revert, mark unresolved.

## Step 11: Report and append to the friction log

Emit this structured report back to the caller:

```
## UI Screenshot Improve — Results

### Journey findings
- [Journey A/B/C] — [one-line cross-page friction, or "none"]

### All findings (ranked)
1. 🔴 [Title] — [Page or Journey] — user story or structural sentence
2. 🟡 [Title] — [Page] — one-line description
(Cap at top 3 🔴 and top 3 🟡 inline. Roll all 🟢 into a single count.)

### Top 3 — outcomes
1. ✅ [Title] — `path/to/file.tsx` — what changed — regression: clean
2. ⚠️ [Title] — `path/to/file.tsx` — what changed — regression: side effect
3. ❌ [Title] — build failed (or regressed), reverted

### Not attempted (out of scope or below top 3)
- brief list

### Completion signal
[Emit `<promise>UI_CLEAN</promise>` only if the quiescence rule below is satisfied. Otherwise omit.]

### After-screenshots
✅ Re-captured — `web/screenshots/` updated
```

Then append an iteration entry to `web/screenshots/friction-log.md` using the template in `references/friction-log-template.md`. The optional helper `scripts/append-friction-log.sh` takes a JSON payload on stdin and writes the formatted entry; copy the template by hand if you prefer.

## Completion signaling (for ralph-loop usage)

Emit `<promise>UI_CLEAN</promise>` only when **both** are true:

1. This cycle AND the two prior cycles each produced **zero 🔴 findings**.
2. At least one **structural** finding was attempted within that three-cycle window — either fixed (per the diversity constraint) or credibly investigated and promoted to an epic.

"Attempted" means fix-1 was a structural or Tier C finding, not a cosmetic tweak. Three quiet cycles with no structural work means the skill ran out of easy wins, not that the surface is clean.

Why: two quiet cycles is too cheap when the skill's own rules let it suppress structural findings into the deferred list. Three cycles plus a required structural attempt makes "UI clean" mean *we actively went looking for harder problems and found none*, not *we ran out of easy wins*.

## Running in a ralph loop

To run this skill unattended (e.g., overnight), wrap it in the `ralph-loop` plugin. The skill's own three-cycle-plus-structural-attempt quiescence rule handles exit:

```
/ralph-loop:ralph-loop "Run the ui-screenshot-improve skill per its SKILL.md. The skill itself defines the UI_CLEAN completion rule — emit <promise>UI_CLEAN</promise> only when its quiescence condition is met." --max-iterations 20 --completion-promise "UI_CLEAN"
```

- `--max-iterations 20` is the real safety net (~60 verified fixes over an overnight run). Adjust to taste.
- The promise string must match `--completion-promise` exactly.
- `/cancel-ralph` stops the loop mid-run.
- Each iteration sees prior commits, screenshots, and `friction-log.md`, so the loop converges.

## Constraints

- **Frontend only** — `web/src/` files only. Go files are out of scope.
- **Scope discipline** — Fix 1 may touch up to 6 files or scaffold a new route/page; fixes 2 and 3 cap at 3 files. Findings larger than fix-1's budget get promoted to an epic, never silently deferred.
- **Build gate is mandatory** — Always revert on build failure.
- **Regression gate is mandatory** — Revert fixes that cause worse new friction than they resolve.
- **User-story rule** — Every 🔴 needs the "why it hurts" sentence. Tier A/B use the user form; Tier C uses the structural form. No exceptions.
- **No cosmetic-only runs** — The diversity constraint is not optional. At least one of three fixes must be structural (layout, flow, or Tier C).
- **Friction log is source of truth** — Read it in Step 1, honor its suppression list *and* the re-examination rule, append to it in Step 11.
- **UI_CLEAN is earned, not declared** — Three consecutive zero-🔴 cycles AND at least one structural attempt in that window. Two cheap cycles no longer suffice.
- **DB state precondition** — Never audit an unseeded DB. Step 1.5 runs `scripts/state-check.sh`; on failure, auto-runs `YES=1 make db-pull` when `$PROD_DB_URL` is set, otherwise halts. See `references/state-check.md` for rationale and override flag.
- **Bundled references** — `references/friction-log-template.md` (log format), `references/tier-c-questions.md` (systemic lenses), `references/structural-vs-cosmetic.md` (fix classification), `references/state-check.md` (DB precondition). Read these when the relevant step cites them.
- **Project conventions** — See `/workspace/CLAUDE.md`.
