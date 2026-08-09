# Lens Auditor Brief — Phase 2

Read `docs/audit/PREAMBLE.md` first. This file is additional and does not replace it.

This brief is shared across runs. Every run-specific number, path, and revision
lives in the Run Card appended to your dispatch — never here. Where this file
needs a count, it tells you the `jq` that produces it from your own run's maps.

You are the first agent in this audit that makes **judgments**. The five scouts
gathered data and deliberately refused to classify. You classify. That is why the
evidence rules below are stricter than theirs.

---

## 1. Where reachability comes from

Read the `maps/` directory of the run directory named in your Run Card FIRST.
Reachability comes from those maps — **never from your own search**. If a map
lacks what you need, record that as a gap in your summary rather than computing
it yourself.

**The maps are large** — the Go map alone runs to several megabytes. Do NOT
`Read` these files whole; you will exhaust your context before you have a single
finding. Query them with `jq`. Substitute your run's `maps/` path:

```bash
jq '.records_count' $MAPS/go-reference-map.json
jq '[.records[] | select(.external_refs==0 and .name_ambiguous!=true)] | length' $MAPS/go-reference-map.json
jq -r '.records[] | select(.kind=="column" and .external_refs==0) | .identity' $MAPS/db-map.json
jq '.records[] | select(.identity=="…")' $MAPS/go-reference-map.json
```

You may run targeted `git grep` to *verify* a specific candidate the maps
surfaced. You may not use your own search to *establish* that something is
unreferenced — that is the maps' job, and yours is bounded by what they cover.

---

## 2. What the maps do and do not prove

These caveats were declared by the scouts themselves. Violating them produces
confident tickets to delete live code. **Derive each count from your own run's
maps** — the `jq` for it is in the first column — and never carry a number
quoted from a previous run.

| Caveat (with the jq that sizes it) | Consequence for you |
|---|---|
| **Records with `name_ambiguous: true`**<br>`jq '[.records[]\|select(.name_ambiguous==true)]\|length' $MAPS/go-reference-map.json` (and the same on `db-map.json`) | The identifier is declared more than once in the repo. `external_refs` is an **upper bound**, not a measurement. Never build a mechanical-tier finding on an ambiguous count in either direction. |
| **Records with `external_refs == 0`**<br>`jq '[.records[]\|select(.external_refs==0)]\|length' $MAPS/go-reference-map.json` | This is **not** a dead-code list. It includes `TestXxx` functions invoked by the test runtime via reflection, exported helpers used only inside their own package, and symbols reachable only through the preamble's nine mechanisms. |
| **Go map scope is exported symbols only** | Unexported identifiers and inline interface method sets are out of scope. Absent from the map ≠ absent from the repo. |
| **Go counting is textual, not type-checked** | Interface satisfaction and embedding-promoted methods are **not** detected by the map. Check them yourself per the preamble's nine mechanisms. |
| **Selector names are counted** | A reference to `x.Foo` counts toward *every* top-level `Foo` in the repo. This inflates `external_refs` and is the mechanism behind most ambiguity. |
| **Frontend: default exports re-bound at import** | Word-boundary matching cannot follow `import Whatever from './x'` where `x` default-exports `Something`. Any zero-ref finding on a default-exported symbol requires hand-checking its import sites. |
| **Index reachability is a permanent limitation** | Index names are query-planner hints, never Go string literals. An index's zero direct refs is evidence of **nothing**. Reachability is inferred only from the underlying table/column. |

---

## 3. Three traps found during Phase 1 verification

The preamble's nine mechanisms cover hidden references that make dead code look
live. These are the cases that bit us in practice — including one that nearly
scored a genuinely dead cluster as live. **The citations illustrate the
*mechanism* and were captured at an earlier baseline; some no longer resolve at
your run's revision, and that is not itself a finding.** Learn the shape, not the
line number.

