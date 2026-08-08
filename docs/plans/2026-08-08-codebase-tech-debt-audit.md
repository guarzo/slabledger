# Codebase Tech-Debt Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Note on plan shape.** This plan orchestrates an *analysis process*, not a code change. Most tasks dispatch agents and end in a **gate check** rather than a unit test — the gate plays the role the test plays elsewhere. Task 2 is the exception: it builds a real validator script and follows a normal TDD cycle.

**Goal:** Run a fleet of read-only agents that audit the SlabLedger codebase for tech debt, verify every finding adversarially, and file one Linear ticket per independently-shippable fix unit.

**Architecture:** Four phases on branch `tech-debt-audit`. Phase 0 (ground truth) is complete and recorded in the spec. Phase 1 dispatches five *scouts* that emit reference maps as data, never judgments. A completeness gate then blocks Phase 2 until those maps are provably whole. Phase 2 runs eight *lens auditors* with exclusive subject ownership, each consuming the maps rather than re-deriving reachability. Phase 3 hands every finding to an adversarial verifier prompted to refute it. Phase 4 dedups, clusters, ranks, and files tickets.

**Tech Stack:** Go 1.26 (read-only analysis), `git ls-files`/`git grep` as the evidence substrate, `jq` for schema validation, Linear MCP for ticket creation.

**Source spec:** `docs/specs/2026-08-07-codebase-tech-debt-audit-design.md`

## Global Constraints

Every task's requirements implicitly include this section. These are copied verbatim from the spec and are binding.

- **Strictly read-only. No code changes.** Tickets and documents are the only output. No agent may edit, create, or delete anything under `internal/`, `cmd/`, or `web/src/`. Final verification asserts this mechanically.
- **Baseline revision is `740976ecf80a4f2ccdaa611d7790ccaa95b48773`.** Every finding records it. Agents MUST NOT re-derive baseline metrics by other means; the baseline table in the spec is authoritative.
- **Git-tracked files only.** Enumerate with `git ls-files`. `ls` and `find` are **forbidden as evidence** — they surface scratch files, build artifacts, and ignored directories. Violating this rule is what produced the false `marketmovers` finding documented in the spec's case study.
- **No agent may assert reachability from its own search.** Reachability is read from the Phase 1 maps, which are computed once.
- **Absence is never proven by `file:line`.** A citation proves presence only. Absence claims cite the *command and its empty output* at the baseline revision.
- **A removal claim requires the full evidence protocol** (spec § Evidence protocol): subject identity, searched universe, exclusions, tag matrix (default **and** `integration`, plus `_test.go`), and explicit answers to all nine runtime-reachability checks. Anything short of that is `confidence: suspected` — recorded, never ticketed.
- **`confidence: mechanical`** is the only tier eligible for an unqualified ticket, and requires every protocol element.
- **A non-empty finding set is not a success criterion.** "No debt found in area X, here is the search that established it" is a valid, useful outcome. Agents must not manufacture findings to appear productive.
- **Lens ownership is exclusive.** Cross-kind observations are filed as a pointer to the owning lens, never as a competing finding.
- **Baseline facts** (do not recompute): 70,280 non-test Go LOC · 61,378 test LOC · 676 Go files · 53 packages (4 untested) · 218 frontend files · 27,759 frontend LOC · 25 migrations · 71 env vars · 6 `t.Skip` calls · 1 TODO marker.

---

## File Structure

All audit output lives under `docs/audit/` on branch `tech-debt-audit`. Nothing else in the repo is touched.

| Path | Responsibility |
|---|---|
| `docs/audit/README.md` | Orientation: what this is, how to read it, baseline revision |
| `docs/audit/PREAMBLE.md` | The shared rules block prepended to **every** agent prompt |
| `docs/audit/schema/finding.schema.json` | JSON Schema for a finding record |
| `docs/audit/schema/scout-report.schema.json` | JSON Schema for a scout report |
| `docs/audit/scripts/validate.sh` | Gate validator: schema + count reconciliation + provenance |
| `docs/audit/maps/*.json` | Phase 1 scout output (5 files) |
| `docs/audit/findings/*.json` | Phase 2 lens output (8 files) |
| `docs/audit/verdicts/*.json` | Phase 3 verifier output |
| `docs/audit/REPORT.md` | Phase 4 consolidated, ranked findings |
| `docs/audit/TICKETS.md` | Ticket bodies as filed, with Linear IDs |

---

## Task 1: Scaffold the audit directory and shared preamble

The preamble is the highest-leverage artifact in this plan. Every agent prompt begins with it, so the evidence protocol is enforced once rather than restated thirteen times with drift.

**Files:**
- Create: `docs/audit/README.md`
- Create: `docs/audit/PREAMBLE.md`
- Create: `docs/audit/schema/finding.schema.json`
- Create: `docs/audit/schema/scout-report.schema.json`

**Interfaces:**
- Produces: `PREAMBLE.md` (prepended to every agent prompt in Tasks 3–12); `finding.schema.json` (the contract Tasks 5–12 emit against); `scout-report.schema.json` (the contract Task 3 emits against).

