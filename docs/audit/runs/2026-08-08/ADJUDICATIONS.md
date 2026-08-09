# Adjudications — audit run 2026-08-08

Controller-authored in Phase 4. Everything here is a controller decision made
*after* all 49 verdicts landed, using material the scouts, lenses, and verifiers
were forbidden to read: prior runs' reports, prior runs' `linear-ids.json`,
current Linear ticket state, and commits after the pinned baseline.

Baseline: `06d9f8ce172dada98f307f860e2e2985c9fb5ca6` (2026-08-08).
Prior run baseline: `740976ecf80a4f2ccdaa611d7790ccaa95b48773` (2026-08-07).

## Arithmetic, re-derived from the artifacts on disk

Not one number below is an agent's self-report. All were recomputed with
`jq`/Python over `findings/*.json` and `verdicts/*.json`.

| | |
|---|---|
| Findings | 49 (unique ids: 49) |
| Verdicts | 49 (unique ids: 49) |
| Unverified findings | 0 |
| Orphan verdicts | 0 |
| `confirmed` | 39 |
| `confirmed_lower_severity` | 9 |
| `refuted` | 1 |
| `unresolvable` | 0 |
| `evidence_reproduces: false` | 3 (DCT-004, DCT-007, DUP-001) |
| `ticketable: false` | 10 |
| **Ticketable set** (confirmed-or-lower AND `ticketable: true`) | **39** |
| Struck by adjudication | 1 (DB-001) |
| **Filed** | **38 findings → 32 ticket units** |

Correspondence between findings and verdicts is exact in both directions: every
finding has precisely one verdict, and no verdict names a finding that does not
exist. That check is the reason a stalled verifier could not have gone unnoticed.

## Verdict-driven exclusions — the ten `ticketable: false`

These are the verifiers' calls, not the controller's, and are recorded here so a
later run does not read their absence from `TICKETS.md` as an oversight.

| id | verdict | why it is not filed |
|---|---|---|
| `DCT-007` | **refuted** | The only refutation in the run. Its load-bearing claim was that `docs/polish-report.md` is an orphan, evidenced by a grep asserted to be empty. That command returns **five hits** in `.claude/skills/polish-all/SKILL.md`, which both creates and reads the file. An absence claim whose absence command is non-empty is refuted per VERIFIER-BRIEF. |
| `DCT-004` | confirmed_lower_severity | Three of its four evidence claims are wrong about scope. It asserted all six describe blocks in `visual-regression.spec.ts` are gated; the block at `:84` has no gate, so 2 of 8 screenshot assertions do run in CI and the committed baseline PNGs are live comparison targets, not orphans. What survives (6 of 8 assertions permanently skipped) is true but too diminished from what was filed to ticket as written. |
| `DB-006` | confirmed | True for one of the two tables it names. `mm_card_mappings` genuinely loses its RLS policy on a rollback through `000021.down`; `mm_sales_comps` never had RLS, so recreating it without one is a faithful reversal. A ticket carrying both would send a developer to fix a non-defect. |
| `ARCH-004` | confirmed | Pointer. Duplicates the `internal/README.md` material already filed as DCT-013 — see the fold below. |
| `FE-006` | confirmed | Pointer. Same `internal/README.md` material, reached from the frontend lens. Same fold. |
| `NAM-005` | confirmed | Duplicate of DCG-001 (`internal/domain/llmutil`), which the dead-code lens filed at `mechanical` — the stronger tier. Verified as an overlap rather than assumed. |
| `DCG-006` | confirmed | Real (the file `internal/testutil/mocks/dh_fusion.go` contains no fusion concept, and its contents are live), but its acceptance criteria bundle two unrelated objects: renaming that file, and correcting a stale comment in `internal/adapters/scheduler/inventory_refresh_test.go:342`. Not one change. **Note for whoever takes the mocks unit:** the rename sits in the same tree and is free to carry along; it is simply not worth a ticket line of its own. |
| `DCT-012` | confirmed | Pointer, and its dead-code framing is wrong on its first item: `/api/status/api-usage`'s handler **is** registered and live at `routes.go:43` behind `RequireAdmin`. What is left is two stale path strings, one in a doc comment and one in the SPA fallback HTML — subsumed by DCT-010's endpoint sweep. |
| `DUP-005` | confirmed | Filed at `suspected`, which LENS-BRIEF §5 defines as recorded-not-ticketed, and the verifier agreed on the merits: a fee-clamp divergence that no current call path can reach. |
| `NAM-004` | confirmed | Also `suspected`, also agreed. Unlike NAM-001 the unit is recorded at the declaration by an adjacent JSON tag; unlike NAM-002 the name is incomplete rather than false. |

## Controller exclusion — remediated after the baseline

