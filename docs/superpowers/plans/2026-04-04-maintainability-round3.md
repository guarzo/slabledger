# Maintainability Round 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Comprehensive maintainability pass — standardize error patterns (advisor, auth, ai), split 4 files approaching/exceeding 500L, add test coverage for handlers and SQLite repositories, and remove vestigial code.

**Architecture:** Documentation edits, error pattern standardization, pure file reorganization (splits), new test files, and one vestigial wrapper removal. No behavioral changes except the `ClassifyAIError` type-safety improvement.

**Tech Stack:** Go 1.26, SQLite, httptest, table-driven tests

**Spec:** `docs/superpowers/specs/2026-04-04-maintainability-round3-design.md`

**Branch:** `guarzo/refactor`

**Cross-cutting rule:** In every code task, remove comments that merely restate what the code does. Keep comments that explain *why* something is non-obvious. Apply only to files touched in that task.

**User preferences:** Do NOT use worktree isolation (already in a worktree). Always run `golangci-lint run` after code changes.

---

## Task 1: E1 — Standardize `advisor/errors.go`

**Files:**
- Create: `internal/domain/advisor/errors.go`
- Modify: `internal/domain/advisor/service_impl.go`
- Modify: `internal/domain/advisor/scoring.go`

- [ ] **Step 1: Create `errors.go`**

```go
package advisor

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodeMaxRoundsExceeded errors.ErrorCode = "ERR_ADVISOR_MAX_ROUNDS"
	ErrCodeToolPanic         errors.ErrorCode = "ERR_ADVISOR_TOOL_PANIC"
	ErrCodeUnsupportedType   errors.ErrorCode = "ERR_ADVISOR_UNSUPPORTED_TYPE"
)

var (
	ErrMaxRoundsExceeded = errors.NewAppError(ErrCodeMaxRoundsExceeded, "exceeded maximum tool call rounds")
	ErrToolPanic         = errors.NewAppError(ErrCodeToolPanic, "tool executor panicked")
	ErrUnsupportedType   = errors.NewAppError(ErrCodeUnsupportedType, "unsupported factor data type")
)

func IsMaxRoundsExceeded(err error) bool { return errors.HasErrorCode(err, ErrCodeMaxRoundsExceeded) }
func IsToolPanic(err error) bool         { return errors.HasErrorCode(err, ErrCodeToolPanic) }
func IsUnsupportedType(err error) bool   { return errors.HasErrorCode(err, ErrCodeUnsupportedType) }
```

- [ ] **Step 2: Update `service_impl.go` — max rounds error (around line 410)**

Replace:
```go
err := fmt.Errorf("exceeded maximum tool rounds (%d); last tools called: %s",
    maxRounds, strings.Join(lastToolNames, ", "))
```

With:
```go
err := ErrMaxRoundsExceeded.WithContext("maxRounds", maxRounds).WithContext("lastTools", strings.Join(lastToolNames, ", "))
```

Remove the `"strings"` import if it's no longer used elsewhere in the file. Keep `"fmt"` if still used.

- [ ] **Step 3: Update `service_impl.go` — tool panic error (around line 370)**

In the panic recovery block, after `errMsg := fmt.Sprintf(...)`, the panic is logged but no domain error is returned (it becomes part of the tool result JSON, not a returned error). Leave this as-is — tool panics are reported as JSON tool results, not function return errors.

- [ ] **Step 4: Update `scoring.go` — unsupported data type (line 77)**

Replace:
```go
return scoring.ScoreCard{}, fmt.Errorf("unsupported factor data type: %T", data)
```

With:
```go
return scoring.ScoreCard{}, ErrUnsupportedType.WithContext("type", fmt.Sprintf("%T", data))
```

- [ ] **Step 5: Verify**

```bash
go build ./internal/domain/advisor/...
go test ./internal/domain/advisor/...
golangci-lint run ./internal/domain/advisor/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/advisor/errors.go internal/domain/advisor/service_impl.go internal/domain/advisor/scoring.go
git commit -m "refactor: standardize advisor errors to ErrorCode/NewAppError pattern

Adds ErrMaxRoundsExceeded, ErrToolPanic, ErrUnsupportedType with error
codes and predicate functions. Matches campaigns/social/picks pattern."
```

---

## Task 2: E2 — Standardize `auth/errors.go`

**Files:**
- Create: `internal/domain/auth/errors.go`
- Modify: `internal/domain/auth/types.go`

- [ ] **Step 1: Create `errors.go`**

```go
package auth

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	ErrCodeUserNotFound errors.ErrorCode = "ERR_AUTH_USER_NOT_FOUND"
)

var (
	ErrUserNotFound = errors.NewAppError(ErrCodeUserNotFound, "user not found")
)

func IsUserNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeUserNotFound) }
```

- [ ] **Step 2: Remove sentinel from `types.go`**

Remove these lines from `types.go`:
```go
import (
	"errors"
	"time"
)

// Sentinel errors for auth operations
var (
	// ErrUserNotFound is returned when a user is not found in the repository
	ErrUserNotFound = errors.New("user not found")
)
```

Replace with just:
```go
import (
	"time"
)
```

The `ErrUserNotFound` variable is now defined in `errors.go`. Callers use `auth.ErrUserNotFound` either way — no caller changes needed.

- [ ] **Step 3: Verify callers still compile**

```bash
go build ./internal/domain/auth/... ./internal/adapters/...
go test ./internal/domain/auth/... ./internal/adapters/storage/sqlite/...
golangci-lint run ./internal/domain/auth/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/auth/errors.go internal/domain/auth/types.go
git commit -m "refactor: standardize auth/errors.go to ErrorCode/NewAppError pattern

Migrates ErrUserNotFound from stdlib errors.New to domain NewAppError.
Adds error code and predicate function."
```

---

## Task 3: E3 — Type-safe `ClassifyAIError`

**Files:**
- Modify: `internal/domain/ai/tracking.go`

- [ ] **Step 1: Add domain errors import**

Add the import (use an alias to avoid conflict with stdlib `errors`):
```go
import (
	"context"
	"strings"
	"time"

	domainerrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)
```

- [ ] **Step 2: Update `ClassifyAIError` function**

Replace the existing function (around line 106):
```go
func ClassifyAIError(err error) (AIStatus, string) {
	if err == nil {
		return AIStatusSuccess, ""
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too_many_requests") ||
		strings.Contains(errMsg, "429") || strings.Contains(errMsg, "capacity exceeded") {
		return AIStatusRateLimited, errMsg
	}
	return AIStatusError, errMsg
}
```

