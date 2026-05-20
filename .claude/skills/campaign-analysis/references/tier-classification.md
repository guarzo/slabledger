# Reviewer Tier-A / Tier-B Classification

The Layer-4 `ca-reviewer` agent uses this catalog to decide whether a proposed
change auto-applies (Tier A) or queues for operator review (Tier B). The rule
of thumb: anything purely additive that cannot regress an existing behavior is
Tier A; anything that alters behavior, prompts, or layer structure is Tier B.

## Tier A — auto-apply

The reviewer applies these changes directly and surfaces them in the session-end
summary. No operator approval required.

- **New row in `references/field-semantics.md`** — additive catalog entry; cannot
  regress existing rows.
- **New `semantics_caveat` on an existing field-semantics row** — strengthens
  the forbidden-uses list; cannot loosen it.
- **New fact-sheet column for a Layer-1 domain agent** — additive only. The
  reviewer may append a new `metric` id and its `endpoint`+`jq`; it may NOT
  remove or rename an existing column.
- **New entry in `docs/private/impossible-data-asks.md`** — additive list of
  data the operator should request from PSA / DH / CardLadder.
- **New entry in `docs/private/campaign-analysis-wishlist.md`** — additive
  improvement ideas not yet ripe for the queue.
- **New feedback memory file** under `~/.claude/projects/-workspace/memory/`
  with the `feedback_*.md` naming convention — additive lesson capture.

## Tier B — suggest only, write to improvement queue

The reviewer writes these to `docs/private/campaign-analysis-improvement-queue.md`
with rationale, proposed diff (unified format), transcript citation, and
severity. The operator reviews at their own pace.

- **New rule in `SKILL.md`** — behavior change.
- **Removed or modified existing rule** — behavior change.
- **Domain-agent prompt rewrite** — behavior change at Layer 1.
- **Layer split, merge, or new agent** — architecture change.
- **Change to the Tier-A / Tier-B classification list itself** — meta-change;
  must always be operator-reviewed.

## Severity levels (Tier B only)

- **blocking** — the next session is likely to repeat the same failure without
  this change. Reviewer should surface prominently in the session summary.
- **improvement** — would have prevented or mitigated the observed failure, but
  the failure is not load-bearing.
- **nice-to-have** — quality-of-life or DX improvement; not failure-driven.

## Reviewer output format

At session end, the reviewer emits a single summary message:

```
Reviewer ran.

Auto-applied N changes:
- <one-line description of change 1>
- <one-line description of change 2>
- ...

Queued M proposals in docs/private/campaign-analysis-improvement-queue.md:
- [SEVERITY] <one-line title of proposal 1>
- [SEVERITY] <one-line title of proposal 2>
- ...
```

If `N == 0` and `M == 0`, the reviewer says so explicitly rather than emitting
a no-op.

## What's NOT a reviewer change

The reviewer does not retroactively edit prior session transcripts, does not
rewrite the operator's strategy doc, and does not modify code outside the
`.claude/skills/campaign-analysis/` tree and the agent files under
`.claude/agents/ca-*`. Anything outside that scope is a Tier-B queue entry at
most.
