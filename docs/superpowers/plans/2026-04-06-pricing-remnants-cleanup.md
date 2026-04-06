# Pricing Remnants Cleanup Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all references to PriceCharting, CardHedger, JustTCG, and the fusion engine from docs, CI, config, skills, and code comments after these services were removed from the codebase.

**Architecture:** Pure cleanup — no behavioral changes. DH (DoubleHolo) is the sole price source. All removed code directories (`internal/adapters/clients/pricecharting/`, `cardhedger/`, `fusionprice/`, `internal/domain/fusion/`) are already deleted. This plan addresses only the stale references left behind in documentation, configuration, and comments.

**Tech Stack:** Markdown, YAML, Shell, Go (comments only)

**Working directory:** `/workspace/.worktrees/cleanupparty` (branch `guarzo/cleanupparty`)

---

### Task 1: Delete stale documentation files

**Files:**
- Delete: `docs/PRICING_VALIDATION.md`
- Delete: `docs/PRICING_DATA.md`
- Delete: `docs/TODO.md`
- Delete: `docs/cleanup-remaining-pricing-remnants.md`

- [ ] **Step 1: Delete the files**

```bash
cd /workspace/.worktrees/cleanupparty
git rm docs/PRICING_VALIDATION.md docs/PRICING_DATA.md docs/TODO.md docs/cleanup-remaining-pricing-remnants.md
```

- [ ] **Step 2: Verify no broken links reference these files from CLAUDE.md**

`CLAUDE.md` line 100 references `docs/PRICING_DATA.md` — the reference says "retained as-is" which is now wrong. Update:

```
Old: See `docs/PRICING_DATA.md` for historical reference (retained as-is).
New: (remove the sentence entirely, or replace with): Previous pricing sources (PriceCharting, CardHedger, JustTCG, fusion engine) were removed on 2026-04-06.
```

Also check `docs/ARCHITECTURE.md`, `docs/DEVELOPMENT.md`, `README.md` for links to the deleted files — these will be cleaned in their respective tasks.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "docs: delete stale pricing validation, pricing data, and TODO docs

These files exclusively documented PriceCharting, CardHedger, JustTCG,
and the fusion engine, all of which were removed on 2026-04-06.
PRICING_DATA.md was previously marked historical but is now fully stale.
cleanup-remaining-pricing-remnants.md was the plan driving this work."
```

---

### Task 2: Rewrite README.md

**Files:**
- Modify: `README.md`

The README has stale references on lines 18, 33, 50, 59, 66-70. The following sections need updating:

- [ ] **Step 1: Update Quick Start section (line 17-18)**

```markdown
Old:
# Set required API tokens
export PRICECHARTING_TOKEN="your_token"

New:
# Set API tokens (optional — enables pricing features)
export DH_ENTERPRISE_API_KEY="your_key"
```

- [ ] **Step 2: Update Features list (line 33)**

```markdown
Old:
- **Multi-Source Price Fusion** - Aggregates CardHedger and PriceCharting for accurate graded pricing

New:
- **DH Pricing** - Graded card pricing via DoubleHolo enterprise API
```

- [ ] **Step 3: Update "How It Works" paragraph (line 50)**

```markdown
Old:
The system compares real-time sold data (from PriceCharting/fusion) against Card Ladder valuations to reveal market direction, helping you choose the optimal sell channel.