**DB-001 is correct at the baseline and must not be ticketed.**

This is a third category the run's stated dedup rule does not cover: not a
duplicate (no open ticket), not a regression (no closed ticket it re-breaks), but
*already fixed one commit past the pinned revision*.

`origin/main` advanced from the baseline to `9b3bdbe7` during the run
(`refactor(db): drop the dead dh_card_cache blob columns (SLA-65) (#625)`). The
baseline did not move — re-pinning mid-run would have invalidated every map,
finding, and verdict derived against `06d9f8ce`. But filing DB-001 would send a
developer to redo finished work.

Verified on both legs, not inferred from the commit subject:

- Migration `000036_drop_card_cache_blobs` drops exactly the five columns DB-001
  names (`demand_json`, `velocity_json`, `trend_json`, `saturation_json`,
  `price_distribution_json` on `dh_card_cache`).
- `git show 9b3bdbe7 -- docs/SCHEMA.md` deletes all five rows, each annotated
  "**Dead since SLA-41** — no longer read or written."

Two riders checked so the exclusion does not over-reach:

- The commit is scoped to `dh_card_cache`. `dh_character_cache` carries
  similarly-named `*_json` columns that are still read and written and
  deliberately stay.
- It touches three files total, and **does not** touch `api_rate_limits`. DB-002
  and every other `docs/SCHEMA.md`-citing finding are unaffected. DB-001 is the
  sole exclusion on these grounds.

## Prior-run reconciliation

### Every prior ticket is closed, so the duplicate branch is unavailable

All 35 tickets from the 2026-08-07 run (`SLA-9` … `SLA-43`, per
`docs/audit/linear-ids.json`) are in a completed state. Not one is open. Under
the run's rule — reproduces + open = duplicate, do not re-file; reproduces +
closed = **regression, ticketable and flagged as such** — there is no
suppress-as-duplicate case available this run. Any prior-run overlap surviving
verification is filed, labelled against its SLA id.

### The intervening SLA-44 … SLA-65 wave all landed *before* the baseline

