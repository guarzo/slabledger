# Campaign Analysis Skill — Depth-and-Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add seven Self-challenge rules, Playbook H (Portfolio Architecture Review), an impossible-asks log integration, and six eval-regression entries to the `campaign-analysis` skill — shifting it from tactical parameter-tuning within fixed structure to strategic portfolio-shape analysis.

**Architecture:** Markdown-only edits to four files in `.claude/skills/campaign-analysis/` plus runtime-only creation of `docs/private/impossible-data-asks.md`. No code, no automated tests. Tasks are sequenced so each commit produces a coherent skill — Self-challenge rules land first (other sections reference them), Playbook H second, Step 3c soft-flag third, impossible-asks wiring fourth, eval entries last.

**Coordination with origin/main:** After two rebases, `origin/main` now carries 8 specific Recommendation rules from the same source session that motivated this spec (commits `7a8a9c28` + `c99e6669`). 7 of those 8 rules are concrete instances of patterns the new Self-challenge rules generalize. The implementation must cite each origin/main rule as a named precedent inside the corresponding Self-challenge rule (1.1, 1.2, 1.3, 1.4, 1.5), per the spec's "Coordination with origin/main rules" section. The Self-challenge rules are not redundant — they fire when the underlying pattern matches but no specific rule names it.

**Tech Stack:** Markdown. Git for commits. The skill is consumed by Claude Code at runtime via the `Skill` tool.

**Source spec:** `docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md` — contains verbatim rule text and reference content. This plan references spec sections by number rather than reproducing all the content; agentic workers must read the spec before each task.

**Branch:** `skill/campaign-analysis-depth-and-scope` (worktree at `.worktrees/campaign-analysis-strategic-shift/`). Single PR. Do not push without explicit user approval.

---

## File Structure

| Path | Responsibility | Change |
|------|----------------|--------|
| `.claude/skills/campaign-analysis/SKILL.md` | Skill entry point — Step 0–6, Recommendation rules, Data conventions, appendix | Add Self-challenge rules section (Tasks 1–2); Step 3c soft-flag + Step 4 pointer + Step 3c Self-challenge pointer (Tasks 3, 5); Step 0 impossible-asks file load (Task 6) |
| `.claude/skills/campaign-analysis/references/playbooks.md` | Playbooks A–G, Steps 5–6, Recommendation rules, Data conventions, Mutations | Add Playbook H (Task 4); Partner-ask verification rule update (Task 7); Step 6 retrospective touch-up (Task 8) |
| `.claude/skills/campaign-analysis/evals/known-failures.md` | Regression checklist of documented failure modes | Append 6 new entries (Task 9) |
| `docs/private/impossible-data-asks.md` | Operator-private log of asks the partner won't answer | Not created in this PR — file is created at runtime on first user-initiated logging |

Per the spec's coherence requirement: each task produces a commit that leaves the skill consistent. Sub-rule 1.6 in Task 1 references the impossible-asks log with a "may be absent" note so the file's actual creation in Task 6/7 isn't a precondition. Playbook H in Task 4 only references rules that already exist at that point.

---

## Task 1: Add Self-challenge rules section header + rules 1.1–1.5 to SKILL.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md` — insert new section between "Recommendation rules" (ends around line 346) and "Data conventions" (starts around line 348)

The verbatim rule text for 1.1–1.5 is in spec section 1 (`docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md`). Read the spec first, then copy 1.1, 1.2, 1.3, 1.4, 1.5 verbatim into a new top-level section titled `## Self-challenge rules`.

- [ ] **Step 1: Read SKILL.md to confirm current line numbers**

Run from worktree root:
```bash
grep -n "^## " .claude/skills/campaign-analysis/SKILL.md
```

Expected: a list of section headers with line numbers. Confirm the section between `## Recommendation rules` and `## Data conventions` is where the new section will land.

- [ ] **Step 2: Read spec section 1.1–1.5 to get verbatim content**

Run:
```bash
sed -n '/^### 1.1/,/^### 1.6/p' docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md | head -120
```

This gives you the exact text of rules 1.1 through 1.5 as approved during brainstorming.

- [ ] **Step 3: Insert the new section into SKILL.md**

Add this between `## Recommendation rules` (and its sub-content) and `## Data conventions`. The section header is `## Self-challenge rules`. Below the header, add the spec's full intro block (including the **"Layered with existing rules"** paragraph that names the Business-mechanic premise gate as a complementary pre-analysis layer and the seven specific Recommendation rules — Cap-cut binding check, Era-fit gate, Throttle lever selection, Fill-drought hypothesis ranking, Dollar-weighted BPCL cross-check, Category vs campaign discipline, Popular-tier exclusion — as the specific instances 1.1–1.7 generalize). Read spec section 1 lines 56–66 verbatim — do not paraphrase.

Then copy rules 1.1, 1.2, 1.3, 1.4, 1.5 verbatim from spec section 1, each as a `### N.N — <title>` subsection. Each rule includes the named-precedent references the spec spells out:

