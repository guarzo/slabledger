# campaign-analysis — known failure modes

A regression checklist of documented failures the skill has corrected. Walk this list before committing future edits to `SKILL.md` or `references/playbooks.md` and confirm each rule's covering text is still reachable from the named anchor.

This is not an automated runner — the skill curls live endpoints and reads a private strategy doc, so eval fixtures would diverge from production state. The list is human-read, captured from session feedback memory notes.

---

## Failure: C10 status doc-vs-API mismatch

- **Date:** 2026-04-26
- **Scenario:** User asked for a campaign-analysis opener. Strategy doc said "C10 paused in app." Live API said `phase: active`.
- **Failure:** Skill trusted the doc's current-state claim. Compounded by reasserting the doc-side claim after the user pushed back.
- **Corrective rule:** API is ground truth on present-tense status (paused, archived, removed, active). Surface the doc as a cleanup candidate when they disagree. Do not re-anchor on the doc later in the same session after a correction.
- **SKILL.md anchor:** Step 1 addendum, item 2
- **Regression check:** Does Step 1 addendum still distinguish present-tense status claims from proposed-change claims, with the API-as-ground-truth rule on present-tense?

---

## Failure: missed inclusion-list mismatch (18 vs 34 chars)

- **Date:** 2026-04-26
- **Scenario:** Vintage Core / Vintage-EX PSA 8 Precision had 18 characters in the live app but 34 in the 4/24 PSA submission text the operator pasted mid-session.
- **Failure:** Skill eyeballed the lists rather than diffing them. Mismatch sat invisible for the entire opener.
- **Corrective rule:** For every campaign with an inclusion list in either source, compute the symmetric diff. Surface any nonempty diff in the data-quality block before drafting movers, with the specific characters listed.
- **SKILL.md anchor:** Step 1a, "Inclusion-list diff" block
- **Regression check:** Does Step 1a still mandate a programmatic symmetric diff (not eyeballing)?

---

## Failure: $0 in-hand misdiagnosed as data-pipeline gap

- **Date:** 2026-04-26
- **Scenario:** `inHandCapitalCents == 0` portfolio-wide. Real cause: every received card had sold; remaining unsold inventory was all PSA-side in-transit.
- **Failure:** Skill assumed broken data and started working around it.
- **Corrective rule:** Ask the user before treating `$0 in-hand` as broken data. Real business state ("everything received has sold") is a common interpretation.
- **SKILL.md anchor:** API footguns block
- **Regression check:** Does the API footguns paragraph for `inHandCapitalCents == 0` still tell the model to ask before assuming a pipeline gap?

---

## Failure: phase:pending flagged as drift

- **Date:** 2026-04-26
- **Scenario:** Retired campaigns have `phase: "pending"` in the API. Strategy doc says "removed."
- **Failure:** Skill flagged the doc-vs-API disagreement as drift requiring a Playbook D mismatch.
- **Corrective rule:** `phase: "pending"` is the soft-delete state — Card Yeti preserves purchase history rather than hard-deleting, so referential integrity on past purchases doesn't break. When the doc says removed/deleted, `pending` is the expected API state, not a mismatch.
- **SKILL.md anchor:** API footguns block
- **Regression check:** Does the API footguns paragraph still treat `phase: "pending"` as soft-delete and rule out the false-positive mismatch?

---

## Failure: bare campaign numbers as UX failure

