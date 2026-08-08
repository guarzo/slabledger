# Controller Adjudications

Rulings made by the controller on findings that no single lens could settle,
and on cross-lens pointers. **Task 14 must read this file before consolidating.**
A pointer refuted here does not become a ticket, regardless of what the
originating lens's JSON still says.

Every ruling below was verified by the controller against source, not accepted
from an agent's report.

---

## NB-010 — REFUTED. Do not ticket. Do not resurrect.

**Claim (from `naming-and-boundaries`, `suspected` pointer):** five symbols are
dead — `DetectCoverageGaps`, `CoverageGap`, `ExtractCharacter`, `ClassifyEra`,
`ClassifyPriceTier`. Basis: all five show `external_refs=0` and
`name_ambiguous=false` in `docs/audit/maps/go-reference-map.json`.

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

## Six `auth.Repository` methods — CONFIRMED as DEADGO-004.

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