New:
The system compares real-time sold data against Card Ladder valuations to reveal market direction, helping you choose the optimal sell channel.
```

- [ ] **Step 4: Update Requirements (line 59)**

```markdown
Old:
- [PriceCharting API token](https://www.pricecharting.com/api) (required)

New:
- DH Enterprise API key (optional — enables pricing)
```

- [ ] **Step 5: Update Environment Variables section (lines 64-71)**

```markdown
Old:
# Required
PRICECHARTING_TOKEN="..."    # Graded prices and sales data

# Optional
CARD_HEDGER_API_KEY="..."    # Supplementary pricing (unlimited plan)
LOG_LEVEL="info"             # debug, info, warn, error

New:
# Pricing (optional)
DH_API_BASE_URL="..."           # DoubleHolo API base URL
DH_ENTERPRISE_API_KEY="..."     # DoubleHolo enterprise API key

# General
LOG_LEVEL="info"                # debug, info, warn, error
```

- [ ] **Step 6: Fix Documentation links (line 126)**

```markdown
Old:
- [Next Steps](docs/NEXT_STEPS.md) - Planned features

New:
(remove — file does not exist)
```

- [ ] **Step 7: Commit**

```bash
git add README.md
git commit -m "docs: update README for DH-only pricing

Remove PriceCharting/CardHedger references from Quick Start, Features,
Requirements, and Environment Variables. DH is now the sole price source."
```

---

### Task 3: Rewrite docs/ARCHITECTURE.md

**Files:**
- Modify: `docs/ARCHITECTURE.md`

Heavy rewrite needed — fusion/PriceCharting/CardHedger references throughout.

- [ ] **Step 1: Update hex diagram (lines 14-20)**

Replace the Adapters layer box:

```markdown
Old:
│  • HTTP Handlers             • PriceCharting, CardHedger,   │
│  • Web Server                • TCGdex.dev                   │
│                              • TCGdex.dev                   │

New:
│  • HTTP Handlers             • DH (DoubleHolo) Pricing      │
│  • Web Server                • TCGdex.dev (card metadata)   │
```

- [ ] **Step 2: Update Domain layer box (line 28)**

```markdown
Old:
│  • Price Fusion            • Market Direction Signals       │

New:
│  • DH Pricing              • Market Direction Signals       │
```

- [ ] **Step 3: Update Package Structure (lines 64-83)**

Remove these lines:
```
    fusion/                 # Price fusion interfaces
```
```
      fusionprice/          # Multi-source price fusion (CardHedger + PriceCharting market data)
```
```
      pricecharting/        # PriceCharting API client
      cardhedger/           # CardHedger secondary pricing
```
And update the `storage/sqlite/` comment to remove `card_id_mappings + sync_state` if those were CardHedger-specific.

- [ ] **Step 4: Remove fusion interfaces block (lines 130-141)**

Delete the entire `// Fusion domain interfaces` code block (SecondaryPriceSource, CardIDResolver).

- [ ] **Step 5: Replace Price Fusion data flow section (lines 164-170)**

```markdown
Old:
### Price Fusion
1. CardHedger → graded price estimates with confidence ranges
2. PriceCharting → market data (active listings, sales velocity, grade prices)
3. Fusion provider merges, detects outliers, computes confidence
4. Result cached in SQLite + memory with configurable TTL

New:
### Pricing
1. DH (DoubleHolo) → graded price estimates, market data, sales history
2. Results cached in SQLite + memory with configurable TTL
```

- [ ] **Step 6: Update Dependency Injection example (lines 176-187)**

```markdown
Old:
// Secondary sources (each implements fusion.SecondaryPriceSource)
secondarySources := []fusion.SecondaryPriceSource{ppSource, chSource}
priceProvImpl := fusionprice.NewFusionProviderWithRepo(pcProvider, secondarySources, ...)

// Campaigns with optional market signals
priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)

New:
// Pricing via DH
priceProvImpl := dhprice.NewProvider(dhClient, ...)

// Campaigns with optional market signals
priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)
```

- [ ] **Step 7: Remove stale Key Design Decisions (lines 264-278)**

Delete the "FetchResult Pattern (Mar 2026)" section and the "CardHedger Integration (Mar 2026)" section entirely. These describe removed infrastructure.

- [ ] **Step 8: Update Domain Interfaces table (lines 295, 316-318)**

```markdown
Old:
| `pricing` | `PriceProvider` | `provider.go` | 5 | Card price lookup (PriceCharting) |
...
| `fusion` | `SecondaryPriceSource` | `source.go` | 3 | Price fusion data (CardHedger) |
| `fusion` | `CardIDResolver` | `source.go` | 3 | External ID cache |
| `fusion` | `PriceHintResolver` | `source.go` | 4 | User-provided price hints |

New:
| `pricing` | `PriceProvider` | `provider.go` | 5 | Card price lookup (DH) |

(remove the three fusion rows entirely — internal/domain/fusion/ no longer exists)
```

- [ ] **Step 9: Commit**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: update ARCHITECTURE.md for DH-only pricing

Remove all PriceCharting, CardHedger, and fusion engine references.
Update hex diagram, package structure, data flow, DI examples,
and domain interfaces table to reflect DH as sole price source."
```

---

### Task 4: Clean up docs/DEVELOPMENT.md

**Files:**
- Modify: `docs/DEVELOPMENT.md`

- [ ] **Step 1: Replace Rate Limiting table (lines 108-112)**

```markdown
Old:
| Provider | Limit | Notes |
|----------|-------|-------|
| PriceCharting | 60/min, 20k/day | Market data (listings, sales velocity) |
| CardHedger | 60/min (unlimited plan) | Secondary pricing estimates (429-monitored) |

New:
| Provider | Limit | Notes |
|----------|-------|-------|
| DH (DoubleHolo) | Enterprise plan | Graded pricing and market data |
```

- [ ] **Step 2: Remove PriceCharting section (lines 142-158)**

Delete the entire `### PriceCharting` subsection including the export command, price field mapping table, and code path reference.

- [ ] **Step 3: Remove CardHedger section (lines 166-177)**

Delete the entire `### CardHedger` subsection.

- [ ] **Step 4: Replace Fusion Price Provider section (lines 179-183)**

```markdown
Old:
### Fusion Price Provider

Merges data from CardHedger (graded price estimates) and PriceCharting (market data) into a single `Price` struct with confidence scores and fusion metadata. Uses `fusion.FetchResult` pattern to avoid shared mutable state.

Code: `internal/adapters/clients/fusionprice/`

New:
### DH Price Provider

Provides graded card pricing via the DoubleHolo enterprise API. Returns price estimates, market data, and sales history for PSA-graded cards.

Code: `internal/adapters/clients/dhprice/`
```

- [ ] **Step 5: Update Troubleshooting table (lines 224-233)**

Remove these rows:
```
| Market signals missing | Verify PriceCharting token is set; check `PriceLookup` wiring |
| CardHedger 429 errors | Unlimited plan; 429s indicate actual rate limiting. Check via /api/status/api-usage |
| `429 rate limited` on PriceCharting | Exceeded 1 req/sec. Wait for block expiry; check `rate_limiter.go` |
| CardHedger `Card is null` | Cert's card not in CardHedger DB. Expected for new/rare cards; null handling in `cert_resolver.go` |
```

Add:
```
| Market signals missing | Verify DH_ENTERPRISE_API_KEY is set; check `PriceLookup` wiring |
```

- [ ] **Step 6: Update Resilience Patterns (line 242)**

```markdown
Old:
- **Rate limits**: PriceCharting 1 req/sec, CardHedger 100 req/min + 700ms pause, auth 10 req/sec

New:
- **Rate limits**: DH enterprise (managed by provider), auth 10 req/sec
```

- [ ] **Step 7: Update migration table (line 70)**

```markdown
Old:
| 000002 | Card ID mappings table + sync state (for CardHedger external ID caching and delta poll state) |

New:
| 000002 | Card ID mappings table + sync state |
```

- [ ] **Step 8: Commit**

```bash
git add docs/DEVELOPMENT.md
git commit -m "docs: update DEVELOPMENT.md for DH-only pricing

Remove PriceCharting, CardHedger, and FusionPrice sections.
Update rate limits, troubleshooting, and resilience patterns."
```

---

### Task 5: Clean up docs/API.md

**Files:**
- Modify: `docs/API.md`

- [ ] **Step 1: Update API usage endpoint description (line 118)**

```markdown
Old:
Returns API call statistics for all pricing providers (cardhedger, pricecharting).

New:
Returns API call statistics for all pricing providers.
```

- [ ] **Step 2: Update API usage response example (lines 124-126)**

```markdown
Old:
      "name": "cardhedger",

New:
      "name": "doubleholo",
```

- [ ] **Step 3: Update pricing endpoint description (line 341)**

```markdown
Old:
Looks up fusion price data for a specific card.

New:
Looks up price data for a specific card.
```

- [ ] **Step 4: Update pricing response sources field (line 371)**

```markdown
Old:
  "sources": ["cardhedger"]

New:
  "sources": ["doubleholo"]
```

- [ ] **Step 5: Update price hints examples (lines 394, 414)**

```markdown
Old:
    "provider": "pricecharting",

New:
    "provider": "doubleholo",
```

- [ ] **Step 6: Update price hints provider validation (line 418)**

```markdown
Old:
`provider` must be `"pricecharting"` or `"cardhedger"`.

New:
`provider` must be `"doubleholo"`.
```

- [ ] **Step 7: Update card-requests endpoint descriptions (lines 535-554)**

Replace "CardHedger" with "external pricing service" in:
- Line 535: "Lists all CardHedger card request submissions" → "Lists all card request submissions"
- Line 545: "Submits a single pending card request to CardHedger" → "Submits a single pending card request"
- Line 554: error text `502` description: "CardHedger API error" → "external API error"
- Line 562: "Submits all pending card requests to CardHedger" → "Submits all pending card requests"

- [ ] **Step 8: Commit**

```bash
git add docs/API.md
git commit -m "docs: update API.md for DH-only pricing

Replace cardhedger/pricecharting provider examples with doubleholo.
Update card-requests endpoint descriptions."
```

---

### Task 6: Clean up internal/README.md

**Files:**
- Modify: `internal/README.md`

- [ ] **Step 1: Update first architecture diagram (lines 23-31)**

Remove these lines from the adapters box:
```
│    │   ├── cardhedger/     (supplementary pricing) │
│    │   ├── fusionprice/    (multi-source fusion)│
│    │   ├── pricecharting/  (graded prices + market) │
```

Add:
```
│    │   ├── dhprice/        (DH pricing)         │
```

- [ ] **Step 2: Update domain box (line 45)**

Remove:
```
│    ├── fusion/         (price fusion interfaces)│
```

- [ ] **Step 3: Update domain packages table (line 94)**

Remove:
```
| `fusion/` | Price fusion interfaces |
```

- [ ] **Step 4: Update second adapters structure diagram (lines 150-158)**

Remove:
```
│   ├── cardhedger/      # CardHedger supplementary pricing
│   ├── fusionprice/     # Multi-source price fusion (CardHedger + PriceCharting)
│   ├── pricecharting/   # PriceCharting graded prices + market data
```

Add:
```
│   ├── dhprice/         # DH (DoubleHolo) pricing
```

- [ ] **Step 5: Update code example (lines 118-128)**

Replace the `pricecharting.NewClient(...)` example with a DH-based example:

```go
// 2. Implement in adapter layer
package dhprice

type Provider struct { ... }

func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
    // API call implementation
}

// 3. Wire in main.go
dhProvider := dhprice.NewProvider(...)
service := someservice.NewService(dhProvider) // Inject interface
```

- [ ] **Step 6: Update anti-pattern example (lines 496-501)**

```go
Old:
import "github.com/guarzo/slabledger/internal/adapters/clients/pricecharting"

type Service struct {
    priceClient *pricecharting.Client // ❌ Direct dependency on adapter
}

New:
import "github.com/guarzo/slabledger/internal/adapters/clients/dhprice"

type Service struct {
    priceClient *dhprice.Provider // ❌ Direct dependency on adapter
}
```

- [ ] **Step 7: Update adapter test example (lines 537-547)**

```go
Old:
// internal/adapters/clients/pricecharting/client_test.go
func TestClient_GetPrice(t *testing.T) {
    ...
    client := pricecharting.NewClientWithHTTP(mockHTTP, "test-token")

New:
// internal/adapters/clients/dhprice/provider_test.go
func TestProvider_GetPrice(t *testing.T) {
    ...
    provider := dhprice.NewProviderWithHTTP(mockHTTP, "test-key")
```

- [ ] **Step 8: Update Large File Awareness table (lines 556-559)**

Remove the rows for deleted files:
```
| `adapters/clients/pricecharting/domain_adapter.go` | 637 | 6-strategy matching pipeline (cohesive) |
| `adapters/clients/fusionprice/fusion_provider.go` | 635 | Multi-source fusion (single purpose) |
```

- [ ] **Step 9: Commit**

```bash
git add internal/README.md
git commit -m "docs: update internal/README.md for DH-only pricing

Remove cardhedger, pricecharting, fusionprice, and fusion directory
references from architecture diagrams, code examples, and file tables."
```

---

### Task 7: Clean up docs/SCHEDULERS.md

**Files:**
- Modify: `docs/SCHEDULERS.md`

- [ ] **Step 1: Update Price Refresh description (line 75)**

```markdown
Old:
**Purpose:** Refreshes stale card prices by calling the fusion price provider.

New:
**Purpose:** Refreshes stale card prices by calling the DH price provider.
```

- [ ] **Step 2: Delete CardHedger Delta Refresh section (lines 91-104)**

Remove the entire `### CardHedger Delta Refresh` subsection.

- [ ] **Step 3: Delete CardHedger Batch section (lines 106-122)**

Remove the entire `### CardHedger Batch` subsection.

- [ ] **Step 4: Update Startup Timing diagram (lines 182-187)**

```markdown
Old:
T=0s     All schedulers start
T=0s     Price refresh, inventory refresh, cache warmup,
         access log cleanup, session cleanup run immediately
T=30s    CardHedger batch runs (populates card_id_mappings)
T=3m     CardHedger delta refresh runs (needs mappings from batch)

New:
T=0s     All schedulers start
T=0s     Price refresh, inventory refresh, cache warmup,
         access log cleanup, session cleanup run immediately
```

- [ ] **Step 5: Update Shutdown note (line 194)**

```markdown
Old:
4. Schedulers with their own `Wait()` (PriceRefresh, CardHedger*) can also be waited on individually

New:
4. Schedulers with their own `Wait()` (PriceRefresh) can also be waited on individually
```

- [ ] **Step 6: Update File Layout (lines 207-208)**

Remove:
```
├── cardhedger_refresh.go    # CardHedger delta poll scheduler
├── cardhedger_batch.go      # CardHedger daily batch scheduler
```

- [ ] **Step 7: Commit**

```bash
git add docs/SCHEDULERS.md
git commit -m "docs: remove CardHedger scheduler sections from SCHEDULERS.md

Delete CardHedger Delta Refresh and Batch sections.
Update price refresh description, startup timing, and file layout."
```

---

### Task 8: Update docs/SCHEMA.md and docs/USER_GUIDE.md

**Files:**
- Modify: `docs/SCHEMA.md`
- Modify: `docs/USER_GUIDE.md`

- [ ] **Step 1: Update SCHEMA.md provider CHECK constraints (lines 56, 76)**

These document old CHECK constraints that were removed in migration 000037.

```markdown
Old (line 56):
| `provider` | TEXT | PK, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |

New:
| `provider` | TEXT | PK, NOT NULL | |
```

```markdown
Old (line 76):
| `provider` | TEXT | NOT NULL, CHECK IN ('pricecharting','pokemonprice','cardmarket','cardhedger','fusion') | |

New:
| `provider` | TEXT | NOT NULL | |
```

- [ ] **Step 2: Update USER_GUIDE.md Market Direction section (line 213)**

```markdown
Old:
For each unsold card, the system compares the most recent sold price (from PriceCharting/fusion sources) against the Card Ladder valuation recorded at purchase:

New:
For each unsold card, the system compares the most recent sold price against the Card Ladder valuation recorded at purchase:
```

- [ ] **Step 3: Update USER_GUIDE.md Card Pricing section (line 239)**

```markdown
Old:
- Comparing prices across PriceCharting and CardHedger

New:
- Comparing prices across grades and sources
```

- [ ] **Step 4: Update USER_GUIDE.md API Status table (lines 260-261)**

```markdown
Old:
| **CardHedger** | 1,000 | Secondary graded price estimates with confidence ranges |
| **PriceCharting** | No hard limit | Market data (active listings, sales velocity) |

New:
| **DH (DoubleHolo)** | Enterprise plan | Graded pricing, market data, sales history |
```

- [ ] **Step 5: Commit**

```bash
git add docs/SCHEMA.md docs/USER_GUIDE.md
git commit -m "docs: update SCHEMA.md and USER_GUIDE.md for DH-only pricing

Update provider CHECK constraint docs to reflect migration 000037 removal.
Replace PriceCharting/CardHedger references in User Guide."
```

---

### Task 9: Clean up CI and dev config

**Files:**
- Modify: `.github/workflows/test.yml`
- Modify: `.devcontainer/post-start.sh`
- Modify: `.devcontainer/README.md`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Remove stale CI env vars (.github/workflows/test.yml lines 88-89)**

```yaml
Old:
      env:
        PRICECHARTING_TOKEN: ${{ secrets.PRICECHARTING_TOKEN }}
        CARD_HEDGER_API_KEY: ${{ secrets.CARD_HEDGER_API_KEY }}

New:
      env:
        DH_ENTERPRISE_API_KEY: ${{ secrets.DH_ENTERPRISE_API_KEY }}
```

- [ ] **Step 2: Update devcontainer post-start.sh (lines 53-58)**

```bash
Old:
    if ! grep -q "PRICECHARTING_TOKEN=your_pricecharting_token_here" .env && \
       ! grep -q "PSA_ACCESS_TOKEN=your_psa_token_here" .env; then
        echo "✅ API keys configured"
    else
        echo "⚠️  Warning: API keys not configured in .env file"
        echo "   Update PRICECHARTING_TOKEN and PSA_ACCESS_TOKEN"
    fi

New:
    if ! grep -q "DH_ENTERPRISE_API_KEY=your_dh_key_here" .env && \
       ! grep -q "PSA_ACCESS_TOKEN=your_psa_token_here" .env; then
        echo "✅ API keys configured"
    else
        echo "⚠️  Warning: API keys not configured in .env file"
        echo "   Update DH_ENTERPRISE_API_KEY and PSA_ACCESS_TOKEN"
    fi
```

- [ ] **Step 3: Update devcontainer README.md (line 91)**

```markdown
Old:
PRICECHARTING_TOKEN=your_actual_token

New:
DH_ENTERPRISE_API_KEY=your_actual_key
```

- [ ] **Step 4: Update docker-compose.yml**

Remove line 17:
```yaml
      PRICECHARTING_TOKEN: ${PRICECHARTING_TOKEN}
```

Replace lines 32-39 (Multi-Source Price Fusion section):
```yaml
Old:
      # =====================================================================
      # Optional: Multi-Source Price Fusion
      # =====================================================================
      # PokemonPriceTracker (primary graded price source)
      POKEMONPRICE_TRACKER_API_KEY: ${POKEMONPRICE_TRACKER_API_KEY:-}

      # CardHedger (supplementary pricing — unlimited plan)
      CARD_HEDGER_API_KEY: ${CARD_HEDGER_API_KEY:-}

New:
      # =====================================================================
      # Optional: DH Pricing
      # =====================================================================
      DH_API_BASE_URL: ${DH_API_BASE_URL:-}
      DH_ENTERPRISE_API_KEY: ${DH_ENTERPRISE_API_KEY:-}
```

Remove lines 54-57 (CardHedger scheduler):
```yaml
      # CardHedger scheduler
      CARD_HEDGER_POLL_INTERVAL: ${CARD_HEDGER_POLL_INTERVAL:-1h}
      CARD_HEDGER_BATCH_INTERVAL: ${CARD_HEDGER_BATCH_INTERVAL:-24h}
      CARD_HEDGER_MAX_CARDS_PER_RUN: ${CARD_HEDGER_MAX_CARDS_PER_RUN:-200}
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/test.yml .devcontainer/post-start.sh .devcontainer/README.md docker-compose.yml
git commit -m "ci: remove stale PriceCharting/CardHedger env vars

Update CI secrets, devcontainer setup, and docker-compose to reference
DH_ENTERPRISE_API_KEY instead of removed provider keys."
```

---

### Task 10: Clean up Claude Code skills

**Files:**
- Modify: `.claude/skills/csv-import/SKILL.md`
- Modify: `.claude/skills/csv-import/references/csv-formats.md`
- Modify: `.claude/skills/new-api-client/references/example-client.md`

- [ ] **Step 1: Update csv-import SKILL.md (lines 189-191)**

```markdown
Old:
- **CL Import**: Triggers CardHedger discovery for all `allocated` and `refreshed` rows (5-minute timeout, background goroutine)
- **PSA Import**: Triggers CardHedger discovery for `allocated` and `updated` rows; cert enrichment queued for card metadata lookup (see `certEnrichmentPending` count in response)
- **Shopify Import**: Triggers CardHedger discovery for `imported` and `updated` rows

New:
- **CL Import**: Triggers DH price refresh for all `allocated` and `refreshed` rows (5-minute timeout, background goroutine)
- **PSA Import**: Triggers DH price refresh for `allocated` and `updated` rows; cert enrichment queued for card metadata lookup (see `certEnrichmentPending` count in response)
- **Shopify Import**: Triggers DH price refresh for `imported` and `updated` rows
```

- [ ] **Step 2: Update csv-formats.md (line 114)**

```markdown
Old:
| `listing title` | string | Raw PSA listing title; used for card name extraction and as CardHedger LLM fallback |

New:
| `listing title` | string | Raw PSA listing title; used for card name extraction |
```

- [ ] **Step 3: Update new-api-client example (line 84)**

```markdown
Old:
return nil, 0, nil, apperrors.ConfigMissing("card_hedger_api_key", "CARD_HEDGER_API_KEY")

New:
return nil, 0, nil, apperrors.ConfigMissing("dh_enterprise_api_key", "DH_ENTERPRISE_API_KEY")
```

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/csv-import/SKILL.md .claude/skills/csv-import/references/csv-formats.md .claude/skills/new-api-client/references/example-client.md
git commit -m "docs: update Claude Code skills for DH-only pricing

Replace CardHedger discovery references with DH price refresh
in csv-import skill. Update example API client env var."
```

---

### Task 11: Fix code comments and test assertions

**Files:**
- Modify: `internal/domain/errors/errors_test.go:181`
- Modify: `internal/testutil/mocks/README.md:58`
- Modify: `internal/domain/campaigns/service_snapshots.go:55,64,87`
- Modify: `internal/adapters/scheduler/inventory_refresh.go:109`

- [ ] **Step 1: Fix errors_test.go assertion message (line 181)**

```go
Old:
		t.Errorf("Context[provider] = %v, want PriceCharting", err2.Context["provider"])

New:
		t.Errorf("Context[provider] = %v, want doubleholo", err2.Context["provider"])
```

- [ ] **Step 2: Fix mocks README.md description (line 58)**

```markdown
Old:
Mocks the PriceCharting price provider interface.

New:
Mocks the DH price provider interface.
```

- [ ] **Step 3: Update service_snapshots.go comments**

Line 55:
```go
Old:
// applyCLCorrection adjusts snapshot values when the fusion pipeline produces unreliable results.

New:
// applyCLCorrection adjusts snapshot values when the pricing pipeline produces unreliable results.
```

Line 64:
```go
Old:
// When multiple sources agree (sourceCount >= 2), the fusion result is trusted even if

New:
// When multiple sources agree (sourceCount >= 2), the pricing result is trusted even if
```

Line 87:
```go
Old:
		// Multi-source fusion that diverges from CL is more likely correct (CL may be stale).

New:
		// Multi-source pricing that diverges from CL is more likely correct (CL may be stale).
```

- [ ] **Step 4: Update inventory_refresh.go comment (line 109)**

```go
Old:
	// are already coalesced at the fusion provider layer via singleflight

New:
	// are already coalesced at the price provider layer via singleflight
```

- [ ] **Step 5: Run tests to verify no breakage**

```bash
cd /workspace/.worktrees/cleanupparty
go test ./internal/domain/errors/ ./internal/domain/campaigns/ ./internal/adapters/scheduler/ -v -count=1
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/errors/errors_test.go internal/testutil/mocks/README.md internal/domain/campaigns/service_snapshots.go internal/adapters/scheduler/inventory_refresh.go
git commit -m "fix: update stale pricing comments and test assertions

Fix 'want PriceCharting' assertion message in errors_test.go.
Replace 'fusion' references in snapshot and inventory comments."
```

---

### Task 12: Update CLAUDE.md pricing reference

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the PRICING_DATA.md reference (line ~100)**

The line currently reads something like:
```
See `docs/PRICING_DATA.md` for historical reference (retained as-is).
```

Replace with:
```
Previous pricing sources (PriceCharting, CardHedger, JustTCG, fusion engine) were removed on 2026-04-06.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md pricing reference

Remove link to deleted PRICING_DATA.md, note removal date instead."
```

---

### Task 13: Final verification

- [ ] **Step 1: Run make check**

```bash
cd /workspace/.worktrees/cleanupparty
make check
```

Expected: passes (lint + architecture import check + file size check).

- [ ] **Step 2: Run full test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 3: Grep for remaining stale references (excluding Keep-As-Is files)**

```bash
# Should only find matches in migration files, dh/types.go (pricecharting_id),
# card_request_repository.go (cardhedger_request_id column), web/src/js/api/client.ts,
# historical docs (2026-03-26-*, plans/*, phase4-*), and PRICING_DATA.md is now deleted
grep -ri "pricecharting\|cardhedger\|fusionprice\|justtcg" \
  --include="*.go" --include="*.md" --include="*.yml" --include="*.yaml" \
  --include="*.sh" --include="*.ts" --include="*.tsx" \
  --exclude-dir=migrations \
  --exclude-dir="docs/plans" \
  . | grep -v "2026-03-26" | grep -v "phase4-" | grep -v "pricecharting_id" | grep -v "cardhedger_request_id" | grep -v "cardhedgerRequestId"
```

Expected: no matches (or only acceptable Keep-As-Is references).

- [ ] **Step 4: If any stale references found, fix and amend the relevant commit**