- **Date:** 2026-04-26
- **Scenario:** Skill referred to campaigns as "C1", "C7", "C11" without ever resolving to names. User had to ask "what is C11?"
- **Failure:** Internal jargon leaked into the user-facing output.
- **Corrective rule:** On every first reference in a turn, write the full name with the number in parentheses: "Vintage Core (C1)", "Vintage-EX PSA 8 Precision (C11)". Use names in tables and bullet leads. When the user asks "what is C#?" — that's a signal of over-reliance; correct course immediately.
- **SKILL.md anchor:** Conversational guidelines, item 4
- **Regression check:** Does conversational guideline 4 still require Name (C#) format on first reference, with explicit fix-on-pushback rule?

---

## Failure: avgBuyPctOfCL mis-framing as buy terms

- **Date:** ~2026-04-17
- **Scenario:** Asked about ramping during max-float window. Skill cited tuning endpoint's `avgBuyPctOfCL` as if it were contract buy terms.
- **Failure:** "i don't have anything at a buy % of over 90, i have no idea what you are talking about" — the operator caught it. `avgBuyPctOfCL` is realized cost ÷ current CL value, a CL-drift indicator, not contract terms.
- **Corrective rule:** Never phrase `avgBuyPctOfCL` as "your buy terms" — say "realized cost is X% of current CL" or "CL has drifted Y pp since purchase". When `avgBuyPctOfCL > contract terms` and ROI is low, the segment is a CL-lead (CL above market, correcting) — narrow scope, don't cut terms.
- **Anchor:** `references/playbooks.md` — Data conventions, "CL-lag vs. CL-lead framing"
- **Regression check:** Does the Data conventions section still distinguish realized-cost-vs-CL from contract terms, and does it call out the CL-lag (edge captured) vs CL-lead (edge lost) framing with `avgBuyPctOfCL` thresholds?

---

## Failure: popular-tier add to C1 (Mew/Snorlax/Eevee)

- **Date:** ~2026-04-17
- **Scenario:** Asked what to add to C1 (Vintage Core).
- **Failure:** Skill recommended adding Mew, Snorlax, Eevee. Mew was intentionally excluded ("Ancient Mew flood"). Popular-tier characters are contested; the operator's edge isn't there.
- **Corrective rule:** Never default-recommend Charizard, Pikachu, Blastoise, Venusaur, Mewtwo, Mew, Umbreon, Eevee, Lugia, Ho-Oh, Gengar, Rayquaza. Narrow-pocket exception only — a specific (character, grade, era) combination from the popular tier is recommendable if `byCharacterGrade` shows `avgBuyPctOfCL ≤ 0.80 AND roi ≥ 0.20 AND soldCount ≥ 3`.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Popular-tier character exclusion"
- **Regression check:** Does the popular-tier exclusion list still ban default recommendations for those 12 characters, with the narrow-pocket exception spelled out?

---

## Failure: cap-diagnostic miss on C7

- **Date:** ~2026-04-17
- **Scenario:** C7 (Crystal/HGSS) had 20% multi-day cap utilization on a $5K daily cap. Crystal cards land $3K-$7K per fill.
- **Failure:** Skill recommended lowering C7's price floor based on low utilization, interpreting it as "supply is thin."
- **Corrective rule:** Before interpreting low cap utilization as supply-constrained, check `cap vs 3 × expected per-fill cost`. When `cap < 3 × expected per-fill cost`, the cap is binding on spike days and a single fill eats most of it — recommend cap raise before concluding supply-constrained.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Cap-diagnostic rule"
- **Regression check:** Does the Cap-diagnostic rule still require the 3× per-fill check before flagging supply-thinness?

---

## Failure: partner-ask false alarms (2026-04-17 DH draft)

- **Date:** 2026-04-17
- **Scenario:** Drafting a data-ask to DH based on `intelligence_count: 0` and "no pop data" from `/dh/status`.
- **Failure:** First draft included both as DH-side gaps. Both were actually local pull bugs — the scheduler only refreshed existing rows, nothing seeded the table; pop data lives in `market_intelligence.population` not `dh/status`.
- **Corrective rule:** Before drafting a partner data-ask, verify the gap isn't on our side: (1) check whether a scheduler/seeder should be populating the field, (2) check whether the data is in a related table we're not surfacing, (3) check whether the partner's documented API already returns this. Only items that fail all three checks go into a partner-ask.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Partner-ask verification"
- **Regression check:** Does the Partner-ask verification rule still require the three-question check (scheduler / related field / partner docs) before filing a data-ask?

---

## Failure: era-fit miss on inclusion-list adds (Leafeon/Rayquaza on C1/C11)

- **Date:** 2026-05-05
- **Scenario:** User asked for parameter updates. Skill drew character-add candidates from `/snapshot.suggestions` "Add top performers" and `/insights.coverageGaps`, both of which sort by portfolio-wide ROI without filtering by era. Recommended Leafeon (first TCG card 2007) and Rayquaza (first TCG card 2003) for Vintage Core (C1, 1999–2003) and Vintage-EX PSA 8 Precision (C11, 1999–2007). Two `PUT /api/campaigns/{id}` mutations landed before the user caught the era mismatch.
- **Failure:** Era fit was treated as an implicit assumption. The actual high-ROI signal was modern alt-arts already caught by C4 / C10 open-net campaigns; `/insights.coverageGaps` flagged the characters as "uncovered" because open-net campaigns don't appear in inclusion-list coverage analysis.
- **Corrective rule:** Before any inclusion-list add (manual, suggestion-echoed, or coverage-gap row), verify the character's first-TCG-card year overlaps the campaign's `yearRange` using the inline generation reference table. For open-net campaigns, treat `coverageGaps` "not in any active campaign inclusion list" reasons as misleading — verify the character isn't already filling on an open-net campaign with positive ROI before treating it as a gap. When the signal seems era-mismatched, trace actual fills via `/api/inventory` `cardYear` / `setName` before drafting.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Era-fit gate (inclusion-list adds)"
- **Regression check:** Does Playbook A's "Inclusion-list adds/trims" item still require the Era-fit gate before any mutation, with the open-net false-positive carve-out called out explicitly?

---

## Failure: confabulated double-invoice-window math model

- **Date:** 2026-05-05
- **Scenario:** Mid-conversation about capital throttling, the skill claimed the 5/16 and 5/29 invoices were competing for one recovery window — a "double invoice window" math model — and built detailed throttle-plan sizing (week-1 vs week-2 spend bias, cap-cut sizing, terms-cut math) on top of it.
- **Failure:** The premise was invented mid-response, never sourced from the strategy doc or `/credit/invoices`. User caught it: *"i think you may have a bad assumption -- there is no double invoice window? regarding the structural changes, they're not changing the payment timing at this point"*. Skill response: *"You're right on both — I was making that up."* Bi-monthly invoices are independent — each gets its own 14-day window with full recovery rate between them.
- **Corrective rule:** Before multi-step financial / capital-cycle analysis, verify the underlying business mechanic against (1) explicit strategy-doc text, (2) `/credit/invoices` + `/credit/summary` cycle history, (3) ask the user. Refuse the "double invoice window," "compressed payment window," and "cycle-week effect" framings unless one of the three sources confirms them. When about to write *"because X interacts with Y in this way"* about the operator's business workflow, that's a premise — cite the source or stop.
- **Anchor:** `SKILL.md` — "Business-mechanic premise gate" section
- **Regression check:** Does `SKILL.md` still carry an explicit pre-analysis gate that requires sourcing for business mechanics before multi-step financial reasoning, with the "double invoice window" framing called out as a refused anti-pattern?

---

## Failure: cap reductions on non-binding caps (C6 / C8 in throttle plan)

- **Date:** 2026-05-05
- **Scenario:** Skill proposed cap cuts on Mid-Era (C6, $5K → $2K) and Gold Stars (C8, $8K → $5K) as part of a week-2 throttle plan. User asked for the actual savings: C6 had 3 of 15 observed days exceeding $2K (~$0–$1K saved over a 4-day pro-rated window); C8 had 0 of 4 days exceeding $5K (the 4/30 raise to $8K had never bound). Only C10 (14 of 18 days exceeding $2K, ~$3K saved over a 4-day pro-rated window) was a real cap-cut candidate.
- **Failure:** The Cap-diagnostic rule covered the inverse direction (low utilization ≠ supply-thin) but had no forward version — the skill proposed cap cuts without checking how observed daily spend distributed against the proposed new cap.
- **Corrective rule:** Before proposing a cap reduction, compute `excess = sum(max(0, spendUSD - proposedNewCap))` and `daysExceeded = count(days where spendUSD > proposedNewCap)` from `/campaigns/{id}/fill-rate`. If `excess < $500` over a 14-day window OR `daysExceeded < 25%`, the reduction is a no-op — skip it or pick a binding lever (price-floor raise, terms cut, revocation, pause). State the math inline when surfacing or rejecting the cap-cut.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Cap-diagnostic rule" → "Cap-cut binding check (inverse direction)"
- **Regression check:** Does the Cap-diagnostic rule still cover both directions (supply-thin claim AND cap-cut proposal), with the binding-check math required inline?

---

## Failure: avgBuyPctOfCL mean-of-ratios distortion (C10 cited at 99%)

- **Date:** 2026-05-05
- **Scenario:** Skill cited `/tuning`'s `avgBuyPctOfCL` of 99% on Modern PSA 10 (C10) as the headline metric and based an action on it. User pushed back ("I haven't seen that"). Real dollar-weighted BPCL computed from `/api/inventory` was 83% (totalCost $19,247 ÷ totalCL $23,243 across 30 unsold). The 99% was inflated by ~4 Japanese S-tier outliers (Wingull S, Wattrel S, etc. — per-card ratios 175%–365% driven by CL variant mismatches between Shiny and base versions).
- **Failure:** The data-conventions section already framed `avgBuyPctOfCL` as a CL-drift indicator vs contract terms, but did not flag the mean-of-ratios computation shape and did not require a dollar-weighted cross-check before citing high values as headline drivers.
- **Corrective rule:** Before citing `avgBuyPctOfCL ≥ 0.90` as a headline mover or driver of an action, fetch `/api/inventory` filtered to the campaign's unsold rows and compute `dollarWeightedBPCL = sum(buyCostCents) / sum(clValueCents)`. If dollar-weighted differs from `/tuning`'s mean-of-ratios by more than ~10pp, surface BOTH numbers and identify the top 5 outlier rows by per-card ratio. Dollar-weighted is the right number for "is the campaign systematically overpaying"; outliers are a separate CL-data-quality signal.
- **Anchor:** `SKILL.md` — API footguns block, "`avgBuyPctOfCL` is a mean of per-card ratios" bullet (cross-referenced from `references/playbooks.md` Data conventions)
- **Regression check:** Does the API footguns block still require a dollar-weighted cross-check from `/api/inventory` before any `avgBuyPctOfCL ≥ 0.90` claim drives an action, with the outlier-identification step called out?

---

## Failure: category claim from single-campaign data (Modern category vs C4)

- **Date:** 2026-05-05
- **Scenario:** Skill wrote *"Modern (C4) has been dark 12 days"* in an opener mover — phrasing that read as if the Modern *category* was dark. Modern PSA 10 (C10) was actively filling at ~$2.5K/d at the time; modern fills had shifted to PSA 10 after C4's 4/23 grade narrowing. The "dark" claim was true for the C4 campaign but false for the Modern category.
- **Failure:** The Conversational guideline 4 (Name (C#) format) disambiguated campaign references but did not address the category-vs-campaign overlap. Several Card Yeti campaigns share their category label with the category itself ("Modern (C4)" → Modern category, "Mid-Era (C6)" → Mid-Era category), making it easy to slip between the two.
- **Corrective rule:** Before any category-level statement, list the campaigns covering the category from canonical numbering + strategy doc, then state the campaign-by-campaign verdict explicitly. The `Name (C#)` format itself can mislead — disambiguate when a campaign shares its label with the category.
- **Anchor:** `SKILL.md` — Conversational guidelines item 4, "Category vs campaign discipline" subsection
- **Regression check:** Does Conversational guideline 4 still require category claims to be backed by aggregation across all campaigns covering the category, with the Modern (C4 + C10) case explicit as the canonical example?

---

## Failure: throttle defaulted to cap, ignored buy-terms lever

- **Date:** 2026-05-04
- **Scenario:** Operator asked to reduce week-2-of-cycle spending. Skill proposed lowering daily caps as the lever. Operator redirected: *"so, rather than a cap — perhaps a CL % change for c10 might be the better play?"* — pointing at Modern PSA 10 (C10) which was realizing 99% BPCL against a 75% contract.
- **Failure:** Skill picked one lever (cap) silently when two were available. Cap clips spike-day spend; terms shifts the entire fill distribution AND improves margin on residual fills. The two have distinct downside profiles and the operator needed both presented to choose.
- **Corrective rule:** Whenever a recommendation reduces spending on a campaign, present **both** cap reduction and buy-terms reduction as peer levers, with the explicit tradeoff: cap = clip-top (risk control), terms = shift-distribution + margin (intentional volume-kill, not margin recovery on a filling segment per CL-lag/CL-lead framing). Don't pick silently.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Throttle lever selection"
- **Regression check:** Does the Throttle lever selection rule still require both cap and terms be presented as peer levers in any spend-reduction proposal, with the cap-vs-terms tradeoff stated explicitly?

---

## Failure: equal-weight hypotheses on Modern fill drought

- **Date:** 2026-05-04
- **Scenario:** Operator asked about Modern (C4) and Modern PSA 10 (C10) drought hypotheses. Skill listed *competition / submission shift / cycle dip* as equal alternatives. Operator pushed back: *"there is no way to prove it either way, but i would strongly suspect we're getting hit with competition, rather than a supply issue"* — picked the ranking the skill should have provided.
- **Failure:** Skill defaulted to a flat menu of hypotheses with no ranking and no evidence-cited reasoning. Operator did the disambiguation work the skill should have done.
- **Corrective rule:** When a campaign or segment goes dark (fill rate dropped >25% WoW, sales stalled 2+ weeks, or zero recent fills on a previously-active segment), do **not** list hypotheses as equal-weight. Walk the four canonical hypotheses (competition / supply lull / cycle dip / inclusion-list mismatch), score each by the evidence present, present in ranked order with one-line reasoning per rank, and propose a discriminating next action. Equal-weight is acceptable only when evidence genuinely doesn't separate the hypotheses — and in that case, name the discriminator the skill or operator could check next.
- **Anchor:** `references/playbooks.md` — Recommendation rules, "Fill-drought hypothesis ranking"
- **Regression check:** Does the Fill-drought hypothesis ranking rule still require ranked presentation with evidence-cited reasoning, the four canonical hypotheses, and a discriminating next action — banning equal-weight menus when evidence supports a ranking?

---

## How to use this list

When making a skill edit:

1. Identify which failure modes the edited section covers.
2. After the edit, walk the regression-check question for each covered failure.
3. If the answer is "no" or "unclear", reconsider the edit — the rule may have been weakened.
4. New failure modes get a new entry: capture date, scenario, failure, corrective rule, anchor, regression check.