- **1.1** ends with the four cross-check types AND names the specific instances inline: Category vs campaign discipline (Conv guideline 4) for category claims; Dollar-weighted BPCL cross-check (API footguns) for metric citations; Cap-cut binding check + Era-fit gate + Throttle lever selection for action proposals; Fill-drought hypothesis ranking for hypothesis sets.
- **1.2** ends with a sentence naming Throttle lever selection as a concrete instance (silently picking cap invites the *"would those changes do anything?"* pushback).
- **1.3** ends with the Era-fit gate's `/snapshot.suggestions` carve-out as the named precedent.
- **1.4** ends with the Category vs campaign discipline as the named precedent.
- **1.5** explicitly cites Cap-diagnostic rule (BOTH directions — supply-thinness and cap-cut binding) and Popular-tier exclusion as precedents.

Do not add 1.6 or 1.7 yet — those land in Task 2.

- [ ] **Step 4: Verify the section is coherent**

Run:
```bash
grep -n "^### 1\." .claude/skills/campaign-analysis/SKILL.md
```

Expected: 5 lines showing `### 1.1 — Verify-before-propose gate (D1)` through `### 1.5 — Disagree with the data when the pattern says otherwise (D4)`. No 1.6, 1.7 yet.

Also verify section ordering:
```bash
grep -n "^## " .claude/skills/campaign-analysis/SKILL.md
```

Expected: `## Self-challenge rules` appears between `## Recommendation rules` and `## Data conventions`.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "skill(campaign-analysis): add Self-challenge rules 1.1-1.5

Rules 1.1-1.5 cover D1 (stops at first plausible answer), D2 (defers
to server suggestions), D3 (describing vs analyzing), and D4 (afraid
to disagree with data). Lives in SKILL.md so the discipline is always
in context, not lazy-loaded via references/.

Each rule names existing specific Recommendation rules as precedents:
1.1 cites Cap-cut binding check, Dollar-weighted BPCL, Category vs
campaign discipline, Era-fit gate, Throttle lever selection, and
Fill-drought hypothesis ranking. 1.2 cites Throttle lever selection.
1.3 cites Era-fit gate's snapshot.suggestions carve-out. 1.4 cites
Category vs campaign discipline. 1.5 cites Cap-diagnostic (both
directions) and Popular-tier exclusion. The Self-challenge rules
generalize these instances; the specific rules name the thresholds.

Section 1 intro positions the rules as complementary to the existing
Business-mechanic premise gate (premise gate fires before analysis;
Self-challenge fires per claim/lever).

Spec: docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Add Self-challenge rules 1.6 and 1.7 to SKILL.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md` — append 1.6 and 1.7 after 1.5

- [ ] **Step 1: Read spec section 1.6 and 1.7**

Run:
```bash
sed -n '/^### 1.6/,/^## Section 2/p' docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md
```

This gives you both rules including the routing table in 1.6.

- [ ] **Step 2: Insert 1.6 and 1.7 after 1.5 in SKILL.md**

Locate the end of rule 1.5 in `.claude/skills/campaign-analysis/SKILL.md`. Append:

- The full text of 1.6 from spec, including the three-row routing table for Internal / Partner-side / Impossible gap types, the discipline paragraph, and the "succeeds-but-fragility" boundary clause.
- The full text of 1.7 from spec, including the External / post-restriction / popular-tier examples and the "/intelligence/niches and /insights.coverageGaps" routing line.

In 1.6's `Impossible` row of the routing table, the spec text already includes "matches a row in `impossible-data-asks.md` by substance." Add this parenthetical at the end of that row's `Surface` column to handle the file-may-be-absent case: *"If the impossible-asks log file is absent, this row never matches — the rule degrades to inline-surface + Partner-ask verification rule for any unmatched gap."* This makes 1.6 work standalone before Task 6/7 land.

- [ ] **Step 3: Verify all 7 rules now present**

Run:
```bash
grep -n "^### 1\." .claude/skills/campaign-analysis/SKILL.md
```

Expected: 7 lines — `### 1.1` through `### 1.7`.

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "skill(campaign-analysis): add Self-challenge rules 1.6-1.7

Rule 1.6 is diagnose-the-gap when verification can't complete (extends
D1 to data-limited cases, routes to wishlist / partner-ask / impossible
log). Rule 1.7 is name-the-population — selection bias discipline that
generalizes the External-isn't-market and post-restriction-only traps.

The 1.6 Impossible row degrades gracefully when the impossible-asks
log file is absent, so this commit is coherent before Task 6/7.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Add Step 3c and Step 4 Self-challenge pointers in SKILL.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md` — add one-line pointers in Step 3c (Conditional actions paragraph) and Step 4 routing preface

- [ ] **Step 1: Locate the Step 3c "Conditional actions" paragraph**

Run:
```bash
grep -n "Conditional actions" .claude/skills/campaign-analysis/SKILL.md
```

Expected: line referencing `**Conditional actions** —` near the bottom of Step 3c.

- [ ] **Step 2: Append Self-challenge pointer to the Conditional actions paragraph**

At the end of the Conditional actions paragraph (after the existing Recommendation rules cross-reference), add this sentence:

> Walk the Self-challenge rules (1.1–1.7) over every mover and action before the response goes out.

This is the user-approved one-line pointer from spec section 1.

- [ ] **Step 3: Locate Step 4 routing preface**

Run:
```bash
grep -n "^## Step 4" .claude/skills/campaign-analysis/SKILL.md
```

The current Step 4 reads: *"Route each user follow-up to a playbook. Load `references/playbooks.md` for the full content..."*

- [ ] **Step 4: Append Self-challenge pointer to Step 4**

After the existing Step 4 sentence, append:

> Each playbook response also walks the Self-challenge rules (1.1–1.7) before going out.

- [ ] **Step 5: Verify both pointers landed**

Run:
```bash
grep -n "Self-challenge rules" .claude/skills/campaign-analysis/SKILL.md
```

Expected: at least 3 hits — the section header `## Self-challenge rules`, the Step 3c pointer, and the Step 4 pointer.

