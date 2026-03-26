# Maintainability & AI-Agent Friendliness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose 6 large files into focused units, improve CLAUDE.md for AI agents, and add CI guardrails.

**Architecture:** Pure file reorganization — no behavioral changes, no interface changes, no new packages. Each task moves functions between files within the same Go package. Documentation edits are additive. CI scripts are new files.

**Tech Stack:** Go 1.26, bash scripts, GitHub Actions YAML, Make

**Spec:** `docs/superpowers/specs/2026-03-26-maintainability-design.md`

**Important:** Every structural task (B1–B6) MUST compile and pass tests before committing. The pattern is identical for each: read the source file, create new files with the extracted functions, remove those functions from the original, verify with `go build` and `go test`. No tests need to be written — these are pure moves.

**User preferences:** Do NOT use worktree isolation for parallel agents (merge conflicts are too painful). Always run `golangci-lint run` and frontend lint/typecheck after code changes.

---

## Task 1: B1 — Split `advisortool/executor.go` into 3 files

**Files:**
- Modify: `internal/adapters/advisortool/executor.go` (keep lines 1–229: core executor, infra, JSON helpers, registerTools)
- Create: `internal/adapters/advisortool/tools_campaign.go` (lines 231–307: 7 campaign tool registrations)
- Create: `internal/adapters/advisortool/tools_portfolio.go` (lines 309–650: 14 portfolio/analysis tools + dashboard + jsonSchema type)

- [ ] **Step 1: Create `tools_campaign.go`**

Create the file with package declaration and the 7 campaign tool registration methods. Cut these functions from `executor.go`:
- `registerListCampaigns` (lines 233–255)
- `registerGetCampaignPNL` (lines 257–261)
- `registerGetPNLByChannel` (lines 263–267)
- `registerGetCampaignTuning` (lines 269–273)
- `registerGetInventoryAging` (lines 275–279)
- `registerGetGlobalInventory` (lines 281–293)
- `registerGetSellSheet` (lines 295–307)

The file needs these imports (subset of executor.go):
```go
package advisortool

import (
	"context"
	"encoding/json"

	"github.com/guarzo/slabledger/internal/domain/ai"
)
```

- [ ] **Step 2: Create `tools_portfolio.go`**

Create the file with the remaining tool registration methods, dashboard type, and `jsonSchema` type. Cut from `executor.go`:
- `registerGetPortfolioHealth` through `registerGetSuggestionStats` (lines 309–537)
- `dashboardSummary` type (lines 540–572)
- `registerGetDashboardSummary` (lines 574–641)
- `jsonSchema` type (lines 643–649)

The file needs these imports:
```go
package advisortool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)
```

- [ ] **Step 3: Remove moved code from `executor.go`**

Delete lines 231–650 from `executor.go`. The file should end after `registerTools()` (line ~229). Keep the `// --- Tool registrations ---` comment removal clean.

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/adapters/advisortool/...`
Expected: SUCCESS (no errors)

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/advisortool/...`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./internal/adapters/advisortool/...`
Expected: No new issues

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/advisortool/executor.go \
       internal/adapters/advisortool/tools_campaign.go \
       internal/adapters/advisortool/tools_portfolio.go
git commit -m "refactor: split advisortool/executor.go into 3 focused files

Extract campaign tool registrations into tools_campaign.go and
portfolio/analysis tools into tools_portfolio.go. No behavioral changes."
```

---

## Task 2: B2 — Split `fusionprice/fusion_provider.go` into 2 files

**Files:**
- Modify: `internal/adapters/clients/fusionprice/fusion_provider.go` (keep: provider struct, GetPrice, getPriceFromSources, attachSourceDetails, Available, Name, Close, GetStats)
- Create: `internal/adapters/clients/fusionprice/card_resolver.go` (LookupCard, applyPCData, cleanupStaleName)

- [ ] **Step 1: Create `card_resolver.go`**

Create the file. Cut these functions from `fusion_provider.go`:
- `LookupCard` method (lines 403–565)
- `applyPCData` function (lines 567–598)
- `cleanupStaleName` method (lines 600–625)

The file needs these imports (determine exact set by what the moved functions reference):
```go
package fusionprice

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)
```

- [ ] **Step 2: Remove moved code from `fusion_provider.go`**