With:
```go
func ClassifyAIError(err error) (AIStatus, string) {
	if err == nil {
		return AIStatusSuccess, ""
	}
	errMsg := err.Error()
	// Prefer structured error code check when available
	if domainerrors.HasErrorCode(err, domainerrors.ErrCodeProviderRateLimit) {
		return AIStatusRateLimited, errMsg
	}
	// Fallback: string matching for errors from providers that don't use AppError yet
	if strings.Contains(errMsg, "rate limit") || strings.Contains(errMsg, "too_many_requests") ||
		strings.Contains(errMsg, "429") || strings.Contains(errMsg, "capacity exceeded") {
		return AIStatusRateLimited, errMsg
	}
	return AIStatusError, errMsg
}
```

- [ ] **Step 3: Verify**

```bash
go build ./internal/domain/ai/...
go test ./internal/domain/ai/...
golangci-lint run ./internal/domain/ai/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/ai/tracking.go
git commit -m "refactor: add ErrorCode-aware classification to ClassifyAIError

Checks for ErrCodeProviderRateLimit via HasErrorCode before falling back
to string matching. No behavioral change for existing callers."
```

---

## Task 4: D1 — Split `tools_portfolio.go`

**Files:**
- Modify: `internal/adapters/advisortool/tools_portfolio.go`
- Create: `internal/adapters/advisortool/tools_portfolio_analysis.go`

- [ ] **Step 1: Read the file to identify exact split point**

Read `internal/adapters/advisortool/tools_portfolio.go` fully. Identify the boundary:
- **Keep in `tools_portfolio.go`** (health/monitoring/dashboard): everything from the package declaration through `registerGetDashboardSummary` (includes `dashboardSummary` struct). These functions are: `registerGetPortfolioHealth`, `registerGetPortfolioInsights`, `registerGetCreditSummary`, `registerGetWeeklyReview`, `registerGetCapitalTimeline`, `registerGetSuggestionStats`, `dashboardSummary` struct, `registerGetDashboardSummary`.
- **Move to `tools_portfolio_analysis.go`**: everything after dashboard — `registerGetExpectedValues`, `registerGetCrackCandidates`, `registerGetCampaignSuggestions`, `registerRunProjection`, `registerGetChannelVelocity`, `registerGetCertLookup`, `registerEvaluatePurchase`, `registerSuggestPrice`, `registerGetAcquisitionTargets`, `registerGetCrackOpportunities`, `registerSuggestPriceBatch` (+ `jsonSchema` struct), `registerGetExpectedValuesBatch`.

- [ ] **Step 2: Create `tools_portfolio_analysis.go`**

Create the new file with `package advisortool` and the needed imports (read the moved functions to determine which imports they require — likely `context`, `encoding/json`, `fmt`, `github.com/guarzo/slabledger/internal/domain/ai`, `github.com/guarzo/slabledger/internal/domain/campaigns`). Cut the analysis functions from the original file and paste them here.

- [ ] **Step 3: Remove moved functions from `tools_portfolio.go`**

Delete the moved functions. Clean up any imports that are no longer needed in the original file.

- [ ] **Step 4: Verify**

```bash
go build ./internal/adapters/advisortool/...
go test ./internal/adapters/advisortool/...
golangci-lint run ./internal/adapters/advisortool/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/advisortool/tools_portfolio.go \
       internal/adapters/advisortool/tools_portfolio_analysis.go
git commit -m "refactor: split tools_portfolio.go into health/monitoring and analysis files

Health, monitoring, and dashboard tools stay in tools_portfolio.go.
Analysis, pricing, and batch tools move to tools_portfolio_analysis.go."
```

---

## Task 5: D2 — Split `import_parsing.go`

**Files:**
- Modify: `internal/domain/campaigns/import_parsing.go`
- Create: `internal/domain/campaigns/import_parsing_metadata.go`

- [ ] **Step 1: Read the file to identify exact split point**

Read `internal/domain/campaigns/import_parsing.go` fully. Identify:
- **Keep in `import_parsing.go`** (core title parsing): package declaration, imports, regex pattern definitions (`hashCardNumberRegex`, `bareCardNumberRegex`, `gradeRegex`, `cardNumberTokenRegex`), `isInvalidCardNumber`, `isGenericSetName`, `ExtractCardNumberFromPSATitle`, `ExtractGrade`, `ParsePSAListingTitle`, `parsePSAListingTitleWithIndex`, `isLetter`, `isLetterOrDigit`, `maxCollectorNumberAlphaPrefix`, `looksLikeCollectorNumber`, `noNumberCardPatterns`, `noNumberStopWords`, `parseNoNumberTitle`.
- **Move to `import_parsing_metadata.go`**: `psaCategoryToSetName` map, `resolvePSACategory`, `variantPattern` struct, `variantPatterns` list, `stripVariantTokens`, `extractCardNameFromPSATitle`, `parseCardMetadataFromTitle`, `resolveSetName`, `stripCollectionSuffix`, `extractYearFromTitle`, `extractVariantFromTitle`, `ExportParseCardMetadataFromTitle`, `ExportIsGenericSetName`.

- [ ] **Step 2: Create `import_parsing_metadata.go`**

Create the new file with `package campaigns` and needed imports (likely `regexp`, `strings`, `github.com/guarzo/slabledger/internal/domain/constants`). Cut the metadata functions from the original file.

- [ ] **Step 3: Remove moved functions from `import_parsing.go`**

Delete the moved functions and their associated data (`psaCategoryToSetName`, `variantPattern`, `variantPatterns`). Clean up imports.

- [ ] **Step 4: Verify**

```bash
go build ./internal/domain/campaigns/...
go test ./internal/domain/campaigns/...
golangci-lint run ./internal/domain/campaigns/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/import_parsing.go \
       internal/domain/campaigns/import_parsing_metadata.go
git commit -m "refactor: split import_parsing.go into title parsing and metadata extraction

Core title parsing (regex, card numbers, grades, PSA listing titles) stays.
Metadata extraction (set resolution, variants, card names) moves to
import_parsing_metadata.go."
```

---

## Task 6: D3 — Split `httpx/client.go`

**Files:**
- Modify: `internal/adapters/clients/httpx/client.go`
- Create: `internal/adapters/clients/httpx/client_helpers.go`

- [ ] **Step 1: Read the file to identify exact split point**

Read `internal/adapters/clients/httpx/client.go` fully. Identify:
- **Keep in `client.go`** (core execution): `Observer` interface, `NoopObserver`, `Config` struct, `DefaultConfig`, `DefaultTransport`, `Option` type, `WithLogger`, `Client` struct, `NewClient`, `Do` (retry + circuit breaker), `doRequest`.
- **Move to `client_helpers.go`**: `Request` struct, `Response` struct, `Get`, `GetJSON`, `Post`, `PostJSON`, `handleHTTPError`, `sanitizeResponseBody`, `extractHTMLSummary`, `isTimeoutError`, `GetCircuitBreakerStats`.

- [ ] **Step 2: Create `client_helpers.go`**

Create the new file with `package httpx` and needed imports. The convenience methods (`Get`, `GetJSON`, `Post`, `PostJSON`) call `c.Do()` which stays in `client.go` — same package, so this works. `handleHTTPError` is called by `doRequest` in `client.go` — since they're in the same package, this also works.