- [ ] **Step 6: Commit**

```bash
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "skill(campaign-analysis): wire Self-challenge rules into Step 3c and Step 4

Adds one-line pointers from the Conditional actions paragraph in Step
3c and from the Step 4 playbook routing preface, so the rules fire on
every output (opener movers/actions and follow-up playbook responses).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Add Playbook H to references/playbooks.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/references/playbooks.md` — add Playbook H between Playbook G and Step 5; update the Contents list at the top

The full Playbook H text is in spec section 2 (about 80 lines).

- [ ] **Step 1: Read spec section 2 in full**

Run:
```bash
sed -n '/^## Section 2 — Playbook H/,/^## Section 3/p' docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md
```

This gives the trigger phrases, boundary statements, fetch list, six aggregations, restructure proposals block, Self-challenge pass cross-reference, and mutations table reference.

- [ ] **Step 2: Update the Contents list at the top of playbooks.md**

The current Contents list (around line 5–20) includes Playbooks A through G. Add a new bullet for Playbook H between G and the Step 5 entry:

```
  - Playbook H — "Review our portfolio shape" (architecture review)
```

- [ ] **Step 3: Insert Playbook H section before Step 5**

Locate `## Step 5 — Strategy doc sync` in playbooks.md. Insert before it a new `### Playbook H — "Review our portfolio shape" / portfolio architecture review` section containing:

1. Trigger phrases (verbatim from spec)
2. Boundary vs Playbook A and F (3 bullets, verbatim)
3. Fetch list (7 endpoints, verbatim)
4. Step 1b currentScope filter reminder
5. Approach — six aggregations as numbered list:
   1. Concentration analysis
   2. Per-campaign profit-per-deployed-dollar
   3. Long-tail audit
   4. Coverage-gap audit (market-demand-driven)
   5. Restriction half-life
   6. Scope critique
6. Restructure proposals output block (5 bullets)
7. Self-challenge rules pass paragraph
8. Mutations cross-reference (point to existing Mutations table)
9. Worked example placeholder paragraph

For the worked example: write a 6-aggregation walkthrough on a synthetic 6-campaign portfolio (use placeholder names like `Campaign A` through `Campaign F` — do not use canonical Card Yeti campaign names since this is an example, not a session). End with three restructure proposals: one deprecation (low-on-both bottom-quartile slot), one scope-narrow (Modern PSA 8 → PSA 9-10 with sub-$150 floor), one external-confirmed → focused inclusion add. Add a Sequence block at the end.

The example shape mirrors Playbook A's worked example. Length target: ~30 lines including the synthetic numbers.

- [ ] **Step 4: Verify Playbook H lands cleanly**

Run:
```bash
grep -n "^### Playbook " .claude/skills/campaign-analysis/references/playbooks.md
```

Expected: 8 lines — Playbooks A through H.

Also confirm Contents list:
```bash
sed -n '1,30p' .claude/skills/campaign-analysis/references/playbooks.md | grep "Playbook"
```

Expected: 8 Playbook entries A–H.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/campaign-analysis/references/playbooks.md
git commit -m "skill(campaign-analysis): add Playbook H portfolio architecture review

New playbook for questioning the entire portfolio shape — concentration,
profit-per-deployed-dollar, long-tail audit, coverage-gap (market-
demand-driven, not External-driven), restriction half-life, scope
critique. Produces sized restructure proposals (deprecate/scope/split/
merge/new) under the existing Capital guardrail and Sizing rules.

Boundary vs Playbook A (parameter tuning within fixed structure) and
Playbook F (single new campaign for one gap) is stated at the top so
the model doesn't conflate them.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Add Step 3c soft-flag paragraph in SKILL.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md` — append soft-flag paragraph after the Step 3c Close paragraph; update the two opener examples

- [ ] **Step 1: Read spec section 4 in full**

Run:
```bash
sed -n '/^## Section 4/,/^## Section 5/p' docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md
```

This contains the soft-flag paragraph text and the four trigger conditions.

- [ ] **Step 2: Locate the Step 3c Close paragraph**

Run:
```bash
grep -n "^\*\*Close\*\*" .claude/skills/campaign-analysis/SKILL.md
```

Expected: one line in Step 3c that begins `**Close** —`.

- [ ] **Step 3: Insert the soft-flag paragraph after the Close paragraph**

The new paragraph from spec section 4, verbatim:

> **Portfolio-shape soft-flag.** After the close question, run the four Playbook H trigger conditions against the data already fetched. If any fire, append one line: *"Portfolio shape note: top 3 hold N% of profit, bottom Q quartile produced X% / N campaigns had <3 fills in 30d / N grade-or-era restrictions held >60d uncontested. Want a Playbook H pass?"* Only the firing conditions appear in the line — don't list non-firing ones. If none fire, omit the line entirely. The line is a question, not a script change — it never auto-runs Playbook H.