Delete the `LookupCard`, `applyPCData`, and `cleanupStaleName` functions. Clean up any imports that are no longer used in `fusion_provider.go` (likely `constants` and possibly `domainCards`).

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/adapters/clients/fusionprice/...`
Expected: SUCCESS

- [ ] **Step 4: Run tests**

Run: `go test ./internal/adapters/clients/fusionprice/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/adapters/clients/fusionprice/...`
Expected: No new issues

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/fusionprice/fusion_provider.go \
       internal/adapters/clients/fusionprice/card_resolver.go
git commit -m "refactor: extract card resolution from fusion_provider.go

Move LookupCard, applyPCData, and cleanupStaleName into card_resolver.go.
Core pipeline (GetPrice, getPriceFromSources) stays in fusion_provider.go."
```

---

## Task 3: B3 — Split `azureai/client.go` into 3 files

**Files:**
- Modify: `internal/adapters/clients/azureai/client.go` (keep: Config, Client, NewClient, StreamCompletion, pollResponseFallback, error types)
- Create: `internal/adapters/clients/azureai/request.go` (doStreamCompletion, buildRequest, buildURL, isAzureOpenAI, useResponsesAPI)
- Create: `internal/adapters/clients/azureai/stream.go` (parseSSEStream, isPermanentError, flattenToolCalls)

- [ ] **Step 1: Create `stream.go`**

Cut from `client.go`:
- `parseSSEStream` method (lines 502–587)
- `isPermanentError` function (lines 589–603)
- `flattenToolCalls` function (lines 605–613)

```go
package azureai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 2: Create `request.go`**

Cut from `client.go`:
- `useResponsesAPI` method (lines 97–101)
- `doStreamCompletion` method (lines 330–417)
- `buildRequest` method (lines 419–480)
- `isAzureOpenAI` function (lines 482–489)
- `buildURL` method (lines 491–500)

```go
package azureai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 3: Remove moved code from `client.go`**

Delete the moved functions. Clean up unused imports. The file should contain: Config, Option, WithLogger, Client struct, NewClient, StreamCompletion, pollResponseFallback, rateLimitError, capacityError, maxStreamRetries, and the compile-time interface check.

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/adapters/clients/azureai/...`
Expected: SUCCESS

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/clients/azureai/...`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./internal/adapters/clients/azureai/...`
Expected: No new issues

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/clients/azureai/client.go \
       internal/adapters/clients/azureai/request.go \
       internal/adapters/clients/azureai/stream.go
git commit -m "refactor: split azureai/client.go into client, request, stream

Extract HTTP request building into request.go and SSE stream parsing
into stream.go. Core client with retry logic stays in client.go."
```

---

## Task 4: B4 — Split `pricecharting/domain_adapter.go` into 3 files

**Files:**
- Modify: `internal/adapters/clients/pricecharting/domain_adapter.go` (keep: LookupCard, GetStats, lookupCardInternal, converters, logTrace, getStatsInternal)
- Create: `internal/adapters/clients/pricecharting/lookup_strategies.go` (tryCache, tryUPC, tryAPI, tryFuzzy, resolveExpectedNumber, extractSetHint)
- Create: `internal/adapters/clients/pricecharting/enrichment.go` (enrichMatch, applyConservativeExits, applyLastSoldByGrade, saleRecordsFromRecentSales)

- [ ] **Step 1: Create `enrichment.go`**

Cut from `domain_adapter.go`:
- `saleRecordsFromRecentSales` function (lines 442–453)
- `applyConservativeExits` method (lines 455–481)
- `applyLastSoldByGrade` method (lines 483–490)
- `enrichMatch` method (lines 512–515)

```go
package pricecharting

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/pricing/analysis"
)
```

- [ ] **Step 2: Create `lookup_strategies.go`**

Cut from `domain_adapter.go`:
- `tryCache` method (lines 237–253)
- `tryUPC` method (lines 255–281)
- `tryAPI` method (lines 283–413)
- `extractSetHint` function (lines 415–440)
- `resolveExpectedNumber` function (lines 492–508)
- `tryFuzzy` method (lines 517–596)

```go
package pricecharting

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 3: Remove moved code from `domain_adapter.go`**

Delete the moved functions. Clean up unused imports. The file should retain: `LookupCard`, `GetStats`, `toLookupPrice`, `toDomainProviderStats`, `lookupCardInternal`, `logTrace`, `getStatsInternal`, and the `SingleSourceBaselineConfidence` constant.

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/adapters/clients/pricecharting/...`
Expected: SUCCESS