- [ ] **Step 1: Create the directory structure**

```bash
cd /workspace/.worktrees/tech-debt-audit
mkdir -p docs/audit/{schema,scripts,maps,findings,verdicts}
```

- [ ] **Step 2: Write `docs/audit/PREAMBLE.md`**

```markdown
# Audit Agent Preamble — READ FULLY BEFORE ANY TOOL CALL

You are one agent in a read-only tech-debt audit of the SlabLedger codebase.

## Absolute rules

1. **READ-ONLY.** You must not edit, create, or delete any file under
   `internal/`, `cmd/`, or `web/src/`. You write exactly one output file, at
   the path your task names. Nothing else.
2. **Baseline revision is `740976ecf80a4f2ccdaa611d7790ccaa95b48773`.**
   Record it on every record you emit.
3. **Git-tracked files only.** Enumerate with `git ls-files`. Do NOT use `ls`
   or `find` as evidence. They surface untracked scratch files, build
   artifacts, and ignored directories that are not part of the codebase.
4. **Never assert reachability from your own search.** Reachability comes from
   the Phase 1 maps in `docs/audit/maps/`. If you need it and it is not there,
   report that as a gap — do not compute it yourself.
5. **Absence is never proven by a `file:line` citation.** A citation proves
   presence. To claim something is unused, cite the *command you ran and its
   empty output*.

## Why these rules exist

An earlier draft of this audit's design claimed
`internal/adapters/clients/marketmovers/` was orphaned dead code. It was false:
that path is not a Go package and is not in git — it was a single untracked
scratch file in one developer's checkout, surfaced by `ls`. Two baseline counts
were also wrong, because `grep up` matched "su**p**abase" and `grep t.Skip`
matched `result.Skipped`.

Three false positives, in one session, from the very method the document was
recommending. Substring matching is not evidence of reachability. You are
working under a strict protocol because the unstructured version demonstrably
fails.

## Go defeats naive reference counting — check all nine

This repository actively uses every mechanism below. Before claiming anything
is unused, answer each explicitly:

| # | Check | Example in this repo |
|---|---|---|
| 1 | Interface satisfaction | `internal/domain/arbitrage/service.go:80` — anonymous interface assertion |
| 2 | Functional options | `internal/domain/pricing/lookup/adapter.go:65` — `WithRequestCache`; deleting it still compiles, behavior silently degrades |
| 3 | Struct embedding | promoted methods have no direct call site |
| 4 | Serialization tags | `internal/domain/liquidation/types.go:23` — JSON tag is a runtime API contract with no Go caller |
| 5 | `//go:embed` | `internal/adapters/storage/postgres/embedded_migrations.go:9` — migrations have zero Go references |
| 6 | Build tags | `internal/integration/cardladder_test.go:4` — `//go:build integration`, excluded from default builds |
| 7 | Registration / DI wiring | `cmd/slabledger/init_services.go`, scheduler builders, `routes.go` |
| 8 | `init()` side effects | package imported only for effect |
| 9 | Reflection / string-keyed lookup / string-built SQL | table and column names as string literals |

## Search discipline

- Word-boundary anchored: `git grep -nE '\bSymbolName\b'`, never a bare substring.
- Search both tag sets: default, and `integration`.
- Include `_test.go` files — test-only usage is still usage, and is itself a finding of a different kind.
- State your searched universe (the `git ls-files` globs) and every exclusion.

## Confidence tiers

- `mechanical` — every element of the evidence protocol satisfied, all nine
  runtime checks answered. Only this tier may be ticketed unqualified.
- `strong` — compelling evidence, one or more checks unresolved. Say which.
- `suspected` — worth recording, not worth a ticket.

Downgrade under uncertainty. A demoted true finding costs little; a promoted
false one costs a reviewer's trust in the entire audit.

## Output discipline

- Emit valid JSON against your task's schema. Nothing else in the file.
- **Finding nothing is a valid, useful result.** Do not manufacture findings to
  appear productive. "No debt in area X, here is the search that establishes
  it" is a real deliverable.
- Restating `golangci-lint` output is not a finding. Lint is clean at baseline;
  your job is what tooling cannot see.
- Stay in your lane. If you spot something owned by another lens, emit it as a
  `pointer` record, not a finding.
