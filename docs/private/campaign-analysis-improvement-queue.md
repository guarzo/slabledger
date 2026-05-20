# Campaign-Analysis Improvement Queue

Tier-B proposals from the `ca-reviewer` agent awaiting operator review. The
reviewer appends entries here at session end; the operator reviews on their
own cadence and either applies the diff, rejects it, or files it under
`wishlist.md` if it's no longer relevant.

## Entry format

Each entry is a level-2 section with this shape:

## YYYY-MM-DD — <one-line title>

- **Severity.** `blocking` | `improvement` | `nice-to-have`
- **Rationale.** Why the reviewer believes this change would have prevented or
  mitigated a failure in the cited session.
- **Proposed diff.** Unified diff format against the target file. If the change
  spans multiple files, one diff block per file.
- **Transcript citation.** A short quote (one or two lines) from the session
  transcript that motivated the proposal, with enough context to locate the
  moment.

## Triage

When the operator processes an entry, they:
1. Apply, reject, or defer the diff.
2. Append a `**Resolution.**` line (`applied YYYY-MM-DD` / `rejected YYYY-MM-DD`
   / `deferred to wishlist YYYY-MM-DD`) to the entry.
3. Leave resolved entries in place for historical traceability; the queue is
   append-only.

---

<!-- Entries appear below this line in reverse-chronological order. -->