- [ ] **Step 5: Run tests**

Run: `go test ./internal/adapters/clients/pricecharting/...`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./internal/adapters/clients/pricecharting/...`
Expected: No new issues

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/clients/pricecharting/domain_adapter.go \
       internal/adapters/clients/pricecharting/lookup_strategies.go \
       internal/adapters/clients/pricecharting/enrichment.go
git commit -m "refactor: split pricecharting/domain_adapter.go into 3 files

Extract lookup strategies (cache, UPC, API, fuzzy) into lookup_strategies.go
and enrichment logic into enrichment.go. Orchestration stays in domain_adapter.go."
```

---

## Task 5: B5 — Split `social/service_impl.go` into 3 files

**Files:**
- Modify: `internal/domain/social/service_impl.go` (keep: service struct, NewService, DetectAndGenerate, llmGenerate, ruleBasedGenerate, detectPostType, detectNewArrivals, filterPriceMovers, filterHotDeals, deduplicateByCardIdentity)
- Create: `internal/domain/social/publishing.go` (Publish, publishAsync, setPublishError, RegenerateCaption, ListPosts, GetPost, UpdateCaption, Delete)
- Create: `internal/domain/social/caption.go` (generateCaptionAsync, generateBackgroundsAsync, logError, safeGo, Wait, parseCaption, truncateCaption, captionResponse, parseCaptionResponse, stripMarkdownFences, sanitizeLLMJSON, generateID)

- [ ] **Step 1: Create `caption.go`**

Cut from `service_impl.go`:
- `generateCaptionAsync` method (lines 432–506)
- `generateBackgroundsAsync` method (lines 508–617)
- `logError` method (lines 834–840)
- `safeGo` method (lines 842–854)
- `Wait` method (line 856)
- `parseCaption` function (lines 858–884)
- `truncateCaption` function (lines 886–900)
- `captionResponse` type (lines 902–907)
- `parseCaptionResponse` function (lines 909–927)
- `stripMarkdownFences` function (lines 929–936)
- `sanitizeLLMJSON` function (lines 938–978)
- `generateID` function (lines 980–983)

Imports needed:
```go
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 2: Create `publishing.go`**

Cut from `service_impl.go`:
- `Publish` method (lines 619–646)
- `publishAsync` method (lines 648–722)
- `setPublishError` method (lines 724–735)
- `RegenerateCaption` method (lines 737–800)
- `ListPosts` method (lines 802–804)
- `GetPost` method (lines 806–824)
- `UpdateCaption` method (lines 826–828)
- `Delete` method (lines 830–832)

Imports needed:
```go
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 3: Remove moved code from `service_impl.go`**

Delete the moved functions. Clean up unused imports (likely `os`, `uuid` can be removed). The file should retain: constants, `service` struct, `NewService`, `var _ Service`, `DetectAndGenerate`, `llmGenerate`, `ruleBasedGenerate`, `detectPostType`, `detectNewArrivals`, `filterPriceMovers`, `filterHotDeals`, `deduplicateByCardIdentity`.

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/domain/social/...`
Expected: SUCCESS

- [ ] **Step 5: Run tests**

Run: `go test ./internal/domain/social/...`
Expected: PASS

- [ ] **Step 6: Run linter**

Run: `golangci-lint run ./internal/domain/social/...`
Expected: No new issues

- [ ] **Step 7: Commit**

```bash
git add internal/domain/social/service_impl.go \
       internal/domain/social/publishing.go \
       internal/domain/social/caption.go
git commit -m "refactor: split social/service_impl.go (984L) into 3 files

