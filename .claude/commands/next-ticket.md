 ---
  description: Claim the lowest-numbered Linear Backlog ticket, validate it against the codebase, and work it — closing stale or false-premise tickets and moving on.
  argument-hint: "[team or filter] [--one]"
  ---

  # /next-ticket $ARGUMENTS

  Work the Linear backlog from the bottom of the number line up. `$ARGUMENTS` may
  name a team, project, or label to scope the search; if empty, use the default
  backlog. `--one` means stop after a single ticket instead of continuing.

  ## Step 0 — Superpowers chain

  Go through the `superpowers:using-superpowers` decision flow before doing
  anything else. Ticket work is implementation work: if the ticket turns out to be
  real and non-trivial, that means `superpowers:brainstorming` →
  `superpowers:writing-plans` → TDD, and a linked worktree before the first write.
  If it's a one-line fix, skip the ceremony — but decide that after validation,
  not before.

  ## Step 1 — Claim before you read

  Find the **lowest-numbered ticket in Backlog** within scope. Then, in a **single
  `save_issue` call**, move it to In Progress *and* assign it to me.

  Do this **before reading any code.** Concurrent sessions race for the same
  backlog; a ticket you validate for ten minutes and then claim is a ticket
  someone else may already be working. Claiming is cheap and reversible —
  validating twice is not.

  ## Step 2 — Validate the premise against the codebase

  Only now open the repo. Check what the ticket actually asserts:


  Gather citations as you go — `file.go:120`, a commit SHA, a test name. Every
  outcome below requires evidence, not an impression.

  ## Step 3 — Route on what you found

  **Already done** → close as **Done** with a comment citing the commit or
  `file:line` that satisfies it. Say plainly what makes it satisfied.

  reproduce, the assumption behind it was wrong) → close as **Canceled** with a
  comment saying exactly what you checked and what you found instead. "What you
  checked" matters as much as the verdict — it's what stops the next person
  re-litigating it.

  **Valid and yours to do** → work it. Follow the workflow the change's size
  warrants (see Step 0). Land it the way this repo lands work, with
  `superpowers:verification-before-completion` before any done claim.

  **Valid but not yours right now** → return it to **Backlog**, unassign, and
  comment *why* — blocked on a decision, needs an environment you don't have,
  depends on unmerged work. A ticket returned without a reason is worse than one
  never claimed. This is the only path back to Backlog; don't use it as a soft
  landing for tickets you'd rather not do.

  ## Step 4 — Move on
## Step 4 — Move on

After Done, Canceled, or a returned ticket, go back to Step 1 for the next
lowest-numbered Backlog ticket. Keep going until the backlog is empty in scope,
`--one` was passed, or a ticket lands you in real implementation work worth
finishing before starting another.

Report each ticket's outcome as you go — identifier, verdict, and the citation
behind it. One line per closed ticket is enough; don't batch the report to the
end.

## Don't

- Don't read code before claiming. That's the whole point of Step 1.
- Don't close anything on a hunch. Done and Canceled both need a citation.
- Don't cancel a ticket because it's awkward, vague, or large — that's a
  return-to-Backlog with a reason, or work to be done.
- Don't silently expand a ticket's scope because you noticed something nearby.
  File a new ticket instead.
