---
description: "Holistic codebase review — architecture drift, duplicate logic, code smells, quality, UX — up to 10 high-impact improvements"
argument-hint: "[backend | frontend | diff | since:<date> | <package-name>]"
---

Use the `skill` tool to load the `improve` skill, then execute its full workflow.

The argument passed to this command is: $ARGUMENTS

Pass that argument to the skill as the scope: empty means full codebase, `backend` for Go only, `frontend` for React only, `diff` for files changed since the last `/improve` run, `since:<date>` (e.g. `since:2026-04-10`) for files changed since an absolute date, or a package name like `inventory` or `dhprice` for a deep dive.