Extract publishing methods into publishing.go and caption/background
generation + utilities into caption.go. Detection logic stays in service_impl.go."
```

---

## Task 6: B6 — Split `campaigns/service_analytics.go` into 2 files

**Files:**
- Modify: `internal/domain/campaigns/service_analytics.go` (keep: PNL methods, snapshot helpers, aging, tuning)
- Create: `internal/domain/campaigns/service_sell_sheet.go` (sell sheet, Shopify, price review, price flags, recommendedPrice)

- [ ] **Step 1: Create `service_sell_sheet.go`**

Cut from `service_analytics.go`:
- `enrichSellSheetItem` method (lines 270–346)
- `recommendChannel` function (lines 348–360)
- `GenerateSellSheet` method (lines 362–393)
- `GenerateGlobalSellSheet` method (lines 395–433)
- `computeRecommendation` function (lines 435–446)
- `computeTargetPrice` function (lines 448–457)
- `MatchShopifyPrices` method (lines 576–645)
- `SetReviewedPrice` method (lines 649–662)
- `GetReviewStats` method (lines 664–666)
- `GetGlobalReviewStats` method (lines 668–670)
- `CreatePriceFlag` method (lines 674–689)
- `ListPriceFlags` method (lines 691–693)
- `ResolvePriceFlag` method (lines 695–697)
- `recommendedPrice` function (lines 699–714)

Imports needed:
```go
package campaigns

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
)
```

- [ ] **Step 2: Remove moved code from `service_analytics.go`**

Delete the moved functions. Remove the `// --- Sell Sheet ---`, `// --- Price Review ---`, and `// --- Price Flags ---` section comments. Clean up unused imports (likely `errors` import can be removed). The file should retain: PNL methods, `hasAnyPriceData`, `snapshotFromPurchase`, aging methods, `applyCLSignal`, `applyOpenFlags`, and `GetCampaignTuning` with its section comment `// --- Tuning ---`.

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/domain/campaigns/...`
Expected: SUCCESS

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/campaigns/...`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run ./internal/domain/campaigns/...`
Expected: No new issues

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/service_analytics.go \
       internal/domain/campaigns/service_sell_sheet.go
git commit -m "refactor: extract sell sheet and price review from service_analytics.go

Move sell sheet generation, Shopify price sync, price review, and price
flag methods into service_sell_sheet.go. PNL, aging, and tuning stay."
```

---

## Task 7: B7 — Migrate `azureai/image_client.go` from raw `http.Client` to `httpx.Client`

**Files:**
- Modify: `internal/adapters/clients/azureai/image_client.go`

The `ImageClient` uses a raw `*http.Client` (line 30) while every other API client in the codebase uses `httpx.Client` for retry + circuit breaker. This is an architectural inconsistency — the image client silently lacks resilience that all other clients get.

**Pattern to follow:** See `internal/adapters/clients/pokemonprice/client.go` lines 46–49 for the canonical `httpx.NewClient` usage.

- [ ] **Step 1: Change `httpClient` field from `*http.Client` to `*httpx.Client`**

In `image_client.go`, change line 30:
```go
// Before:
httpClient *http.Client

// After:
httpClient *httpx.Client
```

- [ ] **Step 2: Update `NewImageClient` constructor to create an `httpx.Client`**

Replace the raw `http.Client` construction (lines 51–55) with:
```go
config := httpx.DefaultConfig("AzureAIImage")
config.DefaultTimeout = 2 * time.Minute // image generation is slow
c := &ImageClient{
    config:     cfg,
    httpClient: httpx.NewClient(config),
}
```

Add `"time"` to imports and ensure `httpx` import is present (it already is for `httpx.DefaultTransport()`).

- [ ] **Step 3: Update `GenerateImage` to use `httpx.Client.Post`**

Replace the manual HTTP request construction + `c.httpClient.Do(httpReq)` block (lines 111–132) with a call to `c.httpClient.Post`:

```go
headers := map[string]string{
    "Content-Type": "application/json",
}
if isAzureOpenAI(endpoint) {
    headers["api-key"] = c.config.APIKey
} else {
    headers["Authorization"] = "Bearer " + c.config.APIKey
}

resp, err := c.httpClient.Post(ctx, url, headers, body, 2*time.Minute)
if err != nil {
    return nil, fmt.Errorf("azureai image: http request: %w", err)
}
respBody := resp.Body
```

Remove the `resp.Body.Close()` defer since `httpx.Client` reads and closes the body internally (it returns `Response.Body` as `[]byte`). Remove the `io.ReadAll` call since `resp.Body` is already `[]byte`. Also remove the manual status code check since `httpx.Client.Post` returns an error for non-2xx status codes.

Check `httpx.Response` struct — if it wraps status code differently, adjust the error handling accordingly.

- [ ] **Step 4: Clean up unused imports**