```

- [ ] **Step 3: Write `docs/audit/schema/finding.schema.json`**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Audit finding",
  "type": "object",
  "required": ["id", "lens", "revision", "subject", "category", "title",
               "severity", "confidence", "evidence", "searched_universe",
               "proposed_fix", "blast_radius", "effort", "acceptance_criteria"],
  "properties": {
    "id":       {"type": "string", "pattern": "^[A-Z]+-[0-9]{3}$"},
    "lens":     {"type": "string"},
    "revision": {"type": "string", "const": "740976ecf80a4f2ccdaa611d7790ccaa95b48773"},
    "subject": {
      "type": "object",
      "required": ["kind", "identity"],
      "properties": {
        "kind":     {"enum": ["symbol", "package", "table", "column", "index",
                              "env", "config", "component", "hook", "type",
                              "route", "claim", "test", "migration"]},
        "identity": {"type": "string", "minLength": 1}
      }
    },
    "category": {"enum": ["dead-code", "naming", "architecture", "duplication",
                          "size", "db", "frontend", "docs"]},
    "title":      {"type": "string", "minLength": 1},
    "severity":   {"enum": ["high", "medium", "low"]},
    "confidence": {"enum": ["mechanical", "strong", "suspected"]},
    "evidence": {
      "type": "array", "minItems": 1,
      "items": {
        "type": "object",
        "required": ["claim", "command", "output"],
        "properties": {
          "claim":     {"type": "string"},
          "command":   {"type": "string"},
          "output":    {"type": "string"},
          "file_line": {"type": "string"}
        }
      }
    },
    "searched_universe": {"type": "string", "minLength": 1},
    "exclusions": {"type": "array", "items": {"type": "string"}},
    "build_tags": {"type": "array", "items": {"type": "string"}},
    "runtime_checks": {
      "type": "object",
      "description": "Required when confidence is mechanical. All nine answered.",
      "properties": {
        "interface_satisfaction": {"type": "string"},
        "functional_options":     {"type": "string"},
        "embedding":              {"type": "string"},
        "serialization":          {"type": "string"},
        "go_embed":               {"type": "string"},
        "build_tags":             {"type": "string"},
        "registration":           {"type": "string"},
        "init_side_effects":      {"type": "string"},
        "reflection_or_string":   {"type": "string"}
      }
    },
    "proposed_fix":        {"type": "string", "minLength": 1},
    "blast_radius":        {"type": "array", "items": {"type": "string"}},
    "effort":              {"enum": ["S", "M", "L"]},
    "acceptance_criteria": {"type": "array", "minItems": 1,
                            "items": {"type": "string"}},
    "verifier": {
      "type": "object",
      "properties": {
        "verdict":   {"enum": ["confirmed", "refuted", "uncertain"]},
        "rationale": {"type": "string"}
      }
    }
  },
  "allOf": [{
    "if":   {"properties": {"confidence": {"const": "mechanical"}}},
    "then": {"required": ["runtime_checks", "build_tags", "exclusions"]}
  }]
}
```

- [ ] **Step 4: Write `docs/audit/schema/scout-report.schema.json`**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Scout report",
  "type": "object",
  "required": ["scout", "revision", "status", "declared_totals", "records"],
  "properties": {
    "scout":    {"type": "string"},
    "revision": {"type": "string", "const": "740976ecf80a4f2ccdaa611d7790ccaa95b48773"},
    "status":   {"enum": ["complete", "partial", "failed"]},
    "declared_totals": {
      "type": "object",
      "description": "Counts this scout claims to have covered; reconciled against baseline.",
      "additionalProperties": {"type": "integer"}
    },
    "gaps":    {"type": "array", "items": {"type": "string"}},
    "records": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["identity", "kind", "defined_at", "command"],
        "properties": {
          "identity":        {"type": "string"},
          "kind":            {"type": "string"},
          "defined_at":      {"type": "string"},
          "external_refs":   {"type": "integer", "minimum": 0},
          "ref_sites":       {"type": "array", "items": {"type": "string"}},
          "build_tags":      {"type": "array", "items": {"type": "string"}},
          "registration_sites": {"type": "array", "items": {"type": "string"}},
          "command":         {"type": "string"}
        }
      }
    }
  }
}
```

- [ ] **Step 5: Write `docs/audit/README.md`**

```markdown
# SlabLedger Tech-Debt Audit

**Baseline revision:** `740976ecf80a4f2ccdaa611d7790ccaa95b48773`
**Design:** `docs/specs/2026-08-07-codebase-tech-debt-audit-design.md`
**Plan:** `docs/plans/2026-08-08-codebase-tech-debt-audit.md`

Read-only audit. No code was changed by this process; tickets are the output.

| Directory | Contents |
|---|---|
| `PREAMBLE.md` | Rules prepended to every agent prompt |
| `schema/` | JSON Schemas for scout reports and findings |
| `scripts/validate.sh` | Completeness gate validator |
| `maps/` | Phase 1 reference maps (data, not judgments) |
| `findings/` | Phase 2 lens findings |
| `verdicts/` | Phase 3 adversarial verdicts |
| `REPORT.md` | Consolidated ranked findings |
| `TICKETS.md` | Ticket bodies as filed |

## How to read a finding

`confidence: mechanical` means every element of the evidence protocol was
satisfied and all nine runtime-reachability checks were answered. `strong`
means one or more checks are unresolved — the finding names which. `suspected`
means it is recorded but deliberately **not** ticketed.

