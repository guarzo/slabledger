# Run Card — audit run 2026-08-08

This card carries every run-specific fact. The shared briefs
(`docs/audit/PREAMBLE.md`, `LENS-BRIEF.md`, `VERIFIER-BRIEF.md`) are deliberately
run-independent and defer to this card wherever they need a number, a path, or a
revision.

## Baseline

| | |
|---|---|
| **Baseline revision** | `06d9f8ce172dada98f307f860e2e2985c9fb5ca6` |
| Committed | 2026-08-08 |
| Subject | `refactor(demand): type the demand repository seam and fix the always-zero listing count (SLA-41) (#624)` |
| Previous run's baseline | `740976ecf80a4f2ccdaa611d7790ccaa95b48773` (2026-08-07) |

Read this revision, never the working tree: `git show <rev>:<path>`,
`git grep <pattern> <rev> -- <pathspec>`. A bare `git grep` reads whatever is
checked out and is not valid evidence.

## Paths

| | |
|---|---|
| Run directory | `docs/audit/runs/2026-08-08/` |
| Maps (`$MAPS`) | `docs/audit/runs/2026-08-08/maps/` |
| Your output | `docs/audit/runs/2026-08-08/findings/<lens>.json` |
| Baseline facts | `docs/audit/runs/2026-08-08/baseline.json` |

Shared, NOT per-run: `docs/audit/schema/`, `docs/audit/scripts/`, and the three
briefs. Never write to any of them.

## Baseline facts

| Fact | Value |
|---|---|
| `go_files` | 682 |
| `packages` | 48 |
| `migrations` | 35 |
| `frontend_files` | 208 |
| `env_vars` | 75 |

## Phase 1 maps — all five gated GATE PASS

| Map | Records | Notes that bound your claims |
|---|---|---|
| `go-reference-map.json` | 5985 | 1691 `name_ambiguous`, 3317 `external_refs==0`, 196 test-only. **4.5 MB — `jq` only, never `Read`.** |
| `frontend-reference-map.json` | 894 | 64 ambiguous, 392 zero-ref, 39 test-only. Covers 186 of 208 files; the other 22 are test files and barrels with no own declarations. |
| `db-map.json` | 580 | 227 ambiguous. Cumulative final-state replay of all 35 migrations; 8 table records are marked NOT in the final schema, naming the migration that dropped them. |
| `config-map.json` | 175 | 75 env vars, 99 config fields, exactly 1 dead field, 0 orphans. |
| `docs-map.json` | 109 | 106 claims: 77 confirmed, 23 refuted, 9 incomplete. Records land in 17 files — that is where records LAND, not the examined universe. See the coverage caveat below. |

### Map caveats that are load-bearing this run

- **Go `external_refs` is an upper bound.** Counting is name-based, not
  type-resolved, and selector names are counted — a reference to `x.Foo` counts
  toward *every* top-level `Foo`. Never build a mechanical-tier claim on an
  ambiguous count in either direction.
- **The 3317 zero-ref Go records are not a dead-code list.** `postgres.MigrationsFS`
  sits in it, reachable only via `//go:embed` (mechanism 5).
- **Mechanisms 1–9 are unresolved for every Go record.** The map does not answer
  them. You must, per the preamble, for any mechanical claim.
- **`registration_sites` in the Go map is a path heuristic**, not proof of wiring.
- **Reference arrays are capped at 10 entries** — a record showing 10 refs may
  have more.
- **Frontend counts are textual.** Non-TS consumers (`.css`, `.html`,
  `vite.config.js`, Playwright specs outside `web/src`, `package.json` scripts)
  are invisible to it. Barrel re-exports route to `registration_sites`, not
  `external_refs`.
- **Config consumers are classified by path**; `.md` matches are documentation,
  not consumers.
- **`docs-map` coverage is not established by its record set.** Controller-derived
  after the gate: five documents the scout reported covering produced zero records
  — `docs/USER_GUIDE.md`, `docs/psa-harvester.md`,
  `docs/CAMPAIGN_ANALYSIS_API_GAPS.md`, `docs/inventory-friction-analysis.md`,
  `docs/polish-report.md`. For those, "examined and clean" is indistinguishable
  from "never opened," and this scout is the only one with no baseline count to
  reconcile (`SCOUT_KEYS[docs-map]=""`), so the gate cannot detect it. Conversely
  `docs/plans/2026-08-08-flat-sibling-checker.md` produced a record despite all of
  `docs/plans/` being declared a gap — so the declared gap list does not prove
  omission either. The 106 positive records are unaffected; only absence-of-record
  is uninformative here.

## Two open leads from Phase 1

Neither was called a finding — reachability is the lenses' judgment, not the
scouts'. Both are handed over without a verdict attached.

1. **Advisor remnant.** `web/src/react/components/advisor/SectionedReport.tsx`,
   its test, and `splitByH2.ts` survive at this baseline despite commit `e46370e5`
   ("fully remove the Azure AI / advisor feature") and migrations
   `000013_drop_advisor_cache` / `000035_drop_ai_advisor_tables`. Corroborated
   independently by the frontend map and the db map. — *frontend-health*, with a
   pointer from *dead-code-go* if any Go side remains.
2. **`ModeConfig.WebMode`** (`internal/platform/config/types.go:7`) is bound to
   the `-web` CLI flag and read only inside `internal/platform/config/`. The
   config map scored it the single dead config field; whether the flag binding
   itself counts as a live consumer (mechanism 7) is a judgment, not a count.
   — *docs-config-tests*.

## Hard constraints

- **Strictly read-only.** Write exactly one file: your own findings JSON. Nothing
  under `internal/`, `cmd/`, `web/src/`, no documentation file, no map, no schema,
  no script. Do not commit.
- **No database access.** A live production database sits behind `DATABASE_URL`.
  Static analysis of migration files only — no connections, no SQL.
- **No secret values.** Never read a real `.env`. Report variable NAMES only.
  `.env.example` is fine; it holds placeholders.
- **Enumerate with git**, at the pinned revision. `ls` and `find` surface this
  repo's live worktrees under `.worktrees/` and `.claude/worktrees/` and are not
  evidence.
- **Do not read any previous run's REPORT.md, TICKETS.md, findings, or verdicts.**
  Deduplication against prior runs is the controller's job in Phase 4, done after
  your work lands. Reading them would bias you toward confirming what was already
  filed and away from noticing what changed. If you stumble on a path under
  `docs/audit/runs/2026-08-07/`, stop reading it.

## Gate before you finish

```bash
AUDIT_BASELINE_FACTS=docs/audit/runs/2026-08-08/baseline.json \
  bash docs/audit/scripts/validate.sh findings docs/audit/runs/2026-08-08/findings/<lens>.json
```

Iterate until it prints `GATE PASS`. Never edit the validator or the schema to
pass. Note the gate enforces that a `mechanical` finding answers all nine runtime
checks **by their exact names**:

`interface_satisfaction`, `functional_options`, `embedding`, `serialization`,
`go_embed`, `build_tags`, `registration`, `init_side_effects`,
`reflection_or_string`

## Return only

The `GATE PASS` line, your finding count broken down by confidence tier, and any
gaps. Do not paste findings into your reply — they are already on disk.
