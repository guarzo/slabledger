# Campaign Analysis Skill — Depth-and-Scope Capability Addition

**Date:** 2026-05-05
**Status:** Approved (brainstorm complete) — pending implementation plan
**Branch:** `skill/campaign-analysis-depth-and-scope`
**Single PR.** Do not push without explicit user approval.

## Motivation

The `.claude/skills/campaign-analysis/` skill has matured into a tactical instrument — disciplined opener structure, per-mover verification of current-scope, strong recommendation rules. But four depth issues and one scope issue remain, observed across recent sessions:

**Depth issues (vertical — thinking harder per question):**

- **D1.** Stops at first plausible answer. Cites the first metric without checking a second. Names the first hypothesis without ranking. Proposes the first lever without verifying it would do anything.
- **D2.** Defers to authority — when `/tuning`, `/portfolio/suggestions`, or `snapshot.suggestions` emit a recommendation, the skill echoes it instead of using it as one input. Server-side suggestions operate on lifetime data, don't know about recent restrictions, and lack scope context.
- **D3.** Conflates describing with analyzing. Endpoint outputs get parsed and presented as analysis. Listing keys, citing `avgBuyPctOfCL`, naming sell-through % — these are *data*. An analyst would add interpretation.
- **D4.** Afraid to disagree with the data. When `/tuning` says "lower CL%" but the underlying pattern says "narrow scope," the skill recommends lowering CL%. Real analysis pushes back on the data's surface conclusion when the pattern says otherwise.

**Scope issue (horizontal — asking bigger questions):**

- **S1.** Treats existing N campaigns as a fixed universe. Optimizes parameters *within* current structure, never questions the structure: are these the right N, is each campaign's scope the right cut, where is profit concentrated, what restrictions have held too long, what restructuring would unlock value parameter tuning can't?

This spec adds four capabilities to address D1–D4 and S1, plus selection-bias discipline (1.7) that surfaced during brainstorming. Constraints respected throughout: opener format unchanged; no semantic changes to existing rules (Sizing, Confidence bands, Capital guardrail, Sequencing, Popular-tier, Sub-$150 modern, Turnover, Cap-diagnostic); not fragmented into "strategic vs tactical mode."

## Scope

**In scope (single PR):**
- New `Self-challenge rules` section in `SKILL.md` — sub-rules 1.1–1.7 covering D1–D4 and selection bias
- New `Playbook H — Portfolio Architecture Review` in `references/playbooks.md`
- Step 3c soft-flag pointer to Playbook H when four threshold conditions fire
- Step 0 wiring to load `docs/private/impossible-data-asks.md` if present
- Partner-ask verification rule update — new step 0 cross-checks against the impossible-asks log
- Step 6 retrospective touch-up — partner-ask logging routes through the impossibility filter
- Six new entries in `evals/known-failures.md` covering D1, D2, D3, D4, S1, and the impossible-asks regression

**Out of scope:**
- Strategic-vs-tactical mode separation (rejected by user)
- Auto-running Playbook H on a calendar cadence (rejected — soft-flag preferred)
- Auto-creation of `docs/private/impossible-data-asks.md` with seed entries (file is created only on first user-initiated logging)
- Any change to existing Recommendation rules (Sizing, Confidence bands, etc.)
- Any change to opener output structure (data-sources block, reconciliation summary, movers, conditional actions, portfolio at a glance, close)

## Files touched

| Capability | Files | Change shape |
|------------|-------|--------------|
| Self-challenge rules (1.1–1.7) | `.claude/skills/campaign-analysis/SKILL.md` | New section between "Recommendation rules" and "Data conventions"; one-line pointers from Step 3c and Step 4 |
| Playbook H | `.claude/skills/campaign-analysis/references/playbooks.md` | New playbook between Playbook G and Step 5; routing entry added in the contents list |
| Step 3c soft-flag | `.claude/skills/campaign-analysis/SKILL.md` | New paragraph after the Close paragraph in Step 3c; one new opener example showing the soft-flag firing |
| Impossible-asks log loading | `.claude/skills/campaign-analysis/SKILL.md` | One bullet added to Step 0 file-load |
| Partner-ask verification rule update | `.claude/skills/campaign-analysis/references/playbooks.md` | New step 0 inserted before existing 3-question check; appendix paragraph on logging-as-impossible; schema documentation |
| Step 6 retrospective touch-up | `.claude/skills/campaign-analysis/references/playbooks.md` | One-line cross-reference to the Partner-ask verification rule's step 0 |
| Eval coverage | `.claude/skills/campaign-analysis/evals/known-failures.md` | Six new entries appended in existing entry shape |
| Impossible-asks log file | `docs/private/impossible-data-asks.md` | NOT created in this PR — created on first user-initiated logging at runtime |

