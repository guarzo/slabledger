# SlabLedger Tech-Debt Audit

**Baseline revision:** `740976ecf80a4f2ccdaa611d7790ccaa95b48773`
**Design:** `docs/specs/2026-08-07-codebase-tech-debt-audit-design.md`
**Plan:** `docs/plans/2026-08-08-codebase-tech-debt-audit.md`

Read-only audit. No code was changed by this process; tickets are the output.

| Directory | Contents |
|---|---|
| `METHODOLOGY.md` | **How to run this audit again** — topology, gates, failure modes |
| `PREAMBLE.md` | Rules prepended to every agent prompt |
| `LENS-BRIEF.md` | Phase 2 lens instructions, including the three traps |
| `VERIFIER-BRIEF.md` | Phase 3 adversarial-verification instructions |
| `ADJUDICATIONS.md` | Controller rulings; binding over any finding or verdict |
| `schema/scout-report.schema.json` | Phase 1 map shape — closed record `kind` and key vocabularies |
| `schema/finding.schema.json` | Phase 2 finding shape — pins the baseline revision, requires the nine named runtime checks on `mechanical` |
| `schema/verdict.schema.json` | Phase 3 verdict shape — keeps prose out of `corrected_severity` |
| `scripts/validate.sh` | Completeness gate validator — `validate.sh {scout\|findings\|verdicts} <file.json>` |
| `scripts/build-report.py` | `REPORT.md` generator (holds the cluster map) — see *Regenerating* below |
| `scripts/build-tickets.py` | `TICKETS.md` generator (reuses that map) — same caveat |
| `maps/` | Phase 1 reference maps — **deleted from the tree**, see *The Phase 1 maps* below |
| `findings/` | Phase 2 lens findings |
| `verdicts/` | Phase 3 adversarial verdicts |
| `REPORT.md` | Consolidated ranked findings |
| `TICKETS.md` | Ticket bodies as filed |
| `linear-ids.json` | Fix unit → filed Linear issue |

## The Phase 1 maps

Both runs' `maps/*.json` — 186,198 lines, 9.8 MB, ~90% of this directory by
volume — were removed from the working tree. They are Phase 1 *data*: a
mechanical inventory of every Go/frontend/db/config/doc identity and its
reference count at the baseline revision. Every judgment drawn from them was
already extracted into `findings/`, `verdicts/`, `REPORT.md` and `TICKETS.md`,
which are retained in full. Nothing that a reader needs was in the maps alone.

They are not regenerable by any script here — Phase 1 is five subagent scouts
(see `METHODOLOGY.md`). But they are permanently recoverable from git:

```bash
# 2026-08-07 run
git show 9ba0643d:docs/audit/maps/go-reference-map.json > /tmp/go-reference-map.json

# 2026-08-08 run
git show 4c5d1b42:docs/audit/runs/2026-08-08/maps/db-map.json > /tmp/db-map.json

# or restore a whole tree
git checkout 9ba0643d -- docs/audit/maps
```

**This matters for open tickets.** The `Reproduce:` line in many ticket bodies is
a `jq` query against `docs/audit/maps/*.json`, and those bodies are live in
Linear. If you are working such a ticket, restore the map to `/tmp` with the
command above and point the `jq` at it — the query itself is unchanged. Re-running
the underlying `command` against the baseline revision also still works and is the
more authoritative check.

## Regenerating

The generators are deterministic for a given version of themselves: same inputs,
same bytes out. They are **not** deterministic across versions, and both filed
runs sit on the far side of one. Commit `4c5d1b42` repaired four rendering
defects the generators carried when the 2026-08-07 and 2026-08-08 artifacts were
produced, so re-running `build-report.py` or `build-tickets.py` over either run's
inputs today yields a different rendering than the committed file.

That is expected. `REPORT.md`, `TICKETS.md`, `ticket-manifest.json` and
`linear-ids.json` are the record of what was reported and filed to Linear, so
they are left exactly as filed; the repairs apply from the next run onward. The
run-2 controller quantified the difference — `runs/2026-08-08/CONTROLLER-NOTES.md`,
under "Deferred / not done".

**Do not "fix" a filed artifact by regenerating it.** If you need the current
rendering, generate into a scratch directory via `$AUDIT_RUN`.

## How to read a finding

`confidence: mechanical` means every element of the evidence protocol was
satisfied and all nine runtime-reachability checks were answered. `strong`
means one or more checks are unresolved — the finding names which. `suspected`
means it is recorded but deliberately **not** ticketed.

Absence claims cite a command and its empty output, never a `file:line`.
See the case study in the design doc for why this distinction is enforced.

## Reading recorded `output`

An `output` field is the historical record of what a command returned at the
baseline revision — it is not always the byte-verbatim stdout. Two conventions
recur, and both are deliberate:

- `(no output — zero matches)` stands in for an empty result. A literally empty
  string would be indistinguishable from a field nobody filled in, which is the
  failure mode this whole protocol exists to prevent.
- `:NN: (doc comment)` elides the body of a matched comment line, keeping the
  line number that makes the hit reproducible while dropping prose that would
  bloat the record.

Anything longer than a few lines is truncated with an explicit marker. To check
a claim, re-run its `command` rather than diffing its `output` — and re-run it
against the baseline revision, since the tree has moved on since.
