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

## 2026-06-01 — Persist session-state deltas inline, not at Step 6

- **Severity.** `blocking`
- **Rationale.** Tom flagged that a previous session discussed re-pausing the three resumed campaigns (Modern, Modern PSA 10, Vintage-EX Precision) on/around 5/27, but no record was written to `docs/private/` or to user memory. Today's session opened by reading the 5/15 resume doc, saw `phase=pending` on the API, and asked Tom to re-explain. This is the third time this class of failure has hit (cf. `feedback_c10_never_paused.md`, `project_campaign_state_apr23.md`). The Step 6 retrospective is the documented mechanism for capturing session deltas, but it only fires at session end — when a session ends abruptly or the operator changes topic, Step 6 is skipped and the delta is lost. The fix is to require an inline write the moment a state-changing action is discussed (pause / resume / parameter change / Brady email drafted), not to rely on end-of-session capture.
- **Proposed diff.**

```diff
--- a/.claude/skills/campaign-analysis/SKILL.md
+++ b/.claude/skills/campaign-analysis/SKILL.md
@@ ## Data integrity
+
+### Inline session-state persistence (mandatory)
+
+Whenever the conversation reaches a decision that changes operator state —
+a campaign pause/resume, a terms change, a daily-cap edit, an inclusion-list
+edit, or a Brady/PSA email being drafted — write a dated artifact to
+`docs/private/YYYY-MM-DD-<short-slug>.md` IN THE SAME TURN as the decision.
+Do not wait for Step 6 (retrospective) to capture state deltas; Step 6
+runs at session end and is skipped when sessions terminate abruptly.
+
+The artifact contains: date, decision summary, campaigns affected, parameter
+before/after, rationale (one paragraph), and any open questions. Append a
+pointer line to the relevant project memory file (e.g.
+`project_may15_pause_partial_resume.md`) so the next session reads it.
+
+Failure mode this prevents: the operator having to re-explain state changes
+across sessions because the previous session's decision was discussed but
+never persisted. Logged 2026-06-01 after the 5/27 re-pause was lost.
```

- **Transcript citation.** Tom: "we do this every single time, how can we not have to rehash conversations over and over again. i have no idea where you recorded it (or apparently didn't), that is something i expect you to manage."

---

## 2026-05-27 — Opener draft must not emit weekly-count sequences or daily-cap claims without cited API values

- **Severity.** `blocking`
- **Rationale.** The Layer-2 adversary caught fabrications in the session-opener draft: a weekly-counts sequence with inverted order, an uncited daily-cap claim, a portfolio-at-a-glance paragraph written before any API calls had returned, and a staleness violation on the week of 5/18. The opener was drafted before the data was in hand, which is the root cause. The SKILL.md opener step should require that all API calls complete and fact-sheets are emitted before any prose synthesis begins — currently the ordering is advisory rather than enforced.
- **Proposed diff.**

```diff
--- a/.claude/skills/campaign-analysis/SKILL.md
+++ b/.claude/skills/campaign-analysis/SKILL.md
@@ ## Step 1 — Opener synthesis
-Synthesize the opener paragraph after receiving fact-sheets from all domain agents.
+**Hard ordering rule:** do not begin drafting any opener prose until ALL Layer-1
+domain-agent fact-sheets for the current session have been emitted. No inline
+estimates, no weekly-count sequences, no daily-cap figures may appear in the draft
+unless they cite a specific fact-sheet row id from this session. The Layer-2 adversary
+will redline any claim that lacks a same-session `[id:...]` citation.
```

- **Transcript citation.** "Layer-2 adversary caught: weekly-counts sequence inverted, uncited daily-cap claim, portfolio-at-a-glance without citations, staleness violation on week 5/18" — session post-mortem summary, failure #4.

---

