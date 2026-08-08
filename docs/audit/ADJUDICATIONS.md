# Controller Adjudications

Rulings made by the controller on findings that no single lens could settle,
and on cross-lens pointers. **Task 14 must read this file before consolidating.**
A pointer refuted here does not become a ticket, regardless of what the
originating lens's JSON still says.

Every ruling below was verified by the controller against source, not accepted
from an agent's report.

---

## NB-010 — REFUTED. Do not ticket. Do not resurrect.

**Claim (from `naming-and-boundaries`, `suspected` pointer):** that five symbols
— `DetectCoverageGaps`, `CoverageGap`, `ExtractCharacter`, `ClassifyEra`,
`ClassifyPriceTier` — show zero external references and are unambiguous in
`docs/audit/maps/go-reference-map.json`.

**Correction to this entry, from `verify-naming` (accepted).** An earlier
revision of this section rendered the claim as "five symbols are dead." That
overstated what the lens filed, and the record should not misreport an agent
that behaved well. The lens claimed only the map reading above — which is
true and reproduces — filed it at `suspected`, and its `exclusions` field
stated outright: *"This lens did NOT run the nine runtime-reachability checks
on these symbols; `CoverageGap` in particular carries JSON tags and could be an
unreferenced-in-Go API contract (mechanism 4)."* That is precisely the
mechanism that makes them live, named by the lens before anyone else found it.

The pointer is still **refuted** — what it points at, a dead-code candidate
cluster, is not real, and a pointer that cannot survive is not routed. But the
lens's hedging discipline here is behavior to preserve, not a failure to
correct. Task 14 must not read this section as a mark against that lens.

**The map data is correct. The inference from it is wrong.**

All five are defined in `internal/domain/inventory/portfolio.go` and every
reference to them is inside package `inventory`. Their `external_refs=0` is
therefore accurate and means only what it says: nothing *outside the package*
names them directly. It is not evidence of deadness. This is caveat 2 of
`LENS-BRIEF.md` §2 ("1,798 zero-ref records are not a dead list") combined with
trap 1 of §3 (the in-package caller).

They are reached through their enclosing function:

- `internal/domain/inventory/portfolio.go:200`
  `func ComputePortfolioInsights(...) *PortfolioInsights` calls
  `ExtractCharacter` (:207, :234), `ClassifyEra` (:224),
  `ClassifyPriceTier` (:229), and `DetectCoverageGaps` (:246).
- `ComputePortfolioInsights` has **3 external call sites**, all in a sibling
  package: `internal/domain/portfolio/snapshot.go:227`,
  `internal/domain/portfolio/service.go:213`, `internal/domain/portfolio/service.go:303`.
- `PortfolioInsights` itself has **10 external refs**.
- The result is serialized to the client: `CoverageGap` is a JSON-tagged struct
  (`internal/domain/inventory/portfolio_types.go:24`), surfaced as
  `CoverageGaps []CoverageGap \`json:"coverageGaps"\`` (`portfolio_types.go:48`),
  and consumed by the frontend at `web/src/types/campaigns/portfolio.ts:59`.

Deleting this cluster would have removed live code serving a frontend type.

**Note for whoever writes the dead-code ticket:** the `external_refs` field
answers "is this named from outside its package," never "is this reachable."
For any candidate whose references are all in-package, reachability must be
re-asked about the enclosing exported function. `dead-code-go`'s DEADGO-001
handled this correctly and is the model.

---

## DEADGO-001 — REFUTED by Phase 3. Do not ticket as written.

Verifier `verify-deadgo-001` refuted a `mechanical`-tier finding. Controller
re-derived every leg below independently.

**Split the claim in two.** Only the first half survives:

- **The Go package IS externally unreachable.** `git grep -n
  'slabledger/internal/platform/cache' -- '*.go'` returns exactly ONE line
  tree-wide: `internal/integration/pricing_test.go:12`, which carries
  `//go:build integration` at line 4. That leg stands.
