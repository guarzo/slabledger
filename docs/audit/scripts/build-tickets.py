#!/usr/bin/env python3
"""Emit docs/audit/TICKETS.md — one ticket body per fix unit, in the format
Task 16 specifies. Reuses the cluster map in build-report.py as the single
source of truth so REPORT.md and TICKETS.md can never drift apart.

Each body is what gets posted to Linear verbatim.
"""
import json
import glob
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
AUDIT = HERE + "/.."
BASELINE = "740976ec"

# Reuse the controller's cluster map without re-declaring it.
src = open(f"{HERE}/build-report.py").read()
ns = {}
exec(src[src.index("UNITS = ["):src.index("def load()")], ns)
UNITS = ns["UNITS"]

LABEL = {
    "Security & data integrity": "Bug",
    "Correctness": "Bug",
    "Dead code": "Improvement",
    "Documentation drift": "Improvement",
    "Test coverage": "Improvement",
    "Naming & structure": "Improvement",
    "Interface segregation & size": "Improvement",
    "Architecture": "Improvement",
}
PRIORITY = {  # Linear: 1=Urgent 2=High 3=Normal 4=Low
    "Security & data integrity": 2,
    "Correctness": 2,
    "Dead code": 3,
    "Documentation drift": 3,
    "Test coverage": 3,
    "Naming & structure": 4,
    "Interface segregation & size": 4,
    "Architecture": 4,
}


# --- Linear markdown survival -------------------------------------------
# Verified against the live endpoint by round-tripping a probe issue:
#   * a backslash inside an inline code span survives as-is;
#   * a backslash in raw prose is eaten as a markdown escape, so `\|`
#     becomes `|` and silently breaks a BRE alternation in a grep command;
#   * a multi-line string inside single backticks is dropped entirely;
#   * fenced blocks survive verbatim, backslashes included.
# So: escape backslashes only outside code spans, and fence anything that
# is multi-line or already contains a backtick.

def prose(s):
    """Escape backslashes in the non-code segments of a markdown string."""
    parts = s.split("`")
    return "`".join(p.replace("\\", "\\\\") if n % 2 == 0 else p
                    for n, p in enumerate(parts))


def code(s, indent=""):
    """Render a command/snippet so Linear stores it byte-for-byte.

    Multi-line fences are deliberately NOT indented to match the list item
    they sit under: Linear preserves leading whitespace inside a fence, and
    an indented `python3 -c "..."` snippet pastes back as an IndentationError.
    """
    if "\n" in s or "`" in s:
        return f"\n```\n{s}\n```"
    return f"`{s}`"


def load():
    F, V = {}, {}
    for p in sorted(glob.glob(f"{AUDIT}/findings/*.json")):
        for f in json.load(open(p)):
            F[f["id"]] = f
    for p in sorted(glob.glob(f"{AUDIT}/verdicts/*.json")):
        for v in json.load(open(p)):
            V[v["id"]] = v
    return F, V


def sev_of(f, v):
    if v["verdict"] == "confirmed_lower_severity" and v.get("corrected_severity"):
        return v["corrected_severity"]
    return f["severity"]


