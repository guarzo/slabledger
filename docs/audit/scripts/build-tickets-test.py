"""Self-test for the two build-tickets.py markdown transforms.

Run: python3 docs/audit/scripts/build-tickets-test.py
Exits non-zero on the first failure. No repo state is read; these are pure
string transforms, so the test needs neither a run directory nor a baseline.
"""
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))

# build-tickets.py runs its work at import time, so lift out just the helper
# block rather than importing the module. Anchor on the definition, not on
# "def load()" — that string also appears earlier as a literal in the
# build-report.py slice, and matching it yields an inverted (empty) range.
src = open(f"{HERE}/build-tickets.py").read()
ns = {"re": re}
exec(src[src.index("_URL = re.compile"):src.index("\ndef load():")], ns)
prose, code, subject = ns["prose"], ns["code"], ns["subject"]

CASES = [
    # (input, expected output, why)
    # --- tokens Linear would autolink: must gain a code span ---
    ("see CLAUDE.md:126", "see `CLAUDE.md:126`", "file:line"),
    ("run check-imports.sh now", "run `check-imports.sh` now", "bare .sh"),
    ("the inventory.Sale struct", "the `inventory.Sale` struct", "Go selector"),
    ("internal/domain/auth.New is", "`internal/domain/auth.New` is", "module path"),
    ("docs/audit/METHODOLOGY.md.", "`docs/audit/METHODOLOGY.md`.",
     "trailing sentence period stays outside the span"),
    ("at foo.go:12:34 exactly", "at `foo.go:12:34` exactly", "file:line:col"),
    ("see USER_GUIDE.md:50-64 there", "see `USER_GUIDE.md:50-64` there",
     "file:line-range"),
    ("both a.md and b.sh", "both `a.md` and `b.sh`", "two tokens in one string"),

    # --- text that must be left completely alone ---
    ("e.g. the parser", "e.g. the parser", "lowercase abbreviation is not an ext"),
    ("version 1.5 shipped", "version 1.5 shipped", "digit after dot"),
    ("ends here. Next starts", "ends here. Next starts", "sentence boundary"),
    ("U.S. only", "U.S. only", "single-char base is excluded"),
    ("already `CLAUDE.md:126` spanned", "already `CLAUDE.md:126` spanned",
     "an existing code span is never re-wrapped"),
    ("see https://linear.app/x/issue.md now", "see https://linear.app/x/issue.md now",
     "real URLs are left exactly as written"),
    ("`a.md` then b.md", "`a.md` then `b.md`",
     "spanned and bare tokens in one string"),

    # --- the backslash rule the transform must not have broken ---
    (r"a \| b", r"a \\| b", "backslash escaped outside a code span"),
    (r"`grep '\bx\b'`", r"`grep '\bx\b'`", "backslash untouched inside a code span"),
]

bad = []
for src_s, want, why in CASES:
    got = prose(src_s)
    if got != want:
        bad.append(f"prose({src_s!r})\n    want {want!r}\n    got  {got!r}\n    ({why})")

SUBJECTS = [
    ({"kind": "table", "identity": "marketmovers_config"},
     "`marketmovers_config` (table)"),
    ({"kind": "symbol", "identity": "internal/domain/auth.Service"},
     "`internal/domain/auth.Service` (symbol)"),
    ({"identity": "no_kind"}, "`no_kind`"),
    ({"kind": "table"}, "`{'kind': 'table'}`"),  # malformed: degrade, don't raise
]
for s, want in SUBJECTS:
    got = subject(s)
    if got != want:
        bad.append(f"subject({s!r})\n    want {want!r}\n    got  {got!r}")

# build-report.py carries its own copy of subject() — it renders REPORT.md,
# which must not disagree with the ticket bodies about what a finding is about.
rsrc = open(f"{HERE}/build-report.py").read()
rns = {}
exec(rsrc[rsrc.index("def subject(s):"):rsrc.index("\ndef sev_of(")], rns)
for s, want in SUBJECTS:
    got = rns["subject"](s)
    if got != want:
        bad.append(f"build-report subject({s!r}) disagrees with build-tickets\n"
                   f"    want {want!r}\n    got  {got!r}")

# The span must never unbalance the backticks in a rendered line.
for src_s, _, _ in CASES:
    if prose(src_s).count("`") % 2:
        bad.append(f"prose({src_s!r}) left an odd number of backticks")

if bad:
    print("FAIL\n\n" + "\n\n".join(bad), file=sys.stderr)
    sys.exit(1)
print(f"GATE PASS: build-tickets markdown transforms — "
      f"{len(CASES)} prose cases, {len(SUBJECTS)} subject cases")
