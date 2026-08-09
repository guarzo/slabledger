# Controller notes — run 2026-08-08

Carried forward into Phase 3 (verification) and Phase 4 (adjudication). Not an
agent artifact; nothing here is gated. Everything is controller-derived or
handed over by a lens in its completion report.

## Phase 2 lens outcomes

Three of the eight original lens agents appeared to have died without writing a
file: `dead-code-go`, `docs-config-tests`, `frontend-health`. All three were
re-dispatched with the preamble inlined verbatim.

**That diagnosis was only two-thirds right.** `dead-code-go` had not died — it was
slow, and wrote a complete file after the check that concluded otherwise. See the
write race below. `docs-config-tests` genuinely failed, confirmed as
`API Error: Response stalled mid-stream`. `frontend-health` never reported and its
status is unknown.

A missing file is NOT an emitted `[]` — the former is a lost lens, the latter is a
result — but a *momentarily* missing file is also not a dead agent, and this run
conflated the two.

Counts below were re-derived by the controller with `jq` from the files on disk.
No agent self-report was accepted as a tally.

### A write race the controller caused, and how it was resolved

`lens-dead-code-go` was believed dead and re-dispatched as `lens-dead-code-go-2`.
It was not dead — it wrote a complete, gated 6-record file *after* the check that
concluded it had produced nothing. Two agents were then live against one output
path, where the later writer silently destroys the earlier one's work.

Resolved by stopping `lens-dead-code-go-2` and keeping the original's file, which
was already complete and gated; the replacement was minutes into its work, so
stopping it lost strictly less. The file was re-gated after the stop (`GATE PASS`,
6 records) and checksummed `b3fc0ab6e6c55c49cb4337342624c711` so a later clobber
is detectable rather than invisible.

**`ListAgents` is not a reliable liveness check here.** It reported "No reachable
agents" at a moment when at least four agents were alive and subsequently
reported. Absence from that listing must not be treated as proof an agent is
gone. The same race was therefore open for `frontend-health` until its original
reported `API Error: Response stalled mid-stream` — a genuine failure, leaving the
replacement as the sole writer for that path. Final tally on the three suspected
deaths: two real stalls (`docs-config-tests`, `frontend-health`), one false alarm
(`dead-code-go`).

## Two Go-map limitations that bound every dead-code claim

Both were discovered by the dead-code lens and are NOT in the map's declared
caveats. They generalize beyond that lens.

1. **`name_ambiguous` cannot see stdlib collisions.** It reflects duplicate
   declarations *inside the repo* only. `mocks.WithTimeout` records
   `external_refs: 41, name_ambiguous: false` — and all 41 hits are
   `context.WithTimeout`. Any symbol sharing a name with a stdlib identifier
   reads as busy and is untrustworthy in both directions.
2. **The map has no field-type reachability.** `mocks.DHPushStatusCall` has zero
   external refs but is the element type of `Calls []DHPushStatusCall` inside a
   live type, so it is required for compilation. A zero-ref record can be
   structurally load-bearing.

**Phase 3 instruction:** DCG-005 carries an explicit exclusion for the second
case. A verifier must not "helpfully" fold the excluded symbol back in and then
refute the finding for including it — the exclusion is correct.

## Dedup leads for Phase 4

### `origin/main` advanced during the run — one commit past the baseline

At Phase 0 `origin/main` resolved to the pinned baseline `06d9f8ce`. By Phase 3
it had advanced to `9b3bdbe7`, *one* commit later:

```
9b3bdbe7 refactor(db): drop the dead dh_card_cache blob columns (SLA-65) (#625)
06d9f8ce  <- pinned baseline
```

**The baseline does not move.** The user's instruction was to resolve once and
pin, and every map, finding, and verdict in this run was derived against
`06d9f8ce`. Re-pinning mid-run would silently invalidate all of them.

But the controller must reconcile against the newer tip before filing, because a
finding that is true at the baseline and *already fixed one commit later* would
send a developer to redo completed work.

**DB-001 is exactly that case.** It claims five unwritten analytics columns on
`dh_card_cache` (`demand_json`, `velocity_json`, `trend_json`, `saturation_json`,
`price_distribution_json`). Migration `000036_drop_card_cache_blobs` in `9b3bdbe7`
drops precisely those five. The finding is *correct at the baseline* and
**must not be ticketed** — it is remediated, not a duplicate and not a regression.
Record it in `ADJUDICATIONS.md` as fixed post-baseline by SLA-65 / #625.

Two riders on that commit, for whoever adjudicates the neighbouring findings:

- It is scoped to `dh_card_cache` only. `dh_character_cache` carries
  similarly-named `*_json` columns that **are** still read and written and
  deliberately stay. If DB-001 swept those in as well, that portion is wrong on
  its own terms and the verifier should have caught it.