def ticket_body(u, F, V):
    """The markdown posted to Linear as the issue description."""
    o = []
    w = o.append
    sevs = {sev_of(F[i], V[i]) for i in u["ids"]}
    top = next((s for s in ["high", "medium", "low"] if s in sevs), "low")
    confs = sorted({F[i]["confidence"] for i in u["ids"]})

    w(f"**Category:** {u['tier']} · **Severity:** {top} · "
      f"**Confidence:** {', '.join(confs)} · **Effort:** {u['effort']}")
    w(f"**Baseline revision:** `{BASELINE}` · "
      f"**Audit findings:** {', '.join(u['ids'])}")
    w("")
    w("Produced by the read-only tech-debt audit. Every claim below survived an "
      "adversarial verification pass whose brief was to refute it. "
      "Full record: `docs/audit/REPORT.md`.")
    w("")

    if u.get("note"):
        w("> [!IMPORTANT]")
        for ln in prose(u["note"]).split("\n"):
            w(f"> {ln}")
        w("")

    for i in u["ids"]:
        f, v = F[i], V[i]
        if len(u["ids"]) > 1:
            w(f"## {i} — {prose(f['title'])}")
        else:
            w(f"## Claim")
            w("")
            w(prose(f["title"]))
        w("")
        w(f"**Subject:** {code(str(f['subject']))}")
        if v.get("what_the_finding_got_wrong"):
            w("")
            w(f"**Verifier correction (already applied to this ticket):** "
              f"{prose(v['what_the_finding_got_wrong'])}")
        w("")
        w("### Evidence")
        w("")
        for e in f["evidence"]:
            w(f"- {prose(e['claim'])}")
            if e.get("file_line"):
                w(f"  - {code(e['file_line'], '  ')}")
            if e.get("command"):
                w(f"  - Reproduce: {code(e['command'], '  ')}")
        w("")
        w("### Proposed fix")
        w("")
        w(prose(f["proposed_fix"]))
        w("")
        w("### Blast radius")
        w("")
        for b in f["blast_radius"]:
            w(f"- {code(b)}")
        w("")
        w("### Acceptance criteria")
        w("")
        for a in f["acceptance_criteria"]:
            w(f"- [ ] {prose(a)}")
        w("")

    w("### Definition of done")
    w("")
    w("- [ ] `make check` passes")
    w("- [ ] `go test -race ./...` passes")
    w("- [ ] `cd web && npm run build && npm test` passes (if this ticket touches `web/`)")
    return "\n".join(o)


def main():
    F, V = load()
    ids = {}
    if os.path.exists(f"{AUDIT}/linear-ids.json"):
        ids = json.load(open(f"{AUDIT}/linear-ids.json"))
    out = []
    w = out.append
    w("# Audit Tickets — draft bodies\n")
    w(f"**Baseline revision:** `{BASELINE}`  ")
    w("**Destination:** Linear team **SlabLedger** (no project — the workspace has none)  ")
    w(f"**Count:** {len(UNITS)} tickets, one per fix unit in `REPORT.md`\n")
    w("Nothing here is filed until the user signs off. The `Linear ID` line on each "
      "ticket is filled in after creation so a partial failure mid-run is recoverable.\n")
    w("---\n")

    manifest = []
    for n, u in enumerate(UNITS, 1):
        fu = f"FU-{n:02d}"
        title = u["n"]
        rec = ids.get(fu)
        w(f"## {fu} — {title}\n")
        if rec:
            w(f"**Linear:** [{rec['id']}]({rec['url']})  ")
        else:
            w(f"**Linear ID:** _(not yet filed)_  ")
        w(f"**Label:** {LABEL[u['tier']]} · **Priority:** {PRIORITY[u['tier']]}\n")
        w("<details><summary>Ticket body as posted</summary>\n")
        w(ticket_body(u, F, V))
        w("\n</details>\n")
        w("---\n")
        manifest.append(dict(fu=fu, title=title,
                             label=LABEL[u["tier"]], priority=PRIORITY[u["tier"]],
                             tier=u["tier"], ids=u["ids"],
                             linear=(rec or {}).get("id", ""),
                             body=ticket_body(u, F, V)))

    open(f"{AUDIT}/TICKETS.md", "w").write("\n".join(out) + "\n")
    json.dump(manifest, open("/tmp/ticket-manifest.json", "w"), indent=1)
    print(f"drafted {len(manifest)} tickets -> docs/audit/TICKETS.md")
    print(f"manifest -> /tmp/ticket-manifest.json")
    for m in manifest:
        print(f"  {m['fu']}  [{m['label']}/P{m['priority']}]  {m['title']}")


main()