- **The `-cache` flag is LIVE and deleting it breaks production.**
  `Dockerfile.harvest:39` is
  `ENTRYPOINT ["/app/psa-harvest", "--cache", "/tmp/psa-harvest-cache.json"]`.
  The chain: `cmd/psa-harvest/main.go:39` → `parseBaselineFlag` (passthrough
  asserted at `baseline_test.go:51,53`) → `main.go:49 config.Load(args)` →
  `loader.go:239` FlagSet with `flag.ContinueOnError` → `loader.go:262` returns
  the `fs.Parse` error → `main.go:44 log.Fatalf`. Removing the registration at
  `loader.go:253` kills the deployed container at startup with
  `flag provided but not defined: -cache`.

`Dockerfile.harvest:35-36` states the intent outright: *"The harvester never
uses the on-disk cache, but config.Load() still runs EnsureDirectories() on the
cache path — default 'data/cache.json' would try to…"*. The flag is passed
deliberately, to redirect a directory-creation side effect. This is stronger
than accidental coupling.

The finding's `acceptance_criteria` #2 would have driven a fixer to strip
`Dockerfile.harvest:39`.

**Root cause, and the lesson for Task 14.** The finding declared
`searched_universe: 'git-tracked *.go files'`. A CLI flag's consumers do not
live in `*.go`. They live in Dockerfiles, compose files, entrypoints, CI
workflows, Makefiles, and shell scripts. **A search universe restricted to
source files is structurally incapable of seeing them** — no amount of care
inside that universe would have caught this.

Any future finding asserting a flag, env var, subcommand, or binary name is
unused must declare a universe that includes deployment and orchestration
files, or it is not ticketable at `mechanical`.

**What IS ticketable here:** the narrow claim that `internal/platform/cache` has
no non-integration-test consumer. Task 14 must re-scope it to that, drop the
flag/config legs entirely, and re-tier it — the nine runtime checks were
answered against the wrong universe, so `mechanical` does not survive.

---

## Six `auth.Repository` methods — filed as DEADGO-004; DOWNGRADED by Phase 3.

**Read the Phase 3 ruling below before acting on anything in this section.**
The confirmation recorded here is superseded in one substantive respect.

Raised out-of-lane by `size-and-complexity` on its own `git grep`, which it
correctly declined to file. Routed to `dead-code-go`, which filed DEADGO-004 at
`mechanical` with all nine runtime checks answered.

Controller verified independently:

- `git grep '\.<method>('` across all `*.go`, excluding `_test.go` and
  `internal/testutil/mocks/`, returns **zero call sites** for all six:
  `GetTokens`, `GetTokensByUserID`, `UpdateTokens`, `DeleteTokens`,
  `DeleteAllUserTokens`, `CleanupExpiredOAuthStates`.
- The finding's `interface_satisfaction` check is argued the right way: it names
  the only two places an `auth.Repository` interface value is held
  (`google.OAuthService.repo`, a `cmd/slabledger` local) and establishes that
  neither calls through it — rather than reasoning from the declaration, which
  would prove nothing.

**Near-miss worth preserving in the ticket.** `CLAUDE.md` advertises a session
cleanup scheduler, and a reasonable reviewer would assume it reaps expired OAuth
states. It does not: `internal/adapters/scheduler/session_cleanup.go:77` calls
`authService.CleanupExpiredSessions(ctx)` — a *different* method on a *different*
type. Expired OAuth states are never reaped by anything. The fixer should
confirm this deliberately before removing `CleanupExpiredOAuthStates`, because
the alternative reading is that the cleanup path was meant to exist and was
never wired — which is a bug ticket, not a deletion ticket.

---

## DEADGO-004 — Phase 3 ruling: SPLIT THE FINDING. Not ticketable as written.

Verifier `verify-deadgo-004` returned `confirmed_lower_severity`,
`ticketable: false`. Controller re-derived each leg.

**Central claim survives.** Zero call sites for all six methods, confirmed
independently at baseline. No test exercises any of them.

