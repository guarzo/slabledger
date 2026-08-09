# How to run this audit again

This is the operating manual for the read-only tech-debt audit that produced
`REPORT.md` and Linear tickets SLA-9 … SLA-43. It records the topology, the
gates, and — more usefully — the specific ways the process tried to fail. Run
it against any baseline revision; nothing here is specific to the 2026-08
findings.

Companion documents: the design doc (`docs/specs/2026-08-07-codebase-tech-debt-audit-design.md`)
argues *why* the topology is shaped this way; the plan
(`docs/plans/2026-08-08-codebase-tech-debt-audit.md`) is the task-by-task
script. This file is what you read first if you are re-running it.

## Standing constraints

These bind every agent in every phase. They are the contents of
`PREAMBLE.md`, which is prepended to every dispatch prompt.

- **Strictly read-only.** No agent edits code. Tickets are the only output.
  Verified at the end by `git diff --stat main..HEAD -- internal/ cmd/ web/src/`
  being empty.
- **No database access.** There is a live production Supabase behind
  `DATABASE_URL`. Static analysis of migration files only — no connections, no
  migrations, no SQL.
- **No secret values.** Never read a real `.env`. Report variable NAMES only.
- **Git-tracked files at a pinned baseline.** Enumerate with `git ls-files`.
  `ls` and `find` are not evidence — they see build output, worktrees, and
  untracked scratch files, and every one of those is a false positive waiting
  to happen.
- **The Linear destination is confirmed with the user before any ticket is
  created.** Filing tickets is outward-facing and tedious to undo.

## Topology

Four phases. The ordering exists because each phase produces something the
next phase is structurally incapable of producing for itself.

```
Phase 1  scouts (5)         → maps/*.json      data, no judgments
   ↓     completeness gate                     truncation is invisible without it
Phase 2  lens auditors (8)  → findings/*.json  judgments, each citing the maps
   ↓
Phase 3  verifiers          → verdicts/*.json  one verdict per finding, briefed to refute
   ↓
Phase 4  controller         → REPORT.md, TICKETS.md, Linear
```

### Phase 1 — scouts produce data, not opinions

Five scouts, each building one reference map: Go references, frontend
references, database objects, config/env, docs. On the 2026-08 run these came
to 4439 / 649 / 572 / 149 / 24 records.

A map record is `{kind, identity, external_refs, ...}` — a count, not a
conclusion. Scouts are explicitly forbidden from saying anything is dead,
misnamed, or wrong. The separation matters because a scout that starts forming
judgments stops enumerating exhaustively.

### The completeness gate is not optional

`scripts/validate.sh` runs between Phase 1 and Phase 2 against
`schema/scout-report.schema.json`, with fixtures in `scripts/fixtures/`
(`good-scout.json`, `truncated-scout.json`). Its job is detecting a *truncated*
map — an agent that stalled mid-stream and returned valid JSON covering 60% of
the repo.

This is the failure mode you cannot see by reading the output: a truncated map
looks exactly like a clean map of a smaller codebase, and every downstream lens
inherits its blind spot as confident silence. Two consolidator subagents died
this way on the real run (§"Deterministic generation" below).

### Phase 2 — eight lenses, each with a declared searched universe

The lenses: architecture, database schema, dead Go code, docs/config/tests,
duplication, frontend health, naming and boundaries, size and complexity.
Yields on the real run, descending: docs/config/tests 21, naming 10, database
schema 8, frontend 8, architecture 5, dead Go code 4, size 4, duplication 3 —
63 findings.

Every finding declares a `searched_universe` string. **This field is where the
audit lies to itself.** A lens that declares `git-tracked *.go files` is
structurally incapable of seeing a consumer in a Dockerfile, a compose file, a
CI workflow, a Makefile, or a container entrypoint. It will report a symbol as
dead with complete confidence and be wrong.

That exact failure produced DEADGO-001 on the real run: a "dead" config flag
that `Dockerfile.harvest:39` passes on every invocation of an hourly production
Fly job. Because `flag.ContinueOnError` turns an unknown flag into
`log.Fatalf`, acting on the finding would have crashloop-ed the job.

**Reachability is a global property.** No subtree-scoped agent can determine
whether the rest of the repo references its subject. Design lens prompts so the
universe is stated, and treat a narrow universe as a reason to escalate to
Phase 3, not as a reason for confidence.

### Phase 3 — verifiers are briefed to destroy, not to appreciate

One verdict per finding, no exceptions, including findings you expect to
survive. `VERIFIER-BRIEF.md` is the prompt. Its load-bearing rules:

- **Default to REFUTED under uncertainty.** A wrong ticket costs a developer a
  wasted branch and teaches them to distrust the whole batch. A
  refuted-in-error finding costs one re-check. Not symmetric.
- **Re-derive; never audit the reasoning.** The finding's own description and
  `runtime_checks` are claims under test, never evidence.
- **Run every evidence command.** A finding whose commands do not reproduce is
  refuted regardless of whether you believe its conclusion. This is the most
  common defect and the cheapest to detect.
- **Severity is in scope.** Real but overstated → `confirmed_lower_severity`
  with the real consequence stated. For security findings especially,
  distinguish "data is exposed" from "ciphertext is exposed" from "a row can be
  overwritten."

Verdicts on the real run: 48 confirmed / 11 confirmed_lower_severity / 4
refuted — a 93% confirmation rate.

### The three traps, which cut both ways

`LENS-BRIEF.md` §3 lists three patterns that make dead code look live. The
verifier brief points out they equally make live code look dead:

1. **In-package callers.** `external_refs=0` means "not named from outside its
   package," never "unreachable." Ask whether the *enclosing exported function*
   is reachable. A package-qualified grep like `auth\.Repository` cannot see an
   in-package `repo Repository` field — on the real run this hid a third holder
   of an interface whose holder count the finding stated as two.
2. **Same-name collision.** Confirm every grep hit refers to the actual
   subject, not an unrelated identifier sharing the name.
3. **Serialization.** A JSON-tagged struct field can be live purely by being
   marshalled. Check `web/src/types/` before believing any Go type is dead.

Plus: interface satisfaction is invisible to a textual map. A method appearing
in an implementation and a mock is expected by construction and proves nothing
in either direction.

### Phase 4 — the controller adjudicates, then generates deterministically

The controller holds context no agent has: it read every finding, every
verdict, and the disagreements between them. Its rulings go in
`ADJUDICATIONS.md`, which **wins over any finding or verdict JSON** and is
handed to the consolidator as binding.

Adjudication is not rubber-stamping. On the real run it produced two
reversals in the mechanical tier — the tier that looks safest:

- **DEADGO-001 refuted** (the Dockerfile case above).
- **DEADGO-004 split.** Filed as six dead methods. Five were genuinely dead;
  the sixth, `CleanupExpiredOAuthStates`, was an *unwired implementation of
  needed behavior* — `oauth_states` has no FK cascade and no TTL, and migration
  000003 dropped the index its cleanup query needs, so every abandoned login
  leaks a permanent row. Ticketing the finding as filed would have deleted the
  fix for a live leak. It became one Bug ticket (wire it) and one Improvement
  ticket (delete the other five).

Both reversals came from the mechanical tier. Adversarial verification earns
its cost precisely where confidence is highest.

## Deterministic generation, not agent transcription

`REPORT.md` is ~2000 lines that mechanically merge 63 findings with 63
verdicts. Two consolidator subagents were dispatched to write it. Both died —
the second with `API Error: Response stalled mid-stream` — having written
nothing.

The fix was to stop using an agent. `scripts/build-report.py` holds the
controller's cluster map (`UNITS`) as the single source of truth and copies
evidence, blast radius, and acceptance criteria **verbatim from the JSON**.
`scripts/build-tickets.py` imports that same map by `exec`-ing the slice out of
`build-report.py`, so the report and the tickets cannot drift.

The judgment — which findings cluster into one mergeable PR — is the
controller's and needs a mind. The merge is jq work and needs a program. Do not
give the second job to an agent: it is long, mechanical, high-token, and
failure looks like silence.

Both scripts print a reconciliation on every run:

```
fix units: 35 / ticketable: 54 rolled up: 55 / missing: none
adjudicated-in: ['DEADGO-004'] / in >1 unit (expected: DEADGO-004 only): ['DEADGO-004']
```

"Deterministic" here means same generator + same inputs → same bytes. It does
not mean an already-filed run round-trips: the generators have been repaired
since both filed runs were produced, so regenerating one of those in place would
rewrite the record. See "Regenerating" in `README.md`.

## Ticketability rule

A finding is ticketable when:

- `verdict` is `confirmed` **or** `confirmed_lower_severity`, **and**
- the verdict's `ticketable` field is `true`, **and**
- `ADJUDICATIONS.md` does not rule otherwise.

`refuted` and `unresolvable` are therefore never ticketable, and the schema
plus `scripts/validate.sh verdicts` reject a verdict that claims otherwise
rather than leaving the rule to the controller's diligence. The two are not
interchangeable: `refuted` asserts the finding is wrong and counts against the
audit's own error rate, while `unresolvable` asserts only that the method could
not reach the claim. Collapsing them would put "How it was refuted" in
`REPORT.md` above a claim nobody refuted, and inflate the error rate with
findings that may well be true.