## Section 1 — Self-challenge rules (new in `SKILL.md`)

A new top-level section between **Recommendation rules** and **Data conventions** in `SKILL.md`. Seven sub-rules (1.1–1.7). Step 3c's "Conditional actions" paragraph and Step 4's playbook routing each gain a one-line pointer: *"Walk the Self-challenge rules before the response goes out."*

Rules live in `SKILL.md` (not `references/playbooks.md`) because they apply to *every* output the skill produces — opener and follow-ups. Lazy-loading via references would risk the model never reading them.

**Layered with existing rules.** The Self-challenge rules complement two pre-existing layers of discipline:

- **The Business-mechanic premise gate** (top-level in `SKILL.md`) fires *before* analysis begins — it refuses to run multi-step financial reasoning on top of an unverified business mechanic (invoice cadence, cycle-week effect, etc.). The Self-challenge rules fire *during* analysis, on every claim and lever.
- **Specific Recommendation rules** (Cap-diagnostic with cap-cut binding check, Era-fit gate, Throttle lever selection, Fill-drought hypothesis ranking, Dollar-weighted BPCL cross-check, Category vs campaign discipline, Popular-tier exclusion, etc.) fire on specific levers, segments, or claim shapes with concrete thresholds. The Self-challenge rules generalize these — when the pattern matches but no specific rule applies, 1.1–1.7 still fire.

When the lever or claim falls under one of those specific rules, follow the specific rule's threshold first; the Self-challenge rule fires when the underlying pattern is the same but no specific rule names it.

### 1.1 — Verify-before-propose gate (D1)

Before any mover, action, or hypothesis-set lands, state the claim → name the cross-check that could falsify it → run it → commit or revise. Four required cross-checks by claim type:

- **Category claims** (e.g. "Mid-Era is dragging") — aggregate across all campaigns in the category, not one row. Specific instance: the **Category vs campaign discipline** in Conversational guideline 4 names Modern (C4 + C10) as the canonical case.
- **Metric citations** (e.g. "ROI 8%") — compute a second metric: dollar-weighted vs mean-of-ratios, portfolio-wide vs scope-filtered. If the two diverge, lead with the dollar-weighted figure and call out the mean-of-ratios as misleading inline. Specific instance: the **Dollar-weighted BPCL cross-check** in API footguns names the threshold (`avgBuyPctOfCL ≥ 0.90` triggers the cross-check) and the divergence rule (>10pp difference triggers both-numbers presentation).
- **Action proposals** — sanity-check the lever binds. Specific instances: **Cap-cut binding check** (Cap-diagnostic rule, inverse direction) requires `excess ≥ $500/14d` or `daysExceeded ≥ 25%` before a cap reduction is non-no-op; **Era-fit gate** (Recommendation rules) requires character year-of-first-release within campaign `yearRange` before any inclusion-list add; **Throttle lever selection** requires both cap and terms presented as peer levers in any spend-reduction proposal.
- **Hypothesis sets** — when listing alternative explanations for an observed pattern (drought, deceleration, anomalous metric), rank by evidence rather than presenting equal-weight. Specific instance: **Fill-drought hypothesis ranking** names the four canonical hypotheses (competition / supply lull / cycle dip / inclusion-list mismatch) and the ranking-by-evidence shape.

When verification kills a draft mover/action, no need to surface the dropped one — just don't list it. When verification *modifies* (downgrades confidence, flips lever, re-ranks hypotheses), surface the modification path inline so the reasoning is auditable.

### 1.2 — Anticipate-the-pushback (Capability 2)

Before any claim or recommendation lands, mentally enumerate the top 2 pushbacks an analyst would raise. The recurring four observed in the source session:

- *"did you actually look at the data?"*
- *"you may have a bad assumption"*
- *"would those changes do anything?"*
- *"is that even possible?"*