Remove `"bytes"`, `"io"`, and `"net/http"` if they are no longer used. Add `"time"` if not already present.

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/adapters/clients/azureai/...`
Expected: SUCCESS

- [ ] **Step 6: Run tests**

Run: `go test ./internal/adapters/clients/azureai/...`
Expected: PASS

- [ ] **Step 7: Run linter**

Run: `golangci-lint run ./internal/adapters/clients/azureai/...`
Expected: No new issues

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/clients/azureai/image_client.go
git commit -m "refactor: migrate image_client.go from raw http.Client to httpx.Client

Gives the image generation client retry + circuit breaker resilience,
consistent with all other API clients in the codebase."
```

---

## Task 8: B8 — Remove `os` import from domain `social/service_impl.go` via `MediaStore` interface

**Files:**
- Create: `internal/domain/social/media.go` (new `MediaStore` interface)
- Modify: `internal/domain/social/service.go` (add `WithMediaStore` option)
- Modify: `internal/domain/social/service_impl.go` (add `mediaStore` field to `service` struct)
- Modify: `internal/domain/social/caption.go` (replace `os.MkdirAll`/`os.WriteFile` calls — this file is created in Task 5)
- Create: `internal/adapters/storage/mediafs/store.go` (filesystem implementation)
- Modify: `cmd/slabledger/init.go` (wire the adapter)

The domain package `internal/domain/social/` imports `os` for writing image files to disk. Domain code should depend only on interfaces, not concrete I/O. This task introduces a `MediaStore` interface in the domain and a filesystem adapter.

- [ ] **Step 1: Create `internal/domain/social/media.go`**

```go
package social

// MediaStore abstracts file storage for generated media (backgrounds, slides).
// Domain code uses this interface instead of os.MkdirAll / os.WriteFile directly.
type MediaStore interface {
	// EnsureDir creates a directory (and parents) if it doesn't exist.
	EnsureDir(path string) error
	// WriteFile writes data to a file, creating it if necessary.
	WriteFile(path string, data []byte) error
}
```

- [ ] **Step 2: Add `mediaStore` field to `service` struct and option**

In `service_impl.go`, add field to the `service` struct:
```go
mediaStore   MediaStore
```

In `service.go`, add a new option after `WithImageGenerator`:
```go
// WithMediaStore sets the media storage backend for generated images.
func WithMediaStore(ms MediaStore) ServiceOption {
	return func(s *service) { s.mediaStore = ms }
}
```

- [ ] **Step 3: Replace `os` calls in `caption.go` with `mediaStore`**

In `caption.go` (created by Task 5), the `generateBackgroundsAsync` method uses:
- `os.MkdirAll(postDir, 0o755)` → `s.mediaStore.EnsureDir(postDir)`
- `os.WriteFile(coverPath, coverResult.ImageData, 0o644)` → `s.mediaStore.WriteFile(coverPath, coverResult.ImageData)`
- `os.WriteFile(cardPath, cardResult.ImageData, 0o644)` → `s.mediaStore.WriteFile(cardPath, cardResult.ImageData)`

Add a guard at the top of `generateBackgroundsAsync`:
```go
if s.mediaStore == nil {
    return
}
```

Remove `"os"` from `caption.go` imports.

- [ ] **Step 4: Remove `"os"` from `service_impl.go` imports**

After Task 5 splits the file, `service_impl.go` should no longer import `os` (the `os` calls moved to `caption.go`). If it still does, remove it. Verify `service_impl.go` has no remaining `os.` references.

- [ ] **Step 5: Create filesystem adapter `internal/adapters/storage/mediafs/store.go`**

```go
package mediafs

import "os"

// Store implements social.MediaStore using the local filesystem.
type Store struct{}

// NewStore creates a new filesystem-backed media store.
func NewStore() *Store { return &Store{} }

// EnsureDir creates a directory (and parents) if it doesn't exist.
func (s *Store) EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// WriteFile writes data to a file, creating it if necessary.
func (s *Store) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 6: Wire in `cmd/slabledger/init.go`**

In the social service initialization (around line 224 where `WithImageGenerator` is appended), add:
```go
socialOpts = append(socialOpts, social.WithMediaStore(mediafs.NewStore()))
```

Add import:
```go
"github.com/guarzo/slabledger/internal/adapters/storage/mediafs"
```

- [ ] **Step 7: Verify no `os` import in domain/social**

Run: `grep -rn '"os"' internal/domain/social/`
Expected: No matches

- [ ] **Step 8: Verify compilation**

Run: `go build ./...`
Expected: SUCCESS

- [ ] **Step 9: Run tests**

Run: `go test ./internal/domain/social/... ./internal/adapters/storage/mediafs/...`
Expected: PASS

- [ ] **Step 10: Run linter**

Run: `golangci-lint run ./internal/domain/social/... ./internal/adapters/storage/mediafs/...`
Expected: No new issues

- [ ] **Step 11: Commit**

```bash
git add internal/domain/social/media.go \
       internal/domain/social/service.go \
       internal/domain/social/service_impl.go \
       internal/domain/social/caption.go \
       internal/adapters/storage/mediafs/store.go \
       cmd/slabledger/init.go
