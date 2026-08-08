# SDD ledger — plan: docs/superpowers/plans/2026-08-08-remove-azure-ai.md

Branch: remove-azure-ai
Merge base: c624b9a70d0e37d3c350f0224c61cd36b8e7615e
Plan commit: 8a58e0d6 (post-Codex-review amendments)

Tasks: 1 frontend, 2 HTTP surface, 3 CLI, 4 construction/scheduler unwire,
5 delete orphaned packages, 6 configuration, 7 migration 000032, 8 docs sweep.

Controller ruling (pre-flight): `internal/domain/llmutil` has zero importers
anywhere in the tree, including the advisor packages — already dead before this
change. Global Constraint "no opportunistic cleanups" governs; it stays. Noted as
a follow-up for the final review, alongside the pre-existing polish-all staleness
Task 8 Step 23 reports.

Task 1: complete (commits 8a58e0d..581db03, review clean — spec ✅, quality approved)
Task 1: minor (deferred): `web/src/react/utils/formatters.ts:125` formatTokens and
  :134 formatLatency are now unused everywhere in web/ — their only caller was the
  deleted AIStatusTab.tsx. Debris from this removal, not a scope violation
  (formatters.ts was not in the brief's file list). Final review to triage.
Task 2: complete (commits 581db03..2c22e04, review clean — spec ✅, quality approved,
  no Critical/Important findings)
Task 3: complete (commits 2c22e04..e135b79, review clean — spec ✅, quality approved,
  no findings at any severity)
Task 4: complete (commits e135b79..00db073, review clean — spec ✅, quality approved,
  no findings at any severity). Reviewer's independent grep confirms every remaining
  AICallTracker/AICallRepo/GapStore reference lives inside the six packages Task 5
  deletes, plus `internal/testutil/mocks/gap_store.go` — carry that mock file to Task 5
  as a watch item.
Task 5: complete (commits 00db073..fedf590, review clean — spec ✅, quality approved,
  no findings at any severity). 50 files, 7007 deletions; `make check` clean (only
  pre-existing file-size WARNs). Reviewer independently confirmed zero remaining
  importers of the six deleted paths, `openai-go` and the Azure SDK modules gone from
  go.mod/go.sum, the two never-touch files unchanged, and the gap_store.go mock deleted
  per the brief's Files block. `go mod tidy` added `kr/text` and `rogpeppe/go-internal`
  as indirect deps (recomputed closure from existing test tooling) — noted, not a
  scope violation.
Task 6: complete (commits fedf590..a2af5c92, review clean — spec ✅, quality approved,
  no findings at any severity). 4 files, 49 deletions: the four `AzureAI*` fields on
  `AdapterConfig`, the `AdvisorRefresh` field and `AdvisorRefreshConfig` type, its
  defaults block, `ADVISOR_MAX_TOOL_ROUNDS` and `AZURE_AI_*` env parsing, and the
  `.env.example` section. Repo-wide grep for AzureAI|AdvisorRefresh|AZURE_AI|
  ADVISOR_MAX_TOOL_ROUNDS outside docs/ returns nothing; validation.go and the config
  tests had zero references, so no surface was missed.
  Process note: the first Task 6 reviewer went idle twice without returning a verdict
  and was stopped; `review-task6b` reviewed from scratch and produced the verdicts above.
Task 7: complete (commits a2af5c92..5069cd3b, spec ✅, quality approved). 3 new files:
  migration 000032 up/down + a 265-line migration test. Reviewer independently re-ran
  `make test-postgres` and the named tests printed `--- PASS`, not `--- SKIP`, so the
  DB-gated evidence is genuine. Verified against the live migrated DB: both tables and
  both AI usage views gone, zero orphaned policies/sequences/indexes, no FK or trigger
  dependencies, `campaign_purchases.ai_suggested_price_cents` still present. Down file
  byte-matches its 000001/000003 sources with post-000028 RLS shape, and every
  role-dependent statement sits inside the `pg_roles` guard.
Task 7: MERGE GATE (Important, not a diff defect, cannot be closed by an implementer):
  the brief requires an explicit PR acknowledgement that production `ai_calls` was
  exported or consciously waived, in the style of 000030's header record. Neither the
  implementer nor the reviewer has production DB access. Auto-deploy runs on push to
  main, so the drop is irreversible on merge — down restores structure, not data.
  Controller ruling: parked for the human, not routed into the fix loop, because no
  code change can satisfy it. Must be surfaced before the branch is merged.
Task 7: minor (deferred): `migration_000032_test.go:47-59` — `TestMigration000032_
  ObjectsDropped` uses `requireTestDB` (no migrations) rather than `setupTestDB`, so run
  alone against an empty DB its four assertions pass vacuously. Precedent-consistent with
  migration_000027/28/29/30 tests; red-phase output ruled it out for this run.
Task 7: minor (deferred): no test pins the "producer removed, data kept" invariant that
  `campaign_purchases.ai_suggested_price_cents` survives 000032. Reviewer verified it
  directly against the migrated DB; one `assert.Contains` would move it into the suite.
Task 8: complete (commits 5069cd3b..a47471a4, review clean — spec ✅, quality approved,
  no Critical/Important findings). 7 files, 271 deletions: docs/LLM_USAGE.md deleted,
  plus edits to CLAUDE.md, docs/{API,ARCHITECTURE,SCHEMA}.md, internal/README.md, and
  .claude/skills/polish-all/SKILL.md. Reviewer independently re-ran all four gates —
  `go build ./...`, `go test -race -timeout 10m ./...`, `make check` (10 siblings /
  1361 checks), and `cd web && npm run build && npm test` (495 passed / 2 skipped) —
  and reproduced the final grep: surviving advisor/azure/ai_calls hits are exactly the
  permitted set (SCHEMA.md tombstones, migration history, migration_000032_test.go,
  one eval fixture). Both never-touch files confirmed byte-identical.
Task 8: minor (deferred): `docs/polish-progress.json:6,12` still mirrors the pre-removal
  polish-all segment table and is now further out of sync with the SKILL.md this task
  updated. Pre-existing staleness, out of the brief's Files list. Final review to triage.
Task 8: non-finding (recorded so it is not re-investigated): go.sum's
  `github.com/Azure/go-ansiterm` and devcontainer.json's `ms-azuretools.vscode-docker`
  match "azure" but are an unrelated transitive dep and a VS Code extension id.

ALL 8 TASKS COMPLETE.

FINAL WHOLE-BRANCH REVIEW (c624b9a7..a47471a4, opus): VERDICT ship as-is, pending one
human decision. Reviewer re-ran every gate itself, including `make test-postgres`
(ok, 12.5s) and `go test -v -run TestMigration000032` (--- PASS), plus two the plan's
gates do not cover: `npx tsc --noEmit` (clean) and eslint (0 errors). Note for future
work: `web/package.json:11` defines `build` as bare `vite build` with NO type checking,
so `npm run build` cannot catch a dangling TS type — tsc must be run separately.
Searched the tree for every advisor/azure/AI symbol across Go, TS, MD, JSON, YAML, SQL,
shell, .github/, fly.toml, Dockerfile, Makefile, scripts/, .devcontainer/: no dangling
reference, nothing still-used deleted. Confirmed the deleted `/advisor` SPA route is not
a 404 regression (router.go:226 catch-all) and that all six services formerly injected
into advisortool retain 5-15 non-test importers each.

Rulings on the four deferred Minors (all: acceptable to ship):
1. formatTokens/formatLatency orphaned — genuinely orphaned by this branch, but tsc and
   eslint clean and Vite tree-shakes them. Follow-up, not a blocker.
2. migration_000032_test.go vacuous-pass risk — REFUTED. `testhelper_test.go:32-38`
   TestMain calls resetTestSchema(), which drops/recreates schema public and runs
   RunMigrations unconditionally when POSTGRES_TEST_URL is set. No path reaches a test
   body with an unmigrated DB. Verified by running the test in isolation: --- PASS.
3. ai_suggested_price_cents survival untested — implicitly pinned: purchase_store.go:66,97
   select the column and purchase_store_test.go exercises those reads against the real
   migrated DB, so a drop would fail the suite, which ran green.
4. docs/polish-progress.json — abandoned run state (base_commit ec4d261b, 2026-04-20, all
   twelve segments still "pending"). SKILL.md holds the authoritative table and was updated.

Controller rulings on the two new Important findings (both PRE-EXISTING, verified at
merge base — neither is a defect in this diff; Global Constraint "no opportunistic
cleanups" governs, same treatment as llmutil):
- `web/src/react/components/advisor/` (SectionedReport.tsx, .test.tsx, splitByH2.ts) has
  zero importers outside itself at HEAD *and* at merge base c624b9a7 — confirmed by my
  own grep. It stays. The plan's Task 1 preamble claim that a `web/` grep for "advisor"
  returns exactly its listed sites is FALSE; it did not bite (Task 1's edits were
  independently re-verified complete), but the directory name will mislead the next
  grepper. Recorded here so it is not re-litigated.
- `internal/testutil/mocks/README.md:86,87,151` lists `social.Service`/`MockSocialService`
  and `picks.Service`/`MockPicksService`. CONTROLLER ERROR CORRECTED: I briefed agents
  that `internal/domain/social` still exists (carried from the plan text). It does not —
  `ls internal/domain/` shows neither social nor picks, and no mock files exist for them.
  Nothing imports them and no agent acted on the bad premise, so no rework follows; the
  stale README rows are pre-existing and out of scope.

Minor (follow-up, not blocking): branch is ~10 commits behind main (csvimport/SLA-35,
leaf taxonomy/SLA-48, SLA-44, SLA-61/63/64). CLAUDE.md was modified on both sides —
expect a real conflict there, and re-run `make check` after rebase because
check-imports.sh derives its sibling count from the tree and main adds csvimport/mmutil.

NO FIX DISPATCH OPENED: zero Critical, and both Importants are pre-existing conditions
or human decisions that no code change can close.

REMAINING BEFORE MERGE (human): export or consciously waive production `ai_calls`
history. Auto-deploy runs on push to main, so the drop lands on merge, not on a
deliberate operator action, and the down migration restores structure, not data.

---

POST-REVIEW: MERGE GATE CLOSED (2026-08-08).
Human granted production DB access (SUPABASE_DB_URL in /workspace/.env, gitignored).
`ai_calls` queried, summarized, and exported to `data/ai_calls_export_2026-08-08.csv`
(gitignored — verified with `git check-ignore` before writing; NEVER commit it).
Export byte-verified against the DB: 1393 rows, 1393 distinct ids, token sum 45,943,775,
cost sum 16857 cents — all four match the database exactly.
Findings: 1393 rows spanning 2026-04-09 → 2026-06-03. Last call was 66 days before today,
independent corroboration the feature was already dead in production. 41% overall error
rate; liquidation failed more often than it succeeded (167 success / 225 error).
`scoring_data_gaps` was empty (0 rows). Schema note: the timestamp column is named
`timestamp`, not `created_at`.

MIGRATION NUMBER COLLISION (human caught it; three reviews missed it).
While the branch sat, main landed its own `000032_restore_dh_state_events_lookup_indexes`
(SLA-58). golang-migrate rejects duplicate versions at load time and migrations run on
startup from embed.FS, so this was a BOOT FAILURE, not a test failure. It merged cleanly —
different filenames, no textual overlap — and surfaced only as an add/add conflict on the
test filename. All three prior reviews reviewed the branch against its own merge base and
could not see main moving underneath.

REBASE onto origin/main, then RENUMBER to 000035. That order was required: renumbering
first produces a test asserting `migrateToVersion(db, 34)` against migrations that exist
only on main. Backup ref: backup/remove-azure-ai-pre-rebase @ a47471a4.
Three conflicts resolved:
  1. builder.go — main refactored to buildXScheduler helpers; kept main's form and
     re-applied this branch's intent (drop gap-cleanup) to the new structure, including
     deleting buildGapCleanupScheduler from main's new builder_schedulers.go.
     `go build ./...` passed but `go vet ./...` then caught three dangling
     GapCleanupScheduler references in builder_test.go — go build does not compile tests.
     Use `go vet ./...` as the gate, not `go build`.
  2. go.mod/go.sum — took main's wholesale, deleted the packages, re-ran `go mod tidy`.
  3. add/add on migration_000032_test.go — main owns that path now. Restored main's,
     wrote ours as migration_000035_test.go (constants 31→34, 32→35).
Migration SQL bodies needed no edits: they reference 000001/3/13/21/28/30, never themselves.
Verified main's 32/33/34 contain no ai_calls / scoring_data_gaps / ai_usage references, so
there is no ordering hazard.
docs/SCHEMA.md: renumbered exactly 8 lines (112, 114, 789, 791, 1053, 1054, 1056, 1057) —
the four AI tombstones. Lines 968 and 1067 were LEFT at 000032 deliberately; they belong to
main's own 000032. A blind sed would have corrupted them.
The plan and spec markdown still say 000032 throughout. Left as-is: dated planning
artifacts describing the state at authoring time, not live references.

GATES RE-RUN AFTER REBASE + RENUMBER — all green:
  go vet ./...                     exit 0
  go test -race -timeout 10m ./... exit 0
  make test-postgres               exit 0 (postgres pkg ok, 15.4s)
  go test -v -run TestMigration000035  --- PASS x2 (not SKIP)
  make check                       exit 0 — 11 siblings / 1606 checks
                                   (was 10/1361; main's csvimport now governed)
  npm run build                    exit 0
  npx tsc --noEmit                 exit 0
  npm test                         exit 0 — 51 files, 503 passed / 2 skipped
gofmt: three files under internal/integration/ are unformatted. Verified PRE-EXISTING —
untouched by this branch and unformatted on origin/main too. Left alone under the Global
Constraint "no opportunistic cleanups."