**Important:** `Request` and `Response` types must be in `client_helpers.go` since the convenience methods construct them, but `Do` and `doRequest` in `client.go` also reference them. Same package — no issue.

- [ ] **Step 3: Remove moved code from `client.go`**

Delete the moved types and functions. Clean up imports (remove `"encoding/json"` if only used by convenience methods, keep `"net/http"` since `doRequest` uses it).

- [ ] **Step 4: Verify**

```bash
go build ./internal/adapters/clients/httpx/...
go test ./internal/adapters/clients/httpx/...
golangci-lint run ./internal/adapters/clients/httpx/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/httpx/client.go \
       internal/adapters/clients/httpx/client_helpers.go
git commit -m "refactor: split httpx/client.go into core execution and convenience helpers

Core types, config, retry/circuit-breaker execution stay in client.go.
Request/Response types, Get/Post convenience methods, error handling
utilities move to client_helpers.go."
```

---

## Task 7: D4 — Split `service_advanced.go`

**Files:**
- Modify: `internal/domain/campaigns/service_advanced.go`
- Create: `internal/domain/campaigns/service_arbitrage.go`

- [ ] **Step 1: Read the file to identify exact split point**

Read `internal/domain/campaigns/service_advanced.go` fully. Identify:
- **Keep in `service_advanced.go`** (cert/value/simulation): `LookupCert`, `QuickAddPurchase`, `GetExpectedValues`, `EvaluatePurchase`, `RunProjection`.
- **Move to `service_arbitrage.go`**: `GetCrackCandidates`, `GetActivationChecklist`, `GetCrackOpportunities`, `GetAcquisitionTargets`.

The split point is after `GetExpectedValues`/`EvaluatePurchase` and before `GetCrackCandidates`. Look for the section marker comment `// --- Crack Candidates ---` or similar.

- [ ] **Step 2: Create `service_arbitrage.go`**

Create the new file with `package campaigns` and needed imports (likely `context`, `fmt`, `sort`, `time`, `github.com/guarzo/slabledger/internal/domain/observability`). Cut the arbitrage functions from the original.

- [ ] **Step 3: Remove moved functions from `service_advanced.go`**

Delete the moved functions. Clean up imports (`"sort"` may only be needed in the arbitrage file).

- [ ] **Step 4: Verify**

```bash
go build ./internal/domain/campaigns/...
go test ./internal/domain/campaigns/...
golangci-lint run ./internal/domain/campaigns/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/service_advanced.go \
       internal/domain/campaigns/service_arbitrage.go
git commit -m "refactor: split service_advanced.go into value analysis and arbitrage

Cert lookup, expected values, purchase evaluation, and projection stay.
Crack candidates, activation checklist, and acquisition targets move to
service_arbitrage.go."
```

---

## Task 8: T1 — Handler tests (campaigns_finance, picks, social)

**Files:**
- Modify: `internal/testutil/mocks/campaign_service.go` (add missing Fn fields if needed)
- Create: `internal/testutil/mocks/picks_service.go`
- Create: `internal/testutil/mocks/social_service.go`
- Create: `internal/adapters/httpserver/handlers/campaigns_finance_test.go`
- Create: `internal/adapters/httpserver/handlers/picks_handler_test.go`
- Create: `internal/adapters/httpserver/handlers/social_test.go`

### Step 1: Add mock for picks.Service

- [ ] **Step 1a: Create `internal/testutil/mocks/picks_service.go`**

```go
package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/picks"
)

// MockPicksService implements picks.Service for testing.
type MockPicksService struct {
	GenerateDailyPicksFn  func(ctx context.Context) error
	GetLatestPicksFn      func(ctx context.Context) ([]picks.Pick, error)
	GetPickHistoryFn      func(ctx context.Context, days int) ([]picks.Pick, error)
	AddToWatchlistFn      func(ctx context.Context, item picks.WatchlistItem) error
	RemoveFromWatchlistFn func(ctx context.Context, id int) error
	GetWatchlistFn        func(ctx context.Context) ([]picks.WatchlistItem, error)
}

var _ picks.Service = (*MockPicksService)(nil)

func (m *MockPicksService) GenerateDailyPicks(ctx context.Context) error {
	if m.GenerateDailyPicksFn != nil {
		return m.GenerateDailyPicksFn(ctx)
	}
	return nil
}

func (m *MockPicksService) GetLatestPicks(ctx context.Context) ([]picks.Pick, error) {
	if m.GetLatestPicksFn != nil {
		return m.GetLatestPicksFn(ctx)
	}
	return nil, nil
}

func (m *MockPicksService) GetPickHistory(ctx context.Context, days int) ([]picks.Pick, error) {
	if m.GetPickHistoryFn != nil {
		return m.GetPickHistoryFn(ctx, days)
	}
	return nil, nil
}

func (m *MockPicksService) AddToWatchlist(ctx context.Context, item picks.WatchlistItem) error {
	if m.AddToWatchlistFn != nil {
		return m.AddToWatchlistFn(ctx, item)
	}
	return nil
}

func (m *MockPicksService) RemoveFromWatchlist(ctx context.Context, id int) error {
	if m.RemoveFromWatchlistFn != nil {
		return m.RemoveFromWatchlistFn(ctx, id)
	}
	return nil
}

func (m *MockPicksService) GetWatchlist(ctx context.Context) ([]picks.WatchlistItem, error) {
	if m.GetWatchlistFn != nil {
		return m.GetWatchlistFn(ctx)
	}
	return nil, nil
}
```

- [ ] **Step 1b: Create `internal/testutil/mocks/social_service.go`**

Read `internal/domain/social/service.go` (or wherever the `social.Service` interface is defined) to get the exact method signatures. Create a mock following the same Fn-field pattern. The interface likely includes: `DetectAndGenerate`, `ListPosts`, `GetPost`, `UpdateCaption`, `Delete`, `RegenerateCaption`, `Publish`, `BackfillImages`.