Below this paragraph, add the four conditions as a numbered list, verbatim from the spec.

- [ ] **Step 4: Update the existing opener examples to show soft-flag behavior**

The Step 3c section has three example openers:
1. "Example opening turn — clean signal with actions"
2. "Example opening turn — hold week, no actions"
3. "Example opening turn — contradiction detected"

Update example 1 to add a `Portfolio shape note: top 3 hold 78% of profit, bottom quartile produced 4%. Want a Playbook H pass?` line at the end, after the Close question. This shows the soft-flag firing.

Leave examples 2 and 3 unchanged — they show the no-firing case (omit the line entirely) and the contradiction case (no soft-flag because no opener body), demonstrating the conditions-driven behavior.

- [ ] **Step 5: Verify the paragraph and example landed**

Run:
```bash
grep -n "Portfolio shape" .claude/skills/campaign-analysis/SKILL.md
```

Expected: 2 hits — the paragraph header `**Portfolio-shape soft-flag.**` and the example soft-flag line `Portfolio shape note: top 3 hold 78%...`.

- [ ] **Step 6: Commit**

```bash
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "skill(campaign-analysis): add Step 3c soft-flag for Playbook H

Tactical openers now append one optional line proposing a Playbook H
pass when any of four conditions fire (top-3 concentration >70%,
campaigns with 0 fills in 30d, External-confirmed coverage gap matched
by market-demand source, or restriction held >60 days uncontested).
The line is a question — never auto-runs the playbook. Opener format
is preserved: omitted entirely when no condition fires.

Updated example 1 to show the soft-flag firing; examples 2 and 3
preserve the omitted-line and no-body cases.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Add Step 0 wiring for impossible-asks log in SKILL.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/SKILL.md` — add one bullet to the Step 0 file-load list

- [ ] **Step 1: Locate Step 0 in SKILL.md**

Run:
```bash
grep -n "^## Step 0" .claude/skills/campaign-analysis/SKILL.md
```

Step 0 currently reads: *"Read `docs/private/campaign-analysis-config.md`..."* and continues with the operator config and strategy doc loads.

- [ ] **Step 2: Add the impossible-asks log file-load**

After the existing Step 0 file-load instructions for the operator config and strategy doc, add this paragraph (from spec section 3.2):

> Read `docs/private/impossible-data-asks.md` if it exists. Hold its contents in working memory for the rest of the session — every partner-ask draft cross-references this list before drafting. If the file is absent, partner-asks proceed without an impossibility filter; surface this in any retrospective draft so the user knows.

The file-load order becomes: operator config → strategy doc → impossible-asks log. The third entry is optional like the strategy doc.

- [ ] **Step 3: Verify the bullet landed**

Run:
```bash
grep -n "impossible-data-asks" .claude/skills/campaign-analysis/SKILL.md
```

Expected: at least 2 hits — the new Step 0 paragraph and the existing Self-challenge rule 1.6 routing table reference (added in Task 2).

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/campaign-analysis/SKILL.md
git commit -m "skill(campaign-analysis): wire impossible-asks log into Step 0

Skill now loads docs/private/impossible-data-asks.md at session start
if present. File is gitignored and operator-private; absence is fine
(impossibility filter degrades to inactive). The Partner-ask
verification rule update in the next task adds the cross-check entry
point.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Update Partner-ask verification rule in references/playbooks.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/references/playbooks.md` — add new step 0 to Partner-ask verification rule; add schema documentation; add "Logging an ask as newly impossible" appendix

- [ ] **Step 1: Locate the existing Partner-ask verification rule**

Run:
```bash
grep -n "^### Partner-ask verification" .claude/skills/campaign-analysis/references/playbooks.md
```

Expected: one line under the Recommendation rules section. The current rule has three numbered questions (scheduler / related field / partner docs).

- [ ] **Step 2: Insert new step 0 before the existing 3-question check**

The existing rule body currently lists questions as a 3-bullet list. Renumber them as steps 1–3 and prepend a new step 0, verbatim from spec section 3.3:

> **0. Cross-check against `docs/private/impossible-data-asks.md`.** Match the proposed ask against the log by *substance, not exact wording* — the noun phrase should match (e.g. "competitor buy%", "competitor pricing on Partner Offers", "what other PSA buyers pay" all match the same row). If matched: don't draft the ask; surface the match inline (*"Can't draft this — logged 2026-05-05 as impossible because PSA won't share supply-side competition data. Falling back to CL movement on filling segments as a proxy per the alternative."*) and stamp today's date in the matched row's `Last revisited` column. If unmatched, proceed to the existing three-question check below.

The three existing questions become steps 1, 2, 3 of the rule. Total rule body: 0 → 1 → 2 → 3.

- [ ] **Step 3: Add schema documentation**

After the renumbered rule body and before any closing paragraph, add a schema documentation block:

> **Schema for `docs/private/impossible-data-asks.md`:**
>
> ```markdown
> # Impossible Data Asks
>
> Asks that are known to be unanswerable by the named partner — logged so the skill stops repeatedly proposing them. Cross-referenced before any partner-ask draft.
>
> | Ask | Why impossible | Alternatives | Logged | Last revisited |
> |-----|----------------|--------------|--------|-----------------|
> | competitor PSA Partner Offers buy% | PSA won't share supply-side competition data — privacy / competitive-landscape reasons | watch CL movement on filling segments as a proxy for competitor pressure; track our own fill-rate-near-cap vs supply-thin via the Cap-diagnostic rule | 2026-05-05 | — |
> ```

- [ ] **Step 4: Add the "Logging an ask as newly impossible" appendix**

After the schema block, add:

> **Logging an ask as newly impossible.** When a draft ask is reviewed and the user determines the partner won't answer it (privacy, competitive reasons, structural infeasibility), the skill appends a row to `docs/private/impossible-data-asks.md` with the user-supplied "Why impossible" and any alternatives discussed. Append, never overwrite. If the file is absent, create it with the header from the schema documentation above and the first row.

- [ ] **Step 5: Verify the rule is coherent**

Run:
```bash
sed -n '/^### Partner-ask verification/,/^### /p' .claude/skills/campaign-analysis/references/playbooks.md
```

Expected output should show:
- Rule heading
- Numbered steps 0, 1, 2, 3 (in that order)
- Schema documentation block
- "Logging an ask as newly impossible" appendix

- [ ] **Step 6: Commit**

```bash
git add .claude/skills/campaign-analysis/references/playbooks.md
git commit -m "skill(campaign-analysis): add impossibility cross-check to Partner-ask rule

