# Overnight `/improve` Autonomous Loop

This used to be a runbook. It's now the `my:overnight-improve` skill from the personal `my` plugin.

## Usage

1. Launch Claude with permissions bypassed (the loop needs to run unattended):

   ```bash
   claude --permission-mode bypassPermissions
   # equivalent: claude --dangerously-skip-permissions
   ```

2. Invoke the skill: `my:overnight-improve`. It will run preflight, read the project config below, and start the loop.

## Project config

`.claude/overnight-config.yaml` — gates, do-nots, iteration cap. Edit there to tune the loop for slabledger.

## Skill source

`~/.dotfiles/ai/marketplace/plugins/my/skills/overnight-improve/`
- `SKILL.md` — the skill body (reads config, generates the loop prompt, invokes ralph-loop)
- `references/config-schema.md` — full schema for `.claude/overnight-config.yaml`
- `references/preflight-checklist.md` — checks run before the loop starts

## Morning review

```bash
git log --oneline main..HEAD         # what got committed
cat .claude/overnight-run-state.md   # every attempt + the == Wrap-up == section
git diff main..HEAD --stat           # scope of changes
gh pr view                           # PHASE 2 should have opened a PR; check CodeRabbit status
```

For each commit, decide: keep, `git revert`, or `git rebase -i` to drop. The PR is already open, so cherry-picking via the GitHub UI works directly.

Exit the bypass-mode session before doing interactive review work, so subsequent sessions return to the plan-mode default.
