# Lens Auditor Brief — Phase 2

Read `docs/audit/PREAMBLE.md` first. This file is additional and does not replace it.

You are the first agent in this audit that makes **judgments**. The five scouts
gathered data and deliberately refused to classify. You classify. That is why the
evidence rules below are stricter than theirs.

---

## 1. Where reachability comes from

Read `docs/audit/maps/*.json` FIRST. Reachability comes from those maps — **never
from your own search**. If a map lacks what you need, record that as a gap in your
summary rather than computing it yourself.

**The maps are large.** `go-reference-map.json` has 4,439 records;
`frontend-reference-map.json` 649; `db-map.json` 572. Do NOT `Read` these files
whole — you will exhaust your context before you have a single finding. Query them
with `jq`. Examples:

```bash
jq '[.records[] | select(.external_refs==0 and .name_ambiguous!=true)] | length' docs/audit/maps/go-reference-map.json
jq -r '.records[] | select(.kind=="column" and .external_refs==0) | .identity' docs/audit/maps/db-map.json
jq '.records[] | select(.identity=="…")' docs/audit/maps/go-reference-map.json
```

You may run targeted `git grep` to *verify* a specific candidate the maps
surfaced. You may not use your own search to *establish* that something is
unreferenced — that is the maps' job, and yours is bounded by what they cover.

---

## 2. What the maps do and do not prove

These caveats were declared by the scouts themselves. Violating them produces
confident tickets to delete live code.

| Caveat | Consequence for you |
|---|---|
| **1,622 Go records and 89 db records are `name_ambiguous: true`** | The identifier is declared more than once in the repo. `external_refs` is an **upper bound**, not a measurement. Never build a mechanical-tier finding on an ambiguous count in either direction. |
| **1,798 Go records have `external_refs == 0`** | This is **not** a dead-code list. It includes `TestXxx` functions invoked by the test runtime via reflection, and exported helpers used only inside their own package. |
| **Go map scope is exported symbols only** | Unexported identifiers and inline interface method sets are out of scope. Absent from the map ≠ absent from the repo. |
| **Go counting is textual, not type-checked** | Interface satisfaction and embedding-promoted methods are **not** detected by the map. Check them yourself per the preamble's nine mechanisms. |
| **Frontend: default exports re-bound at import** | Word-boundary matching cannot follow `import Whatever from './x'` where `x` default-exports `Something`. Any zero-ref finding on a default-exported symbol requires hand-checking its import sites. |
| **Index reachability is a permanent limitation** | Index names are query-planner hints, never Go string literals. An index's zero direct refs is evidence of **nothing**. Reachability is inferred only from the underlying table/column. |

---

## 3. Three traps found during Phase 1 verification

The preamble's nine mechanisms cover hidden references that make dead code look
live. These are the cases that bit us in practice — including one that nearly
scored a genuinely dead cluster as live.

1. **In-package caller.** `internal/platform/cache/cache.go:49` calls
   `NewFileCacheBackend`, which reads as a live consumer — until you check that
   nothing outside the package calls `cache.New`, the only function that reaches
   line 49. **An in-package caller of an in-package helper proves nothing about
   external reachability.** A package can be entirely dead while being internally
   busy.

2. **Same-name collision.** `api_rate_limits.calls_last_hour` greps non-zero, but
   every hit is the `api_usage_summary` *view's* own computed column of the same
   name, aggregating a different table. An unrelated same-name symbol makes dead
   code look live. Check that a hit refers to *your* subject.

3. **Stale comment ≠ dead object.** Four still-live tables carry stale `sqlite.`
   provenance comments identical in form to the one on the genuinely dead
   `marketmovers_config`. The comment being dead does not make the object dead.

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
- Baseline revision: `740976ecf80a4f2ccdaa611d7790ccaa95b48773`. Analyze
  git-tracked files at this revision only. `ls` and `find` are not evidence.

Before finishing, validate:

```bash
bash docs/audit/scripts/validate.sh findings <your-output-path>
```

Iterate until it prints GATE PASS. Never edit the validator or schema to pass.

**Return only:** the GATE line, your finding count broken down by confidence tier,
and any gaps. Do not paste findings into your reply.
