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

## How to use this list

When making a skill edit:

1. Identify which failure modes the edited section covers.
2. After the edit, walk the regression-check question for each covered failure.
3. If the answer is "no" or "unclear", reconsider the edit — the rule may have been weakened.
4. New failure modes get a new entry: capture date, scenario, failure, corrective rule, anchor, regression check.