```go
package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// MockSocialService implements social.Service for testing.
type MockSocialService struct {
	DetectAndGenerateFn   func(ctx context.Context) error
	ListPostsFn           func(ctx context.Context, statusFilter string, limit, offset int) ([]social.SocialPost, error)
	GetPostFn             func(ctx context.Context, id string) (*social.PostDetail, error)
	UpdateCaptionFn       func(ctx context.Context, id, caption, hashtags string) error
	DeleteFn              func(ctx context.Context, id string) error
	RegenerateCaptionFn   func(ctx context.Context, id string, stream func(ai.StreamEvent)) error
	PublishFn             func(ctx context.Context, id string) error
}

var _ social.Service = (*MockSocialService)(nil)

func (m *MockSocialService) DetectAndGenerate(ctx context.Context) error {
	if m.DetectAndGenerateFn != nil {
		return m.DetectAndGenerateFn(ctx)
	}
	return nil
}

func (m *MockSocialService) ListPosts(ctx context.Context, statusFilter string, limit, offset int) ([]social.SocialPost, error) {
	if m.ListPostsFn != nil {
		return m.ListPostsFn(ctx, statusFilter, limit, offset)
	}
	return nil, nil
}

func (m *MockSocialService) GetPost(ctx context.Context, id string) (*social.PostDetail, error) {
	if m.GetPostFn != nil {
		return m.GetPostFn(ctx, id)
	}
	return nil, nil
}

func (m *MockSocialService) UpdateCaption(ctx context.Context, id, caption, hashtags string) error {
	if m.UpdateCaptionFn != nil {
		return m.UpdateCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *MockSocialService) Delete(ctx context.Context, id string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

func (m *MockSocialService) RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error {
	if m.RegenerateCaptionFn != nil {
		return m.RegenerateCaptionFn(ctx, id, stream)
	}
	return nil
}

func (m *MockSocialService) Publish(ctx context.Context, id string) error {
	if m.PublishFn != nil {
		return m.PublishFn(ctx, id)
	}
	return nil
}
```

**Important:** Read the actual `social.Service` interface definition before writing this mock. The method signatures above are best guesses — verify and adjust them to match the actual interface exactly. The mock MUST compile against `var _ social.Service = (*MockSocialService)(nil)`.

- [ ] **Step 1c: Verify mocks compile**

```bash
go build ./internal/testutil/mocks/...
go test ./internal/testutil/mocks/...
```

### Step 2: campaigns_finance handler tests

- [ ] **Step 2a: Check MockCampaignService for finance Fn fields**

Read `internal/testutil/mocks/campaign_service.go`. Check whether it has Fn fields for: `GetCreditSummaryFn`, `GetCashflowConfigFn`, `UpdateCashflowConfigFn`, `ListInvoicesFn`, `UpdateInvoiceFn`, `GetPortfolioHealthFn`, `GetPortfolioChannelVelocityFn`, `GetPortfolioInsightsFn`, `GetCampaignSuggestionsFn`, `GetCapitalTimelineFn`, `GetWeeklyReviewSummaryFn`, `ListRevocationFlagsFn`, `FlagForRevocationFn`, `GenerateRevocationEmailFn`.

If any are missing, add them following the existing Fn-field pattern. Each needs: a field `XxxFn func(...) (...)`, and a method `func (m *MockCampaignService) Xxx(...) (...) { if m.XxxFn != nil { return m.XxxFn(...) }; return zerovals }`.

- [ ] **Step 2b: Create `campaigns_finance_test.go`**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHandleCreditSummary(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*mocks.MockCampaignService)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetCreditSummaryFn = func(ctx context.Context) (*campaigns.CreditSummary, error) {
					return &campaigns.CreditSummary{
						BalanceCents: 50000,
						LimitCents:   100000,
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service error",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetCreditSummaryFn = func(ctx context.Context) (*campaigns.CreditSummary, error) {
					return nil, errors.New("db connection failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tt.setupMock(svc)
			h := NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/credit/summary", nil)
			rec := httptest.NewRecorder()
			h.HandleCreditSummary(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				var result map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
			}
		})
	}
}

