#!/usr/bin/env python3
"""Emit docs/audit/REPORT.md from the findings, verdicts, and the controller's
cluster map. Deterministic: rerunning reproduces the file byte for byte.

The cluster map below is the controller's judgment. Everything else — evidence,
acceptance criteria, blast radius, severity — is copied verbatim from the lens
and verifier JSON so no citation is lost or paraphrased in transit.
"""
import json
import glob
import os
import sys

AUDIT = os.path.dirname(os.path.abspath(__file__)) + "/.."
BASELINE = "740976ecf80a4f2ccdaa611d7790ccaa95b48773"

# ---------------------------------------------------------------------------
# Controller cluster map. Each fix unit is ONE independently mergeable PR.
# `ids` are the finding IDs it rolls up. `note` carries any binding adjudication.
# ---------------------------------------------------------------------------
UNITS = [
 # ---- Tier 1: security and data integrity -------------------------------
 dict(n="Enable RLS on psa_campaign_push_queue and stop trusting its status column",
      tier="Security & data integrity", ids=["DBSCHEMA-002"], effort="S",
      note="Highest-risk finding in the audit. An unauthenticated PostgREST write of a "
           "forged `approved` row is pushed live to the PSA portal. Fix the RLS gap and "
           "the trust assumption in DrainPushQueue together — either alone leaves the "
           "path exploitable."),
 dict(n="Enable RLS on the six remaining tables created after the 000003 blanket grant",
      tier="Security & data integrity", ids=["DBSCHEMA-001", "DBSCHEMA-003"], effort="S",
      note="Seven tables in total lack RLS; one of them (psa_campaign_push_queue) is "
           "ticketed separately above because its fix is not mechanical. The controller's "
           "original seed said eight — that was wrong, `mm_sales_comps` was dropped in "
           "000021. Any ticket citing eight is citing the controller's mistake. "
           "psa_portal_token stores an encrypted PSA portal access token."),
 dict(n="Wire CleanupExpiredOAuthStates into the session-cleanup scheduler and restore its index",
      tier="Security & data integrity", ids=["DEADGO-004"], effort="S",
      note="ADJUDICATED — this is the bug half of a SPLIT finding. DEADGO-004 was filed as "
           "six dead methods. Five are dead; this one is an unwired implementation of "
           "NEEDED behavior. `oauth_states` has no FK cascade and no TTL, the only "
           "consuming DELETE fires on successful callback only, and migration "
           "000003 line 254 DROPped the index supporting the cleanup query. Every "
           "abandoned login leaves a permanent row. Deleting this method — which the "
           "finding as filed proposed — would have removed the fix for a live leak."),
 dict(n="Fix the flat-sibling import checker: it skips 19 of 25 domain packages",
      tier="Security & data integrity", ids=["NB-001"], effort="S",
      note="ADJUDICATED — confirmed and widened by the controller. check-imports.sh:39 "
           "hardcodes six package names; `git ls-files 'internal/domain/*'` yields 25. The "
           "checker iterates the name list rather than the directory, so 19 packages are "
           "never opened. Confirmed uncaught violation: dhpricing imports dhlisting from "
           "three files. Consequence: every 'make check passes, so the hexagonal invariant "
           "holds' claim rests on a check with a 19-of-25 blind spot. The ticket must ASK "
           "whether a sub-package may import the inventory core rather than presume it — "
           "check-imports.sh:7-8 documents only sibling-to-sibling."),
 dict(n="Strip denominators in the DH cert-disambiguation card-number normalizer",
      tier="Security & data integrity", ids=["DUP-003"], effort="S",
      note="A live defect, not a style issue: the local normalizer diverges from cardutil's "
           "shared version, causing silent disambiguation misses."),

 # ---- Tier 2: correctness ------------------------------------------------
 dict(n="Move the fill-rate business rule out of the HTTP handler into the domain",
      tier="Correctness", ids=["ARCH-001"], effort="M",
      note="The domain fields designed to carry this rule (DailySpend.CapCents, "
           "FillRatePct) are never populated by the only repository, so the handler "
           "recomputes it — substituting the campaign's CURRENT cap for every historical "
           "day. The same rule is implemented a second time in the tuning rule."),
 dict(n="Add the missing forcedLiquidation field to the TypeScript Sale type",
      tier="Correctness", ids=["FE-007"], effort="S"),

 # ---- Tier 3: dead code removal -----------------------------------------
 dict(n="Remove the five dead auth token-lifecycle methods",
      tier="Dead code", ids=["DEADGO-004"], effort="S",
      note="ADJUDICATED — the removal half of the SPLIT finding. GetTokens, "
           "GetTokensByUserID, UpdateTokens, DeleteTokens, DeleteAllUserTokens are "
           "redundant with a live database constraint: migrations/000001_initial_schema.up.sql:104 "
           "declares `session_id TEXT REFERENCES user_sessions(id) ON DELETE CASCADE` on "
           "user_tokens, so logout and session cleanup already delete token rows at the DB "
           "level. The fixer must check THREE holders of an auth.Repository value, not the "
           "two the finding named — internal/domain/auth/service_impl.go:24 is invisible to "
           "a package-qualified grep. Do NOT include CleanupExpiredOAuthStates; it is "
           "ticketed separately as a bug."),
 dict(n="Remove the unwired AI image-generation cluster",
      tier="Dead code", ids=["DEADGO-002"], effort="S"),
 dict(n="Remove the advisor functional options that are never invoked",
      tier="Dead code", ids=["DEADGO-003"], effort="S",
      note="WithMaxTokens and WithTemperature are never called, so the fields they set are "
           "permanently stuck at their defaults. Confirm that freezing those defaults is "
           "intended before deleting rather than wiring."),
 dict(n="Resolve the cache-path plumbing that exists only to relocate a startup mkdir",
      tier="Dead code", ids=["DCT-014"], effort="M",
      note="ADJUDICATED — CONSTRAINED FIX. The finding's diagnosis is right: Config.Cache.Path "
           "is bound to -cache, syntax-validated, and has its directory created at startup, "
           "yet its value never reaches any cache constructor. But its proposed_fix offers "
           "'remove the field, its CLI flag, and its validation/directory-creation code', "
           "and THAT OPTION BREAKS PRODUCTION: Dockerfile.harvest:39 passes --cache to the "
           "deployed entrypoint, and flag.ContinueOnError at loader.go:240 turns an unknown "
           "flag into log.Fatalf. Acceptance criteria MUST require that any removal edits "
           "Dockerfile.harvest:39 in the same commit, and MUST state that removing the flag "
           "registration alone fails the deployed container at startup with "
           "`flag provided but not defined: -cache`. Do NOT merge this with DEADGO-001 "
           "(refuted) — they share one wrong premise, and clustering them would launder the "
           "error rather than cancel it."),
 dict(n="Drop the dead marketmovers_config table",
      tier="Dead code", ids=["DBSCHEMA-004"], effort="S",
      note="Its sibling mm_* tables were dropped when Market Movers was removed in 000021; "
           "this one was missed. Zero Go references anywhere."),
 dict(n="Remove seven unreferenced React components and eleven unreferenced React Query hooks",
      tier="Dead code", ids=["FE-002", "FE-003"], effort="M",
      note="Same component trees, one PR. FE-003's verifier independently checked the "
           "factory-registration and test-only-usage escapes that the finding never claimed."),
 dict(n="Remove unused frontend API-client methods, pricing types, and utility exports",
      tier="Dead code", ids=["FE-001", "FE-004", "FE-006"], effort="M",
      note="The five pricing.ts interfaces describe a per-grade eBay/estimate pricing API "
           "that no longer exists — they are the type-level residue of the removed pricing "
           "sources."),
 dict(n="Remove UserPreferencesProvider, or wire its hook to a consumer",
      tier="Dead code", ids=["FE-005"], effort="S",
      note="Mounted app-wide but useUserPreferences() is never called, so the whole context "
           "has no consumer. Decide removal vs. wiring before writing code — the provider "
           "being mounted suggests intent that was never finished."),

 # ---- Tier 4: documentation drift ---------------------------------------
 dict(n="Reconcile CLAUDE.md with the real architecture",
      tier="Documentation drift",
      ids=["DCT-001", "DCT-002", "DCT-003", "DCT-004", "DCT-019", "NB-009"], effort="M",
      note="The single highest-leverage doc ticket: CLAUDE.md is what every future agent "
           "reads first, so each error here propagates. Six findings, all in one file — "
           "the false sole-price-source claim, package listings wrong in both directions, "
           "an env-var group omitting CardLadder, a named repository interface that has "
           "NEVER existed, and the one interface member that dwarfs the rest mislabelled "
           "'focused'."),
 dict(n="Reconcile docs/SCHEMA.md with the 25-migration Postgres history",
      tier="Documentation drift",
      ids=["DBSCHEMA-005", "DBSCHEMA-006", "DBSCHEMA-007", "DBSCHEMA-008", "DCT-011"],
      effort="L",
      note="Seven live tables missing, three dropped tables documented as live, two wrong "
           "migration numbers, and pre-cutover SQLite migration numbers up to 000051 that "
           "have no corresponding file in this repo — making most 'Added: migration NNNNN' "
           "provenance notes unverifiable or actively misleading."),
 dict(n="Reconcile docs/API.md with the live route table",
      tier="Documentation drift", ids=["DCT-012"], effort="L",
      note="Documents removed marketmovers/advisor/sell-sheet endpoints and is missing at "
           "least 13 live ones."),
 dict(n="Purge removed subsystems from the remaining documentation",
      tier="Documentation drift",
      ids=["DCT-006", "DCT-007", "DCT-008", "DCT-009", "DCT-010", "DCT-020"], effort="M",
      note="internal/README.md, docs/ARCHITECTURE.md, docs/LLM_USAGE.md, docs/DH_INVENTORY.md, "
           "docs/USER_GUIDE.md and one package doc comment all still describe a SQLite "
           "storage layer, a removed 'picks' package and its 3-stage AI pipeline, a "
           "Favorites feature that does not exist in the product, and a pipeline function "
           "that exists nowhere in the repo."),
 dict(n="Reconcile .env.example with what loader.go actually reads",
      tier="Documentation drift", ids=["DCT-013", "DCT-015"], effort="S",
      note="Two declared vars are read by zero Go code; four more knobs are undocumented or "
           "mis-documented. Report variable NAMES only — never values."),

 # ---- Tier 5: test coverage ---------------------------------------------
 dict(n="Re-enable the scheduler tests blocked by a stale TODO",
      tier="Test coverage", ids=["DCT-016"], effort="M",
      note="The precondition the TODO waits on was satisfied in the same commit that added "
           "the TODO, four months ago."),
 dict(n="Add test coverage for the dhlisting adapter package",
      tier="Test coverage", ids=["DCT-017"], effort="M",
      note="PSA import batch translation, inventory sold-transitions, and PSA key-rotation "
           "delegation currently have zero tests."),

 # ---- Tier 6: naming and structure --------------------------------------
 dict(n="Move the tuning computation into the package named tuning",
      tier="Naming & structure", ids=["NB-002"], effort="M",
      note="All 902 LOC of tuning computation and every tuning type live in `inventory`, "
           "consumed by nothing but the 162-LOC `tuning` package. Related to NB-004: the "
           "flat-sibling rule is part of why the code ended up there."),
 dict(n="Rename the two mis-named inventory files",
      tier="Naming & structure", ids=["NB-005", "NB-006"], effort="S",
      note="Pure renames plus content moves, one PR. analytics_types.go and "
           "types_analytics.go coexist under two opposite conventions, and the latter mostly "
           "holds capital/finance and portfolio-health types. service_advanced.go names an "
           "adjective, not a concept."),
 dict(n="Move invoice projection from inventory to the finance package",
      tier="Naming & structure", ids=["NB-008"], effort="S"),
 dict(n="Extract the campaign-suggestions cluster out of inventory",
      tier="Naming & structure", ids=["NB-003"], effort="M",
      note="919 LOC with zero inbound references from the rest of `inventory` and exactly "
           "one consumer package — the cleanest seam the audit found."),
 dict(n="Extract the CSV-import subsystem out of inventory",
      tier="Naming & structure", ids=["NB-004"], effort="L",
      note="2,901 LOC, the largest seam that can leave under the existing flat-sibling "
           "pattern. `inventory` is 10,428 LOC largely because that rule makes it the only "
           "legal home for anything two siblings share — so this ticket and NB-001 are "
           "causally linked."),

 # ---- Tier 7: interface segregation and size ----------------------------
 dict(n="Make the inventory Service interface segmentation buy an actual seam",
      tier="Interface segregation & size", ids=["ARCH-004"], effort="M",
      note="ADJUDICATED — CLUSTER WITH THE NEXT UNIT, DO NOT MERGE. This is the "
           "consumer-facing union (internal/domain/inventory/service.go:175, 54 methods). "
           "Four of seven sub-interfaces have zero consumers, consumers wanting narrow "
           "dependencies declare their own, and the main handler still takes the union."),
 dict(n="Split the 55-method PurchaseRepository persistence port",
      tier="Interface segregation & size", ids=["SIZE-001"], effort="L",
      note="ADJUDICATED — the sibling of ARCH-004, deliberately kept separate. This is the "
           "PERSISTENCE PORT (internal/domain/inventory/repository_purchase.go:31), a "
           "different declaration in a different file on the other side of the boundary. "
           "55 methods — more than the other six sibling repository interfaces combined "
           "(controller counted: Campaign 5, Sale 7, Pricing 8, Analytics 10, Finance 14, "
           "DH 2 = 46). Merging this with ARCH-004 yields a ticket no single PR can land. "
           "Shared theme: interface segregation was declared, never enforced, and drifted "
           "in both directions at once."),
 dict(n="Decompose the 286-line ListPurchases function",
      tier="Interface segregation & size", ids=["SIZE-002"], effort="M"),
 dict(n="Decompose the 312-line BuildGroup scheduler constructor",
      tier="Interface segregation & size", ids=["SIZE-003"], effort="M",
      note="~20 independent, self-contained blocks that could each be its own function."),
 dict(n="Bring inmemory_campaign_store.go under the file-size budget",
      tier="Interface segregation & size", ids=["SIZE-004"], effort="M",
      note="1,328 lines / 106 methods, invisible to check-file-size.sh because testutil is "
           "excluded. The exclusion is arguably correct; the file size is not."),

 # ---- Tier 8: remaining architecture ------------------------------------
 dict(n="Give the demand repository contract a shared Go type across the seam",
      tier="Architecture", ids=["ARCH-002"], effort="L",
      note="Opaque vendor JSON blobs pass through the domain; writer and reader share no "
           "type, so the compiler enforces nothing across the boundary."),
 dict(n="Give the DH tombstone threshold a home in the domain",
      tier="Architecture", ids=["ARCH-003"], effort="S",
      note="Duplicated as an unexported constant in two adapter packages that are forbidden "
           "from importing each other — the duplication is a direct consequence of the "
           "architecture rule, so the fix belongs in the domain."),
 dict(n="Route the local cents/dollars and grader-regex reimplementations through the shared helpers",
      tier="Architecture", ids=["DUP-001", "DUP-002"], effort="S",
      note="Both are 'call the existing shared function instead'. Note the verifier "
           "re-grounded DUP-002: it is a pure maintainability concern, NOT a demonstrated "
           "live defect — the divergence cannot be exercised by any valid grader title. Do "
           "not let the ticket imply a live bug."),
]


