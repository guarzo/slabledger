# Run prompt — 2026-08-08

The instruction that produced this run, reproduced **verbatim** as sent. It is
kept because the run's parameters (baseline pinning, run-scoped output, the
prior-findings rule, the non-negotiables, the Linear preconditions) are not
recoverable from the artifacts alone, and the next run is expected to be a
variation on this text rather than a fresh invention.

Two things are deliberately *not* edited out:

- The reading list appears twice. That is how it was sent; fixing it here would
  make this file a paraphrase instead of a record.
- The destination team is not named. It was withheld on purpose and supplied in
  a second turn — `slabledger is the team, go ahead and create them` — after the
  controller stopped and asked, per the last section.

Standing process lives in `docs/audit/METHODOLOGY.md`; this file is the record of
one invocation, not a spec.

---

Re-run the read-only tech-debt audit against the current tip of origin/main.

The process is already written down. Read these first, in this order, and follow
them rather than inventing a method:

1. `docs/audit/METHODOLOGY.md` — the operating manual. Topology, gates, and the
   specific ways this process tried to fail last time. This is authoritative.
2. `docs/audit/README.md` — directory map and how to read a finding.
3. `docs/audit/PREAMBLE.md` — standing constraints. Prepend verbatim to every
   agent dispatch prompt.
4. `docs/audit/LENS-BRIEF.md` and `docs/audit/VERIFIER-BRIEF.md` — the Phase 2
   and Phase 3 agent prompts.
5. `docs/audit/schema/*.json` and `docs/audit/scripts/validate.sh` — the
   completeness gates.


1. `docs/audit/METHODOLOGY.md` — the operating manual. Topology, gates, and the
   specific ways this process tried to fail last time. This is authoritative.
2. `docs/audit/README.md` — directory map and how to read a finding.
3. `docs/audit/PREAMBLE.md` — standing constraints. Prepend verbatim to every
   agent dispatch prompt.
4. `docs/audit/LENS-BRIEF.md` and `docs/audit/VERIFIER-BRIEF.md` — the Phase 2
   and Phase 3 agent prompts.
5. `docs/audit/schema/*.json` and `docs/audit/scripts/validate.sh` — the
   completeness gates.

The design doc (`docs/specs/2026-08-07-codebase-tech-debt-audit-design.md`) and
the task-by-task plan (`docs/plans/2026-08-08-codebase-tech-debt-audit.md`) are
background; read them only if the methodology leaves something ambiguous.

## Parameters for this run

- **Baseline revision:** run `git fetch origin` first, then pin to
  `git rev-parse origin/main`. Resolve it once at the start, record the full SHA
  in this run's README and in every finding's baseline field, and verify
  `git merge-base --is-ancestor <baseline> HEAD` at the end. Every scout, lens,
  and verifier reads that pinned revision — not the working tree, which may have
  drifted mid-run.
- **Output directory:** `docs/audit/runs/<baseline-date>/`, where
  `<baseline-date>` is the commit date of the baseline in `YYYY-MM-DD` form
  (`git show -s --format=%cs <baseline>`). This run's `maps/`, `findings/`,
  `verdicts/`, `ADJUDICATIONS.md`, `REPORT.md`, `TICKETS.md`, and
  `linear-ids.json` go there. Do NOT overwrite any earlier run's artifacts —
  they are the record of what was filed against which baseline. The process
  documents (`METHODOLOGY.md`, `PREAMBLE.md`, the two briefs, `schema/`,
  `scripts/`) stay where they are and are shared; they are baseline-independent
  by design. Parameterize the two generator scripts on the run directory rather
  than copying them. Note that the first run (2026-08-07, tickets SLA-9…SLA-43)
  wrote flat into `docs/audit/` — treat those top-level `REPORT.md`,
  `TICKETS.md`, and `linear-ids.json` as that run's artifacts.
- **Prior findings are context, not exclusions.** Before Phase 4, read the
  `REPORT.md` and `linear-ids.json` of every previous run and check the current
  state of the tickets they filed. A finding that reproduces at this baseline
  and whose ticket is still open is a duplicate — note the SLA id in
  `ADJUDICATIONS.md` and do not re-file it. A finding that reproduces and whose
  ticket was closed is a regression and IS ticketable, flagged as such. Do not
  let prior reports bias Phases 1–3: the scouts and lenses must not read them,
  or they will re-derive last run's conclusions instead of the code.

## Non-negotiables (from METHODOLOGY, restated so they don't get lost)

- Strictly read-only. Tickets are the only output. Prove it at the end:
  `git diff --name-only <baseline>..HEAD -- internal/ cmd/ web/src/` empty, and
  `git status --porcelain` clean of source changes.
- No database access. No connections, no SQL, no migrations — static analysis of
  migration files only.
- No secret values. Never read a real `.env`. Variable NAMES only.
- Enumerate with `git ls-files` at the pinned baseline, never `ls`/`find`. This
  repo keeps active worktrees under `.worktrees/` and `.claude/worktrees/`, and
  `find` will happily treat every one of them as source.
- Run `scripts/validate.sh` between every phase. A stalled agent returns valid
  JSON covering 60% of the repo, and that is invisible by inspection.
- Every finding declares its `searched_universe`. Treat a narrow universe as
  grounds to escalate to Phase 3, never as grounds for confidence.
- One verdict per finding, no exceptions, including the ones you expect to
  survive. Verifiers default to REFUTED under uncertainty and re-derive rather
  than audit the finding's reasoning. Adversarially verify the mechanical tier
  hardest — both reversals last run were there.
- Generate `REPORT.md` and `TICKETS.md` with `build-report.py` /
  `build-tickets.py`. Do not dispatch an agent to write them; two died trying
  last time, having written nothing.
- Re-derive every count yourself with jq/Python from the artifacts on disk.
  Never accept an agent's self-reported tally.

## Before filing anything to Linear

Stop and confirm the destination team with me. Then apply the ticketability rule
(`confirmed` OR `confirmed_lower_severity`, verdict's `ticketable` true, no
contrary adjudication) and the Linear markdown escaping rules in METHODOLOGY —
and round-trip every created issue back out of Linear, diffing against what was
sent, before declaring the batch filed.