Address inline (one short clause) or downgrade confidence one band. If you can't address a pushback, drop the claim — silence beats a brittle assertion. Specific instance: the **Throttle lever selection** rule is itself an anticipate-the-pushback move — silently picking cap when the operator might prefer terms invites the *"would those changes do anything?"* pushback; presenting both levers as peers prevents it.

### 1.3 — Server suggestions are inputs, not outputs (D2)

`/portfolio/suggestions`, `snapshot.suggestions`, and `/tuning` recommendations operate on lifetime data and don't know about recent restrictions, scope context, or the operator's edge. Treat as one input among several. Never echo a server suggestion verbatim. Either reframe it into your own analysis ("the server flags X; the underlying pattern is actually Y, so the right move is Z"), or drop it. The stale-suggestion filter and Step 1b currentScope filter still apply on top of this rule. Specific instance: the **Era-fit gate** carve-out for echoed `/snapshot.suggestions` "Add top performers" entries — the suggestion endpoint sorts by portfolio-wide ROI without era-filtering, so blindly echoing it produced the Leafeon/Rayquaza-on-Vintage-Core failure. The Era-fit gate names the era-filter requirement; 1.3 generalizes the discipline of not echoing without reframing.

### 1.4 — Describing is not analyzing (D3)

Listing JSON keys, citing `avgBuyPctOfCL`, naming a sell-through %, restating a `coverageGaps` row — these are data, not analysis. An analyst adds interpretation:

- *"this number is misleading because of an outlier — dollar-weighted it's 4%, not 18%"*
- *"the segment looks strong but it's a single fill; the trailing N-week sample is empty"*
- *"the suggestion says terms-down but the pattern is CL-lead, so narrow-scope is the actual lever"*

Every parsed datum in user-facing output carries one short interpretive clause. Without it, drop the datum. Specific instance: the **Category vs campaign discipline** in Conversational guideline 4 — writing `Modern (C4) has been dark 12 days` reads as a category-level claim when the underlying datum is one-campaign-level. The discipline names the disambiguation requirement; 1.4 generalizes the move (every parsed datum needs interpretation that names what it actually is, not what it sounds like).

### 1.5 — Disagree with the data when the pattern says otherwise (D4)

The `/tuning` and `/portfolio/suggestions` outputs are surface conclusions over lifetime data. When the underlying pattern (CL-lead vs CL-lag, cap-binding vs supply-thin, post-restriction sample vs pre-restriction noise, popular-tier excluded for edge reasons) contradicts the surface conclusion, push back on the data and state which signal you trust and why. The **Cap-diagnostic rule** (both directions — supply-thinness AND cap-cut binding) and **Popular-tier exclusion** are existing precedents for this discipline; this rule generalizes the move so it fires beyond those specific cases. Never recommend a lever just because the endpoint named it — recommend it because the pattern *and* the lever-binds check both support it.

### 1.6 — Diagnose-the-gap when verification can't complete (extension of D1; ties to Capability 4)

When the Verify-before-propose gate (1.1) can't finish a cross-check because the data isn't available — endpoint conflates two concepts, field is missing, sample is post-restriction-only, aggregation is mean-of-ratios with no dollar-weighted equivalent, segment is unseeded — don't just downgrade confidence and move on. Name the gap inline and propose a concrete fix, routed to the right destination by type:

| Gap type | Surface | Route to |
|----------|---------|----------|
| **Internal / client-side** (missing endpoint field, scheduler not seeding, query reading wrong table, response shape needs splitting `realized` vs `contract`) | Inline in the response: *"Couldn't verify lever-binds for X because /tuning conflates realized buy% with contract terms — proposing a `contractBuyPct` field on the byGrade rows so this check is one-shot."* | Append to **internal-work** table in `docs/private/campaign-analysis-wishlist.md` at the moment it's encountered, not Step 6 |
| **Partner-side** (DH or PSA endpoint missing data they could provide) | Inline + draft a question for the operator to send. Apply the Partner-ask verification rule (three-question check for local-side cause first) | Append to dated `docs/private/YYYY-MM-DD-<partner>-data-ask.md` only after the Partner-ask verification rule passes (including step 0 impossibility cross-check) |
| **Impossible** (matches a row in `impossible-data-asks.md` by substance) | Inline: *"Can't verify segment competition pressure — that's the PSA supply-side data we logged as impossible 2026-04-14. Falling back to CL movement on filling segments as a proxy per the alternative."* Stamp `Last revisited` on the matched row | No new entry; no new draft |