**Three defects, one of them substantive.**

1. *Holder count is THREE, not two.* The finding names two places an
   `auth.Repository` value is held. It missed
   `internal/domain/auth/service_impl.go:24 repo Repository` — invisible to its
   `auth\.Repository` grep because the reference is unqualified inside package
   `auth`. This is trap 1 (the in-package caller) striking the *verification*
   step rather than the finding step. A grep for a package-qualified name can
   never see the package's own uses.
2. *Evidence #1 claims 4 lines; the command returns 5.* Doc comments match.
3. **The six are not one fix unit.** Five are removable. One is a bug.

**The five token methods ARE removable.** `GetTokens`, `GetTokensByUserID`,
`UpdateTokens`, `DeleteTokens`, `DeleteAllUserTokens` are redundant with a live
database constraint: `migrations/000001_initial_schema.up.sql:104` declares
`session_id TEXT REFERENCES user_sessions(id) ON DELETE CASCADE` on
`user_tokens`. Logout and session cleanup already delete token rows at the DB
level. The Go methods are a second, unused path to an outcome the schema
guarantees.

**`CleanupExpiredOAuthStates` must be WIRED, not deleted.** It is an unwired
implementation of needed behavior:

- `oauth_states` has no FK cascade and no TTL. Controller confirmed the only
  three statements touching the table, all in
  `internal/adapters/storage/postgres/auth_oauth_state_store.go`: `INSERT` (:11),
  `DELETE … WHERE state = $1 AND expires_at > $2` (:24 — consumes exactly one
  row, on successful callback only), and `DELETE … WHERE expires_at <= $1` (:43
  — the orphaned cleanup). **Every abandoned login leaves a permanent row.**
- `migrations/000003_supabase_security_and_perf_fixes.up.sql:254` DROPs
  `idx_oauth_states_expires` — the index supporting exactly this cleanup query.
  The path was not merely unwired; its support was actively removed.
- The near-miss recorded earlier in this file is the same fact from the other
  side: `internal/adapters/scheduler/session_cleanup.go:77` calls
  `CleanupExpiredSessions`, a different method. Nothing reaps OAuth states.

**Instruction to Task 14.** DEADGO-004 becomes **two** fix units:

- *Removal ticket* — the five token methods, citing the `ON DELETE CASCADE` at
  `migrations/000001_initial_schema.up.sql:104` as the reason they are safe to
  drop, and listing all THREE holders the fixer must check.
- *Bug ticket* — wire `CleanupExpiredOAuthStates` into `session_cleanup.go`
  alongside `CleanupExpiredSessions`, and restore an index on
  `oauth_states.expires_at`. This is unbounded table growth on an auth table,
  not dead code.

Ticketing the finding as filed would have deleted the fix for a live leak.

---

## NB-001 — CONFIRMED and widened by the controller.

`scripts/check-imports.sh:39` hardcodes
`SUB_PACKAGES="arbitrage portfolio tuning finance export dhlisting"` — six names.
`git ls-files 'internal/domain/*'` yields **25** packages. The checker iterates
the name list rather than the directory, so the other 19 are never opened and
their imports cannot violate anything it can detect.

Confirmed uncaught violation: `dhpricing` imports `dhlisting` — which *is* one of
the six the rule names — from three files:
`internal/domain/dhpricing/service.go:7`, `internal/domain/dhpricing/types.go:11`,
`internal/domain/dhpricing/service_test.go:9`.

Also invisible to the checker, listed for the fixer but **not asserted as
violations**: `advisor -> ai, scoring`; `demand -> inventory`;
`dhpricing -> inventory`; `psacampaign -> inventory`; `pricing -> inventory`.
Whether a sub-package may import the `inventory` core is not settled by
`check-imports.sh:7-8`, which documents only sibling-to-sibling. The ticket must
ask that question rather than presume the answer.