- It also edits `docs/SCHEMA.md` (documenting `character_name` and the
  sticky-COALESCE upsert semantics). Any DCT finding quoting `docs/SCHEMA.md`
  must be checked against the newer tip before filing, for the same reason.

No other finding in this run is touched by `9b3bdbe7`; it changes three files
total (`docs/SCHEMA.md` and the two halves of migration 000036).

### Every prior-run ticket is CLOSED — so reproductions are regressions

All 35 tickets from the 2026-08-07 run (`SLA-9` … `SLA-43`, per
`docs/audit/linear-ids.json`) are in a completed state in Linear. Not one is
still open.

Under the run's stated rule that makes the adjudication uniform: **reproduces +
ticket closed = regression, which IS ticketable and must be flagged as such.**
There is no "reproduces + still open = duplicate, do not re-file" case available
this run, because no prior ticket is open. Any prior-run overlap that survives
verification is therefore filed, labelled as a regression against its SLA id —
not silently suppressed.

A second wave of tickets exists that is **not** in the prior run's
`linear-ids.json`: `SLA-44` … `SLA-65`, filed between the two runs. These are
also closed, and several bear directly on this run's subject matter — `SLA-45`
(flat-sibling checker escape), `SLA-48` (leaf/non-leaf taxonomy), `SLA-46`
(CLAUDE.md end-to-end sweep), `SLA-56` (field-level `docs/API.md` audit),
`SLA-65` (above). Adjudication must check findings against this wave too, not
only against `linear-ids.json`; several of this run's architecture and docs
findings sit on ground SLA-45/46/48/56 already covered.

- **NAM-001 overlap: SETTLED — file it, it is neither duplicate nor regression.**
  Controller-resolved in Phase 4 (the lens correctly refused to open the file).
  The prior-run counterpart is `FE-008`, "Pointer: two SellSheetItem price fields
  carry cent values but their name (and JSON tag) omit the Cents suffix." It was
  a `suspected`-tier **pointer** and was **never ticketed** —
  `docs/audit/ADJUDICATIONS.md:377` records "FE-008 is `suspected`-tier —
  `ticketable: false`. Do not ticket it," and `:428` lists it among findings
  "correctly held back on exactly that rule." It appears in the prior `REPORT.md`
  (`:2009`) but has no entry in `linear-ids.json`.

  So neither branch of the dedup rule applies: there is no open ticket to be a
  duplicate of, and no closed ticket to have regressed against. NAM-001 is a
  **first filing**. It is also strictly broader — five *Go* fields
  (`TargetSellPrice`, `MinimumAcceptPrice`, `TotalCostBasis`,
  `TotalExpectedRevenue`, `TotalProjectedProfit` in
  `internal/domain/inventory/analytics_types.go`) against FE-008's two *TS*
  fields, i.e. this run reached the producing side the pointer only glimpsed
  downstream. Its ticket should note that the 2026-08-07 run observed the
  frontend half and held it back as a pointer.

- **NAM-001 (original lens report, superseded by the entry above).** The naming lens reports that NAM-001
  overlaps an observation in the 2026-08-07 run's
  `docs/audit/findings/frontend-health.json`. It saw a single line incidentally
  in grep output and did not open the file, per the Run Card prohibition on
  lenses reading prior runs. **The overlap is unverified and must be settled by
  the controller in Phase 4**, where reading prior runs is not only permitted
  but required. Recall the rule: reproduces + ticket open = duplicate, note the
  SLA id and do not re-file; reproduces + ticket closed = regression, which IS
  ticketable and must be flagged as such.

## Verification priorities for Phase 3

- **DUP-001 is the run's only `mechanical` finding** (severity `high`). Both
  verdict reversals in the previous run were in the mechanical tier. It gets the
  most adversarial verifier and the least benefit of the doubt.
- **DB-004 and DB-005 carry acceptance criteria satisfiable only against a live
  database** (`pg_stat_user_indexes`, `EXPLAIN`). The audit has no database
  access by construction. If they survive verification, their tickets must say
  plainly that proving the fix requires production access this audit never had —
  or the verifier should return `unresolvable`.
- **DUP-003 / the auth pointer.** The Go map cannot settle reachability for
  `internal/domain/auth.New`: `external_refs=280` but `name_ambiguous=true` with
  5 distinct `New` definitions. Any finding resting on that count is selector-name
  noise and must not reach `mechanical`.

## Rejected candidates — do not re-file

Recorded by the naming lens so a later phase does not spend the round re-deriving
them:

- `psacampaign.CampaignFormData` unsuffixed price fields — struct doc states
  "Prices here are whole USD to match the portal wire". Deliberate.
- `csvimport` `VariantPrice` / `UnitPrice` — documented dollars at the parse
  boundary. Deliberate.
