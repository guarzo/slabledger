# campaign-analysis-config schema

The campaign-analysis skill loads operator-specific configuration from `docs/private/campaign-analysis-config.md` at Step 0. This file documents the expected shape so a new operator (or a sanitized worktree restore) can recreate it.

## Format

The config file is markdown with a YAML-style structured block. The skill reads it as prose — there's no parser — but keeping the shape predictable makes the skill's Step 0 instructions stable.

## Required fields

| Field | Type | Description |
|-------|------|-------------|
| `operatorName` | string | Display name used in the analyst persona, e.g. "Card Yeti" |
| `operatorPersona` | string (1-3 sentences) | Framing for the analyst tone — business model, exit channels, edge thesis |
| `productionURL` | string | Production API base URL, e.g. `https://slabledger.example.com` (no trailing slash) |
| `canonicalCampaigns` | list | Ordered campaign list (1..N) used for the "Portfolio at a glance" line and Playbook A's updated-campaign-list output |

Each entry in `canonicalCampaigns`:

| Field | Type | Description |
|-------|------|-------------|
| `number` | int | Canonical position (1-indexed) |
| `name` | string | Campaign name as it appears in the API and strategy doc |
| `status` | enum | `active`, `paused`, `pending`, or `removed` |

## Optional fields

| Field | Type | Description |
|-------|------|-------------|
| `operationalPriorities` | list[string] | Tags that promote informational signals to mover candidates. Currently recognized: `dh_listing_gap` (treats received-but-not-listed as a mover instead of background noise) |
| `capitalSummaryConventions` | object | Operator-specific framing for the capital summary line — `healthyThresholdWeeks` (float), `alertLevelLabels` (`{ ok, warning, critical }`) |

## Example

````markdown
# campaign-analysis configuration

operatorName: Card Yeti
operatorPersona: |
  Card Yeti operates PSA Partner Offers — buying graded Pokemon cards at
  contract terms against CardLadder values, reselling through DH (eBay +
  Shopify multi-channel), card shows, and LGS as the liquidation backstop.
  Edge is on CL-lag patterns: high-$ vintage/mid-era pockets where CL
  drifts up after purchase.

productionURL: https://slabledger.example.com

canonicalCampaigns:
  - { number: 1, name: "Vintage Core", status: active }
  - { number: 2, name: "Vintage-EX PSA 8 Precision", status: active }
  - { number: 3, name: "EX/e-Reader Era", status: active }
  - { number: 7, name: "Crystal/HGSS", status: active }
  - { number: 10, name: "Modern Premium", status: paused }

operationalPriorities:
  - dh_listing_gap

capitalSummaryConventions:
  healthyThresholdWeeks: 5
  alertLevelLabels:
    ok: "Healthy"
    warning: "Tight"
    critical: "Critical"
````

## What to do when the config file is missing

The skill's Step 0 says: *"If the file is missing, continue with generic analysis. You won't know the operator name, production URL, or canonical campaign numbers — note this to the user and proceed with data-only analysis."*

In practice, missing config means:
- No persona framing — the analyst speaks generically
- No production fallback — only localhost is available; Step 2's reachability check skips production
- Bare numbers used for campaigns until names are resolved via `/api/campaigns` (the conversational guideline #4 "use names not numbers" rule still applies, but mapping numbers to names depends on API ordering)
- `dh_listing_gap` defaults to informational (not promoted to a mover)