Consequence: any audit conclusion of the form "`make check` passes, so the
hexagonal invariant holds" rests on a check with a 19-of-25 blind spot. This
also supports NB-004 — the flat-sibling rule makes `inventory` the only legal
home for anything two siblings share, which is part of why it reached 10,428 LOC.

---

## `mm_sales_comps` — controller seed was wrong.

The controller seeded `db-schema` with eight tables lacking RLS. `mm_sales_comps`
was created by migration `000005` and dropped by
`internal/adapters/storage/postgres/migrations/000021_drop_market_movers.up.sql:8`
(`DROP TABLE IF EXISTS mm_sales_comps;`). It does not exist at HEAD.

The lens caught the error rather than inheriting it. Verified by the controller
at the migration file. **The live RLS gap is 7 tables, not 8.** Any ticket
citing 8 is citing the controller's mistake.

---

## ARCH-004 vs SIZE-001 — TWO findings, one theme. Cluster; do not merge.

Both describe an oversized interface, so Task 14 will be tempted to dedup them.
They target different types on different sides of the port boundary:

- **ARCH-004** — `internal/domain/inventory/service.go:175`, `type Service interface`.
  The **consumer-facing** union, 54 methods. The defect is that a declared
  segmentation buys no seam: four of seven sub-interfaces have zero consumers,
  and consumers wanting narrow dependencies declare their own instead.
- **SIZE-001** — `internal/domain/inventory/repository_purchase.go:31`,
  `type PurchaseRepository interface`. The **persistence port**, 55 methods,
  more than the other six sibling repository interfaces combined (controller
  independently counted: Campaign 5, Sale 7, Pricing 8, Analytics 10,
  Finance 14, DH 2 — 46 combined).

Different declarations, different files, different layers. Merging them yields a
ticket no single PR can land. Keep them as two fix units under one theme:
interface segregation was declared, never enforced, and drifted in both
directions at once.

---

## Phase 3 outcome — binding inputs to Task 14

All 12 verdict files gated PASS. **63 verdicts for 63 findings** — full
coverage, no gaps, no double-assignment. Tally: 48 `confirmed`,
11 `confirmed_lower_severity`, 4 `refuted`. 54 ticketable, 9 not.

**The four refutations, and what survives each:**

| ID | Why refuted | Salvage |
|---|---|---|
| `DEADGO-001` | Search universe blind to Dockerfile consumers | **Yes** — re-scope to "`internal/platform/cache` has no non-integration-test consumer," drop the flag/config legs, re-tier below `mechanical` |
| `DEADGO-004` | Bundles a removal with a bug | **Yes** — split into two tickets, per its section above |
| `NB-010` | Symbols are live and serialize to the frontend | **No** — do not resurrect |
| `NB-007` | Accurate filename, no evidence anyone was misled | **No** as a rename; the `ParseRange`-belongs-in-`platform` observation was never assessed and is not evidence |

**`DCT-005` is refuted on citation, not on substance.** It cites `CLAUDE.md:145`
for the `make check` description; line 145 is an unrelated bullet about mock
patterns and the real sentence is at `CLAUDE.md:167`. The underlying complaint
is independently true: `Makefile:146`'s `check:` target runs four scripts —
`lint`, `check-imports.sh`, `check-file-size.sh`, and
`check-playwright-version.sh` — and `CLAUDE.md` documents only three.
**Task 14 should re-file this with the corrected line number rather than drop
it.** A refuted verdict means the finding as written is not actionable; it does
not always mean there is nothing there.

**`DCT-018` carries `evidence_reproduces: false` with a `confirmed` verdict.**
This is not a gate violation to fix: the mismatch is a `wc -l` citation (25 vs
the actual 31, doc comment accounts for the difference) on a finding whose own
recommendation is "no ticket required." `ticketable: false`. No action.

**One correction ran the other way.** `verify-naming` corrected the controller's
own paraphrase in the NB-010 section above, and the correction was accepted and
applied. Recorded here so Task 14 knows the record has been amended and can
trust the amended text.