Partner-ask verification now runs a step 0 fuzzy-match against
docs/private/impossible-data-asks.md before the existing three-question
check. Matching by substance (not wording) catches asks the user has
already determined the partner won't answer, like 'competitor PSA
Partner Offers buy%' under any rewording.

Adds the schema documentation inline so the file format is readable
next to the rule that uses it. The 'Logging as newly impossible'
appendix specifies the append-only flow and graceful first-creation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Update Step 6 retrospective with Partner-ask cross-reference

**Files:**
- Modify: `.claude/skills/campaign-analysis/references/playbooks.md` — add one-line cross-reference to Step 6's "DH-side asks" bucket

- [ ] **Step 1: Locate the Step 6 retrospective DH-side asks bucket**

Run:
```bash
grep -n "DH-side asks" .claude/skills/campaign-analysis/references/playbooks.md
```

Expected: a line in Step 6 bucket #2 reading something like *"DH-side asks — things we believe DH should be populating but isn't..."*

- [ ] **Step 2: Append the cross-reference**

At the end of bucket #2's description, before bucket #3 begins, add the one-line note from spec section 3.4:

> Before logging any item to a dated `docs/private/YYYY-MM-DD-<partner>-data-ask.md` file, run the Partner-ask verification rule's step 0 against the impossible-asks log. Drop any items that match.

- [ ] **Step 3: Verify the line landed**

Run:
```bash
grep -n "Partner-ask verification rule's step 0" .claude/skills/campaign-analysis/references/playbooks.md
```

Expected: 1 hit in Step 6.

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/campaign-analysis/references/playbooks.md
git commit -m "skill(campaign-analysis): wire Step 6 retrospective into impossibility filter

Step 6's DH-side-asks bucket now cross-references Partner-ask
verification rule step 0 before any item lands in a dated partner-ask
draft file. Same impossibility filter, two entry points (mid-analysis
via 1.6 routing, end-of-session via Step 6).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Append six new entries to evals/known-failures.md

**Files:**
- Modify: `.claude/skills/campaign-analysis/evals/known-failures.md` — append 6 entries before the `## How to use this list` footer

The eval entries follow the existing entry shape (Date, Scenario, Failure, Corrective rule, Anchor, Regression check). All dated 2026-05-05.

- [ ] **Step 1: Read spec section 5 for entry titles, anchors, and regression checks**

Run:
```bash
sed -n '/^## Section 5/,/^## Open questions/p' docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md
```

This lists the 6 entries with their anchors and regression checks. The Scenario fields need to be drawn from the source-session JSONL — see Step 2.

- [ ] **Step 2: Pull short scenario summaries from the source session**

The source session at `/home/vscode/.claude/projects/-workspace/8951aaf1-df9d-4fe6-aac1-b9ed0decaa89.jsonl` contains the moments where each failure surfaced. For each of the 6 entries, write a 1–2 sentence Scenario field grounded in something specific from that session — a paraphrased pushback or observed behavior. The user's recurring pushbacks observed in the source session (per spec section 1.2) include *"did you actually look at the data?"*, *"you may have a bad assumption"*, *"would those changes do anything?"*, *"is that even possible?"* — at least one of these should appear in the D1/D3/D4 entries.

If the JSONL is large, a quick grep on user-message lines is enough — the goal is grounding, not transcription:
```bash
grep -i "did you actually" /home/vscode/.claude/projects/-workspace/8951aaf1-df9d-4fe6-aac1-b9ed0decaa89.jsonl | head -3
```

- [ ] **Step 3: Locate the footer in known-failures.md**

Run:
```bash
grep -n "^## How to use this list" .claude/skills/campaign-analysis/evals/known-failures.md
```

The 6 new entries land before this footer.

- [ ] **Step 4: Append entry 1 — D1 stops at first plausible answer**

