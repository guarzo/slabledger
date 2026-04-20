---
name: ui-screenshot-improve
description: Visual UI improvement skill for SlabLedger, focused on finding and removing points of user friction — places where a real user would get confused, stuck, or blocked. Captures fresh screenshots of every page via `make screenshots`, walks canonical user journeys, audits each page through a friction-first lens (unclear next step, hidden actions, ambiguous state, broken-looking empty states, illegible info), then identifies and fixes the top 3 highest-friction issues with build verification and a before/after regression check. Also surfaces product-level gaps (missing first-run flow, dead-end navigation, pages that feel like a different product) and can scaffold a new route under the single-fix budget. Use whenever the user asks about UI polish, UX quality, visual bugs, usability, layout problems, or wants to run a visual improvement pass. Self-contained unit for overnight improvement loops — one call equals one screenshot cycle plus up to 3 verified fixes, with a persistent friction log that lets successive iterations converge instead of re-litigating the same findings.
---

# UI Screenshot Improve

A self-contained visual improvement skill:
**load log → state check → capture → stills-diff → journey check → live probe (mandatory) → per-page audit → rank → select → fix → recapture → regress check + ship-verify → report & log**.

Each invocation reads what prior runs already fixed, verifies the DB has realistic data, takes fresh screenshots, probes 2+ user flows live via Playwright MCP (rotating flows across cycles), walks the primary user journeys, finds the top 3 highest-friction issues, implements targeted code fixes with build verification, then confirms the fixes didn't introduce new friction — and live-verifies anything shipped that the static harness doesn't capture — before reporting.

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
3. **Retire** — move to the log's Retired section with a one-line justification. Retired items are out of the re-examination loop permanently. Retirement rules depend on the reason:
   - **Convention ratified / page removed / user confirmed intentional** — paper-only justification is fine; these are genuinely closed.
   - **"Not reachable with current data" / "couldn't verify from stills"** — retirement requires a *live probe* (Playwright MCP) that reproduces the state under which the item would appear. Without that probe, the item is not retired — it must be re-raised or promoted to an epic. Paper-only retirements for reachability reasons are what lets the skill dodge work it should be doing.

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

Mobile equivalents land under `web/screenshots/mobile/`. Live probes from Step 4.75 and Step 10b write into `web/screenshots/ad-hoc/` — create it on first use (`mkdir -p web/screenshots/ad-hoc`).

## Step 3: Read screenshots (with rotation and a stills-diff floor)

The 12 harness pages capture initial-load state only. Re-reading the same 12 pages every cycle drains the well fast — after 2-3 cycles, new friction comes from interactions and under-sampled surfaces, not from re-squinting at the dashboard. Handle this in two ways:

### 3a. Stills-diff floor (every cycle, cheap)

