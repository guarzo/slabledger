#!/usr/bin/env python3
"""Emit docs/audit/TICKETS.md — one ticket body per fix unit, in the format
Task 16 specifies. Reuses the cluster map in build-report.py as the single
source of truth so REPORT.md and TICKETS.md can never drift apart.

Each body is what gets posted to Linear verbatim.

Determinism is per generator version, not across them: commit 4c5d1b42 repaired
rendering defects postdating both filed runs, so this script no longer reproduces
the committed TICKETS.md of either. See build-report.py's docstring and
runs/2026-08-08/CONTROLLER-NOTES.md.
"""
import json
import glob
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
AUDIT_HOME = HERE + "/.."
# Run-scoped root; see build-report.py. Defaults to AUDIT_HOME so the flat
# 2026-08-07 layout resolves its inputs without any environment (which is not
# the same as reproducing that run's committed TICKETS.md — see the docstring).
AUDIT = os.environ.get("AUDIT_RUN", AUDIT_HOME)
# Repo-relative form of AUDIT. This one is load-bearing in a way build-report's
# is not: it is rendered into every ticket body POSTed to Linear, where a stale
# `docs/audit/REPORT.md` would send each fixer to a previous run's report.
AUDIT_DISPLAY = os.path.relpath(
    os.path.abspath(AUDIT), os.path.abspath(AUDIT_HOME + "/../.."))
if AUDIT_DISPLAY.startswith(".."):  # run dir outside the repo; see build-report.py
    AUDIT_DISPLAY = os.path.abspath(AUDIT)
# Same single source as build-report.py and validate.sh: the run's pinned
# revision, abbreviated for display. A hand-typed short hash here could drift
# from the revision the gate actually enforces, and a ticket that cites the
# wrong baseline sends its fixer to the wrong tree.
BASELINE = json.load(open(os.environ.get(
    "AUDIT_BASELINE_FACTS", f"{AUDIT}/baseline.json")))["revision"][:8]

# Where the filing script picks up the bodies to POST. Defaults inside the repo
# so a reboot or a /tmp sweep between "generate" and "file" cannot strand a
# half-filed run; override for a scratch run.
MANIFEST = os.environ.get("TICKET_MANIFEST", f"{AUDIT}/ticket-manifest.json")

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
#
# A third transformation, found by round-tripping the 2026-08-08 batch: Linear
# autolinks any bare dotted token in raw prose that resembles a hostname, and
# `CLAUDE.md:126`, `check-imports.sh`, `inventory.Sale` and
# `internal/domain/auth.New` all qualify. The label text survives, so nothing is
# lost — the issue gains a dead `http://` href over a filename. Cosmetic, but it
# is on nearly every line of evidence in the batch. A code span suppresses it,
# which is also how these tokens should have been marked up in the first place.

_URL = re.compile(r"https?://\S+")
# A dotted token is only wrapped when the segment after the dot is a known file
# extension or an exported Go identifier. That is deliberately narrow: it leaves
# "e.g." (lowercase, not an extension), "1.5" (digit) and sentence boundaries
# (dot then space) alone. Known gap: a dotted token already inside a markdown
# link would be double-wrapped, so findings prose must not contain one.
_EXT = ("md|sh|go|tsx?|jsx?|json|py|sql|ya?ml|css|html|txt|mod|sum|env|toml"
        "|lock|xml|csv")
_DOTTED = re.compile(r"""
    (?<![\w./-])                       # start of token
    (
      (?:[\w-]+/)*                     # optional path prefix
      (?: [\w-]{2,} \. [A-Z][\w-]*     # pkg.Symbol — Go selector. The >=2-char
                                       # base is what keeps "U.S." intact.
        | [\w-]+ \. (?:""" + _EXT + r""")\b )   # file.md, file.sh, ...
      (?:\.[A-Za-z][\w-]*)*            # further dotted segments
      (?::\d+(?:-\d+)?(?::\d+)?)?      # :line, :line-line, :line:col
    )
    (?![\w/-])                         # end of token
""", re.X)