The discipline this enforces: bad recommendations driven by data limitations should produce visible artifacts (a wishlist entry, a partner-ask draft, or a logged-and-routed impossibility) every time the limitation bites, not occasionally when the retrospective remembers. The existing Step 6 bucket-3 wishlist append still runs at session close — but inline surfacing during analysis means the gap-to-fix path is faster and the operator sees *which* analysis the gap broke.

When 1.1 verification *succeeds* but turns up a fragility (e.g. cross-check passed only because of one outlier; sample is healthy but post-restriction-only and small), surface that as a confidence note inside the recommendation, not as a wishlist entry — the data isn't broken, it's just thin. Wishlist additions are reserved for gaps that would change the recommendation if closed.

### 1.7 — Name the population the sample represents (selection bias)

Before any aggregation across campaigns or characters, name what the sample actually is. The operator's purchase history is the operator's *behavior*, not the *market* — External campaign reflects what they bought ad-hoc, not what's available to buy. Post-restriction-only samples reflect post-restriction behavior, not the underlying segment. Popular-tier character samples reflect what survived contested bidding, not what the operator's edge could capture. State the selection in one clause whenever the population isn't "all market activity":

> *"External shows Houndoom at 19% ROI, but External is the operator's catch-all — it confirms Houndoom is sellable when bought ad-hoc, not that there's untapped market demand for it."*

For market-demand questions, source signal from `/intelligence/niches` and `/insights.coverageGaps`, not operator purchase history.

This rule generalizes — it's not just External. Selection bias is the connective failure mode behind several already-named ones (popular-tier exclusion, post-restriction sample caveats, Step 1b currentScope filter); 1.7 makes the underlying discipline explicit so it fires when a new selection-bias trap appears.

### Step 3c and Step 4 pointers

Add to the **Conditional actions** paragraph in Step 3c, after the Recommendation-rules cross-reference: *"Walk the Self-challenge rules (1.1–1.7) over every mover and action before the response goes out."*

Add to **Step 4** routing preface, before the Playbook A trigger phrases: *"Each playbook response also walks the Self-challenge rules (1.1–1.7) before going out."*

## Section 2 — Playbook H: Portfolio Architecture Review

New playbook in `references/playbooks.md` between Playbook G and Step 5. Routing entry added to the contents list.

**Trigger phrases:** *"review our portfolio shape", "what bigger moves should we consider", "what should we restructure", "are these the right campaigns", "portfolio architecture", "prune the portfolio".*

**Boundary vs Playbook A and F** (state at the top of the playbook):

- Playbook A tunes parameters *within* fixed campaign structure.
- Playbook F proposes *one* new campaign for *one* coverage gap.
- Playbook H questions the entire portfolio shape and proposes merges, splits, scope changes, deprecations, AND new campaigns as a coherent restructure.

**Fetch in parallel** (most should be in opener cache):

- `GET /api/campaigns` — config (year, grade, price, inclusion, phase, `updatedAt` for restriction-age proxy)
- `GET /api/portfolio/snapshot` — composite (health, insights, weekly-history)
- `GET /api/campaigns/{id}/pnl` ×N — per-campaign profit + total spend (used for profit-per-deployed-dollar)
- `GET /api/campaigns/{id}/pnl-by-channel` ×N — channel mix per campaign
- `GET /api/portfolio/insights` — `byCharacter`, `byCharacterGrade`
- `GET /api/portfolio/weekly-history?weeks=8` — trailing-mean baselines + fills-in-trailing-30d
- `GET /api/intelligence/niches?window=30d&limit=20` — coverage-gap demand signal

Per-campaign fetches in parallel, not sequential.

**Apply Step 1b currentScope filter** before any cross-campaign aggregation. Restate the filter outcomes inline.

**Approach — six aggregations, one restructure block:**

1. **Concentration analysis.** Total profit across active campaigns; cumulative % held by top 3. Same for deployed capital. Output: *"Top 3 by profit: Vintage Core (C1) 38%, Vintage-EX (C2) 22%, EX/e-Reader (C3) 18% = 78%. Bottom 4 combined: 6%. Top 3 by deployed capital: C1 41%, C3 19%, C2 14% = 74%."* Apply 1.4 (describe → analyze) — interpret the spread, don't just print it.