git commit -m "refactor: remove os import from social domain via MediaStore interface

Introduce MediaStore interface in domain/social and filesystem adapter
in adapters/storage/mediafs. Domain code no longer depends on os package."
```

---

## Task 9: A — Documentation improvements

**Files:**
- Modify: `CLAUDE.md` (add Testing & Mocks section, error recipe, key reference files, file size guidance, fix migration count)
- Modify: `docs/ARCHITECTURE.md` (fix Go version)

- [ ] **Step 1: Expand Testing section in CLAUDE.md**

Find the existing `## Testing` section (line ~156) and replace it with the expanded version. The current content is:
```
## Testing

- **Unit tests**: Mock all external deps, use `internal/testutil/mocks`
- **Integration tests**: `internal/integration/` with `-tags integration` flag, requires API keys in `.env`
- Always run `go test -race` before committing
```

Replace with:
```markdown
## Testing

- **Pattern**: Table-driven tests with `[]struct` for all test cases
- **Mocks**: Import from `internal/testutil/mocks/` — never create inline mocks
  - Uses Fn-field pattern: override any method by setting `mock.CreateCampaignFn = func(...) { ... }`
  - Full guide: `internal/testutil/mocks/README.md`
- **Error assertions**: Use `errors.Is(err, campaigns.ErrCampaignNotFound)` with sentinel errors
- **Deterministic data**: Use fixed seeds for Monte Carlo, atomic counters for IDs
- **Unit tests**: Mock all external deps, use `internal/testutil/mocks`
- **Integration tests**: `internal/integration/` with `-tags integration` flag, requires API keys in `.env`
- Always run `go test -race` before committing
```

- [ ] **Step 2: Add file size guidance to Code Style section**

In the `## Code Style` section (line ~162), add this line after "Functional options pattern...":
```
- Keep source files under 500 lines. If a file grows beyond this, look for natural split points (separate strategies, separate concerns, utilities)
```

- [ ] **Step 3: Add error recipe to Common Recipes section**

After the "Add a new domain interface" recipe (around line 320), add:

```markdown
### Add a new domain error

1. Add error code in `internal/domain/<package>/errors.go`: `ErrCodeMyError errors.ErrorCode = "ERR_MY_ERROR"`
2. Add sentinel: `var ErrMyError = errors.NewAppError(ErrCodeMyError, "description")`
3. Add predicate: `func IsMyError(err error) bool { return errors.HasErrorCode(err, ErrCodeMyError) }`
4. Test with `errors.Is(err, ErrMyError)` in callers
```

- [ ] **Step 4: Add Key Reference Files section**

Add a new section after `## Documentation` (at the end of CLAUDE.md):

```markdown
## Key Reference Files

- `internal/README.md` — Architecture rules, decision tree for code placement, anti-patterns
- `internal/testutil/mocks/README.md` — Mock patterns with examples
- `docs/API.md` — All endpoint request/response shapes
- `docs/SCHEMA.md` — Full database schema with indexes
- `.env.example` — All environment variables with comments
```

- [ ] **Step 5: Fix migration count in CLAUDE.md**

Find `(17 pairs, 000001–000017)` in the Database section (line ~86) and replace with `(19 pairs, 000001–000019)`.

- [ ] **Step 6: Fix Go version in ARCHITECTURE.md**

In `docs/ARCHITECTURE.md` line 7, change `Go 1.25.2` to `Go 1.26`.

- [ ] **Step 7: Commit**

```bash
git add CLAUDE.md docs/ARCHITECTURE.md
git commit -m "docs: improve CLAUDE.md for AI agents, fix ARCHITECTURE.md version

Add testing/mock patterns, error handling recipe, key reference files,
file size guidance. Fix migration count (17→19) and Go version (1.25.2→1.26)."
```

---