1. **In-package caller.** A package-internal call site reads as a live consumer —
   until you check whether anything *outside* the package reaches the function
   that contains it. **An in-package caller of an in-package helper proves
   nothing about external reachability.** A package can be entirely dead while
   being internally busy. (Illustrated at the 2026-08-07 baseline by
   `internal/platform/cache/`, whose `cache.go:49` called `NewFileCacheBackend`
   while nothing outside the package called the only function reaching it. That
   package no longer exists — which is what the trap predicts happens next.)

2. **Same-name collision.** `api_rate_limits.calls_last_hour` greps non-zero, but
   every hit is the `api_usage_summary` *view's* own computed column of the same
   name, aggregating a different table. An unrelated same-name symbol makes dead
   code look live. Check that a hit refers to *your* subject.

3. **Stale comment ≠ dead object.** Still-live tables carry stale `sqlite.`
   provenance comments identical in form to the one on a genuinely dead table.
   The comment being dead does not make the object dead.

---

## 4. Encoding: named fields, never prose

Phase 1 lost two rounds to this, so it is a hard rule.

Every qualifier a later phase must count — confidence, each of the nine
`runtime_checks`, ambiguity, exclusions — must be a **named field with an
enumerated domain**, exactly as `docs/audit/schema/finding.schema.json` defines.
Never encode a qualification as English inside another field, and never
contaminate `command` with commentary.

`command` must be a command a verifier can paste into a shell and re-run. If it
would produce a shell error, your evidence does not reproduce, and a Task 13
verifier will correctly refute your finding.

Why this is non-negotiable: a Phase 1 scout recorded verdicts as prose. Its own
two self-counts of its own file were 8 and 13. The real number was 19. It was not
being careless — nothing in the format forced the count to be countable.

---

## 5. Confidence tiers

- **`mechanical`** — requires ALL NINE runtime-reachability checks answered in
  `runtime_checks`, plus `build_tags` and `exclusions`. The gate rejects
  mechanical findings with fewer than nine. This is the **only** ticketable tier.
- **`strong`** — compelling, but one or more checks unresolved. Name which ones.
- **`suspected`** — recorded, not ticketed.

Assign these honestly. A `strong` finding that is genuinely strong is more
valuable to this audit than a `mechanical` one that cannot survive an adversarial
verifier — Task 13 dispatches agents whose explicit job is to **refute** your
findings, defaulting to refuted under uncertainty. Overclaiming does not get work
shipped; it gets your finding killed and wastes a verification round.

---

## 6. Scope discipline

You own **exactly one subject kind**. If you notice something owned by another
lens, emit a record with `category` set to the owning lens and confidence
`suspected` — a pointer, never a competing finding. Duplicate findings across
lenses have to be untangled by hand in Task 14.

---

## 7. Acceptance criteria

Every `acceptance_criteria` entry must state how a **fixer** proves the change
correct — not how you found it. For example: "`go build ./...` and
`go test ./...` pass with the symbol removed."

**The audit never performs removal.** You are strictly read-only.

---

## 8. Finding nothing is a valid result

Emit `[]` and describe, in your summary, the search that established it. A lens
that returns nothing after a real search has produced real information.

**Do not manufacture findings.** A non-empty finding set is not a success
criterion for this audit, and padding a thin lens with `suspected` noise imposes
real cost on Task 13 and Task 14.

---

## 9. Constraints

- Strictly **read-only**. Write exactly one file: your own findings JSON.
- Never modify anything under `internal/`, `cmd/`, `web/src/`, any documentation
  file, the maps, the schemas, or `validate.sh`.
- Do not commit. The controller commits.
- **Baseline revision: the one named in your Run Card.** Analyze git-tracked files
  at that revision only. `ls` and `find` are not evidence.

Before finishing, validate (paths and facts from your Run Card):

```bash
AUDIT_BASELINE_FACTS=<run-dir>/baseline.json \
  bash docs/audit/scripts/validate.sh findings <your-output-path>
```

Iterate until it prints GATE PASS. Never edit the validator or schema to pass.

**Return only:** the GATE line, your finding count broken down by confidence tier,
and any gaps. Do not paste findings into your reply.