Append before the footer. Anchor uses dual-reference per spec section 5 (general Self-challenge rule + the existing specific Recommendation rules that 1.1 generalizes):

```markdown
---

## Failure: stops at first plausible answer (D1)

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences from the source session — e.g. "User asked which segment was dragging Mid-Era; skill named the first byGrade row without aggregating across all Mid-Era campaigns. User pushed back: 'did you actually look at the data?'"]
- **Failure:** Skill cited the first metric / first hypothesis / first lever without checking a second source, ranking alternatives, or sanity-checking the lever binds. The same source session produced six surgical Recommendation rules covering specific cases (Cap-cut binding, Dollar-weighted BPCL, Category vs campaign, Era-fit gate, Throttle lever selection, Fill-drought hypothesis ranking); 1.1 generalizes the discipline.
- **Corrective rule:** Verify-before-propose gate — state claim → name cross-check that could falsify it → run it → commit or revise. Four required cross-check types: category claims (aggregate across all campaigns), metric citations (dollar-weighted vs mean-of-ratios), action proposals (lever-binds sanity check), hypothesis sets (rank by evidence).
- **Anchor (general):** `SKILL.md` Self-challenge rule 1.1
- **Anchor (specific instances, dual-reference):** `references/playbooks.md` Cap-diagnostic rule cap-cut binding check + `SKILL.md` API footguns dollar-weighted BPCL cross-check + `SKILL.md` Conversational guideline 4 Category vs campaign discipline + `references/playbooks.md` Era-fit gate + `references/playbooks.md` Throttle lever selection + `references/playbooks.md` Fill-drought hypothesis ranking
- **Regression check:** Does Self-challenge rule 1.1 still mandate cross-check-by-claim-type (category aggregation, dollar-weighted vs mean-of-ratios divergence, lever-binds sanity check, hypothesis ranking)? Does it still cite the six existing specific Recommendation rules above as named precedents?
```

- [ ] **Step 5: Append entry 2 — D2 defers to server suggestions**

```markdown
---

## Failure: defers to server suggestions verbatim (D2)

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences — e.g. "/portfolio/suggestions surfaced 'lower CL% on C1' from lifetime data; skill echoed the suggestion without noting the recent restriction had already excluded the contributing grades."]
- **Failure:** Skill echoed a server suggestion verbatim instead of treating it as one input among several. Server suggestions operate on lifetime data, don't know about recent restrictions, and lack scope context. The Era-fit gate's "echoed `/snapshot.suggestions` 'Add top performers'" carve-out names the era-filter requirement; 1.3 generalizes the discipline of not echoing without reframing.
- **Corrective rule:** Server suggestions are inputs, not outputs. Reframe each suggestion into your own analysis or drop it. Stale-suggestion filter and Step 1b currentScope filter still apply on top.
- **Anchor (general):** `SKILL.md` Self-challenge rule 1.3
- **Anchor (specific instance, dual-reference):** `references/playbooks.md` Era-fit gate (echoed `/snapshot.suggestions` "Add top performers" carve-out)
- **Regression check:** Does Self-challenge rule 1.3 still require server suggestions to be reframed or dropped, never echoed verbatim? Does it cite the Era-fit gate's echoed-suggestion carve-out as the named precedent?
```

- [ ] **Step 6: Append entry 3 — D3 describing vs analyzing**

```markdown
---

## Failure: conflates describing with analyzing (D3)

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences — e.g. "Skill listed `byCharacter` JSON keys and cited an avgBuyPctOfCL number without explaining what the number meant for portfolio decisions. User pushed back: 'did you actually look at the data?'"]
- **Failure:** Listing JSON keys, citing field values, restating coverageGaps rows treated as analysis. Skill output was a parsed-data dump, not interpretation. The Category vs campaign discipline (Conversational guideline 4) names one specific instance — `Modern (C4) has been dark 12 days` reads as a category-level claim when the underlying datum is one-campaign-level. 1.4 generalizes the move.
- **Corrective rule:** Every parsed datum in user-facing output carries one short interpretive clause — what the number means, why it might mislead, what the picture really is. Without the interpretation, drop the datum.
- **Anchor (general):** `SKILL.md` Self-challenge rule 1.4
- **Anchor (specific instance, dual-reference):** `SKILL.md` Conversational guideline 4 Category vs campaign discipline
- **Regression check:** Does Self-challenge rule 1.4 still require an interpretive clause on every parsed datum, and cite the Category vs campaign discipline as the named precedent?
```

- [ ] **Step 7: Append entry 4 — D4 afraid to disagree with data**

```markdown
---

## Failure: afraid to disagree with the data when pattern says otherwise (D4)

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences — e.g. "/tuning suggested lowering CL% on a campaign whose realized buy% was high. Pattern was CL-lead (CL above market, correcting), not terms-too-high. Skill recommended the terms cut anyway, echoing the surface conclusion."]
- **Failure:** Skill recommended a lever the endpoint named without checking whether the underlying pattern supported it. The Cap-diagnostic rule (both directions) and Popular-tier exclusion are existing precedents for pushing back on a surface conclusion when the pattern says otherwise; 1.5 generalizes them.
- **Corrective rule:** When `/tuning` or `/portfolio/suggestions` outputs a surface conclusion that contradicts the underlying pattern, push back and state which signal you trust and why. The pattern AND the lever-binds check both have to support a recommendation.
- **Anchor (general):** `SKILL.md` Self-challenge rule 1.5
- **Anchor (specific instances, dual-reference):** `references/playbooks.md` Cap-diagnostic rule (both directions: supply-thinness check AND cap-cut binding check) + `references/playbooks.md` Popular-tier exclusion
- **Regression check:** Does Self-challenge rule 1.5 still generalize the Cap-diagnostic (both directions) and Popular-tier-exclusion precedents into a broader push-back-on-data discipline?
```