2. **Per-campaign profit-per-deployed-dollar.** `pnl.netProfitCents / pnl.totalSpendCents` for each active campaign. Rank ascending, flag the bottom quartile. Each flagged campaign gets one diagnosis line: low-volume-high-margin slot earning its capital, low-margin-high-volume slot whose footprint is justified by volume, or low-on-both deprecation candidate.

3. **Long-tail audit.** Campaigns with < 3 fills in trailing 30d (compute from `weekly-history` × 4 weeks, cross-check against `inventory.createdAt`). For each: deprecation candidate, supply-thin candidate (apply Cap-diagnostic rule before concluding), or recently-launched-still-ramping (skip if `updatedAt` < 30d). Don't propose deprecation on supply-thin without checking the cap math.

4. **Coverage-gap audit (market-demand-driven, not External-driven).** Source candidate segments from `/intelligence/niches` (high `opportunity_score` AND `current_coverage = 0`) and `/portfolio/insights.coverageGaps`. Apply 1.7 — operator purchase history is not a market signal. External fills can *confirm* a candidate (the operator already touched the segment and it sold), but cannot *originate* one. For each surviving candidate:
   - Filter against existing inclusion lists across all campaigns to confirm it's genuinely uncovered
   - Apply 1.5 popular-tier exclusion + narrow-pocket exception
   - Size expected revenue at a proposed daily cap with a confidence band
   - Surface as a focused inclusion-list add (cheap, fits an existing campaign), or a new focused campaign (Playbook F coupling)

   When `/intelligence/niches` returns 0 rows (known intermittent state — see API footguns), surface that data gap inline per 1.6 and route to the wishlist; don't substitute External as a fallback signal.

5. **Restriction half-life.** For every active campaign with a grade restriction or era restriction, check `updatedAt` from `/api/campaigns` as the proxy for when the restriction landed. Cross-reference the strategy doc's "Changes Submitted YYYY-MM-DD" sections for the actual restriction date when the API proxy is wrong. Any restriction held > 30 days gets one of three verdicts:
   - **Still binding** — the segment the restriction excluded would still hurt portfolio ROI today (cite the dollar-weighted impact)
   - **Loose** — the original loss pattern is no longer in the data; consider widening the scope
   - **Indeterminate** — sample is post-restriction-only; can't tell. Apply 1.6 — surface as a fixable internal gap (e.g. "would need a `restriction_history` table to compare pre/post-restriction performance")

6. **Scope critique.** For each active campaign, apply the lens *"is this cut the right cut?"* Examples to look for: a Modern campaign restricted to PSA 8 where the live data shows PSA 9–10 with a sub-$150 floor would be the better cut; a Vintage campaign whose price floor excludes a profitable mid-tier segment; a campaign that should be split into two by era; two campaigns that should be merged because their fills overlap. State the alternative cut concretely, not abstractly.

**Restructure proposals (output block).** Synthesize the six aggregations into a numbered list of proposed restructures. Each item carries:

- **Move type:** deprecate / scope change / split / merge / new (Playbook F coupling) / inclusion expansion
- **Affected campaigns** by name + canonical number
- **Sized expected impact:** `est. +$X.XK/mo at current fill (Confidence: H|M|L)` per the existing Sizing rule. Capital-positive deprecations and scope-narrowings can be sized as recovery (`est. +$X.XK recovery within N days`)
- **Capital guardrail check:** apply the existing rule. Net-deploy moves under tight/critical posture get caveated or blocked
- **Sequencing:** if proposals interact, end with a Sequence block per the existing rule

**Self-challenge rules pass.** Walk Section 1's 1.1–1.7 over the restructure proposals before they go out. Most relevant: 1.1 verify-before-propose (does the long-tail diagnosis hold under aggregation across all campaigns in the category?), 1.5 disagree-with-data (the bottom-quartile-by-dollar may include a high-margin specialty slot the data underrates), 1.6 diagnose-the-gap (the restriction half-life check often hits indeterminate sample — surface and route).

**Mutations.** Same as Playbooks A and F. Deprecation = `PUT /api/campaigns/{id}` with `phase: "pending"`. Scope change = `PUT /api/campaigns/{id}` with new fields. New campaign = `POST /api/campaigns`. Apply only on user approval; never silent.

