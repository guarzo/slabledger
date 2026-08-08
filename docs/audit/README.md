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
| `schema/` | JSON Schemas for scout reports and findings |
| `scripts/validate.sh` | Completeness gate validator |
| `scripts/build-report.py` | Deterministic `REPORT.md` generator (holds the cluster map) |
| `scripts/build-tickets.py` | Deterministic `TICKETS.md` generator (reuses that map) |
| `maps/` | Phase 1 reference maps (data, not judgments) |
| `findings/` | Phase 2 lens findings |
| `verdicts/` | Phase 3 adversarial verdicts |
| `REPORT.md` | Consolidated ranked findings |
| `TICKETS.md` | Ticket bodies as filed |
| `linear-ids.json` | Fix unit → filed Linear issue |

## How to read a finding

`confidence: mechanical` means every element of the evidence protocol was
satisfied and all nine runtime-reachability checks were answered. `strong`
means one or more checks are unresolved — the finding names which. `suspected`
means it is recorded but deliberately **not** ticketed.

Absence claims cite a command and its empty output, never a `file:line`.
See the case study in the design doc for why this distinction is enforced.