## Task 10: C — CI guardrails

**Files:**
- Create: `scripts/check-imports.sh`
- Create: `scripts/check-file-size.sh`
- Modify: `.github/workflows/test.yml` (add architecture check step)
- Modify: `Makefile` (add `check` target)

- [ ] **Step 1: Create `scripts/check-imports.sh`**

```bash
#!/usr/bin/env bash
# Fail if any domain package imports an adapter package.
# This is the core hexagonal architecture invariant.
set -euo pipefail

violations=$(grep -rn '"github.com/guarzo/slabledger/internal/adapters' internal/domain/ 2>/dev/null || true)

if [ -n "$violations" ]; then
  echo "ERROR: Domain packages must not import adapter packages."
  echo ""
  echo "Violations found:"
  echo "$violations"
  echo ""
  echo "Domain code should depend only on interfaces defined in internal/domain/."
  exit 1
fi

echo "Architecture check passed: no domain → adapter imports."
```

- [ ] **Step 2: Create `scripts/check-file-size.sh`**

```bash
#!/usr/bin/env bash
# Warn at 500 lines, fail at 600 lines for non-test Go source files.
# Test files (*_test.go) are excluded — table-driven tests grow naturally.
set -euo pipefail

WARN_LIMIT=500
FAIL_LIMIT=600
failed=0
warned=0

while IFS= read -r file; do
  lines=$(wc -l < "$file")
  if [ "$lines" -gt "$FAIL_LIMIT" ]; then
    echo "FAIL: $file ($lines lines, limit $FAIL_LIMIT)"
    failed=1
  elif [ "$lines" -gt "$WARN_LIMIT" ]; then
    echo "WARN: $file ($lines lines, guideline $WARN_LIMIT)"
    warned=1
  fi
done < <(find internal/ cmd/ -name '*.go' ! -name '*_test.go' -type f 2>/dev/null)

if [ "$failed" -eq 1 ]; then
  echo ""
  echo "File size check FAILED. Split files above $FAIL_LIMIT lines."
  exit 1
fi

if [ "$warned" -eq 1 ]; then
  echo ""
  echo "File size check passed with warnings. Consider splitting files above $WARN_LIMIT lines."
else
  echo "File size check passed: all source files under $WARN_LIMIT lines."
fi
```

- [ ] **Step 3: Make scripts executable**

Run:
```bash
chmod +x scripts/check-imports.sh scripts/check-file-size.sh
```

- [ ] **Step 4: Run both scripts to verify they pass**

Run: `./scripts/check-imports.sh`
Expected: "Architecture check passed: no domain → adapter imports."

Run: `./scripts/check-file-size.sh`
Expected: PASS (possibly with warnings for files near 500 lines, but no failures after B1–B6 splits)

- [ ] **Step 5: Add architecture check to CI workflow**

In `.github/workflows/test.yml`, add a step after "Download dependencies" (line 35) and before "Run tests" (line 36):

```yaml
    - name: Check architecture invariants
      run: ./scripts/check-imports.sh
```

- [ ] **Step 6: Add `check` target to Makefile**

Add to the `.PHONY` line: `check`

Add after the `lint` target (line ~85):

```makefile
# Full quality check (lint + architecture + file size)
check: lint
	./scripts/check-imports.sh
	./scripts/check-file-size.sh
```

Update the `help` target to include check:
```
	@echo "  check         Run full quality check (lint + architecture + file size)"
```

- [ ] **Step 7: Run `make check` to verify**

Run: `make check`
Expected: All checks pass (formatting, vetting, linting, imports, file size)

- [ ] **Step 8: Commit**

```bash
git add scripts/check-imports.sh scripts/check-file-size.sh \
       .github/workflows/test.yml Makefile
git commit -m "ci: add architecture and file size guardrails

Add check-imports.sh (fails if domain imports adapters) and
check-file-size.sh (fails at 600 lines, warns at 500). Wire into
CI workflow and Makefile 'check' target."
```

---

## Task 11: Final verification

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: SUCCESS

- [ ] **Step 2: Full test suite**

Run: `go test -race ./...`
Expected: All PASS

- [ ] **Step 3: Full quality check**

Run: `make check`
Expected: All pass

- [ ] **Step 4: Verify file sizes**

Run: `./scripts/check-file-size.sh`
Expected: No failures. Confirm the 6 original large files are now under 500 lines each (or close to it).