- [ ] **Step 8: Append entry 5 — S1 fixed-universe scope failure**

```markdown
---

## Failure: treats existing N campaigns as fixed universe (S1)

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences — e.g. "User asked what bigger moves to consider. Skill optimized parameters within current structure rather than questioning the structure — never proposed deprecation, scope changes, splits, or merges."]
- **Failure:** Tactical analysis only. Skill never aggregated profit-per-deployed-dollar to identify the bottom quartile, never audited the long tail for deprecation candidates, never questioned whether existing scope cuts were the right cuts.
- **Corrective rule:** New Playbook H — Portfolio Architecture Review. Six aggregations (concentration, profit-per-deployed-dollar, long-tail, coverage-gap demand-driven, restriction half-life, scope critique) producing sized restructure proposals. Tactical openers also gain a soft-flag pointer when any of four threshold conditions fire.
- **SKILL.md anchor:** `references/playbooks.md` Playbook H + `SKILL.md` Step 3c soft-flag
- **Regression check:** Does Playbook H still cover all six aggregations and produce sized restructure proposals? Does the Step 3c soft-flag fire on any of the four conditions (top-3 concentration > 70%, ≥1 campaign with 0 fills in 30d, External-confirmed coverage gap matched by demand source, restriction held > 60 days)?
```

- [ ] **Step 9: Append entry 6 — repeated impossible-ask proposals**

```markdown
---

## Failure: repeated proposal of impossible data asks

- **Date:** 2026-05-05
- **Scenario:** [1-2 sentences — e.g. "Across multiple sessions, skill proposed asking PSA for competitor buy% data even though the user had previously confirmed PSA won't share supply-side competition information."]
- **Failure:** Without persistent state about which asks the partner refuses to answer, the skill re-proposes the same impossibility every session. Wastes user attention; erodes trust in the partner-ask drafts.
- **Corrective rule:** New `docs/private/impossible-data-asks.md` log loaded at Step 0; Partner-ask verification rule gains a step 0 fuzzy-match cross-check by substance (not wording) before the existing three-question check. Matched asks are surfaced inline with the alternatives and the matched row's `Last revisited` column gets stamped today.
- **SKILL.md anchor:** Step 0 file-load + `references/playbooks.md` Partner-ask verification rule step 0
- **Regression check:** Does Step 0 still load `docs/private/impossible-data-asks.md` if present? Does the Partner-ask verification rule still run a step 0 fuzzy-match cross-check before the existing three-question check, and stamp `Last revisited` on matches?
```

- [ ] **Step 10: Verify all 6 entries land before the footer**

Run:
```bash
grep -c "^## Failure: " .claude/skills/campaign-analysis/evals/known-failures.md
```

Expected: 22 entries (8 original + 5 from origin/main `7a8a9c28` + 3 from origin/main `c99e6669` + 6 new from this PR). Confirm via:

```bash
git -C /workspace/.worktrees/campaign-analysis-strategic-shift log --oneline -10 -- .claude/skills/campaign-analysis/evals/known-failures.md
```

Expected to show recent edits on `origin/main` plus this branch's pending commit.

Also confirm the footer is still last:
```bash
tail -5 .claude/skills/campaign-analysis/evals/known-failures.md
```

Expected: ends with the existing "How to use this list" footer text.

- [ ] **Step 11: Commit**