Absence claims cite a command and its empty output, never a `file:line`.
See the case study in the design doc for why this distinction is enforced.
```

- [ ] **Step 6: Commit**

```bash
git add docs/audit/
git commit -m "docs(audit): scaffold audit directory, shared preamble, and schemas"
```

---

## Task 2: Build and test the completeness-gate validator

This is the one task with real executable logic, so it gets a real TDD cycle. The validator is what makes silent scout failure impossible — without it, a truncated map makes live code look unreferenced and nothing downstream notices.

**Files:**
- Create: `docs/audit/scripts/validate.sh`
- Create: `docs/audit/scripts/fixtures/good-scout.json`
- Create: `docs/audit/scripts/fixtures/truncated-scout.json`

**Interfaces:**
- Consumes: `docs/audit/schema/scout-report.schema.json`, `finding.schema.json` (Task 1).
- Produces: `validate.sh <mode> <file>` where mode is `scout` or `findings`; exit 0 = pass, exit 1 = fail with a reason on stderr. Task 4 gates on it.

- [ ] **Step 1: Verify `jq` is available**

```bash
jq --version
```

Expected: a version string. If missing, install with `apt-get install -y jq` before proceeding.

- [ ] **Step 2: Write the failing test fixtures**

`docs/audit/scripts/fixtures/good-scout.json` — declares totals matching baseline:

```json
{
  "scout": "go-reference-map",
  "revision": "740976ecf80a4f2ccdaa611d7790ccaa95b48773",
  "status": "complete",
  "declared_totals": {"go_files": 676, "packages": 53},
  "gaps": [],
  "records": [
    {"identity": "inventory.Campaign", "kind": "symbol",
     "defined_at": "internal/domain/inventory/types_core.go:31",
     "external_refs": 42, "command": "git grep -nE '\\bCampaign\\b'"}
  ]
}
```

`docs/audit/scripts/fixtures/truncated-scout.json` — the failure the gate exists to catch (claims complete, covers 400 of 676 files):

```json
{
  "scout": "go-reference-map",
  "revision": "740976ecf80a4f2ccdaa611d7790ccaa95b48773",
  "status": "complete",
  "declared_totals": {"go_files": 400, "packages": 53},
  "gaps": [],
  "records": []
}
```

- [ ] **Step 3: Run the validator to verify it fails (it does not exist yet)**

```bash
bash docs/audit/scripts/validate.sh scout docs/audit/scripts/fixtures/good-scout.json
```

Expected: FAIL — `No such file or directory`.

- [ ] **Step 4: Write `docs/audit/scripts/validate.sh`**

```bash
#!/usr/bin/env bash
# Completeness gate validator for the tech-debt audit.
# Usage: validate.sh {scout|findings} <file.json>
# Exit 0 = pass. Exit 1 = fail, reason on stderr.
set -uo pipefail

BASELINE_REV="740976ecf80a4f2ccdaa611d7790ccaa95b48773"
MODE="${1:-}"
FILE="${2:-}"

fail() { echo "GATE FAIL: $*" >&2; exit 1; }

[ -n "$MODE" ] && [ -n "$FILE" ] || fail "usage: validate.sh {scout|findings} <file.json>"
[ -f "$FILE" ] || fail "no such file: $FILE"
jq empty "$FILE" 2>/dev/null || fail "$FILE is not valid JSON"

# Baseline facts. Authoritative; see the spec's baseline table.
declare -A BASELINE=( [go_files]=676 [packages]=53 [migrations]=25
                      [frontend_files]=218 [env_vars]=71 )

if [ "$MODE" = "scout" ]; then
  rev=$(jq -r '.revision // ""' "$FILE")
  [ "$rev" = "$BASELINE_REV" ] || fail "revision is '$rev', expected $BASELINE_REV"

  status=$(jq -r '.status // ""' "$FILE")
  case "$status" in
    complete) ;;
    partial|failed) fail "scout status is '$status' — dependent lenses are blocked" ;;
    *) fail "scout status missing or invalid: '$status'" ;;
  esac

  # Count reconciliation: any declared total that names a baseline key must match.
  while IFS=$'\t' read -r key val; do
    [ -n "${BASELINE[$key]:-}" ] || continue
    [ "$val" = "${BASELINE[$key]}" ] || \
      fail "declared_totals.$key = $val, baseline says ${BASELINE[$key]}"
  done < <(jq -r '.declared_totals | to_entries[] | "\(.key)\t\(.value)"' "$FILE")

  # Provenance: every record must carry the command that produced it.
  missing=$(jq '[.records[] | select((.command // "") == "")] | length' "$FILE")
  [ "$missing" = "0" ] || fail "$missing record(s) missing provenance 'command'"

  n=$(jq '.records | length' "$FILE")
  echo "GATE PASS: scout $(jq -r .scout "$FILE") — $n records, totals reconciled"

