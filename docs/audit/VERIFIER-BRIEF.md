# Verifier Brief — Phase 3 (Adversarial Verification)

Read `docs/audit/PREAMBLE.md` and `docs/audit/LENS-BRIEF.md` first. Both bind you.
This file is additional.

## Your job is to REFUTE

A lens auditor wrote the finding you are checking. Your job is **not** to
appreciate it. Your job is to destroy it, and to report survival only when you
genuinely could not.

**Default to REFUTED under uncertainty.** If you cannot establish the finding is
correct, the verdict is `refuted` — not `partially_confirmed`, not "probably
fine." An audit that ships a wrong ticket costs a developer a wasted branch and
teaches them to distrust every other ticket in the batch. A refuted-in-error
finding costs one re-check. These are not symmetric.

## Re-derive; never audit the reasoning

The finding's own `description` and `runtime_checks` are **claims under test**,
never evidence. Do not reason from them. Re-derive the conclusion from source and
from the `maps/` directory of the run directory named in your Run Card, then
compare what you found to what it claims.

Concretely, for every finding:

1. Run each `command` in its `evidence` array. Does it execute without error?
   Does its output actually say what the finding says it says?
2. Establish the central claim independently, by your own route.
3. Only then read the finding's reasoning, and look for the gap between it and
   what you found.

A finding whose evidence commands do not reproduce is **refuted**, regardless of
whether you believe its conclusion. Non-reproducing evidence is the single most
common failure and the cheapest to detect.

## The three traps cut both ways

`LENS-BRIEF.md` §3 lists three traps that make dead code look live. They also
make live code look dead, and one of them already produced a false positive in
this audit — pointer NB-010 claimed five dead symbols whose every reference was
in-package, reached through an exported enclosing function, and serialized to the
frontend. See `docs/audit/ADJUDICATIONS.md`.

So, for any finding asserting something is unused, unreferenced, or removable:

- **In-package callers.** `external_refs=0` means "not named from outside its
  package." It never means "unreachable." Ask whether the *enclosing exported
  function* is reachable.
- **Serialization.** A JSON-tagged struct field can be live purely by being
  marshalled. Check `web/src/types/` before believing any Go type is dead.
- **Same-name collision.** Confirm every grep hit refers to the finding's actual
  subject and not an unrelated identifier that happens to share the name.
- **Interface satisfaction.** The Go map is textual and cannot see it. A method's
  presence in an implementation and a mock is expected by construction and proves
  nothing in either direction.

## Severity is in scope

A finding can be real and still overstated. If the defect exists but the stated
consequence does not follow, that is `confirmed_lower_severity`, and you must say
what the real consequence is. Security findings especially: distinguish "data is
exposed" from "ciphertext is exposed" from "a row can be overwritten."

## Output

Write one JSON array to the path given in your dispatch. One object per finding
you were assigned — **every one, including those you confirm**:

```json
[
  {
    "id": "DEADGO-001",
    "verdict": "confirmed | confirmed_lower_severity | refuted | unresolvable",
    "evidence_reproduces": true,
    "basis": "How YOU established or failed to establish the claim. Cite file:line or a command you ran. Not a restatement of the finding.",
    "what_the_finding_got_wrong": "Empty string if nothing.",
    "corrected_severity": "Only if verdict is confirmed_lower_severity; else empty string.",
    "ticketable": true
  }
]
```

`unresolvable` is for claims that cannot be settled by static analysis at all
(index reachability, live database grants, production row counts). It is an
honest verdict, not a failure — but it always means `ticketable: false`. The
schema enforces that, and so does `scripts/validate.sh verdicts`.

There is no "unresolvable overall, but ticketable on the part I did verify."
If one leg of a finding is verifiable and another is not, the verifiable leg
*is* the finding: return `confirmed_lower_severity`, scope
`what_the_finding_got_wrong` to the part you could not reach, and set a
`corrected_severity` that reflects only what you established. A ticket must
never carry a confidence no one earned.

`ticketable` is your judgment on whether a developer could act on this finding
and prove the fix correct from its `acceptance_criteria` alone.

## Constraints

- **Strictly read-only.** Write exactly one file: your own verdict JSON.
- Never modify anything under `internal/`, `cmd/`, `web/src/`, the findings
  files, the maps, the schemas, or any documentation.
- Never connect to a database, run migrations, or execute SQL. There is a live
  production database behind `DATABASE_URL`. Static analysis of files only.
- Do not read any real `.env` file. Report secret NAMES only, never values.
- **Baseline revision: the one named in your Run Card**; git-tracked files
  only, via `git ls-files`. `ls` and `find` are not evidence.
- Do not commit. The controller commits.

**Return only:** a one-line tally (`confirmed / lower / refuted / unresolvable`)
and anything you believe the controller must know. Do not paste verdicts into
your reply — they are in your file.