def _despan(s):
    """Wrap bare dotted tokens in code spans so Linear will not autolink them."""
    out, last = [], 0
    for m in _URL.finditer(s):  # real URLs are left exactly as written
        out.append(_DOTTED.sub(r"`\1`", s[last:m.start()]))
        out.append(m.group(0))
        last = m.end()
    out.append(_DOTTED.sub(r"`\1`", s[last:]))
    return "".join(out)


def prose(s):
    """Escape backslashes in the non-code segments of a markdown string, and
    code-span anything Linear would otherwise autolink."""
    parts = s.split("`")
    return "`".join(_despan(p.replace("\\", "\\\\")) if n % 2 == 0 else p
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


def subject(s):
    """Render a finding's {kind, identity} pair for a human.

    It used to be `str(dict)`, which shipped a raw Python repr —
    `{'kind': 'table', 'identity': 'marketmovers_config'}` — into every filed
    ticket in the 2026-08-07 and 2026-08-08 batches. The dict form is kept as a
    fallback so a malformed subject degrades instead of raising mid-render.
    """
    if isinstance(s, dict) and s.get("identity"):
        kind = s.get("kind", "")
        return code(str(s["identity"])) + (f" ({kind})" if kind else "")
    return code(str(s))


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
      f"Full record: `{AUDIT_DISPLAY}/REPORT.md`.")
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
        w(f"**Subject:** {subject(f['subject'])}")
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


def load_linear_ids(path):
    """Normalize either `linear-ids.json` shape to `{FU-xx: {id, url}}`.

    The 2026-08-07 run wrote a flat `{FU-xx: {id, url}}` map. The 2026-08-08 run
    wrote a run document: baseline, team, findings_covered and a `not_filed`
    record, with the per-unit rows under `tickets` and the id under `linear_id`.
    Fed the second shape, the coverage check below reported all 32 units missing
    and every metadata key as "extra" — a hard exit at the one moment the run is
    least recoverable. The document shape is the better record (the flat one
    cannot say which findings were deliberately not filed), so normalize here
    instead of rewriting a filed run's artifact to suit the reader.
    """
    doc = json.load(open(path))
    if isinstance(doc, dict) and isinstance(doc.get("tickets"), list):
        return {t["fu"]: {"id": t.get("linear_id") or t.get("id", ""),
                          "url": t.get("url", "")}
                for t in doc["tickets"] if t.get("fu")}
    return doc


def main():
    F, V = load()
    ids = {}
    if os.path.exists(f"{AUDIT}/linear-ids.json"):
        ids = load_linear_ids(f"{AUDIT}/linear-ids.json")
    fus = {f"FU-{n:02d}" for n in range(1, len(UNITS) + 1)}
    # METHODOLOGY.md lists "every fix unit filed" as a final check. Enforce it
    # here rather than asserting it in a table: an unfiled unit used to degrade
    # to a "_(not yet filed)_" placeholder that no gate would ever notice.
    if ids:
        missing, extra = sorted(fus - set(ids)), sorted(set(ids) - fus)
        if missing or extra:
            print(f"linear-ids.json does not cover the emitted fix units: "
                  f"missing={missing} extra={extra}", file=sys.stderr)
            sys.exit(1)
        # Covering every FU is not enough: a record missing `id` used to raise a
        # bare KeyError mid-render, and one missing `url` rendered a live-looking
        # link to nowhere. Both are cheap to check and expensive to notice later.
        broken = sorted(f"{fu}({', '.join(k for k in ('id', 'url') if not rec.get(k))})"
                        for fu, rec in ids.items()
                        if not isinstance(rec, dict) or not rec.get("id") or not rec.get("url"))
        if broken:
            print(f"linear-ids.json records missing id/url: {broken}", file=sys.stderr)
            sys.exit(1)
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

    with open(f"{AUDIT}/TICKETS.md", "w") as fh:
        fh.write("\n".join(out) + "\n")
    with open(MANIFEST, "w") as fh:
        json.dump(manifest, fh, indent=1)
    print(f"drafted {len(manifest)} tickets -> {AUDIT_DISPLAY}/TICKETS.md")
    print(f"manifest -> {MANIFEST}")
    for m in manifest:
        print(f"  {m['fu']}  [{m['label']}/P{m['priority']}]  {m['title']}")


main()
