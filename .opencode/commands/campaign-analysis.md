---
description: "Conversational campaign analysis — portfolio health, P&L, liquidation, tuning, aging inventory, DH status, coverage gaps"
argument-hint: "[optional: health | weekly | tuning | campaign <id-or-name> | gaps | dh]"
---

Use the `skill` tool to load the `campaign-analysis` skill, then execute its full workflow.

The argument passed to this command is: $ARGUMENTS

If empty (the usual case), run the default conversational snapshot and follow up based on what the user asks next. Otherwise pass the argument as an explicit mode: `health`, `weekly`, `tuning`, `gaps`, `dh`, or `campaign <id-or-name>` for a single-campaign deep dive.