**Worked example.** A 6-aggregation walkthrough on a 6-campaign portfolio, ending with three restructure proposals (one deprecation, one scope-narrow, one external-confirmed → focused inclusion add), sequenced. Implementation will draft the example in the same shape as Playbook A's worked example, not a separate format.

## Section 3 — Impossible-asks log

### 3.1 — File: `docs/private/impossible-data-asks.md`

Gitignored, follows the existing `docs/private/` pattern. **Skill never creates this file silently.** First creation happens only when the user says "log this as impossible" (or equivalent) on a draft partner-ask — at that point the skill writes the header and the first row from the user-supplied details. Until then, the file simply doesn't exist and the cross-check is a no-op (with a note in the response that the impossibility filter is inactive).

Schema (locked):

```markdown
# Impossible Data Asks

Asks that are known to be unanswerable by the named partner — logged so the skill stops repeatedly proposing them. Cross-referenced before any partner-ask draft (see Partner-ask verification rule in playbooks.md).

| Ask | Why impossible | Alternatives | Logged | Last revisited |
|-----|----------------|--------------|--------|-----------------|
| competitor PSA Partner Offers buy% | PSA won't share supply-side competition data — privacy / competitive-landscape reasons | watch CL movement on filling segments as a proxy for competitor pressure; track our own fill-rate-near-cap vs supply-thin via the Cap-diagnostic rule | 2026-05-05 | — |
```

The schema documentation also lives in `references/playbooks.md` under the Partner-ask verification rule, so users editing the file by hand have the format reference next to the rule that uses it.

This PR does NOT create the file. It will be created at runtime on first use.

### 3.2 — Step 0 wiring in `SKILL.md`

Add to the file-load list at Step 0:

> Read `docs/private/impossible-data-asks.md` if it exists. Hold its contents in working memory for the rest of the session — every partner-ask draft cross-references this list before drafting. If the file is absent, partner-asks proceed without an impossibility filter; surface this in any retrospective draft so the user knows.

The Step 0 file-load order becomes: operator config → strategy doc → impossible-asks log. The third entry is optional like the strategy doc — missing file means continue, not stop.

### 3.3 — Partner-ask verification rule update in `references/playbooks.md`

Insert a new step 0 *before* the existing three-question check, in the rule body:

> **0. Cross-check against `docs/private/impossible-data-asks.md`.** Match the proposed ask against the log by *substance, not exact wording* — the noun phrase should match (e.g. "competitor buy%", "competitor pricing on Partner Offers", "what other PSA buyers pay" all match the same row). If matched: don't draft the ask; surface the match inline (*"Can't draft this — logged 2026-05-05 as impossible because PSA won't share supply-side competition data. Falling back to CL movement on filling segments as a proxy per the alternative."*) and stamp today's date in the matched row's `Last revisited` column. If unmatched, proceed to the existing three-question check below.

The existing three-question check (scheduler / related field / partner docs) becomes steps 1–3 of the rule, unchanged. Total rule body now: 0 → 1 → 2 → 3.

Add at the end of the rule:

> **Logging an ask as newly impossible.** When a draft ask is reviewed and the user determines the partner won't answer it (privacy, competitive reasons, structural infeasibility), the skill appends a row to `docs/private/impossible-data-asks.md` with the user-supplied "Why impossible" and any alternatives discussed. Append, never overwrite. If the file is absent, create it with the header from the schema documentation above and the first row.

### 3.4 — Step 6 retrospective touch-up

The retrospective's "DH-side asks" bucket (item 2) gains a one-line note:

> Before logging any item to a dated `docs/private/YYYY-MM-DD-<partner>-data-ask.md` file, run the Partner-ask verification rule's step 0 against the impossible-asks log. Drop any items that match.

No structural change to Step 6, just the cross-check pointer.

### 3.5 — How 1.6 connects

Section 1 sub-rule 1.6's `Impossible` row in its routing table cross-references the same Partner-ask verification rule's step 0. So a gap encountered mid-analysis hits the same impossibility filter as one encountered in retrospective. One source of truth, two entry points.

## Section 4 — Step 3c soft-flag wiring for Playbook H

In `SKILL.md` Step 3c, after the **Close** paragraph (the one ending *"Want me to dig into the C3 sell-through jump..."*), insert a new paragraph:

> **Portfolio-shape soft-flag.** After the close question, run the four Playbook H trigger conditions against the data already fetched. If any fire, append one line: *"Portfolio shape note: top 3 hold N% of profit, bottom Q quartile produced X% / N campaigns had <3 fills in 30d / N grade-or-era restrictions held >60d uncontested. Want a Playbook H pass?"* Only the firing conditions appear in the line — don't list non-firing ones. If none fire, omit the line entirely. The line is a question, not a script change — it never auto-runs Playbook H.

The four conditions:

1. Top-3 campaign concentration > 70% of total profit AND bottom quartile producing < 5%
2. ≥1 campaign with 0 fills in trailing 30d (deprecation candidate)
3. External campaign has a character/segment with `soldCount ≥ 10 AND roi ≥ 0.20` not covered by any focused campaign — *but* the soft-flag only fires when the same character also appears in `/intelligence/niches` or `/insights.coverageGaps` (apply 1.7 — External alone is not signal)
4. ≥1 grade or era restriction held > 60 days that hasn't been re-justified this session

Update the Step 3c examples to show one with the soft-flag firing, one without — so the model has both shapes in context.

The opener-format constraint is preserved: the soft-flag is one optional appended line, not a restructure of the opener. Skipped silently when conditions don't fire.

## Section 5 — `evals/known-failures.md` updates

Append six new entries, each in the existing entry shape (Date, Scenario, Failure, Corrective rule, SKILL.md anchor, Regression check). All dated 2026-05-05; scenarios grounded in the source-session JSONL at `/home/vscode/.claude/projects/-workspace/8951aaf1-df9d-4fe6-aac1-b9ed0decaa89.jsonl`.

1. **D1 — Stops at first plausible answer.**
   - Anchor (general): `SKILL.md` Self-challenge rule 1.1
   - Anchor (specific instances, dual-reference): `references/playbooks.md` Cap-diagnostic rule cap-cut binding check + `SKILL.md` API footguns dollar-weighted BPCL cross-check + `SKILL.md` Conversational guideline 4 Category vs campaign discipline + `references/playbooks.md` Era-fit gate + `references/playbooks.md` Throttle lever selection + `references/playbooks.md` Fill-drought hypothesis ranking
   - Regression check: Does Self-challenge rule 1.1 still mandate cross-check-by-claim-type with category aggregation, dollar-weighted vs mean-of-ratios divergence, lever-binds sanity check, AND hypothesis-set ranking? Does it still cite the six existing specific instances as precedents that the rule generalizes?

2. **D2 — Defers to server suggestions verbatim.**
   - Anchor (general): `SKILL.md` Self-challenge rule 1.3
   - Anchor (specific instance, dual-reference): `references/playbooks.md` Era-fit gate (echoed `/snapshot.suggestions` "Add top performers" carve-out)
   - Regression check: Does Self-challenge rule 1.3 still require server suggestions to be reframed or dropped, never echoed verbatim, on top of the existing stale-suggestion filter and Step 1b currentScope filter? Does it cite the Era-fit gate's echoed-suggestion carve-out as the specific precedent?

3. **D3 — Conflates describing with analyzing.**
   - Anchor (general): `SKILL.md` Self-challenge rule 1.4
   - Anchor (specific instance, dual-reference): `SKILL.md` Conversational guideline 4 Category vs campaign discipline
   - Regression check: Does Self-challenge rule 1.4 still require an interpretive clause attached to every parsed datum in user-facing output, drop the datum when the interpretation isn't there, AND cite the Category vs campaign discipline as the specific precedent?

4. **D4 — Afraid to disagree with the data when pattern says otherwise.**
   - Anchor (general): `SKILL.md` Self-challenge rule 1.5
   - Anchor (specific instances, dual-reference): `references/playbooks.md` Cap-diagnostic rule (both directions: supply-thinness check AND cap-cut binding check) + `references/playbooks.md` Popular-tier exclusion
   - Regression check: Does Self-challenge rule 1.5 still generalize the Cap-diagnostic (both directions) and Popular-tier-exclusion precedents into a broader push-back-on-data discipline?

5. **S1 — Treats existing N campaigns as fixed universe.**
   - Anchor: `references/playbooks.md` Playbook H + `SKILL.md` Step 3c soft-flag
   - Regression check: Does Playbook H still cover all six aggregations (concentration, profit-per-deployed-dollar, long-tail, coverage-gap demand-driven, restriction half-life, scope critique) and produce sized restructure proposals? Does the Step 3c soft-flag fire on any of the four conditions?