- `dhlisting/psa_import_language.go` — correctly placed; maps to DH's language
  enum for DH's own endpoint.
- `AISuggestedPriceCents` — reads as an Azure-AI removal remnant, but
  `docs/superpowers/specs/2026-08-08-remove-azure-ai-design.md:51` records
  "keep the data, drop the producer" as the deliberate decision.

## Scope gaps to carry into the next run's scout briefs

- **`VERIFIER-BRIEF.md` promises a verdict token the gate rejects.** The brief
  offers `unresolvable` in three places (`:74`, `:84`, `:104`) and this run's
  DB-004/DB-005 dispatches explicitly invited it, but
  `docs/audit/schema/verdict.schema.json` enumerates exactly
  `confirmed | confirmed_lower_severity | refuted` with
  `additionalProperties: false`. A verifier who used it would have failed the
  gate and, under deadline, been pushed into a *wrong* verdict rather than an
  honest one. Surfaced by `verify-db-schema` and confirmed by the controller
  against the schema file. It never bit this run — both DB findings turned out
  decidable from the DDL alone — so "0 unresolvable" describes the findings, not
  a working escape hatch. Fix one side or the other before the next run; the
  brief is shared infrastructure and was deliberately not edited mid-run.

- **`db-map` record vocabulary excludes** triggers, functions, CHECK constraints,
  and GRANT/REVOKE state. None of it was audited this run.
- **Duplication detection was body-similarity based.** Duplication expressed as
  parallel SQL string literals, parallel column lists, or parallel type
  declarations is invisible to it — DUP-002 was found by reading, not by the
  scan. A structured SQL-fragment comparison is the next pass.
- **Go↔TS tag mirroring was spot-checked, not systematic.** One struct pair
  (`inventory.Sale` vs `web/src/types/campaigns/core.ts`) was checked and held.
  A systematic tag-vs-interface diff was never run.
- **`docs-map` coverage hole** — see the caveat in `RUN-CARD.md`. Five documents
  reported as covered produced zero records, and the scout's gate has no baseline
  count that could have caught it.

## Post-run repairs (2026-08-08, after filing)

Four generator/spec defects were fixed after SLA-66…SLA-97 were filed. All four
are in shared infrastructure under `docs/audit/`; nothing under `internal/`,
`cmd/` or `web/src/` was touched, so the run stayed read-only with respect to
the codebase it audited.

1. **`Subject:` rendered a Python dict repr.** Both generators interpolated the
   finding's `{kind, identity}` pair with `str()`, shipping
   `{'kind': 'table', 'identity': 'marketmovers_config'}` into REPORT.md and into
   every ticket of the 2026-08-07 and 2026-08-08 batches. Now a `subject()`
   helper in each script renders `` `identity` (kind) ``; the two copies are held
   in sync by `scripts/build-tickets-test.py`.
2. **Linear autolinked bare dotted tokens.** Found by round-tripping this batch:
   `CLAUDE.md:126`, `check-imports.sh`, `inventory.Sale` and
   `internal/domain/auth.New` all read as hostnames to Linear's autolinker, which
   attached a dead `http://` href to each. Label text survived, so nothing was
   lost — but it was on nearly every evidence line. `prose()` now wraps such
   tokens in code spans, which is also the correct markup.
3. **`unresolvable` was promised by the brief and rejected by the gate.** The
   defect logged above. Resolved on the schema side rather than by deleting the
   token: `refuted` asserts a finding is *wrong* and counts against the audit's
   error rate, while `unresolvable` asserts only that static analysis could not
   reach the claim — collapsing them would print "How it was refuted" above a
   claim nobody refuted. The enum now carries it, with `ticketable: false`
   enforced by both the schema and `validate.sh verdicts`, and the brief's soft
   "unless it stands on another leg" exception hardened to a rule.
4. **`linear-ids.json` shape mismatch.** This run wrote a document
   (`{baseline, team, tickets: [...]}`); `build-tickets.py` expected the
   2026-08-07 flat `{FU-xx: {id, url}}` map. Fed the document, its coverage check
   reported all 32 units missing and hard-exited — at the one moment a filed run
   is least recoverable. `load_linear_ids()` now normalizes both. The document
   shape is kept, because the flat one cannot record `not_filed`.

**This run's filed artifacts were deliberately not regenerated.** `TICKETS.md`,
`ticket-manifest.json` and `linear-ids.json` are the record of what was
transmitted to Linear, and `REPORT.md` is kept consistent with them. Fixes 1 and
2 are cosmetic and take effect on the next run; regeneration was verified against
a scratch copy instead (`changed lines: subject=38, code-span=195, other=0`). The
32 live Linear issues still carry the old rendering — updating them is a separate,
outward-facing action.
