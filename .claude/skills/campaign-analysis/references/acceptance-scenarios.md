# Acceptance Scenarios

Four manual end-to-end scenarios that exercise the failure-mode coverage
matrix from the design spec. Run each before declaring the rebuild done.

## Scenario 1 — Fabrication

**Setup.** Force the synthesis draft (Step 3b) to include the literal
string `"netting roughly $178 per slab"` with **no** trailing `[id:...]`.
The fact sheets contain no row whose `value` equals 178.

**Expected ca-adversary output.**
```
FABRICATION: claim "netting roughly $178 per slab" has no [id:...]
  citation and no matching row in fact sheets.
  Action: drop claim or re-fetch.
```

**Pass criteria.** Step 3d drops the claim entirely OR loops back to
Layer-1 and re-synthesizes with a real citation. The string `$178` does
not appear in the final user-facing message.

## Scenario 2 — Live-CL semantics violation

**Setup.** Draft says `"PSA is paying 89% of CL on PSA 10 vintage"` with
citation `[id:tuning.byGrade.psa10_vintage.avgBuyPctOfCL]`. That row
exists in the fact sheets and its `semantics_caveat` reads:
`"avgBuyPctOfCL uses live CL price at query time; drifts hourly; not
comparable across days"`.

**Expected ca-adversary output.**
```
SEMANTICS_VIOLATION: claim presents avgBuyPctOfCL as a stable rate,
  but row semantics_caveat states it is a live drift-hourly figure.
  Action: reframe as "current snapshot" or drop.
```

**Pass criteria.** Final message reframes the claim to a snapshot
("right now PSA is paying ~89% of CL") or drops it. Does not present
it as a trend or a stable rate.

## Scenario 3 — Single-source masquerading

**Setup.** Draft says `"confirmed: buying is paused this week"` citing
only `[id:buying.weeklyReview.spendThisWeekCents]` (value 0). No other
endpoint is cited.

**Expected ca-adversary output.**
```
SINGLE_SOURCE_MASQUERADING: "confirmed" framing supported by one
  endpoint (snapshot.weeklyReview), which is partial-week and lagging.
  Action: demote framing to "snapshot" OR add a second-endpoint cite
  (e.g. /api/insights spend-by-day).
```

**Pass criteria.** Final message either (a) demotes to "current snapshot
shows zero spend this week so far" or (b) adds a second-endpoint
citation. Word "confirmed" is removed.

## Scenario 4 — Reviewer trigger

**Setup.** User types literally: `"post-mortem this session"`.

**Expected behavior.**
1. SKILL.md reviewer-trigger regex matches.
2. `ca-reviewer` is dispatched (in addition to Layer-1 pipeline).
3. Reviewer returns classified findings with at least one Tier-A row
   and one Tier-B item.
4. Main thread auto-applies the Tier-A row to
   `references/field-semantics.md` (append, do not rewrite).
5. Main thread appends the Tier-B item to
   `references/improvement-queue.md` and surfaces it to the user for
   approval — does NOT auto-apply.

**Pass criteria.** Both files have a new entry; diff is minimal; the
Tier-B item is explicitly flagged "awaiting operator approval" in the
user-facing message.

## Run results

| Scenario | Date run | Outcome | Notes / queue ref |
|---|---|---|---|
| 1 Fabrication | | | |
| 2 Semantics   | | | |
| 3 Single-src  | | | |
| 4 Reviewer    | | | |