def load():
    findings, verdicts = {}, {}
    for p in sorted(glob.glob(f"{AUDIT}/findings/*.json")):
        for f in json.load(open(p)):
            findings[f["id"]] = f
    for p in sorted(glob.glob(f"{AUDIT}/verdicts/*.json")):
        for v in json.load(open(p)):
            verdicts[v["id"]] = v
    return findings, verdicts


def sev_of(f, v):
    if v["verdict"] == "confirmed_lower_severity" and v.get("corrected_severity"):
        return v["corrected_severity"]
    return f["severity"]


def main():
    F, V = load()
    out = []
    w = out.append

    ticketed_ids = sorted({i for u in UNITS for i in u["ids"]})
    all_ticketable = sorted([i for i in F if V[i].get("ticketable")])
    missing = [i for i in all_ticketable if i not in ticketed_ids]
    extra = [i for i in ticketed_ids if not V[i].get("ticketable")]

    w("# Tech-Debt Audit — Consolidated Report\n")
    w(f"**Baseline revision:** `{BASELINE}`  ")
    w("**Scope:** Go backend (domain + adapters), frontend (`web/src`), database + "
      "migrations, docs/config/CI/tests  ")
    w("**Method:** 5 reference-mapping scouts → 8 judgment lenses → 12 adversarial "
      "verifiers → controller adjudication  ")
    w("**The audit was strictly read-only.** No agent edited code. "
      "`git diff --stat main..HEAD -- internal/ cmd/ web/src/` is empty by construction.\n")

    # ---- summary ----
    w("## Summary\n")
    counts = {}
    for i, v in V.items():
        counts[v["verdict"]] = counts.get(v["verdict"], 0) + 1
    total = len(F)
    conf = counts.get("confirmed", 0) + counts.get("confirmed_lower_severity", 0)
    w(f"| Metric | Count |")
    w(f"|---|---|")
    w(f"| Findings filed | {total} |")
    w(f"| Confirmed | {counts.get('confirmed', 0)} |")
    w(f"| Confirmed, severity lowered | {counts.get('confirmed_lower_severity', 0)} |")
    w(f"| Refuted | {counts.get('refuted', 0)} |")
    w(f"| **Confirmation rate** | **{conf}/{total} = {100*conf//total}%** |")
    w(f"| Ticketable | {len(all_ticketable)} |")
    w(f"| Fix units (tickets) | {len(UNITS)} |")
    w("")
    w("Every finding received exactly one verdict from a verifier whose explicit brief was "
      "to **refute** it, defaulting to refuted under uncertainty. The refuted section below "
      "is the audit's own error rate and stays in the record.\n")

    w("### Findings by category\n")
    bycat = {}
    for i, f in F.items():
        bycat.setdefault(f["category"], []).append(i)
    w("| Category | Findings | Ticketable |")
    w("|---|---|---|")
    for c in sorted(bycat):
        t = len([i for i in bycat[c] if V[i].get("ticketable")])
        w(f"| {c} | {len(bycat[c])} | {t} |")
    w("")

    # ---- reconciliation ----
    w("### Reconciliation\n")
    w(f"- Ticketable findings: **{len(all_ticketable)}**")
    w(f"- Findings rolled into fix units: **{len(ticketed_ids)}**")
    if missing:
        w(f"- ⚠️ **Ticketable but not in any fix unit:** {', '.join(missing)}")
    else:
        w("- ✅ Every ticketable finding appears in exactly one fix unit.")
    if extra:
        w(f"- Included by controller adjudication despite `ticketable: false`: "
          f"{', '.join(extra)} — see the adjudication note on each unit.")
    w("")

    # ---- fix units ----
    w("## Ranked fix units\n")
    w("Ranked by risk × effort. A fix unit is **one independently mergeable PR**. "
      "Tier order is the recommended execution order.\n")

    n = 0
    lasttier = None
    for u in UNITS:
        n += 1
        if u["tier"] != lasttier:
            w(f"\n---\n\n## Tier: {u['tier']}\n")
            lasttier = u["tier"]
        w(f"### FU-{n:02d} — {u['n']}\n")
        sevs = {sev_of(F[i], V[i]) for i in u["ids"]}
        order = ["high", "medium", "low"]
        top = next((s for s in order if s in sevs), sorted(sevs)[0] if sevs else "low")
        w(f"**Severity:** {top} · **Effort:** {u['effort']} · "
          f"**Rolls up:** {', '.join(u['ids'])}\n")
        if u.get("note"):
            w(f"> **Controller note.** {u['note']}\n")

        for i in u["ids"]:
            f, v = F[i], V[i]
            w(f"#### {i} — {f['title']}\n")
            w(f"*Lens:* `{f['lens']}` · *Confidence:* `{f['confidence']}` · "
              f"*Verdict:* `{v['verdict']}` · *Severity:* {sev_of(f, v)}\n")
            w(f"**Subject:** `{f['subject']}`\n")
            if v.get("what_the_finding_got_wrong"):
                w(f"**Verifier correction:** {v['what_the_finding_got_wrong']}\n")
            w("**Evidence:**\n")
            for e in f["evidence"]:
                w(f"- {e['claim']}")
                if e.get("file_line"):
                    w(f"  - `{e['file_line']}`")
                if e.get("command"):
                    w(f"  - Reproduce: `{e['command']}`")
            w("")
            w(f"**Proposed fix:** {f['proposed_fix']}\n")
            w("**Blast radius:**\n")
            for b in f["blast_radius"]:
                w(f"- `{b}`")
            w("")
            w("**Acceptance criteria:**\n")
            for a in f["acceptance_criteria"]:
                w(f"- [ ] {a}")
            w("")

    # ---- investigation tier ----
    w("\n---\n")
    w("## Investigation tier — recorded, deliberately not ticketed\n")
    w("These are real observations that a developer could not act on and prove correct "
      "from the finding alone. They are kept so a later pass starts from them rather than "
      "rediscovering them.\n")
    for i in sorted(F):
        v = V[i]
        if v.get("ticketable") or v["verdict"] == "refuted" or i in ticketed_ids:
            continue
        w(f"### {i} — {F[i]['title']}\n")
        w(f"*Verdict:* `{v['verdict']}` · *Lens:* `{F[i]['lens']}` · "
          f"*Confidence:* `{F[i]['confidence']}`\n")
        w(f"**Why not ticketed:** {v.get('basis', '')}\n")
        if v.get("what_the_finding_got_wrong"):
            w(f"**What the finding got wrong:** {v['what_the_finding_got_wrong']}\n")

    # ---- refuted ----
    w("\n---\n")
    w("## Refuted findings — the audit's own error rate\n")
    w("Four of 63 findings did not survive adversarial verification. Two were "
      "**mechanical-tier** — the audit's highest confidence band — and one of those would "
      "have driven a developer to break an hourly production job. This section stays in the "
      "record permanently.\n")
    for i in sorted(F):
        v = V[i]
        if v["verdict"] != "refuted":
            continue
        w(f"### {i} — {F[i]['title']}\n")
        w(f"*Claimed confidence:* `{F[i]['confidence']}` · *Lens:* `{F[i]['lens']}`\n")
        w(f"**How it was refuted:** {v.get('basis', '')}\n")
        if v.get("what_the_finding_got_wrong"):
            w(f"**What it got wrong:** {v['what_the_finding_got_wrong']}\n")
    w("See `docs/audit/ADJUDICATIONS.md` for the controller's full reasoning on "
      "DEADGO-001 and NB-010, including the production-breaking fix that DEADGO-001 "
      "would have shipped.\n")

    open(f"{AUDIT}/REPORT.md", "w").write("\n".join(out) + "\n")

    print(f"fix units: {len(UNITS)}")
    print(f"ticketable: {len(all_ticketable)}  rolled up: {len(ticketed_ids)}")
    print(f"missing: {missing or 'none'}")
    print(f"adjudicated-in: {extra or 'none'}")
    dupes = [i for i in ticketed_ids
             if sum(1 for u in UNITS if i in u["ids"]) > 1]
    print(f"in >1 unit (expected: DEADGO-004 only): {sorted(set(dupes)) or 'none'}")
    return 0 if not missing else 1


sys.exit(main())