6. **Repeated proposal of impossible data asks.**
   - Anchor: `SKILL.md` Step 0 + `references/playbooks.md` Partner-ask verification rule step 0
   - Regression check: Does Step 0 still load `docs/private/impossible-data-asks.md` if present? Does the Partner-ask verification rule still run a step 0 fuzzy-match cross-check before the existing three-question check, and stamp `Last revisited` on matches?

The "How to use this list" footer at the bottom of `known-failures.md` doesn't change.

## Open questions

None. All design decisions resolved during brainstorming:

- Q1 (where verify/pushback rules live) → option (c): named section in `SKILL.md` with pointers from Step 3c and Step 4.
- Q2 (Playbook H trigger semantics) → option (b): on-demand + soft-flag from tactical openers when four threshold conditions fire.
- Q3 (impossible-asks log schema/location/cadence) → schema with `Last revisited` column, file at `docs/private/impossible-data-asks.md`, fuzzy-match by substance, read at Step 0, never silently written.

## Coordination with origin/main rules

After two rebases onto `origin/main`, eight specific Recommendation rules have landed since this spec was first drafted (commits `7a8a9c28` and `c99e6669`). All eight came out of the same source session that motivated this spec. Seven of the eight are concrete instances of the patterns the Self-challenge rules generalize:

| Specific rule (origin/main) | Generalized as | Implementation note |
|------------------------------|----------------|---------------------|
| Cap-cut binding check (Cap-diagnostic, inverse direction) | 1.1 lever-binds; 1.5 Cap-diagnostic precedent | Cite as named precedent in 1.1 |
| Dollar-weighted BPCL cross-check (API footguns) | 1.1 metric citations: dollar-weighted vs mean-of-ratios | Cite as named precedent in 1.1 |
| Category vs campaign discipline (Conv guideline 4) | 1.1 category claims; 1.4 describing-vs-analyzing | Cite in both 1.1 and 1.4 |
| Era-fit gate (Recommendation rules) | 1.1 lever-binds (era-fit on inclusion-add); 1.3 server-suggestion reframing | Cite in both 1.1 and 1.3 |
| Throttle lever selection (Recommendation rules) | 1.1 lever-binds (peer levers); 1.2 anticipate-pushback | Cite in both 1.1 and 1.2 |
| Fill-drought hypothesis ranking (Recommendation rules) | 1.1 hypothesis-set ranking | Cite in 1.1 |
| Business-mechanic premise gate (top-level `SKILL.md`) | Sibling to Self-challenge rules — different scope | Reference from Section 1 intro paragraph as a complementary layer |
| Playbook A default close = campaign list (Output structure) | No spec overlap | No coordination needed |

The Self-challenge rules are not redundant — they fire when the underlying pattern matches but no specific rule names it. They name the discipline so future failure modes get covered without requiring a new specific rule each time. The implementation must cite each origin/main rule as a named precedent inside the corresponding Self-challenge rule, so a reader walking the rules can find both the general discipline AND the specific threshold to apply when the pattern matches.

## Implementation note

This spec is for `superpowers:writing-plans` to consume next. The plan should sequence the edits to minimize merge friction (Self-challenge rules first since other sections reference them; Playbook H second; Step 3c soft-flag third; impossible-asks wiring fourth; eval updates last).

The skill files must remain coherent if any one section is reverted — i.e. the Self-challenge rules should reference impossible-asks log handling in 1.6 *with* a note that the log file may be absent, so the rule still works pre-Section-3. Same for Playbook H referencing 1.7 — the rule must exist before the playbook is added. The named precedents in 1.1, 1.2, 1.3, 1.4, 1.5 all reference rules that already exist on `origin/main` — no graceful-degradation needed.

Verification at PR time:
- `walking the regression checks` for the six new failure entries
- Reading the updated `SKILL.md` end-to-end to confirm pointers (Step 3c, Step 4, Step 0) all resolve
- Reading the updated `playbooks.md` end-to-end to confirm Playbook H boundary statements and the Partner-ask verification rule step 0 are coherent
- No code changes — Markdown only — so no test gate beyond manual readthrough