func TestHandlePortfolioHealth(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*mocks.MockCampaignService)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetPortfolioHealthFn = func(ctx context.Context) (*campaigns.PortfolioHealth, error) {
					return &campaigns.PortfolioHealth{}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service error",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetPortfolioHealthFn = func(ctx context.Context) (*campaigns.PortfolioHealth, error) {
					return nil, errors.New("failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tt.setupMock(svc)
			h := NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/health", nil)
			rec := httptest.NewRecorder()
			h.HandlePortfolioHealth(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleListInvoices(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*mocks.MockCampaignService)
		wantStatus int
	}{
		{
			name: "success returns list",
			setupMock: func(m *mocks.MockCampaignService) {
				m.ListInvoicesFn = func(ctx context.Context) ([]campaigns.Invoice, error) {
					return []campaigns.Invoice{{ID: "inv-1"}}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "success returns empty list",
			setupMock: func(m *mocks.MockCampaignService) {
				m.ListInvoicesFn = func(ctx context.Context) ([]campaigns.Invoice, error) {
					return nil, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service error",
			setupMock: func(m *mocks.MockCampaignService) {
				m.ListInvoicesFn = func(ctx context.Context) ([]campaigns.Invoice, error) {
					return nil, errors.New("failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tt.setupMock(svc)
			h := NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/credit/invoices", nil)
			rec := httptest.NewRecorder()
			h.HandleListInvoices(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleWeeklyReview(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*mocks.MockCampaignService)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetWeeklyReviewSummaryFn = func(ctx context.Context) (*campaigns.WeeklyReviewSummary, error) {
					return &campaigns.WeeklyReviewSummary{
						PurchasesThisWeek: 5,
						SalesThisWeek:     3,
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service error",
			setupMock: func(m *mocks.MockCampaignService) {
				m.GetWeeklyReviewSummaryFn = func(ctx context.Context) (*campaigns.WeeklyReviewSummary, error) {
					return nil, errors.New("failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tt.setupMock(svc)
			h := NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/weekly-review", nil)
			rec := httptest.NewRecorder()
			h.HandleWeeklyReview(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
```

**Note:** Also add tests for `HandleGetCashflowConfig`, `HandleUpdateCashflowConfig` (with JSON body), `HandleUpdateInvoice` (with JSON body + not-found error path), `HandleCreateRevocationFlag` (with JSON body + conflict error path), `HandlePortfolioChannelVelocity`, `HandlePortfolioInsights`, `HandleCampaignSuggestions`, and `HandleCapitalTimeline` following the same pattern. Read each handler method in `campaigns_finance.go` to understand the exact service method called and error handling before writing the test.

### Step 3: picks handler tests

- [ ] **Step 3a: Create `picks_handler_test.go`**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/picks"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newPicksHandler(svc *mocks.MockPicksService) *PicksHandler {
	return NewPicksHandler(svc, mocks.NewMockLogger())
}

func TestHandleGetPicks(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*mocks.MockPicksService)
		wantStatus int
	}{
		{
			name: "success with picks",
			setupMock: func(m *mocks.MockPicksService) {
				m.GetLatestPicksFn = func(ctx context.Context) ([]picks.Pick, error) {
					return []picks.Pick{
						{ID: 1, CardName: "Charizard", SetName: "Base Set", Grade: "PSA 10", TargetBuyPrice: 50000, ExpectedSellPrice: 75000, Date: time.Now()},
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "success empty list",
			setupMock: func(m *mocks.MockPicksService) {
				m.GetLatestPicksFn = func(ctx context.Context) ([]picks.Pick, error) {
					return nil, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service error",
			setupMock: func(m *mocks.MockPicksService) {
				m.GetLatestPicksFn = func(ctx context.Context) ([]picks.Pick, error) {
					return nil, errors.New("failed")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{}
			tt.setupMock(svc)
			h := newPicksHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/picks", nil)
			rec := httptest.NewRecorder()
			h.HandleGetPicks(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				var result map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode: %v", err)
				}
				if _, ok := result["picks"]; !ok {
					t.Error("response missing 'picks' key")
				}
			}
		})
	}
}

func TestHandleGetPickHistory(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		setupMock  func(*mocks.MockPicksService)
		wantStatus int
	}{
		{
			name:  "success with default days",
			query: "",
			setupMock: func(m *mocks.MockPicksService) {
				m.GetPickHistoryFn = func(ctx context.Context, days int) ([]picks.Pick, error) {
					if days != 7 {
						return nil, errors.New("expected default 7 days")
					}
					return []picks.Pick{}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "success with custom days",
			query: "?days=30",
			setupMock: func(m *mocks.MockPicksService) {
				m.GetPickHistoryFn = func(ctx context.Context, days int) ([]picks.Pick, error) {
					if days != 30 {
						return nil, errors.New("expected 30 days")
					}
					return []picks.Pick{}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid days out of range",
			query:      "?days=999",
			setupMock:  func(m *mocks.MockPicksService) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{}
			tt.setupMock(svc)
			h := newPicksHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/picks/history"+tt.query, nil)
			rec := httptest.NewRecorder()
			h.HandleGetPickHistory(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleAddWatchlistItem(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		setupMock  func(*mocks.MockPicksService)
		wantStatus int
	}{
		{
			name: "success",
			body: `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`,
			setupMock: func(m *mocks.MockPicksService) {
				m.AddToWatchlistFn = func(ctx context.Context, item picks.WatchlistItem) error {
					return nil
				}
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing required fields",
			body:       `{"card_name":""}`,
			setupMock:  func(m *mocks.MockPicksService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate",
			body: `{"card_name":"Charizard","set_name":"Base Set","grade":"PSA 10"}`,
			setupMock: func(m *mocks.MockPicksService) {
				m.AddToWatchlistFn = func(ctx context.Context, item picks.WatchlistItem) error {
					return picks.ErrWatchlistDuplicate
				}
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockPicksService{}
			tt.setupMock(svc)
			h := newPicksHandler(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/picks/watchlist", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandleAddWatchlistItem(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleGetWatchlist(t *testing.T) {
	svc := &mocks.MockPicksService{
		GetWatchlistFn: func(ctx context.Context) ([]picks.WatchlistItem, error) {
			return []picks.WatchlistItem{
				{ID: 1, CardName: "Pikachu", SetName: "Base Set", Grade: "PSA 9", Active: true},
			}, nil
		},
	}
	h := newPicksHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/picks/watchlist", nil)
	rec := httptest.NewRecorder()
	h.HandleGetWatchlist(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
```

**Note:** Also add `TestHandleDeleteWatchlistItem` with success (204), not-found (404), and invalid-id (400) cases. The delete handler uses `r.PathValue("id")` — use `req.SetPathValue("id", "1")` in tests (Go 1.22+ routing).

### Step 4: social handler tests

- [ ] **Step 4a: Create `social_test.go`**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/auth"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/middleware"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newSocialHandler(svc *mocks.MockSocialService) *SocialHandler {
	return NewSocialHandler(svc, nil, mocks.NewMockLogger(), "/tmp/media", "http://localhost:8081")
}

func authenticatedRequest(method, url string, body *strings.Reader) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, body)
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	user := &auth.User{ID: 1, Username: "testuser", Email: "test@example.com"}
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
	return req.WithContext(ctx)
}

func TestHandleListPosts(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		setupMock  func(*mocks.MockSocialService)
		wantStatus int
	}{
		{
			name:  "success all posts",
			query: "",
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(ctx context.Context, status string, limit, offset int) ([]social.SocialPost, error) {
					return []social.SocialPost{
						{ID: "post-1", Status: social.StatusDraft, Caption: "Test", CreatedAt: time.Now()},
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:  "filter by status",
			query: "?status=draft",
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(ctx context.Context, status string, limit, offset int) ([]social.SocialPost, error) {
					if status != "draft" {
						return nil, errors.New("expected draft filter")
					}
					return []social.SocialPost{}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid status filter",
			query:      "?status=invalid",
			setupMock:  func(m *mocks.MockSocialService) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "service error",
			query: "",
			setupMock: func(m *mocks.MockSocialService) {
				m.ListPostsFn = func(ctx context.Context, status string, limit, offset int) ([]social.SocialPost, error) {
					return nil, errors.New("db error")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			tt.setupMock(svc)
			h := newSocialHandler(svc)

			req := authenticatedRequest(http.MethodGet, "/api/social/posts"+tt.query, nil)
			rec := httptest.NewRecorder()
			h.HandleListPosts(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleListPosts_Unauthenticated(t *testing.T) {
	svc := &mocks.MockSocialService{}
	h := newSocialHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/social/posts", nil)
	rec := httptest.NewRecorder()
	h.HandleListPosts(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetPost(t *testing.T) {
	tests := []struct {
		name       string
		postID     string
		setupMock  func(*mocks.MockSocialService)
		wantStatus int
	}{
		{
			name:   "success",
			postID: "post-1",
			setupMock: func(m *mocks.MockSocialService) {
				m.GetPostFn = func(ctx context.Context, id string) (*social.PostDetail, error) {
					return &social.PostDetail{
						SocialPost: social.SocialPost{ID: id, Status: social.StatusDraft},
					}, nil
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "not found",
			postID: "nonexistent",
			setupMock: func(m *mocks.MockSocialService) {
				m.GetPostFn = func(ctx context.Context, id string) (*social.PostDetail, error) {
					return nil, social.ErrPostNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			tt.setupMock(svc)
			h := newSocialHandler(svc)

			req := authenticatedRequest(http.MethodGet, "/api/social/posts/"+tt.postID, nil)
			req.SetPathValue("id", tt.postID)
			rec := httptest.NewRecorder()
			h.HandleGetPost(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleUpdateCaption(t *testing.T) {
	tests := []struct {
		name       string
		postID     string
		body       string
		setupMock  func(*mocks.MockSocialService)
		wantStatus int
	}{
		{
			name:   "success",
			postID: "post-1",
			body:   `{"caption":"New caption","hashtags":"#pokemon #psa"}`,
			setupMock: func(m *mocks.MockSocialService) {
				m.UpdateCaptionFn = func(ctx context.Context, id, caption, hashtags string) error {
					return nil
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "post not found",
			postID: "nonexistent",
			body:   `{"caption":"New caption","hashtags":"#pokemon"}`,
			setupMock: func(m *mocks.MockSocialService) {
				m.UpdateCaptionFn = func(ctx context.Context, id, caption, hashtags string) error {
					return social.ErrPostNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			tt.setupMock(svc)
			h := newSocialHandler(svc)

			req := authenticatedRequest(http.MethodPut, "/api/social/posts/"+tt.postID, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("id", tt.postID)
			rec := httptest.NewRecorder()
			h.HandleUpdateCaption(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestHandleDelete(t *testing.T) {
	tests := []struct {
		name       string
		postID     string
		setupMock  func(*mocks.MockSocialService)
		wantStatus int
	}{
		{
			name:   "success",
			postID: "post-1",
			setupMock: func(m *mocks.MockSocialService) {
				m.DeleteFn = func(ctx context.Context, id string) error {
					return nil
				}
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "not found",
			postID: "nonexistent",
			setupMock: func(m *mocks.MockSocialService) {
				m.DeleteFn = func(ctx context.Context, id string) error {
					return social.ErrPostNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockSocialService{}
			tt.setupMock(svc)
			h := newSocialHandler(svc)

			req := authenticatedRequest(http.MethodDelete, "/api/social/posts/"+tt.postID, nil)
			req.SetPathValue("id", tt.postID)
			rec := httptest.NewRecorder()
			h.HandleDelete(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
```

**Note:** `HandleGenerate` (async, 202) and `HandleRegenerateCaption` (SSE streaming) are complex to test and are out of scope for this round. `HandleBackfillImages` requires the optional `backfiller` — add a simple test if time permits.

### Step 5: Verify all handler tests

- [ ] **Step 5a: Run handler tests**

```bash
go test ./internal/adapters/httpserver/handlers/... -v -run "TestHandleCreditSummary|TestHandlePortfolioHealth|TestHandleListInvoices|TestHandleWeeklyReview|TestHandleGetPicks|TestHandleGetPickHistory|TestHandleAddWatchlistItem|TestHandleGetWatchlist|TestHandleListPosts|TestHandleGetPost|TestHandleUpdateCaption|TestHandleDelete"
```

Expected: All PASS

- [ ] **Step 5b: Lint**

```bash
golangci-lint run ./internal/adapters/httpserver/handlers/... ./internal/testutil/mocks/...
```

### Step 6: Commit

- [ ] **Step 6: Commit all handler tests and mocks**

```bash
git add internal/testutil/mocks/picks_service.go \
       internal/testutil/mocks/social_service.go \
       internal/testutil/mocks/campaign_service.go \
       internal/adapters/httpserver/handlers/campaigns_finance_test.go \
       internal/adapters/httpserver/handlers/picks_handler_test.go \
       internal/adapters/httpserver/handlers/social_test.go
git commit -m "test: add handler tests for campaigns_finance, picks, social

Adds MockPicksService and MockSocialService to testutil/mocks.
Tests cover happy paths and key error paths (not found, invalid input,
service errors). SSE streaming handlers are out of scope."
```

---

## Task 9: T2 — SQLite tests (purchases_repository, sales_repository)

**Files:**
- Create: `internal/adapters/storage/sqlite/purchases_repository_test.go`
- Create: `internal/adapters/storage/sqlite/sales_repository_test.go`

- [ ] **Step 1: Create `purchases_repository_test.go`**

The existing `setupTestDB(t)` helper (defined in `prices_test.go`) creates an in-memory SQLite DB with all migrations applied. Use it directly — it's in the same package.

```go
package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func newTestPurchase(campaignID, certNumber string) *campaigns.Purchase {
	now := time.Now().Truncate(time.Second)
	return &campaigns.Purchase{
		ID:           "pur-" + certNumber,
		CampaignID:   campaignID,
		CardName:     "Charizard",
		CertNumber:   certNumber,
		CardNumber:   "4",
		SetName:      "Base Set",
		Grader:       "PSA",
		GradeValue:   9.0,
		BuyCostCents: 50000,
		PurchaseDate: now.Format("2006-01-02"),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func createTestCampaign(t *testing.T, db *DB, id, name string) {
	t.Helper()
	_, err := db.DB.ExecContext(context.Background(),
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at, ebay_fee_pct)
		 VALUES (?, ?, 'active', datetime('now'), datetime('now'), 13.25)`, id, name)
	if err != nil {
		t.Fatalf("failed to create test campaign: %v", err)
	}
}

func TestCreatePurchase(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-1", "12345678")
		err := repo.CreatePurchase(ctx, p)
		if err != nil {
			t.Fatalf("CreatePurchase failed: %v", err)
		}
	})

	t.Run("duplicate cert number", func(t *testing.T) {
		p := newTestPurchase("camp-1", "12345678")
		p.ID = "pur-dup"
		err := repo.CreatePurchase(ctx, p)
		if !campaigns.IsDuplicateCertNumber(err) {
			t.Errorf("expected ErrDuplicateCertNumber, got: %v", err)
		}
	})
}

func TestGetPurchase(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")
	p := newTestPurchase("camp-1", "99999999")
	_ = repo.CreatePurchase(ctx, p)

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetPurchase(ctx, p.ID)
		if err != nil {
			t.Fatalf("GetPurchase failed: %v", err)
		}
		if got.CertNumber != "99999999" {
			t.Errorf("CertNumber = %q, want %q", got.CertNumber, "99999999")
		}
		if got.CardName != "Charizard" {
			t.Errorf("CardName = %q, want %q", got.CardName, "Charizard")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetPurchase(ctx, "nonexistent")
		if !campaigns.IsPurchaseNotFound(err) {
			t.Errorf("expected ErrPurchaseNotFound, got: %v", err)
		}
	})
}

func TestListPurchasesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	createTestCampaign(t, db, "camp-2", "Campaign 2")

	_ = repo.CreatePurchase(ctx, newTestPurchase("camp-1", "11111111"))
	_ = repo.CreatePurchase(ctx, newTestPurchase("camp-1", "22222222"))
	_ = repo.CreatePurchase(ctx, newTestPurchase("camp-2", "33333333"))

	t.Run("filters by campaign", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "camp-1", 100, 0)
		if err != nil {
			t.Fatalf("ListPurchasesByCampaign failed: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("got %d purchases, want 2", len(list))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "camp-1", 1, 0)
		if err != nil {
			t.Fatalf("ListPurchasesByCampaign failed: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("got %d purchases, want 1", len(list))
		}
	})

	t.Run("empty campaign", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "nonexistent", 100, 0)
		if err != nil {
			t.Fatalf("ListPurchasesByCampaign failed: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("got %d purchases, want 0", len(list))
		}
	})
}

func TestCountPurchasesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	_ = repo.CreatePurchase(ctx, newTestPurchase("camp-1", "44444444"))
	_ = repo.CreatePurchase(ctx, newTestPurchase("camp-1", "55555555"))

	count, err := repo.CountPurchasesByCampaign(ctx, "camp-1")
	if err != nil {
		t.Fatalf("CountPurchasesByCampaign failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestUpdatePurchaseBuyCost(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "66666666")
	_ = repo.CreatePurchase(ctx, p)

	t.Run("success", func(t *testing.T) {
		err := repo.UpdatePurchaseBuyCost(ctx, p.ID, 75000)
		if err != nil {
			t.Fatalf("UpdatePurchaseBuyCost failed: %v", err)
		}
		got, _ := repo.GetPurchase(ctx, p.ID)
		if got.BuyCostCents != 75000 {
			t.Errorf("BuyCostCents = %d, want 75000", got.BuyCostCents)
		}
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseBuyCost(ctx, "nonexistent", 100)
		if !campaigns.IsPurchaseNotFound(err) {
			t.Errorf("expected ErrPurchaseNotFound, got: %v", err)
		}
	})
}

func TestUpdatePurchaseGrade(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "77777777")
	_ = repo.CreatePurchase(ctx, p)

	err := repo.UpdatePurchaseGrade(ctx, p.ID, 10.0)
	if err != nil {
		t.Fatalf("UpdatePurchaseGrade failed: %v", err)
	}
	got, _ := repo.GetPurchase(ctx, p.ID)
	if got.GradeValue != 10.0 {
		t.Errorf("GradeValue = %f, want 10.0", got.GradeValue)
	}
}

func TestUpdatePurchaseCardMetadata(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "88888888")
	_ = repo.CreatePurchase(ctx, p)

	err := repo.UpdatePurchaseCardMetadata(ctx, p.ID, "Blastoise", "9", "Base Set")
	if err != nil {
		t.Fatalf("UpdatePurchaseCardMetadata failed: %v", err)
	}
	got, _ := repo.GetPurchase(ctx, p.ID)
	if got.CardName != "Blastoise" {
		t.Errorf("CardName = %q, want %q", got.CardName, "Blastoise")
	}
}

func TestUpdatePurchaseCampaign(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	createTestCampaign(t, db, "camp-2", "Campaign 2")
	p := newTestPurchase("camp-1", "10101010")
	_ = repo.CreatePurchase(ctx, p)

	err := repo.UpdatePurchaseCampaign(ctx, p.ID, "camp-2", 500)
	if err != nil {
		t.Fatalf("UpdatePurchaseCampaign failed: %v", err)
	}
	got, _ := repo.GetPurchase(ctx, p.ID)
	if got.CampaignID != "camp-2" {
		t.Errorf("CampaignID = %q, want %q", got.CampaignID, "camp-2")
	}
}

func TestGetPurchaseIDByCertNumber(t *testing.T) {
	db := setupTestDB(t)
	repo := NewPurchasesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "20202020")
	_ = repo.CreatePurchase(ctx, p)

	t.Run("found", func(t *testing.T) {
		id, err := repo.GetPurchaseIDByCertNumber(ctx, "20202020")
		if err != nil {
			t.Fatalf("GetPurchaseIDByCertNumber failed: %v", err)
		}
		if id != p.ID {
			t.Errorf("id = %q, want %q", id, p.ID)
		}
	})

	t.Run("not found returns empty string", func(t *testing.T) {
		id, err := repo.GetPurchaseIDByCertNumber(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("GetPurchaseIDByCertNumber failed: %v", err)
		}
		if id != "" {
			t.Errorf("id = %q, want empty string", id)
		}
	})
}
```

**Note:** Also add tests for `ListUnsoldPurchases` (requires creating a sale to verify the LEFT JOIN exclusion), `ListAllUnsoldPurchases`, `UpdatePurchaseCardYear`, and `UpdatePurchaseCLValue` following the same patterns.

- [ ] **Step 2: Create `sales_repository_test.go`**

```go
package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func newTestSale(purchaseID string) *campaigns.Sale {
	now := time.Now().Truncate(time.Second)
	return &campaigns.Sale{
		ID:             "sale-" + purchaseID,
		PurchaseID:     purchaseID,
		SaleChannel:    campaigns.ChannelEbay,
		SalePriceCents: 75000,
		SaleFeeCents:   9975,
		SaleDate:       now.Format("2006-01-02"),
		DaysToSell:     14,
		NetProfitCents: 15025,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestCreateSale(t *testing.T) {
	db := setupTestDB(t)
	purchaseRepo := NewPurchasesRepository(db.DB)
	salesRepo := NewSalesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "30303030")
	_ = purchaseRepo.CreatePurchase(ctx, p)

	t.Run("success", func(t *testing.T) {
		s := newTestSale(p.ID)
		err := salesRepo.CreateSale(ctx, s)
		if err != nil {
			t.Fatalf("CreateSale failed: %v", err)
		}
	})

	t.Run("duplicate sale for same purchase", func(t *testing.T) {
		s := newTestSale(p.ID)
		s.ID = "sale-dup"
		err := salesRepo.CreateSale(ctx, s)
		if !campaigns.IsDuplicateSale(err) {
			t.Errorf("expected ErrDuplicateSale, got: %v", err)
		}
	})
}

func TestGetSaleByPurchaseID(t *testing.T) {
	db := setupTestDB(t)
	purchaseRepo := NewPurchasesRepository(db.DB)
	salesRepo := NewSalesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	p := newTestPurchase("camp-1", "40404040")
	_ = purchaseRepo.CreatePurchase(ctx, p)
	_ = salesRepo.CreateSale(ctx, newTestSale(p.ID))

	t.Run("found", func(t *testing.T) {
		got, err := salesRepo.GetSaleByPurchaseID(ctx, p.ID)
		if err != nil {
			t.Fatalf("GetSaleByPurchaseID failed: %v", err)
		}
		if got.SalePriceCents != 75000 {
			t.Errorf("SalePriceCents = %d, want 75000", got.SalePriceCents)
		}
		if got.SaleChannel != campaigns.ChannelEbay {
			t.Errorf("SaleChannel = %q, want %q", got.SaleChannel, campaigns.ChannelEbay)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := salesRepo.GetSaleByPurchaseID(ctx, "nonexistent")
		if !campaigns.IsSaleNotFound(err) {
			t.Errorf("expected ErrSaleNotFound, got: %v", err)
		}
	})
}

func TestListSalesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	purchaseRepo := NewPurchasesRepository(db.DB)
	salesRepo := NewSalesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")
	createTestCampaign(t, db, "camp-2", "Campaign 2")

	p1 := newTestPurchase("camp-1", "50505050")
	p2 := newTestPurchase("camp-1", "60606060")
	p3 := newTestPurchase("camp-2", "70707070")
	_ = purchaseRepo.CreatePurchase(ctx, p1)
	_ = purchaseRepo.CreatePurchase(ctx, p2)
	_ = purchaseRepo.CreatePurchase(ctx, p3)
	_ = salesRepo.CreateSale(ctx, newTestSale(p1.ID))
	_ = salesRepo.CreateSale(ctx, newTestSale(p2.ID))
	_ = salesRepo.CreateSale(ctx, newTestSale(p3.ID))

	t.Run("filters by campaign", func(t *testing.T) {
		list, err := salesRepo.ListSalesByCampaign(ctx, "camp-1", 100, 0)
		if err != nil {
			t.Fatalf("ListSalesByCampaign failed: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("got %d sales, want 2", len(list))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		list, err := salesRepo.ListSalesByCampaign(ctx, "camp-1", 1, 0)
		if err != nil {
			t.Fatalf("ListSalesByCampaign failed: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("got %d sales, want 1", len(list))
		}
	})

	t.Run("empty campaign", func(t *testing.T) {
		list, err := salesRepo.ListSalesByCampaign(ctx, "nonexistent", 100, 0)
		if err != nil {
			t.Fatalf("ListSalesByCampaign failed: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("got %d sales, want 0", len(list))
		}
	})
}

func TestListUnsoldPurchases_ExcludesSoldPurchases(t *testing.T) {
	db := setupTestDB(t)
	purchaseRepo := NewPurchasesRepository(db.DB)
	salesRepo := NewSalesRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign 1")

	sold := newTestPurchase("camp-1", "80808080")
	unsold := newTestPurchase("camp-1", "90909090")
	_ = purchaseRepo.CreatePurchase(ctx, sold)
	_ = purchaseRepo.CreatePurchase(ctx, unsold)
	_ = salesRepo.CreateSale(ctx, newTestSale(sold.ID))

	list, err := purchaseRepo.ListUnsoldPurchases(ctx, "camp-1")
	if err != nil {
		t.Fatalf("ListUnsoldPurchases failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d unsold purchases, want 1", len(list))
	}
	if list[0].CertNumber != "90909090" {
		t.Errorf("expected unsold purchase cert 90909090, got %s", list[0].CertNumber)
	}
}
```

- [ ] **Step 3: Verify SQLite tests**

```bash
go test ./internal/adapters/storage/sqlite/... -v -run "TestCreatePurchase|TestGetPurchase|TestListPurchasesByCampaign|TestCountPurchasesByCampaign|TestUpdatePurchase|TestGetPurchaseIDByCertNumber|TestCreateSale|TestGetSaleByPurchaseID|TestListSalesByCampaign|TestListUnsoldPurchases"
```

Expected: All PASS

- [ ] **Step 4: Lint**

```bash
golangci-lint run ./internal/adapters/storage/sqlite/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/storage/sqlite/purchases_repository_test.go \
       internal/adapters/storage/sqlite/sales_repository_test.go
git commit -m "test: add SQLite tests for purchases and sales repositories

Covers CRUD, pagination, duplicate constraints, not-found errors,
and LEFT JOIN unsold filtering. Uses in-memory SQLite with migrations."
```

---

## Task 10: C1 — Remove vestigial `lookupByQueryWithRetry`

**Files:**
- Modify: `internal/adapters/clients/pricecharting/pc_client.go`
- Modify: `internal/adapters/clients/pricecharting/lookup_strategies.go`
- Modify: `internal/adapters/clients/pricecharting/circuit_breaker_integration_test.go`

- [ ] **Step 1: Remove wrapper from `pc_client.go`**

Delete lines 23-29 (the comment and function):
```go
// lookupByQueryWithRetry performs lookup with retry logic.
// This is a stable API boundary that delegates to the internal implementation.
// The httpx.Client handles retry and circuit breaker logic internally.
// Kept as a wrapper for API stability and potential future custom retry logic.
func (p *PriceCharting) lookupByQueryWithRetry(ctx context.Context, query string) (*PCMatch, error) {
	return p.lookupByQueryInternal(ctx, query)
}
```

- [ ] **Step 2: Update callers in `lookup_strategies.go`**

Replace at line 95:
```go
apiMatch, lookupErr := p.lookupByQueryWithRetry(ctx, q)
```
With:
```go
apiMatch, lookupErr := p.lookupByQueryInternal(ctx, q)
```

Replace at line 254:
```go
match, err := p.lookupByQueryWithRetry(ctx, altQuery)
```
With:
```go
match, err := p.lookupByQueryInternal(ctx, altQuery)
```

- [ ] **Step 3: Update `circuit_breaker_integration_test.go`**

Replace all occurrences of `lookupByQueryWithRetry` with `lookupByQueryInternal` (3 call sites around lines 127, 176, 245).

Rename `TestPriceCharting_LookupByQueryWithRetry` to `TestPriceCharting_LookupByQueryInternal`.

- [ ] **Step 4: Verify**

```bash
go build ./internal/adapters/clients/pricecharting/...
go test ./internal/adapters/clients/pricecharting/...
golangci-lint run ./internal/adapters/clients/pricecharting/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/pricecharting/pc_client.go \
       internal/adapters/clients/pricecharting/lookup_strategies.go \
       internal/adapters/clients/pricecharting/circuit_breaker_integration_test.go
git commit -m "refactor: remove vestigial lookupByQueryWithRetry wrapper

Pure passthrough with no retry logic — httpx.Client handles retries
internally. Callers now call lookupByQueryInternal directly."
```

---

## Task 11: C2 — CLAUDE.md verification and update

**Files:**
- Modify: `CLAUDE.md` (if anything is stale)

- [ ] **Step 1: Verify migration count**

```bash
ls internal/adapters/storage/sqlite/migrations/*.up.sql | wc -l
```

Compare with CLAUDE.md's stated count ("30 migration pairs"). Update if different.

- [ ] **Step 2: Verify architecture tree**

Check that the directory tree in CLAUDE.md still reflects reality after the file splits. The tree lists package directories, not individual files, so splits within existing packages won't affect it. Verify no new packages were created.

- [ ] **Step 3: Check for stale references**

Grep CLAUDE.md for any file paths or function names that were changed in this round. There should be no stale references since all changes are within existing packages.

- [ ] **Step 4: Commit if changes were made**

```bash
# Only if CLAUDE.md was modified:
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for maintainability round 3 changes"
```

---

## Task 12: Final verification

- [ ] **Step 1: Full build**

```bash
go build ./...
```
Expected: SUCCESS

- [ ] **Step 2: Full test suite**

```bash
go test -race -count=1 ./...
```
Expected: All PASS

- [ ] **Step 3: Quality checks**

```bash
make check
```
Expected: Lint clean, architecture check passes, file size check passes. Only `main.go` (568L) should show a warning. The 4 files we split should all be under 500L.

- [ ] **Step 4: Verify file sizes of split files**

```bash
wc -l internal/adapters/advisortool/tools_portfolio.go \
     internal/adapters/advisortool/tools_portfolio_analysis.go \
     internal/domain/campaigns/import_parsing.go \
     internal/domain/campaigns/import_parsing_metadata.go \
     internal/adapters/clients/httpx/client.go \
     internal/adapters/clients/httpx/client_helpers.go \
     internal/domain/campaigns/service_advanced.go \
     internal/domain/campaigns/service_arbitrage.go
```
Expected: All under 500 lines.