elif [ "$MODE" = "findings" ]; then
  bad_rev=$(jq --arg r "$BASELINE_REV" \
    '[.[] | select(.revision != $r)] | length' "$FILE")
  [ "$bad_rev" = "0" ] || fail "$bad_rev finding(s) have the wrong revision"

  # A mechanical claim must answer all nine runtime checks.
  weak=$(jq '[.[] | select(.confidence == "mechanical")
              | select((.runtime_checks // {} | keys | length) < 9)] | length' "$FILE")
  [ "$weak" = "0" ] || \
    fail "$weak mechanical finding(s) answer fewer than 9 runtime checks"

  # Absence must be proven by command output, never by a bare citation.
  noev=$(jq '[.[] | select([.evidence[] | select((.command // "") == "")] | length > 0)] | length' "$FILE")
  [ "$noev" = "0" ] || fail "$noev finding(s) have evidence without a command"

  n=$(jq 'length' "$FILE")
  echo "GATE PASS: findings — $n records, all evidence has provenance"
else
  fail "unknown mode: $MODE"
fi
```

- [ ] **Step 5: Run the validator against both fixtures**

```bash
chmod +x docs/audit/scripts/validate.sh
bash docs/audit/scripts/validate.sh scout docs/audit/scripts/fixtures/good-scout.json
echo "good exit: $?"
bash docs/audit/scripts/validate.sh scout docs/audit/scripts/fixtures/truncated-scout.json
echo "truncated exit: $?"
```

Expected: good → `GATE PASS` and exit 0. Truncated → `GATE FAIL: declared_totals.go_files = 400, baseline says 676` and exit 1.

**If the truncated fixture passes, stop.** The gate is the only thing standing between a silently truncated map and a queue of confident false findings. Fix it before continuing.

- [ ] **Step 6: Commit**

```bash
git add docs/audit/scripts/
git commit -m "docs(audit): add completeness-gate validator with fixtures"
```

---

## Task 3: Dispatch the five Phase 1 scouts

Scouts emit data, never judgments. Dispatch all five **in parallel** — a single message with five Agent calls — since they share no dependencies.

**Files:**
- Create: `docs/audit/maps/go-reference-map.json`
- Create: `docs/audit/maps/frontend-reference-map.json`
- Create: `docs/audit/maps/db-map.json`
- Create: `docs/audit/maps/config-map.json`
- Create: `docs/audit/maps/docs-map.json`

**Interfaces:**
- Consumes: `docs/audit/PREAMBLE.md`, `schema/scout-report.schema.json` (Task 1).
- Produces: five scout reports conforming to `scout-report.schema.json`, consumed by Tasks 5–12.

- [ ] **Step 1: Dispatch all five scouts in one message**

Each prompt is the literal contents of `docs/audit/PREAMBLE.md`, followed by the scout block below. Use `subagent_type: "general-purpose"`, `run_in_background: false`.

**Scout A — `go-reference-map`:**

```
You are the `go-reference-map` scout. Emit DATA ONLY — no judgments, no
findings, no opinions about whether anything should be removed.

Produce `docs/audit/maps/go-reference-map.json` conforming to
`docs/audit/schema/scout-report.schema.json`.

For every exported symbol in every tracked Go package, record:
  - identity: fully-qualified (e.g. `inventory.Campaign`)
  - kind: type|func|method|const|var|interface
  - defined_at: path:line
  - external_refs: count of references OUTSIDE its own package
  - ref_sites: up to 10 example path:line references
  - build_tags: tag sets under which references were found
  - registration_sites: any reference in cmd/, init_services.go, scheduler
    builders, or routes.go — these indicate DI wiring
  - command: the exact command producing external_refs

Enumerate with `git ls-files '*.go'`. Search word-boundary anchored
(`git grep -nE '\bName\b'`). Search BOTH default and `integration` build tags.
Include `_test.go` files, and mark refs that are test-only.

declared_totals MUST include: {"go_files": <count>, "packages": <count>}
from `git ls-files '*.go' | wc -l` and `go list ./... | wc -l`.

If you cannot cover everything, set status to "partial" and list what you
missed under "gaps". Do NOT claim "complete" for partial coverage — the gate
checks your totals against the baseline and a false claim fails the phase.
```

**Scout B — `frontend-reference-map`:** same preamble; inventory every component, hook, and exported type under `git ls-files 'web/src/*.ts' 'web/src/*.tsx'`, with reference counts, the route table, and API call sites. `declared_totals` includes `{"frontend_files": <count>}`.

**Scout C — `db-map`:** same preamble; from the 25 files matching `git ls-files '*/migrations/*.up.sql'`, inventory every table, column, and index, mapped to reading/writing Go code — **including string-literal SQL**, since table names appear as strings. Record which migration created each object and whether a later migration dropped it. `declared_totals` includes `{"migrations": 25}`.

**Scout D — `config-map`:** same preamble; every field in `internal/platform/config/types.go` and every var in `.env.example`, mapped to consumption sites. Flag config fields with no consumer, and `.env.example` entries with no config field. `declared_totals` includes `{"env_vars": 71}`.

**Scout E — `docs-map`:** same preamble; extract every *factual claim* about code from `CLAUDE.md`, `internal/README.md`, and `docs/*.md` — each with the code location that confirms or refutes it. Record `claim`, `doc_location`, `code_location`, and `status` (confirmed/refuted/unverifiable). Known refuted claims to include: `CLAUDE.md:137` (DH "sole price source" vs. live CardLadder) and `CLAUDE.md:34-58` (omits `demand`, `dhevents`, `dhpricing`, `liquidation`, `psacampaign`).

- [ ] **Step 2: Confirm all five map files exist**

```bash
ls -1 docs/audit/maps/*.json | wc -l
```

Expected: `5`. If fewer, re-dispatch the missing scouts before proceeding.

- [ ] **Step 3: Commit**

```bash
git add docs/audit/maps/
git commit -m "docs(audit): Phase 1 scout reference maps"
```

---

## Task 4: Run the completeness gate

Nothing in Phase 2 may start until this passes. A gate failure stops the phase — partial fleet output is never merged.

**Files:**
- Modify: `docs/audit/README.md` (append gate results)

**Interfaces:**
- Consumes: `validate.sh` (Task 2), the five maps (Task 3).
- Produces: a pass/fail decision gating Tasks 5–12.

- [ ] **Step 1: Validate every scout report**

```bash
for f in docs/audit/maps/*.json; do
  echo "=== $f ==="
  bash docs/audit/scripts/validate.sh scout "$f" || echo ">>> BLOCKED"
done
```

Expected: `GATE PASS` for all five.

- [ ] **Step 2: Sampled independent recomputation**

Dispatch one verifier agent (`general-purpose`, preamble + block below):

```
Independently recompute a random sample of 10 records from each file in
docs/audit/maps/. For each sampled record, re-run the reference count yourself
using your own word-boundary-anchored `git grep` over `git ls-files` output.

Report a table: identity | scout's count | your count | AGREE/DISAGREE.

Do not fix anything. Report disagreements only. Any disagreement fails the
gate and forces a scout rerun.
```

Expected: all AGREE. **Any disagreement fails the gate** — rerun the offending scout, then repeat Steps 1–2.

- [ ] **Step 3: Record the gate outcome**

Append to `docs/audit/README.md`:

```markdown
## Completeness gate

Passed <DATE>. Five scouts reported `complete`; declared totals reconciled
against the baseline; 10-record sample per map independently recomputed with
no disagreements.
```

- [ ] **Step 4: Commit**

```bash
git add docs/audit/README.md
git commit -m "docs(audit): completeness gate passed, Phase 2 unblocked"
```

---

## Tasks 5–12: Dispatch the eight lens auditors

Each lens is one task so it can be reviewed and rejected independently. Dispatch all eight **in parallel** — they share no dependencies, and each owns a disjoint subject kind.

**Files (one per task):**
- Task 5 → `docs/audit/findings/dead-code-go.json`
- Task 6 → `docs/audit/findings/naming-and-boundaries.json`
- Task 7 → `docs/audit/findings/architecture.json`
- Task 8 → `docs/audit/findings/duplication.json`
- Task 9 → `docs/audit/findings/size-and-complexity.json`
- Task 10 → `docs/audit/findings/db-schema.json`
- Task 11 → `docs/audit/findings/frontend-health.json`
- Task 12 → `docs/audit/findings/docs-config-tests.json`

**Interfaces:**
- Consumes: `PREAMBLE.md`, `finding.schema.json` (Task 1); all five maps (Task 3, gated by Task 4).
- Produces: a JSON array of findings per lens, validated by Task 13.

- [ ] **Step 1: Dispatch all eight lenses in one message**

Every prompt = literal `PREAMBLE.md` + this shared block + the lens block:

```
Read `docs/audit/maps/*.json` FIRST. Reachability comes from those maps —
never from your own search. If a map lacks what you need, record that as a gap
rather than computing it yourself.

Emit a JSON array of findings to <your output path>, each conforming to
`docs/audit/schema/finding.schema.json`.

You own exactly one subject kind. If you notice something owned by another
lens, emit a record with category set to the owning lens and confidence
"suspected" — a pointer, never a competing finding.

Assign confidence honestly:
  - "mechanical" requires ALL NINE runtime-reachability checks answered in
    `runtime_checks`. The gate rejects mechanical findings with fewer than 9.
  - "strong" — compelling but one or more checks unresolved. Name which.
  - "suspected" — recorded, not ticketed.

Every `acceptance_criteria` entry must state how a FIXER proves the change
correct (e.g. "`go build ./...` and `go test ./...` pass with the symbol
removed"). The audit itself never performs removal.

Finding nothing is a valid result. Emit `[]` with a note in your summary
describing the search that established it. Do not manufacture findings.
```

| Task | Lens | Owns | Hunts |
|---|---|---|---|
| 5 | `dead-code-go` | Go symbols/packages | Unreferenced Go code, per the full evidence protocol |
| 6 | `naming-and-boundaries` | package/symbol names | Names that no longer match behavior; code in the wrong package; concepts split or wrongly merged. Note: `internal/domain/inventory/` is 10,428 LOC — assess whether it should be decomposed |
| 7 | `architecture` | layering | What `check-imports.sh` misses: concrete types leaked through interfaces, domain logic in adapters, anemic pass-through layers |
| 8 | `duplication` | cross-cutting logic | Exact and near-duplicate logic, parallel implementations of one concept, copy-paste divergence |
| 9 | `size-and-complexity` | files/functions | God objects, over-broad interfaces. The 4 known >500-line files are a starting point, not the scope |
| 10 | `db-schema` | tables/columns/indexes/migrations | Unused schema, migrations for dropped features, `docs/SCHEMA.md` drift. **Seed:** `marketmovers_config` (created `000001:514`, RLS at `000003:169-170`, never dropped by `000021`, zero Go refs) and 5 migration lines carrying stale `sqlite.` provenance comments — promote or refute per protocol |
| 11 | `frontend-health` | TS/TSX symbols | Unused components/hooks, TS type drift vs Go JSON tags, dead routes, duplicated API patterns |
| 12 | `docs-config-tests` | doc claims, env vars, tests | Doc/code contradictions (start from `docs-map`), dead env vars (from `config-map`), the 6 `t.Skip` calls, the 4 untested packages, and the disabled scheduler tests at `internal/adapters/scheduler/price_refresh_test.go:25` |

- [ ] **Step 2: Confirm all eight finding files exist**

```bash
ls -1 docs/audit/findings/*.json | wc -l
```

Expected: `8`.

- [ ] **Step 3: Commit**

```bash
git add docs/audit/findings/
git commit -m "docs(audit): Phase 2 lens findings"
```

---

## Task 13: Validate findings and run adversarial verification

**Files:**
- Create: `docs/audit/verdicts/verdicts.json`

**Interfaces:**
- Consumes: the eight finding files (Tasks 5–12), `validate.sh` (Task 2).
- Produces: a verdict per finding, consumed by Task 14.

- [ ] **Step 1: Gate every finding file**

```bash
for f in docs/audit/findings/*.json; do
  echo "=== $f ==="
  bash docs/audit/scripts/validate.sh findings "$f" || echo ">>> REJECTED"
done
```

Expected: `GATE PASS` for all eight. A rejected file goes back to its lens; do not hand-patch it.

- [ ] **Step 2: Dispatch one verifier per finding, in parallel batches**

Prompt = `PREAMBLE.md` + this block:

```
Your job is to REFUTE the finding below. You are not a reviewer looking for
merit — you are an adversary looking for the reason it is wrong. Default to
"refuted" when uncertain.

FINDING: <the full JSON record>

For any removal claim, actively hunt the nine mechanisms in the preamble
before accepting it. The highest-yield refutations in this codebase are:
  - a functional option satisfying an interface at runtime
    (`WithRequestCache`-style) — deleting it still compiles
  - `//go:embed` assets with zero Go references
  - `//go:build integration` files excluded from default builds
  - JSON struct tags that are live API contracts with no Go caller
  - table/column names appearing only as string literals in SQL

Emit one verdict object:
{"id": "<finding id>", "verdict": "confirmed|refuted|uncertain",
 "rationale": "<what you checked and what you found>",
 "commands_run": ["..."]}

A refutation must cite the evidence that refutes it. "Seems fine" is not a
verdict.
```

- [ ] **Step 3: Aggregate verdicts and report the confirmation rate**

```bash
jq -s 'add | group_by(.verdict) | map({verdict: .[0].verdict, n: length})' \
  docs/audit/verdicts/*.json
```

A refutation rate near zero is itself suspicious — it suggests the verifiers rubber-stamped rather than adversarially checked. Spot-check three `confirmed` verdicts by hand before accepting the batch.

- [ ] **Step 4: Commit**

```bash
git add docs/audit/verdicts/
git commit -m "docs(audit): Phase 3 adversarial verdicts"
```

---

## Task 14: Consolidate, dedup, cluster, and rank

**Files:**
- Create: `docs/audit/REPORT.md`

**Interfaces:**
- Consumes: findings (Tasks 5–12) + verdicts (Task 13).
- Produces: ranked fix units, each mapping to exactly one ticket in Task 16.

- [ ] **Step 1: Merge confirmed findings**

```bash
jq -s 'add' docs/audit/findings/*.json > /tmp/all-findings.json
jq -s 'add' docs/audit/verdicts/*.json  > /tmp/all-verdicts.json
jq --slurpfile v /tmp/all-verdicts.json \
  '[.[] | . as $f | ($v[0][] | select(.id == $f.id)) as $vd
     | select($vd.verdict == "confirmed") | . + {verdict: $vd}]' \
  /tmp/all-findings.json > /tmp/confirmed.json
jq 'length' /tmp/confirmed.json
```

- [ ] **Step 2: Dedup across lenses**

Multiple lenses will surface one underlying problem from different angles — the CardLadder doc drift will appear in `docs-config-tests` and `naming-and-boundaries` at minimum. Merge into one finding, **preserving all evidence arrays**. Never drop a citation during merge.

- [ ] **Step 3: Cluster into fix units**

A fix unit is **one mergeable PR**. Several findings often collapse into one unit: "Reconcile CLAUDE.md with the real pricing architecture" covers the sole-price-source claim, the five omitted domain packages, and any stale `internal/README.md` text.

- [ ] **Step 4: Rank by risk × effort and write `REPORT.md`**

Sections: Summary (counts by category/confidence, confirmation rate) · Ranked fix units (each: title, findings rolled up, evidence, proposed fix, blast radius, effort, acceptance criteria) · Investigation tier (`uncertain` + `suspected`, explicitly not ticketed) · Refuted findings, with reasons — this section is the audit's own error rate, and it stays in the record.

- [ ] **Step 5: Commit**

```bash
git add docs/audit/REPORT.md
git commit -m "docs(audit): Phase 4 consolidated report"
```

---

## Task 15: Verify the Linear precondition

**Blocking prerequisite.** At plan-writing time the Linear MCP server was authenticated but its data tools were **not** registered in the session — only `authenticate` and `complete_authentication` were exposed, confirmed by a fresh-context probe. Phase 4 has nothing to write to until this is resolved.

**Interfaces:**
- Produces: a confirmed team ID for Task 16.

- [ ] **Step 1: Confirm the Linear data tools are present**

List available `mcp__linear_slabledger__*` tools. If only the two auth stubs appear, run `/mcp` to reconnect or restart Claude Code — tools register at connection time, not at auth time. **Do not proceed to Task 16 until real tools are available.**

- [ ] **Step 2: Enumerate teams and confirm the destination**

Call the team-listing tool. Report names, keys, and IDs to the user. The expectation is a single Slabledger team; **if more than one appears, stop and ask** rather than picking.

- [ ] **Step 3: Create one throwaway validation ticket**

Title: `TEST — audit pipeline validation (safe to delete)`. Body: a note that it validates ticket creation and can be deleted. Confirm it appears in Linear, then delete or close it.

This proves write access before 15–30 real tickets depend on it.

---

## Task 16: File the Linear tickets

**Files:**
- Create: `docs/audit/TICKETS.md`

**Interfaces:**
- Consumes: `REPORT.md` (Task 14), confirmed team ID (Task 15).

- [ ] **Step 1: Draft every ticket body into `docs/audit/TICKETS.md` first**

Write all bodies to the file **before** creating anything in Linear, so the user reviews the full set in one place and a partial failure mid-run is recoverable. Each ticket:

```markdown
### <Imperative title>

**Category:** <category> · **Confidence:** <tier> · **Effort:** S|M|L
**Baseline revision:** 740976ec

**Claim:** <what is wrong>

**Evidence:**
- `path/to/file.go:120` — <what is there>
- Command: `<cmd>` → <output, or "(no output)" for absence claims>

**Proposed fix:** <what to do>
**Blast radius:** <files/packages>

**Acceptance criteria:**
- [ ] <how the fixer proves it correct>
- [ ] `make check` passes
- [ ] `go test -race ./...` passes
```

- [ ] **Step 2: Get explicit user sign-off on the ticket set**

Show the count and the titles. Filing 15–30 tickets is outward-facing and tedious to undo, so confirm before creating — not after.

- [ ] **Step 3: Create the tickets**

One per fix unit, into the Task 15 team. Record each returned Linear ID next to its body in `TICKETS.md`.

- [ ] **Step 4: Commit**

```bash
git add docs/audit/TICKETS.md
git commit -m "docs(audit): filed tickets with Linear IDs"
```

---

## Task 17: Final verification

**Interfaces:**
- Consumes: everything.

- [ ] **Step 1: Prove the audit changed no code**

```bash
git diff --stat main..HEAD -- internal/ cmd/ web/src/
```

Expected: **empty output.** Any output is a violation of the read-only constraint and must be reverted.

- [ ] **Step 2: Confirm the baseline still holds**

```bash
make check && go test ./... 2>&1 | tail -5
```

Expected: identical to baseline — lint 0 issues, 4 file-size warnings, tests pass.

- [ ] **Step 3: Confirm every confirmed finding is accounted for**

```bash
echo "confirmed: $(jq 'length' /tmp/confirmed.json)"
echo "tickets:   $(grep -c '^### ' docs/audit/TICKETS.md)"
```

Every confirmed finding maps to exactly one ticket or is explicitly folded into another in `REPORT.md`. Reconcile any mismatch.

- [ ] **Step 4: Verify no mechanical finding skipped the protocol**

```bash
jq -s 'add | [.[] | select(.confidence == "mechanical")
        | select((.runtime_checks // {} | keys | length) < 9)] | length' \
  docs/audit/findings/*.json
```

Expected: `0`.

- [ ] **Step 5: Report completion**

State: findings by category, confirmation vs. refutation rate, tickets filed with IDs, and what was deliberately left in the investigation tier. Report the refutation rate honestly — it is the audit's own error rate and the best available evidence of whether the protocol worked.

---

## Rollback

The audit writes only `docs/audit/` and `docs/plans|specs/`. To abandon:

```bash
git checkout main
git worktree remove .worktrees/tech-debt-audit --force
git branch -D tech-debt-audit
```

Tickets already filed in Linear must be closed manually — that is the one non-reversible output, which is why Task 16 Step 2 gates on explicit sign-off.
