# Campaign Analysis Skill — Implementation Plan

## Context

A PRD (`tmp/prd/Card_Yeti_Campaign_Analysis_Skill_PRD.docx`) proposes a Claude Code skill for analyzing campaign performance. However, the PRD was written without knowledge of the codebase. **~80% of the proposed analytics already exist** as API endpoints in the running web app. The PRD's proposed raw SQL queries, schema discovery, and margin engine are unnecessary — the hexagonal architecture already provides all of this through a rich service layer and REST API.

**What we're actually building:** A Claude Code custom slash command (`/campaign-analysis`) that:
- Fetches live data from the existing API via `curl` to `localhost:8081`
- Cross-references data with the strategy document (`docs/private/CAMPAIGN_STRATEGY.md`)
- Engages the user in a **conversational discussion** about improvements (not reports or emails)
- Is invoked explicitly via `/campaign-analysis` only (no auto-invocation)

## What Already Exists (No Backend Changes Needed)

| Capability | Endpoint |
|---|---|
| Campaign list + config | `GET /api/campaigns` |
| Per-campaign P&L | `GET /api/campaigns/{id}/pnl` |
| Channel breakdown | `GET /api/campaigns/{id}/pnl-by-channel` |
| Fill rate / daily spend | `GET /api/campaigns/{id}/fill-rate?days=30` |
| Days-to-sell histogram | `GET /api/campaigns/{id}/days-to-sell` |
| Inventory aging + market signals | `GET /api/campaigns/{id}/inventory` |
| Tuning recommendations | `GET /api/campaigns/{id}/tuning` |
| Portfolio health | `GET /api/portfolio/health` |
| Portfolio insights (by char/grade/era) | `GET /api/portfolio/insights` |
| AI suggestions | `GET /api/portfolio/suggestions` |
| Weekly review | `GET /api/portfolio/weekly-review` |
| Capital timeline | `GET /api/portfolio/capital-timeline` |
| Channel velocity | `GET /api/portfolio/channel-velocity` |
| Credit summary | `GET /api/credit/summary` |
| Crack candidates | `GET /api/campaigns/{id}/crack-candidates` |
| Expected values | `GET /api/campaigns/{id}/expected-values` |
| Health check (no auth) | `GET /api/health` |

## Implementation

### Single file to create: `.claude/commands/campaign-analysis.md`

This is the only file needed. No backend changes, no new endpoints, no supporting scripts.

**Directory to create:** `.claude/commands/` (doesn't exist yet)

### Command Structure

```
---
description: "Analyze campaign performance with live API data and strategy context"
argument-hint: "[health | weekly | tuning | campaign <N>]"
allowed-tools: ["Bash", "Read", "Glob", "Grep"]
---
```

### Analysis Modes (determined by `$ARGUMENTS`)

| Argument | Mode | Primary Endpoints | Purpose |
|---|---|---|---|
| *(none)* | Full Overview | campaigns, portfolio/health, weekly-review, channel-velocity, credit/summary, capital-timeline | Broad portfolio scan → conversational deep dive |
| `health` | Quick Health | portfolio/health, credit/summary | Traffic-light status per campaign |
| `weekly` | Weekly Review | portfolio/weekly-review, portfolio/health, credit/summary, portfolio/suggestions | Monday review cadence from strategy doc |
| `tuning` | Tuning Discussion | campaigns (list), campaigns/{id}/tuning for each, portfolio/suggestions | Parameter adjustment discussion |
| `campaign N` | Single Campaign | campaigns/{N}, campaigns/{N}/pnl, pnl-by-channel, fill-rate, inventory, tuning, days-to-sell | Deep dive on one campaign |

### Command Content Design

The command file will instruct Claude to:

1. **Verify server** — `curl -sf http://localhost:8081/api/health`. If down, suggest starting it.

2. **Read strategy doc** — `Read docs/CAMPAIGN_STRATEGY.md` for business context (margin model, exit channels, campaign design intent, operational cadence, risk triggers).

3. **Fetch data** — Parallel `curl` calls to the appropriate endpoints for the selected mode. All endpoints require auth (`session_id` cookie) except `/api/health`. The command instructs Claude to try without auth first, prompt for the cookie on 401.

4. **Conversational analysis** — Present findings with specific numbers, cross-reference against strategy doc expectations, highlight concerns, then ask the user what to dig into. Key principles:
   - Lead with the most actionable finding
   - Use specific dollar amounts and percentages (API returns cents → convert)
   - Connect data to strategy doc sections (margin targets, fill rate expectations, exit routing)
   - Ask follow-up questions rather than dumping data
   - Flag risks: credit limit proximity, duplicate accumulation, slow inventory, CL accuracy
   - Note small sample sizes when data is insufficient

5. **Strategy cross-referencing** — The command maps API campaign IDs to strategy doc campaign names (Vintage Core, EX/e-Reader, Modern, etc.) based on config fields. It compares:
   - Actual fill rates vs. expected rates from strategy doc
   - Actual margins vs. theoretical margin formula (CL × 7.65% − $3 at 80% terms)
   - Exit channel usage vs. documented routing hierarchy
   - PSA 7 handling (no GameStop exit)
   - Credit utilization vs. $50K limit with invoicing cycle awareness

### Auth Handling

Campaign endpoints use session-based auth. The command handles this:
1. First attempt without auth cookie (dev mode may skip auth)
2. On 401, prompt user for their `session_id` cookie value
3. Retry with `-b "session_id=VALUE"` on all subsequent calls

### Monetary Values

All API responses use **cents**. The command instructs Claude to:
- Divide by 100 for display
- Format as `$X,XXX.XX`
- Buy terms are decimals (0.80 = 80%)

## Key Files Referenced

| File | Role |
|---|---|
| `.claude/commands/campaign-analysis.md` | **CREATE** — The custom command |
| `docs/CAMPAIGN_STRATEGY.md` | Read at runtime for business context |
| `internal/adapters/httpserver/router.go` | Reference for all API endpoint paths |
| `internal/domain/campaigns/analytics_types.go` | JSON response shapes for analytics endpoints |
| `internal/domain/campaigns/tuning_types.go` | JSON response shapes for tuning endpoint |

## What the PRD Proposed That We're NOT Building

| PRD Proposal | Why Not |
|---|---|
| Raw SQL queries (`queries/*.sql`) | Service layer + API already provides all analytics |
| Schema discovery | Not needed — repository pattern handles schema |
| Margin calculation engine | Already implemented in service layer with more sophistication |
| `validate-adjustment.sh` script | Conversational approach replaces automated validation |
| `adjustment-email.md` template | User wants conversation, not email generation |
| `weekly-report.md` template | Conversational mode replaces static reports |
| Auto-invocation triggers | User chose explicit `/campaign-analysis` only |
| Structured data export (CSV/JSON) | API already returns JSON; not needed for conversational flow |

## Verification

1. **Create the command file** and confirm `/campaign-analysis` appears in Claude Code's command list
2. **Test with server running** — invoke `/campaign-analysis` and verify it fetches real data
3. **Test each mode** — `health`, `weekly`, `tuning`, `campaign 1`
4. **Test server-down handling** — stop server, invoke command, confirm helpful error message
5. **Test auth flow** — verify 401 handling prompts for cookie correctly
6. **Verify strategy cross-referencing** — confirm campaign names are correctly matched and margin comparisons are accurate