A second wave exists that is absent from the prior run's `linear-ids.json`,
filed between the two runs. `git log --grep='SLA-\(4[4-9]\|5[0-9]\|6[0-5]\)'
06d9f8ce` returns 21 commits — SLA-44, 45, 47, 48, 49, 50, 51, 55, 56, 57, 58,
59, 60, 61, 62, 63, 64 — **every one an ancestor of the pinned baseline.**
SLA-65 (#625) is correctly absent, being the post-baseline commit above.

The consequence is uniform and worth stating plainly: a finding that reproduces
at this baseline is a genuine gap those fixes did not close, not a
pre-remediation artifact. Several findings sit on adjacent ground and their
tickets say so rather than pretending the prior work did not happen:

| finding | adjacent closed ticket | relationship |
|---|---|---|
| `DCT-013` | SLA-46 (#560), SLA-48 (#610) | **Regression.** SLA-46 swept CLAUDE.md against the tree; the sibling roster is missing `csvimport` again. SLA-48 defined the leaf taxonomy whose worked example now names three packages deleted in the baseline's parent commit. |
| `ARCH-001` | SLA-45 (#574) | Same script, different hole. SLA-45 closed a target-side flat-sibling escape; this is the fail-open behaviour of the hexagonal checks. |
| `ARCH-002` | SLA-48 (#610) | Leans on the leaf taxonomy SLA-48 created; the defect is that the taxonomy is not enforced upward from `platform`. |
| `DCT-008`, `DCT-009` | SLA-46 (#560) | Ground that CLAUDE.md-scoped sweep did not cover (`docs/`, `internal/README.md`). |

### NAM-001 is a first filing — neither duplicate nor regression

The naming lens flagged an overlap with the prior run but correctly refused to
open the file, per the Run Card prohibition. Controller-resolved here, where
reading prior runs is required.

The counterpart is prior-run `FE-008`, "Pointer: two `SellSheetItem` price fields
carry cent values but their name (and JSON tag) omit the `Cents` suffix." It was
`suspected`-tier and **deliberately never ticketed**:
`docs/audit/ADJUDICATIONS.md:377` records "FE-008 is `suspected`-tier —
`ticketable: false`. Do not ticket it," and `:428` lists it among findings
"correctly held back on exactly that rule." It appears in the prior `REPORT.md`
(`:2009`) and has no entry in `linear-ids.json`.

So neither branch of the dedup rule applies: no open ticket to duplicate, no
closed ticket to regress against. NAM-001 is a **first filing** — a fourth
adjudication category alongside duplicate, regression, and remediated. It is also
strictly broader, reaching five *Go* fields in
`internal/domain/inventory/analytics_types.go` where FE-008 saw two *TS* fields
downstream. Its ticket notes that the 2026-08-07 run observed the frontend half
and held it back.

## Fold-ins — filed inside another unit rather than separately

- **ARCH-004 and FE-006 fold into DCT-013.** All three land on the same
  `internal/README.md` material from three different lenses; both verifiers said
  so explicitly and set `ticketable: false` to reflect duplication, not doubt.
- **NAM-005 folds into DCG-001** (`internal/domain/llmutil`), which was filed at
  the stronger `mechanical` tier.
- **DCT-003 clusters with DCT-002.** DCT-003 is not a defect report — it is the
  verdict on a Phase 1 lead, concluding `ModeConfig.WebMode` is **live** and the
  config map's single dead field is a false positive. Its entire fix is one
  explanatory comment. It rides with DCT-002 because both turn on the `-web`
  flag, and the finding says so itself: "The genuine debt in this area is DCT-002,
  not this field."
- **DCT-012's residue is subsumed by DCT-010**, which sweeps the unregistered
  endpoint paths.

## Severity corrections carried into the tickets

Nine verdicts came back `confirmed_lower_severity`, each with specific
counter-evidence rather than a general discount. The corrections are load-bearing
and every affected ticket carries the correction in its own body — a ticket that
repeated the lens's original framing would be shipping a claim this audit
disproved. The three that most change what a developer would do:

- **DUP-001** (high → medium). The lens's evidence item 4, "no test in
  `dhpricing` exercises the reviewed/override precedence," is **false**:
  `internal/domain/dhpricing/service_test.go:225` covers it, including the
  cross-offset instant-vs-lexicographic case both source comments name as the
  subtle failure mode. The lens missed it by grepping for the resolver name where
  the test reaches it through exported `SyncPurchasePrice` — LENS-BRIEF trap 1
  running in the direction that makes covered code look uncovered. This was the
  run's only `mechanical` finding and got the hardest verifier precisely because
  both reversals in the previous run were in that tier.
- **DB-004** (high → low). The "~27% of query time" framing is inherited from
  migration 000003's own comment and does not attach to this gap:
  `dh_push_status` is `NOT NULL DEFAULT ''`, so the `IS NULL` branch is dead and
  the intended non-partial index would deliver no measurable benefit at this
  baseline. Its acceptance criterion 4 is unprovable as written and is rewritten
  before filing.
- **DUP-003** (medium → low). `authService` is **not** a second production
  implementation — its own doc comment declares it a deliberate lightweight
  double, with two methods returning `ERR_NOT_IMPLEMENTED` by design. The ticket
  must not say the two implementations "have already drifted."

## Constraints observed, and what they cost

- **No database access.** DB-004 and DB-005 arrived with acceptance criteria
  satisfiable only against a live database (`pg_stat_user_indexes`, `EXPLAIN`).
  Both were resolved *statically* instead — containment for DB-005, the
  `NOT NULL DEFAULT ''` column definition for DB-004. Their tickets state plainly
  that nothing is claimed about live index usage.

  **`unresolvable` was never actually available**, which this run only discovered
  after the fact. `docs/audit/VERIFIER-BRIEF.md` offers it as a fourth verdict in
  three places (`:74`, `:84`, `:104`) and the DB-004/DB-005 dispatches explicitly
  invited it, but `docs/audit/schema/verdict.schema.json` enumerates exactly
  `confirmed | confirmed_lower_severity | refuted` with
  `additionalProperties: false` — the gate would have rejected the token. No
  verifier hit the mismatch because no honest `unresolvable` case arose, so this
  run's "0 unresolvable" is a true statement about the findings and **not**
  evidence the option was exercisable. A verifier who genuinely needed it would
  have been forced by the gate into a wrong verdict. Fix the brief or the schema
  before the next run.
- **No secret values.** Variable names only throughout; no real `.env` was read.
- **DCT-005's quota claim is an assumption, not a measurement.** The verifier was
  barred from running the integration suite, and the ticket says so rather than
  presenting the quota burn as observed.

## Scope gaps — for the next run's scout briefs

Recorded because absence of a finding in these areas is uninformative, not clean:

- `db-map`'s record vocabulary excludes triggers, functions, CHECK constraints,
  and GRANT/REVOKE state. None was audited.
- Duplication detection was body-similarity based, so duplication expressed as
  parallel SQL column lists is invisible to it — **DUP-002, the strongest live
  defect in the run, was found by reading, not by the scan.**
- Go↔TS tag mirroring was spot-checked, not systematic. One struct pair was
  checked and held; FE-003/FE-004 are therefore not necessarily the whole of it.
- `docs-map` has a coverage hole: five documents the scout reported covering
  produced zero records, and it is the only scout with no baseline count that
  could have caught it. See `RUN-CARD.md`.
