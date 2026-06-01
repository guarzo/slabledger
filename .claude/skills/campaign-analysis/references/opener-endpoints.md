# Opener — Endpoint Reference

This file describes the underlying API fetches the Layer-1 domain agents perform
on the operator's behalf. Read when manually debugging an agent, running a
one-off snapshot outside the multi-agent flow, or sanity-checking that an agent
covered everything it should.

The canonical opener flow is the Layer-1 → Layer-2 → Layer-3 multi-agent pipeline
in `SKILL.md` Step 3. This file is the operational substrate beneath it.

## Mandatory fetches (every opener)

Fetch all of these in parallel, every time:

| Endpoint | What it provides |
|----------|------------------|
| `GET /api/campaigns` | Name ↔ UUID resolution; filter `phase=archived` and `kind=external` |
| `GET /api/portfolio/snapshot` | Composite: `health`, `insights`, `weeklyReview`, `weeklyHistory` (8w), `channelVelocity`, `suggestions`, `creditSummary`, `invoices` — replaces 8 individual calls |
| `GET /api/inventory` | Per-purchase detail. The opener uses `inHandUnsoldCount` / `inHandCapitalCents` / `inTransitUnsoldCount` / `inTransitCapitalCents` already on `snapshot.health`; fetch `/api/inventory` for per-card detail |
| `GET /api/dh/status` | Listed vs in-inventory vs pending counts |
| `GET /api/dh/pending` | Per-item pending-push queue with `daysQueued` and `dhConfidence` (high <24h, medium <7d, low >7d) |
| `GET /api/intelligence/niches?window=30d&limit=20` | Coverage-gap demand signal — high `opportunity_score` + zero `current_coverage` = candidate |
| `GET /api/intelligence/campaign-signals` | Per-campaign acceleration/deceleration. Empty body has `signals: []`, `data_quality: "empty"` |
| `GET /api/opportunities/crack` | Slabs worth cracking — capital-positive, bypasses guardrail |
| `GET /api/opportunities/acquisition` | Raw-to-graded mispricings — feeds Playbook F |
| `GET /api/campaigns/{id}/tuning` ×N | Grade-level ROI, `avgBuyPctOfCL`, sample sizes — one call per active campaign with ≥10 purchases, **in parallel** |
| `GET /api/campaigns/{id}/fill-rate` ×N | Daily spend vs cap (30-day rolling) — one call per active campaign, **in parallel** |

**Per-campaign fetches must be parallel, not sequential.** `/tuning` byGrade and
`/fill-rate` are the highest-resolution tuning signals in the API; the opener's
movers should look there before leaning on `/portfolio/suggestions`.

## Procedural rules attached to specific endpoints

- **`snapshot.suggestions`** — apply the stale-suggestion filter (drop
  suggestions targeting fields on a campaign whose `updatedAt` is within 72h)
  before surfacing any entry. Treat the remainder as one input among several;
  per-campaign `/tuning` + `/insights` segmentation has higher-resolution signal.
- **`snapshot.insights`** — extract `byCharacter` (filter `soldCount ≥ 3`, sort
  by `roi` desc), `byGrade`, `byPriceTier`, `byCharacterGrade` standouts, and
  `coverageGaps` before drafting the opener. **Apply the Step 1b currentScope
  filter to every campaign-attributed segment row before it can drive a mover.**
  Listing only response keys is not analysis.
- **`/dh/status` listing gap** — informational by default. Promote to a mover
  candidate ONLY if the operator config lists `dh_listing_gap` in
  `operationalPriorities`.

For JSON shapes and field names of every endpoint above, consult
`references/api-cheatsheet.md` before writing parsing code.

## Conditional fetches (use only when warranted)

- `GET /api/campaigns/{id}/projections` — only when validating a specific tuning
  suggestion's projected impact. The endpoint is heavy; prefer `/tuning` byGrade
  for sizing.