Selecting only `verdict == "confirmed"` — which the plan text originally did —
silently drops every downgraded-but-real finding. On the real run that was 11
of 54. For `confirmed_lower_severity` always use the verdict's
`corrected_severity`, never the finding's own claimed severity.

**Confidence bars a finding from being the basis of its own ticket, not from
riding along in one.** `suspected` findings do not become tickets. They may be
folded into a ticket that exists on other evidence — the real run did this
twice, and disclosed it in both ticket headers. Neither the generator nor the
gate enforces this, so it must be adjudicated in `ADJUDICATIONS.md` **before**
filing. An exception no artifact records is indistinguishable from a mistake.

## Filing to Linear

The MCP endpoint at `mcp.linear.app/mcp` speaks `text/event-stream`; parse
responses with `grep '^data: ' | sed 's/^data: //'` or the equivalent. Facts
worth not rediscovering:

- The create/update tool is **`save_issue`** — there is no `create_issue`.
  Omitting `id` creates; passing `id` updates. **There is no delete tool** —
  a mistakenly-created issue can only be canceled from the API and deleted from
  the UI.
- The team parameter is **`team`**, not `teamId`.
- Priority is `0=None 1=Urgent 2=High 3=Medium 4=Low`.
- Read the OAuth token from `~/.claude/.credentials.json` into a variable.
  Never print, echo, or write it anywhere.

The creation loop appends one JSON line per created ticket to a done-file and
skips anything already recorded, so a mid-run failure is re-runnable without
duplicates. IDs land in `linear-ids.json`, which `build-tickets.py` reads back
so `TICKETS.md` links to what was actually filed.

### Linear's markdown parser corrupts ticket bodies silently

Found by round-tripping every issue back out of Linear after creation and
diffing against what was sent. Eleven of 35 bodies came back different; two of
the differences were data loss:

| Construct | What Linear stores | Consequence |
|---|---|---|
| a backslash in **raw prose** | the backslash is eaten as a markdown escape | `grep 'A\B'` (backslash-pipe alternation) becomes `grep 'A|B'` — a BRE alternation silently turns into a literal pipe, so the acceptance criterion matches nothing and reads as "already fixed" |
| a multi-line string in **single backticks** | dropped entirely | a 15-line reproduce snippet vanished from DUP-003 |
| a backtick inside a backtick-wrapped string | mangled spacing | cosmetic |
| `file.md:137` in prose | autolinked | cosmetic |

Probed against a throwaway issue, the survival rules are:

- a backslash **inside an inline code span** survives as-is;
- a backslash **in raw prose** is eaten — double it;
- a **fenced block** survives verbatim, backslashes included;
- do **not** indent a fence to match its list item — Linear preserves the
  leading whitespace, and an indented `python3 -c "…"` snippet pastes back as
  an `IndentationError`.

`build-tickets.py` implements this in `prose()` (escape backslashes only
outside code spans) and `code()` (fence anything multi-line or containing a
backtick). **Verify by round-trip, not by assuming the POST succeeded**: assert
that every evidence command, `file:line`, blast-radius entry, and acceptance
criterion appears verbatim in the description Linear hands back.

## Final verification

Prove each of these, don't assert them:

| Check | Command / invariant |
|---|---|
| No code changed | `git diff --name-only main..HEAD -- internal/ cmd/ web/src/` is empty; same for `git status --porcelain` |
| Baseline holds | `git merge-base --is-ancestor <baseline> HEAD` |
| Every finding verified | findings count == verdicts count, no ID in one and not the other |
| Every ticketable finding covered | `ticketable − covered` is empty |
| Nothing covered that shouldn't be | `covered − ticketable` is empty except explicit adjudications |
| Every fix unit filed | manifest FUs == `linear-ids.json` keys |
| No mechanical finding skipped the protocol | every `confidence: mechanical` finding has a verdict |

## What to keep if you change nothing else

1. **Adversarially verify the mechanical tier.** Both reversals on the real run
   were in the tier that looked safest.
2. **Make every agent declare its searched universe**, and treat a narrow one
   as grounds for escalation rather than confidence.
3. **Gate for truncation between phases.** A stalled agent returns valid JSON.
4. **Generate the long mechanical artifacts with a program**, not an agent.
5. **Round-trip anything you write to an external system** before believing it
   landed.
6. **Never accept an agent's self-reported count.** Every tally in this audit
   was re-derived by the controller with `jq`/Python from the artifacts on
   disk. They matched — which is the point: you only know that because you
   checked.