Regardless of which cycle variant you're running, start by hashing the current PNG set and comparing to the last cycle's hashes. Any page whose hash changed unexpectedly (i.e., you didn't ship a fix there) is a potential regression — flag it immediately and read that page. This catches silent breakage even when the audit's focus is elsewhere.

```bash
md5sum web/screenshots/*.png web/screenshots/mobile/*.png > /tmp/screenshots-now.md5
# diff against web/screenshots/.last-hashes.md5 if present; overwrite at cycle end
```

### 3b. Cycle variant — read depth rotates

- **Cycles where cycle_number % 3 != 0** (default — most cycles): read mobile AND desktop for `dashboard`, `insights`, `inventory`, `inventory-expanded`, `campaigns`, `campaign-detail`. Desktop only for `admin-*`, `tools`. Pull mobile of admin/tools only if desktop surfaces a layout concern with a likely mobile dimension.
- **Every 3rd cycle** (cycle_number % 3 == 0): the static audit shrinks to the stills-diff floor from 3a — no per-page friction pass. Step 4.75's interactive probe expands from 2 flows to **4 flows × 5 min** each, chosen from surfaces the statics do not capture. This cycle's findings come from interactions, not stills. "We re-read the same 12 pages and found nothing" is not a valid outcome three cycles in a row.

Track `cycle_number` from the friction log by counting iteration entries.

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

## Step 4.75: Interactive probe (mandatory every cycle)

Static screenshots are a narrow slice of the product — they catch initial page loads and miss almost everything that happens *after* someone clicks. The cycles most vulnerable to self-deception are the ones where the 12 statics look fine; that's when the real friction lives in the interactions.

**Every cycle must include a live probe of at least 2 user flows, end-to-end.** Do not skip this step because the statics are clean — that is exactly when to probe.

### Probe transport — primary, then fallback

**Primary: Playwright MCP.** Use the MCP tools (`browser_navigate`, `browser_snapshot`, `browser_click`, `browser_take_screenshot`) against a running backend — either `make screenshots` started in the background, or `cd web && npm run dev` with the Go backend on `:8081`.

**If the MCP is unavailable** (browser won't launch, e.g., Chrome crashpad errors in this devcontainer), fall back to an ad-hoc Node script under `web/screenshots/ad-hoc/` that uses `@playwright/test` directly against the already-working `chrome-headless-shell` binary that `make screenshots` uses. See `web/screenshots/ad-hoc/probe-iter8.mjs` for the template: spawn chromium with `--no-sandbox`, mock `/api/auth/user`, navigate, screenshot into the `ad-hoc/` directory. This is allowed by the `web/src/`-only constraint because `ad-hoc/` is not `web/tests/` — it's per-cycle scratch, not the committed test harness.

To configure the MCP to use the working chromium-headless-shell in this devcontainer, `/workspace/.mcp.json` registers a `playwright_local` server with `--browser chromium --executable-path <chrome-headless-shell path> --no-sandbox --headless`. That's the MCP pathway that works here. Other environments may not need this — try the plugin default first, fall back to ad-hoc if it fails within one attempt, don't spend the cycle debugging MCP.

Rotate the flow set each cycle so successive runs don't re-examine the same surface. Pick 2 from this list, never the same 2 in consecutive cycles:

| Flow | What to exercise |
|---|---|
| **Intake → listed** | tools page cert scan → assign to campaign → navigate to campaign-detail → expand an inventory row → attempt a push/list action |
| **Campaign tuning edit** | campaigns → pick one → Tuning tab → adjust a parameter → save → observe how change is confirmed |
| **Record a sale** | inventory row → mark sold / record sale → fill the form → submit → confirm row updates and dashboard P/L reflects it |
| **Bulk sell-sheet** | select multiple inventory rows → add to sell sheet → switch to sell-sheet filter → print/export |
| **Filters + search** | /inventory → toggle each filter pill → search for a cert # → clear filters → observe empty/result states |
| **Keyboard navigation** | land on any page → tab through focusable elements → Enter on rows/chips → Escape on dialogs → verify ring visibility and order |
| **Dialog / modal** | trigger any dialog (price flag, fix DH match, campaign form) → observe backdrop, dismiss, focus trap, mobile full-viewport |
| **Empty → populated** | a filter or state with 0 results, then an action that populates it — does the transition feel intentional? |

For each flow, spend ~5 minutes. Record what you found — or explicitly write "nothing surprised me in flow X" so future runs know the flow was probed this cycle and came up clean. Vague "looks fine" does not count as a probe.

### Ship-verification: anything you built this cycle must be probed live

If this cycle's fix creates a new route, new page, new state, or a surface that the static harness does not capture, **that surface must be visited live before the cycle closes** (Step 10). Navigate to it via Playwright MCP, take a screenshot into `web/screenshots/ad-hoc/<cycle>-<name>.png`, and read it back. Shipping a new page blind — as happened with the `/invoices` scaffold — is the failure mode this rule exists to prevent. Do not write "build passed + tests passed" and treat that as visual verification.

Findings from Step 4.75 are first-class and commonly outrank findings from the static audit. Log them alongside Tier A findings in Step 6.

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

A run where all three picks are 🟢 is not a *failure* (polish-only cycles are valid — see Step 11's cycle classification), but it does not advance the UI_CLEAN counter. If you land here, first double-check Tier A and Tier C for 🔴/🟡 you may have suppressed or missed — genuine polish-only is fine, but it often means the audit was too shallow.

### Suppression

Skip any finding already on the deferred list **unless** its re-examination threshold (3 cycles) has been hit (Step 1) or you have new evidence it matters.

### Diversity constraint

At most **2 of the 3** selected fixes may be *cosmetic* — label/copy, spacing/sizing, variant/color swap, adding or removing a single element. At least one fix must be *structural* — layout change affecting how users scan the page (grid/flex restructure, element reorder, surfacing a hidden action, collapsing a split flow) — or a Tier C finding. A cycle of three cosmetic fixes is still valid (it just gets classified `polish-only` in Step 11 and does not advance the UI_CLEAN counter), but it's a sign the audit missed harder findings — double-check Tier A and Tier C before settling for three cosmetics.

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

## Step 10: Regression check + ship-verification

Build success ≠ UX success. Three checks, in order.

### 10a. Harness regression check

For each page you edited that the harness already covers:

1. Re-read the post-fix screenshot (desktop and mobile where you pulled both).
2. Confirm the targeted friction is actually resolved, not papered over.
3. Scan the rest of the page for **new** friction introduced — shifted layout, newly-misaligned elements, unintended contrast changes, broken empty states.

### 10b. Ship-verification for anything outside the harness

If this cycle's fix shipped a new route, a new page, a new dialog state, or any surface the static harness does not capture, that surface **must be probed live before the cycle closes**. Use Playwright MCP:

1. Start the dev server (`cd /workspace/web && npm run dev`) or the screenshot-harness backend, whichever is already running.
2. Navigate to the new surface via `browser_navigate`.
3. `browser_snapshot` to confirm accessibility tree renders cleanly and key elements are reachable.
4. `browser_take_screenshot` into `web/screenshots/ad-hoc/<cycle>-<slug>.png`, then read it back and audit it with the same Tier A lens used in Step 5.
5. If the surface has mobile-relevant layout, `browser_resize` to 390×844 (iPhone 14) and repeat.

"Build passed + unit tests passed" is not visual verification. Shipping a new page blind is what we're preventing. If the live probe turns up new friction on the just-shipped surface, record it as a `⚠️ Resolved with side effects` outcome for next cycle — don't ignore it.

### 10c. Record per fix

- ✅ **Clean** — resolved, no new friction, shipped-surface probed live (if applicable).
- ⚠️ **Resolved with side effects** — resolved but a minor new issue appeared on the edited page or the newly-shipped surface. Log it for next cycle.
- ❌ **Regressed** — new friction worse than what was fixed. Revert, mark unresolved.
- 🕳️ **Shipped but unverified** — fix appeared to build cleanly but the new surface could not be probed live this cycle. This blocks the cycle from counting as `substantive`; next cycle must run the live probe before doing anything else.

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

### User-visible impact
One honest sentence, per fix, naming the user and the scenario in which they'd notice the difference. Not the mechanism, not the file — the *experience*.

Good: *"A user whose PSA invoice is overdue can now click the dashboard chip and see which invoices are open and how much is outstanding, instead of staring at a pill that does nothing."*

Bad (trivial or mechanism-shaped): *"Changed color from green to neutral."* *"Renamed Act to Action."* *"Added a link."*

If you cannot write an honest user-visible-impact sentence for a fix — or if the honest sentence reads as *"a user in an uncommon corner-case state would see a slightly different color"* — the fix is 🟢 polish, not 🟡. Polish fixes do not count toward the quiescence counter (see the Completion signaling section).

### Not attempted (out of scope or below top 3)
- brief list

### Cycle classification
One of: `substantive` (≥1 fix with a real user-visible-impact sentence), `polish-only` (all fixes 🟢 or trivial), `quiet` (no fixes shipped). This classification drives the quiescence counter.

### Completion signal
[Emit `<promise>UI_CLEAN</promise>` only if the quiescence rule in the Completion signaling section is satisfied. Otherwise omit, or emit `<promise>UI_CLEAN_BY_EXHAUSTION</promise>` if the exhaustion clause applies.]

### After-screenshots
✅ Re-captured — `web/screenshots/` updated
```

Then append an iteration entry to `web/screenshots/friction-log.md` using the template in `references/friction-log-template.md`. The optional helper `scripts/append-friction-log.sh` takes a JSON payload on stdin and writes the formatted entry; copy the template by hand if you prefer.

## Completion signaling (for ralph-loop usage)

The skill emits one of three signals at the end of a cycle: `UI_CLEAN`, `UI_CLEAN_BY_EXHAUSTION`, or nothing.

### `UI_CLEAN` — earned by sustained substantive work

Emit `<promise>UI_CLEAN</promise>` only when **all** of the following hold:

1. This cycle AND the two prior cycles each produced **zero 🔴 findings** AND were classified `substantive` in Step 11 (not `polish-only` or `quiet`).
2. At least one cycle within the last 2 shipped a **structural** fix (not just a "structural attempt"). This forces continuous structural pressure — once the skill has shipped a structural fix, the next two cycles can be cosmetic/polish, but the third must ship another structural fix or UI_CLEAN cannot be emitted.
3. Every flow listed in Step 4.75 has been probed at least once in the last 6 cycles — "we haven't looked at the record-sale flow in 8 iterations" is a reason the surface is under-sampled, not clean.

Why these three and not the old rules: the old rule ("3 cycles + 1 structural attempt in the window") let the skill ship one scaffolded route, then coast on two cosmetic cycles of neutral-color tweaks and copy changes. That's not what a user means by "UI clean." The new rules require each counted cycle to ship real user-visible work, require structural fixes to recur, and require the skill to have actually looked at a meaningful slice of the product's interactions.

### `UI_CLEAN_BY_EXHAUSTION` — earned by honest searching

Sometimes a product genuinely is in good shape and the skill runs out of eligible findings. Emit `<promise>UI_CLEAN_BY_EXHAUSTION</promise>` when **all** of the following hold:

1. The last **4 consecutive cycles** each produced zero 🔴 findings.
2. Across those 4 cycles, at least 6 distinct flows have been probed live (Step 4.75) and each probe report reads as "nothing surprised me" or similar — concrete-but-negative, not vague.
3. At least 3 Tier C questions in each of the last 2 cycles returned "no gap" answers, rotating across pillars (Nav & IA / Onboarding / Missing Destinations / Product Coherence / System-Level Surfaces).
4. No structural fix was shipped in the window — not because the skill was lazy, but because no structural candidate survived good-faith search. This is the honest "we looked, and there isn't anything left" signal.

The two signals exist so the skill doesn't have to choose between lying (emit UI_CLEAN when nothing substantive happened) and spinning forever (refuse to exit). If the product is genuinely clean, use UI_CLEAN_BY_EXHAUSTION. If the skill is still finding real work, keep going.

### Polish-only and quiet cycles are not failures, but they do not advance the counter

A `polish-only` cycle (only 🟢-adjacent fixes, or fixes whose user-visible-impact sentence reads as trivial) is a valid cycle — it's fine to ship small improvements. But it does not count toward the 3-cycle UI_CLEAN window. This prevents the failure mode where the skill strings together cosmetic cycles to earn an unearned clean signal.

A `quiet` cycle (no fixes shipped because nothing surfaced) also doesn't advance UI_CLEAN, but it does advance UI_CLEAN_BY_EXHAUSTION's 4-cycle counter *only if* Step 4.75 probes were run and at least 3 Tier C questions were answered. A quiet cycle that skipped the probe is a dodging cycle.

## Running in a ralph loop

To run this skill unattended (e.g., overnight), wrap it in the `ralph-loop` plugin. Two possible exit signals — wire whichever you care about, or both:

```
/ralph-loop:ralph-loop "Run the ui-screenshot-improve skill per its SKILL.md. Emit <promise>UI_CLEAN</promise> when the substantive-cycle quiescence rule is met, or <promise>UI_CLEAN_BY_EXHAUSTION</promise> when the honest-search quiescence rule is met." --max-iterations 15 --completion-promise "UI_CLEAN"
```

- **Expect fewer cycles than before.** The live probe and ship-verification steps add roughly 5-10 minutes per cycle. Budget 12-20 minutes per cycle on overnight runs, and lower `--max-iterations` to 12-15 (down from 20). Fewer, deeper cycles beat more, shallower ones — that's the whole point of the tuning.
- `--completion-promise` accepts a single exact string. If you want to stop on either `UI_CLEAN` or `UI_CLEAN_BY_EXHAUSTION`, decide up front which you care about more and target that one; the skill will still emit the other into its report when applicable, so you'll see it in the log even if it doesn't end the loop.
- `/ralph-loop:cancel-ralph` stops the loop mid-run.
- Each iteration sees prior commits, screenshots, and `friction-log.md`, so the loop converges. The skill is allowed — encouraged — to reference what prior cycles probed so the rotation of flows in Step 4.75 stays honest.

## Constraints

- **Frontend only** — `web/src/` files only. Go files are out of scope. Test-harness files under `web/tests/` are also out of scope — when the skill ships a new route or page that the harness doesn't cover, use live Playwright MCP probes (Step 10b) instead of modifying the test spec.
- **Scope discipline** — Fix 1 may touch up to 6 files or scaffold a new route/page; fixes 2 and 3 cap at 3 files. Findings larger than fix-1's budget get promoted to an epic, never silently deferred.
- **Build gate is mandatory** — Always revert on build failure.
- **Regression gate is mandatory** — Revert fixes that cause worse new friction than they resolve.
- **Ship-verification is mandatory** — Anything shipped that the static harness doesn't capture must be probed live before the cycle closes (Step 10b). "Build passed + unit tests passed" is not visual verification.
- **Live probe is mandatory every cycle** — Step 4.75 is no longer optional. Every cycle probes at least 2 user flows (4 on every 3rd cycle), rotating the flow set. Cycles that skip the probe do not count toward UI_CLEAN.
- **User-story rule** — Every 🔴 needs the "why it hurts" sentence. Tier A/B use the user form; Tier C uses the structural form. No exceptions.
- **User-visible-impact rule** — Every fix's report in Step 11 carries a user-visible-impact sentence naming the user and the scenario. If the sentence reads as mechanism-shaped or trivial, the fix is 🟢 polish and the cycle is classified `polish-only` — which does not advance the UI_CLEAN counter.
- **Diversity constraint** — At most 2 of 3 fixes may be cosmetic. Separately: across any 2-cycle window that counts toward UI_CLEAN, at least one shipped fix must be structural (layout, flow, Tier C, or scaffolded route). Structural *attempts* (investigated but deferred to epic) do not satisfy this — shipped work does.
- **Friction log is source of truth** — Read it in Step 1, honor its suppression list *and* the re-examination rule, append to it in Step 11. Retirements for "not reachable with current data" require a live probe, not a paper justification.
- **UI_CLEAN is earned, not declared** — Three consecutive `substantive` zero-🔴 cycles with recurring structural pressure, OR four consecutive cycles of honest-search exhaustion. See the Completion signaling section.
- **DB state precondition** — Never audit an unseeded DB. Step 1.5 runs `scripts/state-check.sh`; on failure, auto-runs `YES=1 make db-pull` when `$PROD_DB_URL` is set, otherwise halts. See `references/state-check.md` for rationale and override flag.
- **Bundled references** — `references/friction-log-template.md` (log format), `references/tier-c-questions.md` (systemic lenses), `references/structural-vs-cosmetic.md` (fix classification), `references/state-check.md` (DB precondition). Read these when the relevant step cites them.
- **Project conventions** — See `/workspace/CLAUDE.md`.