```bash
git add .claude/skills/campaign-analysis/evals/known-failures.md
git commit -m "skill(campaign-analysis): add 6 regression entries for D1-D4, S1, impossibility

D1 stops-at-first, D2 defers-to-server, D3 describes-not-analyzes, D4
afraid-to-disagree, S1 fixed-universe-scope, and repeated-impossible-ask
each get an entry with a regression check that resolves to the
Self-challenge rule, Playbook H, soft-flag, or impossibility-filter
anchor. All scenarios grounded in the source session JSONL.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Final readthrough + PR draft (no push)

**Files:** Read all four modified skill files end-to-end to confirm coherence.

- [ ] **Step 1: Read SKILL.md end-to-end**

Run:
```bash
cat .claude/skills/campaign-analysis/SKILL.md | wc -l
cat .claude/skills/campaign-analysis/SKILL.md | head -200
cat .claude/skills/campaign-analysis/SKILL.md | sed -n '200,400p'
cat .claude/skills/campaign-analysis/SKILL.md | sed -n '400,$p'
```

Confirm:
- Step 0 file-load includes impossible-asks log
- Step 3c has the Conditional actions Self-challenge pointer AND the Portfolio-shape soft-flag paragraph
- Self-challenge rules section exists with 1.1–1.7
- Step 4 has the playbook-response Self-challenge pointer

- [ ] **Step 2: Read references/playbooks.md end-to-end**

Run:
```bash
cat .claude/skills/campaign-analysis/references/playbooks.md | wc -l
cat .claude/skills/campaign-analysis/references/playbooks.md
```

Confirm:
- Contents list shows Playbooks A–H
- Playbook H section is between G and Step 5 with all six aggregations
- Partner-ask verification rule has steps 0, 1, 2, 3, schema doc, and "Logging as newly impossible" appendix
- Step 6 retrospective DH-side asks bucket has the cross-reference line

- [ ] **Step 3: Read evals/known-failures.md end-to-end**

Run:
```bash
cat .claude/skills/campaign-analysis/evals/known-failures.md
```

Confirm 14 failure entries (8 original + 6 new), all 6 new entries dated 2026-05-05, footer still last.

- [ ] **Step 4: Walk regression checks for all 14 entries**

For each entry's `Regression check` question, walk to the named anchor and answer the question with yes / no. Any "no" or "unclear" means the rule got weakened — investigate and fix before completing this task.

- [ ] **Step 5: Confirm git log shape**

Run:
```bash
git log --oneline main..HEAD
```

Expected: 9 commits (one per task 1–9), each with a `skill(campaign-analysis):` prefix.

- [ ] **Step 6: Prepare PR draft locally — DO NOT PUSH**

Per spec: "Single PR. Do not push without explicit user approval."

Draft the PR body in a local file:
```bash
cat > /tmp/pr-body.md <<'EOF'
## Summary

Adds seven Self-challenge rules (1.1–1.7), Playbook H portfolio architecture review, Step 3c soft-flag, and impossible-asks log integration to the campaign-analysis skill — shifting it from tactical parameter-tuning within fixed campaign structure to strategic portfolio-shape analysis with proactive data-gap diagnosis.

Spec: `docs/specs/2026-05-05-campaign-analysis-depth-and-scope-design.md`
Plan: `docs/plans/2026-05-05-campaign-analysis-depth-and-scope.md`

## Capability map

| Capability | Lands in |
|------------|----------|
| Verify-before-propose gate (D1) | SKILL.md Self-challenge rule 1.1 |
| Anticipate-the-pushback | SKILL.md Self-challenge rule 1.2 |
| Server suggestions are inputs (D2) | SKILL.md Self-challenge rule 1.3 |
| Describing isn't analyzing (D3) | SKILL.md Self-challenge rule 1.4 |
| Disagree with data (D4) | SKILL.md Self-challenge rule 1.5 |
| Diagnose-the-gap | SKILL.md Self-challenge rule 1.6 |
| Selection bias | SKILL.md Self-challenge rule 1.7 |
| Portfolio architecture (S1) | references/playbooks.md Playbook H + Step 3c soft-flag |
| Impossible-asks log | SKILL.md Step 0 + Partner-ask verification rule step 0 |
| Eval coverage | evals/known-failures.md 6 new entries |

## Constraints respected

- Opener output format unchanged (soft-flag is one optional appended line)
- No semantic changes to existing Recommendation rules
- Not fragmented into "strategic vs tactical mode"

## Test plan

- [ ] Re-read SKILL.md end-to-end; confirm Step 0/3c/4 pointers and Self-challenge section coherent
- [ ] Re-read references/playbooks.md end-to-end; confirm Playbook H, Partner-ask rule update, Step 6 touch-up
- [ ] Walk all 22 regression checks in evals/known-failures.md; each answers "yes"
- [ ] Confirm Self-challenge rules 1.1, 1.2, 1.3, 1.4, 1.5 each cite their named precedents from origin/main
- [ ] Run a /campaign-analysis session and verify the soft-flag fires when conditions are met
- [ ] Run a /campaign-analysis session that hits a data gap; verify 1.6 routes correctly

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
echo "PR body draft saved to /tmp/pr-body.md"
cat /tmp/pr-body.md
```

- [ ] **Step 7: Report status to user; wait for push approval**

Output to user:

> Branch `skill/campaign-analysis-depth-and-scope` is ready. 9 commits in worktree at `.worktrees/campaign-analysis-strategic-shift/`. PR body drafted at `/tmp/pr-body.md`. **Not pushed** per the standing "do not push without explicit approval" rule. Want me to push and open the PR, or do you want to review locally first?

Then stop. Do not push.

---

## Self-review against the spec

Walking the spec section-by-section to confirm coverage:

1. **Spec Section 1 (Self-challenge rules 1.1–1.7)** → Tasks 1, 2, 3 cover rules and pointers. ✓
2. **Spec Section 2 (Playbook H)** → Task 4. ✓
3. **Spec Section 3 (Impossible-asks log)** → Tasks 6, 7, 8 cover Step 0, Partner-ask rule, Step 6 touch-up. The file itself is intentionally not created in this PR — runtime-only. ✓
4. **Spec Section 4 (Step 3c soft-flag)** → Task 5. ✓
5. **Spec Section 5 (eval entries)** → Task 9. ✓

All 6 spec sections have corresponding tasks. No placeholders in the plan steps — all rule text references the spec by exact section, all commands are runnable, all commit messages are written. Type/method consistency check: the rule numbers (1.1–1.7), playbook letter (H), and file paths used in later tasks match what's defined in earlier tasks.
