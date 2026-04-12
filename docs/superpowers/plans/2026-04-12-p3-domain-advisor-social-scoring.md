# P3 — domain/advisor+social+scoring Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 10 silent-failure, logic, duplication, and test quality issues in `internal/domain/advisor/`, `internal/domain/social/`, and `internal/domain/scoring/`.

**Architecture:** Fixes are confined to the three named packages. Mock replacements require adding canonical mock types to `internal/testutil/mocks/` for `advisor` and `social` packages. Coordinate with P4 so `testutil/mocks/` changes don't conflict (P4 adds mocks for `favorites` and `picks`; this plan adds mocks for `advisor` and `social`).

**Tech Stack:** Go 1.26, table-driven tests.

**Worktree:** `.worktrees/plan-p3-domain-advisor`

---

## Setup

```bash
git worktree add .worktrees/plan-p3-domain-advisor -b feature/polish-p3-domain-advisor
cd .worktrees/plan-p3-domain-advisor
go build ./internal/domain/advisor/... ./internal/domain/social/... ./internal/domain/scoring/...
go test -race ./internal/domain/advisor/... ./internal/domain/social/... ./internal/domain/scoring/...
```

---

## Task 1: Fix `ErrInsufficientData` guard logic — `scoring/scorer.go:11` (HIGH)

**Problem:** The guard at line 11 is:
```go
if len(req.Factors) < MinFactors && len(req.DataGaps) > 0 {
```
With `&&`, the error only fires when BOTH conditions are true. If factors is empty (zero-factor case) but DataGaps is empty too, the error is not returned. The intent is to return an error when we lack enough data (zero factors OR zero gaps from DataGaps perspective). The correct operator is `||`.

Actually re-read: the original intent is "if we have too few factors **and** there are known data gaps, reject." But consider: a zero-factor request with no gaps would still produce a useless score. The safer fix per the spec is `||`:

**Files:**
- Modify: `internal/domain/scoring/scorer.go:11`
- Modify (tests): `internal/domain/scoring/scorer_test.go`

- [ ] **Step 1: Write failing tests for the zero-factor and zero-gap cases**

In `internal/domain/scoring/scorer_test.go`, add:

```go
func TestScore_ZeroFactors_ReturnsError(t *testing.T) {
    req := ScoreRequest{
        EntityID:   "test",
        EntityType: "campaign",
        Factors:    nil, // zero factors
        DataGaps:   nil, // no gaps either
    }
    _, err := Score(req, DefaultWeightProfile())
    if err == nil {
        t.Fatal("expected ErrInsufficientData for zero factors, got nil")
    }
}

func TestScore_ZeroGaps_WithFewFactors_ReturnsError(t *testing.T) {
    req := ScoreRequest{
        EntityID:   "test",
        EntityType: "campaign",
        Factors:    []Factor{{Name: "roi", Value: 0.5, Confidence: 0.8}},
        DataGaps:   nil, // no gaps — but only 1 factor, below MinFactors
    }
    _, err := Score(req, DefaultWeightProfile())
    // With current && logic: no error (because DataGaps is empty)
    // With correct || logic: error (because Factors < MinFactors)
    if err == nil {
        t.Fatal("expected ErrInsufficientData for too few factors, got nil")
    }
}
```

Run to confirm they fail:
```bash
go test -race ./internal/domain/scoring/... -run "TestScore_Zero"
```

- [ ] **Step 2: Fix the guard**

```go
// Before:
if len(req.Factors) < MinFactors && len(req.DataGaps) > 0 {

// After:
if len(req.Factors) < MinFactors || len(req.DataGaps) > 0 {
```

Wait — `|| len(req.DataGaps) > 0` would reject any request with data gaps, which is too aggressive. The correct reading from the spec is: `&&` should be `||` for the **factors** condition. The actual intended guard is:

"Return ErrInsufficientData when there are too few factors (regardless of gaps) OR when factors are missing AND there are known gaps."

The minimal fix is:
```go
if len(req.Factors) < MinFactors {
    return ScoreCard{}, &ErrInsufficientData{
        Available: len(req.Factors),
        Required:  MinFactors,
        Gaps:      req.DataGaps,
    }
}
```

This correctly rejects zero-factor requests regardless of gap presence.

- [ ] **Step 3: Run tests**

```bash
go test -race ./internal/domain/scoring/...
```
Expected: new tests pass, existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/scoring/scorer.go internal/domain/scoring/scorer_test.go
git commit -m "fix: ErrInsufficientData guard now fires when factors < MinFactors regardless of DataGaps"
```

---

## Task 2: Propagate errors in `AnalyzeCampaign`/`AssessPurchase` — `advisor/service_impl.go:103-111` (HIGH)

**Problem:** Errors from `CampaignData`/`PurchaseData` and `BuildScoreCard` are silently dropped — the service proceeds without scoring context. The caller can't detect degraded analysis.

**Files:**
- Modify: `internal/domain/advisor/service_impl.go:98-120`

- [ ] **Step 1: Read the current block**

```go
func (s *service) AnalyzeCampaign(ctx context.Context, campaignID string, stream func(StreamEvent)) error {
    var scoreCard *scoring.ScoreCard
    if s.scoringData != nil {
        data, err := s.scoringData.CampaignData(ctx, campaignID)
        if err == nil && data != nil {
            sc, scErr := BuildScoreCard(campaignID, "campaign", data, scoring.CampaignAnalysisProfile)
            if scErr == nil {
                scoreCard = &sc
                s.recordGaps(ctx, sc, "", "")
            }
        }
    }
    ...
```

- [ ] **Step 2: Add warn logging on errors**

The errors should not cause the analysis to fail (degraded analysis is better than no analysis). But they should be logged:

```go
func (s *service) AnalyzeCampaign(ctx context.Context, campaignID string, stream func(StreamEvent)) error {
    var scoreCard *scoring.ScoreCard
    if s.scoringData != nil {
        data, err := s.scoringData.CampaignData(ctx, campaignID)
        if err != nil {
            s.logger.Warn(ctx, "AnalyzeCampaign: failed to load scoring data — proceeding without scorecard",
                observability.String("campaignID", campaignID),
                observability.Err(err))
        } else if data != nil {
            sc, scErr := BuildScoreCard(campaignID, "campaign", data, scoring.CampaignAnalysisProfile)
            if scErr != nil {
                s.logger.Warn(ctx, "AnalyzeCampaign: failed to build scorecard — proceeding without",
                    observability.String("campaignID", campaignID),
                    observability.Err(scErr))
            } else {
                scoreCard = &sc
                s.recordGaps(ctx, sc, "", "")
            }
        }
    }
    ...
```

Apply the same pattern to `AssessPurchase` — find it:

```bash
grep -n "AssessPurchase\|PurchaseData\|BuildScoreCard" internal/domain/advisor/service_impl.go | head -20
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/advisor/...
go test -race ./internal/domain/advisor/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/advisor/service_impl.go
git commit -m "fix: log scoring data/scorecard errors in AnalyzeCampaign and AssessPurchase instead of silently discarding"
```

---

## Task 3: Fix hardcoded `toolCalls[0].Name` — `advisor/service_impl.go:397-399` (MEDIUM)

**Problem:** Hardcoded `toolCalls[0].Name` at line 397-399 is incorrect when parallel tool calls are made — only the first tool is checked.

**Files:**
- Modify: `internal/domain/advisor/service_impl.go:395-405`

- [ ] **Step 1: Read the current block**

```bash
sed -n '390,415p' internal/domain/advisor/service_impl.go
```

- [ ] **Step 2: Fix to iterate all tool calls**

```go
// Before (checking only first tool call):
if len(toolCalls) > 0 && toolCalls[0].Name == "expected_schema" {
    // process toolCalls[0]
}

// After (iterate all tool calls):
for _, tc := range toolCalls {
    if tc.Name == "expected_schema" {
        // process tc
        break // or handle multiple if needed
    }
}
```

Adapt to match the actual code pattern at that location.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/advisor/...
go test -race ./internal/domain/advisor/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/advisor/service_impl.go
git commit -m "fix: iterate all tool calls instead of hardcoding toolCalls[0] in advisor service"
```

---

## Task 4: Deduplicate `cardIdentityKey` — `social/service_detect.go:72,338` (MEDIUM)

**Problem:** Anonymous struct `cardIdentityKey` is defined twice in the same file at lines 72 and 338.

**Files:**
- Modify: `internal/domain/social/service_detect.go`

- [ ] **Step 1: Find both definitions**

```bash
grep -n "cardIdentityKey\|type.*struct.*cardKey" internal/domain/social/service_detect.go
```

- [ ] **Step 2: Extract to a single named type**

Add a named type near the top of the file (after imports):

```go
// cardIdentityKey is used as a map key for deduplication by card identity.
type cardIdentityKey struct {
    CardName string
    SetName  string
    Grade    float64
}
```

Remove the anonymous struct definitions at both locations and replace with `cardIdentityKey`.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/social/...
go test -race ./internal/domain/social/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/social/service_detect.go
git commit -m "refactor: deduplicate cardIdentityKey anonymous struct into named type in service_detect.go"
```

---

## Task 5: Fix context leak in `social/caption.go:50-52` (MEDIUM)

**Problem:** `errCancel` (the cancel function from `context.WithCancel` or `context.WithTimeout`) is not deferred unconditionally — it's only called in the error path, causing a context leak on success.

**Files:**
- Modify: `internal/domain/social/caption.go:48-55`

- [ ] **Step 1: Read the current block**

```bash
sed -n '44,60p' internal/domain/social/caption.go
```

- [ ] **Step 2: Move cancel to a defer**

```go
// Before (only cancels on error):
ctx, cancel := context.WithTimeout(ctx, timeout)
if something {
    cancel()
    return err
}
// ... no cancel on success path

// After (always cancels):
ctx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
```

Simply add `defer cancel()` immediately after the context creation.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/social/...
go test -race ./internal/domain/social/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/social/caption.go
git commit -m "fix: defer cancel unconditionally to prevent context leak in caption.go"
```

---

## Task 6: Deduplicate LLM streaming block — `social/publishing.go:146` (MEDIUM)

**Problem:** `RegenerateCaption` duplicates ~30 lines of LLM streaming logic from `generateCaptionAsync`.

**Files:**
- Modify: `internal/domain/social/publishing.go`

- [ ] **Step 1: Read both functions**

```bash
grep -n "func.*RegenerateCaption\|func.*generateCaptionAsync" internal/domain/social/publishing.go
```

```bash
sed -n '<line1>,<line1+40>p' internal/domain/social/publishing.go
sed -n '<line2>,<line2+40>p' internal/domain/social/publishing.go
```

- [ ] **Step 2: Extract shared helper**

```go
// generateCaptionFromPrompt handles the common LLM streaming logic.
func (s *service) generateCaptionFromPrompt(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
    // ... shared LLM streaming block ...
}
```

Update both `RegenerateCaption` and `generateCaptionAsync` to call `generateCaptionFromPrompt`.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/domain/social/...
go test -race ./internal/domain/social/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/domain/social/publishing.go
git commit -m "refactor: extract shared LLM streaming helper to eliminate duplication between RegenerateCaption and generateCaptionAsync"
```

---

## Task 7: Simplify `computeConfidence` — `scoring/scorer.go:67-101` (MEDIUM)

**Problem:** `computeConfidence` double-iterates factors with redundant post-loop guards (`if len(factors) > 0` checked after a loop that already verified length).

**Files:**
- Modify: `internal/domain/scoring/scorer.go:67-101`

- [ ] **Step 1: Read the current function**

```go
func computeConfidence(factors []Factor, weights []FactorWeight) float64 {
    totalExpected := len(weights)
    if totalExpected == 0 {
        return 0.2
    }
    coverage := float64(len(factors)) / float64(totalExpected) * 0.3
    var sumAbs, sumConf float64
    var positive, negative int
    for _, f := range factors {
        sumAbs += math.Abs(f.Value)
        sumConf += f.Confidence
        if f.Value > 0.1 { positive++ } else if f.Value < -0.1 { negative++ }
    }
    strength := 0.0
    if len(factors) > 0 { strength = (sumAbs / float64(len(factors))) * 0.3 }
    quality := 0.0
    if len(factors) > 0 { quality = (sumConf / float64(len(factors))) * 0.2 }
    ...
}
```

- [ ] **Step 2: Simplify**

```go
func computeConfidence(factors []Factor, weights []FactorWeight) float64 {
    if len(weights) == 0 {
        return 0.2
    }
    n := len(factors)
    coverage := float64(n) / float64(len(weights)) * 0.3
    strength, quality := 0.0, 0.0
    positive, negative := 0, 0
    for _, f := range factors {
        strength += math.Abs(f.Value)
        quality += f.Confidence
        if f.Value > 0.1 {
            positive++
        } else if f.Value < -0.1 {
            negative++
        }
    }
    if n > 0 {
        strength = (strength / float64(n)) * 0.3
        quality = (quality / float64(n)) * 0.2
    }
    agreement := 0.0
    if significant := positive + negative; significant > 0 {
        agreement = float64(max(positive, negative)) / float64(significant) * 0.2
    }
    return clamp(coverage+strength+quality+agreement, 0.2, 0.95)
}
```

- [ ] **Step 3: Build and test (behavior must be identical)**

```bash
go test -race ./internal/domain/scoring/...
```
Expected: all tests pass with no numerical differences.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/scoring/scorer.go
git commit -m "refactor: simplify computeConfidence to eliminate double iteration and redundant guards"
```

---

## Task 8: Replace inline mocks in `advisor/service_test.go` (MEDIUM)

**Problem:** `advisor/service_test.go:12-72` defines inline `mockLLMProvider` and `mockToolExecutor` structs. These should use canonical mocks from `internal/testutil/mocks/`.

**Files:**
- Modify: `internal/domain/advisor/service_test.go`
- Create: `internal/testutil/mocks/advisor_mocks.go` (if mocks don't exist)

- [ ] **Step 1: Check for existing canonical mocks**

```bash
ls internal/testutil/mocks/ | grep -i advisor
```

- [ ] **Step 2: Create canonical mocks if needed**

If `internal/testutil/mocks/advisor_mocks.go` doesn't exist, create it:

```go
package mocks

import (
    "context"
    "github.com/guarzo/slabledger/internal/domain/advisor"
)

// MockLLMProvider is a test double for advisor.LLMProvider.
type MockLLMProvider struct {
    CompleteFn func(ctx context.Context, req advisor.CompletionRequest) (advisor.CompletionResponse, error)
}

func (m *MockLLMProvider) Complete(ctx context.Context, req advisor.CompletionRequest) (advisor.CompletionResponse, error) {
    if m.CompleteFn != nil {
        return m.CompleteFn(ctx, req)
    }
    return advisor.CompletionResponse{}, nil
}

// MockToolExecutor is a test double for advisor.ToolExecutor.
type MockToolExecutor struct {
    ExecuteFn func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)
}

func (m *MockToolExecutor) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
    if m.ExecuteFn != nil {
        return m.ExecuteFn(ctx, name, input)
    }
    return nil, nil
}
```

Check the actual interface signatures in `internal/domain/advisor/` before writing:

```bash
grep -n "type LLMProvider\|type ToolExecutor\|Complete\|Execute" internal/domain/advisor/llm.go internal/domain/advisor/tools.go 2>/dev/null | head -20
```

- [ ] **Step 3: Replace inline mocks in service_test.go**

Remove the inline struct definitions at lines 12-72 and replace all usages with `mocks.MockLLMProvider` and `mocks.MockToolExecutor`.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/domain/advisor/... ./internal/testutil/...
go test -race ./internal/domain/advisor/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/advisor/service_test.go internal/testutil/mocks/
git commit -m "refactor: replace inline mocks in advisor/service_test.go with canonical mocks from testutil/mocks"
```

---

## Task 9: Replace inline mock in `social/service_impl_test.go:261` (MEDIUM)

**Problem:** `social/service_impl_test.go:261` has an inline `mockSocialRepo` (137 lines). Replace with canonical mock.

**Files:**
- Modify: `internal/domain/social/service_impl_test.go`
- Create: `internal/testutil/mocks/social_mocks.go` (if not exists)

- [ ] **Step 1: Check for existing mock**

```bash
ls internal/testutil/mocks/ | grep -i social
```

- [ ] **Step 2: Find the social repository interface**

```bash
grep -n "type.*Repository\|interface" internal/domain/social/repository.go 2>/dev/null | head -20
```

- [ ] **Step 3: Create canonical mock following Fn-field pattern**

```go
package mocks

import (
    "context"
    "github.com/guarzo/slabledger/internal/domain/social"
)

// SocialRepositoryMock is a test double for social.Repository.
type SocialRepositoryMock struct {
    GetContentFn    func(ctx context.Context, id string) (*social.Content, error)
    SaveContentFn   func(ctx context.Context, c *social.Content) error
    ListContentFn   func(ctx context.Context) ([]social.Content, error)
    DeleteContentFn func(ctx context.Context, id string) error
    // Add all interface methods following this pattern
}

func (m *SocialRepositoryMock) GetContent(ctx context.Context, id string) (*social.Content, error) {
    if m.GetContentFn != nil {
        return m.GetContentFn(ctx, id)
    }
    return nil, nil
}
// ... implement all interface methods
```

Check the actual interface to implement all methods correctly.

- [ ] **Step 4: Replace inline mock in service_impl_test.go**

- [ ] **Step 5: Build and test**

```bash
go build ./internal/domain/social/... ./internal/testutil/...
go test -race ./internal/domain/social/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/social/service_impl_test.go internal/testutil/mocks/
git commit -m "refactor: replace inline mockSocialRepo in social/service_impl_test.go with canonical mock"
```

---

## Task 10: Remove or consolidate advisor re-export shims (LOW)

**Problem:** `advisor/llm.go`, `tools.go`, `tracking.go` are pure re-export shims with no logic — they just re-export types from other packages. Either consolidate into one file or delete if unused.

**Files:**
- Modify or delete: `internal/domain/advisor/llm.go`, `tools.go`, `tracking.go`

- [ ] **Step 1: Check what these files contain**

```bash
wc -l internal/domain/advisor/llm.go internal/domain/advisor/tools.go internal/domain/advisor/tracking.go
cat internal/domain/advisor/llm.go
cat internal/domain/advisor/tools.go
cat internal/domain/advisor/tracking.go
```

- [ ] **Step 2: Check if they're imported anywhere**

```bash
grep -rn "advisor\.LLM\|advisor\.Tool\|advisor\.Track" internal/ cmd/ | grep -v "_test.go" | head
```

- [ ] **Step 3: Consolidate or delete**

If all three files are shims with no unique logic:
- Merge their content into a single `internal/domain/advisor/interfaces.go`
- Delete the three original files

If they have separate concerns, add a comment to each explaining why it's separate.

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test -race ./internal/domain/advisor/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/advisor/
git commit -m "refactor: consolidate advisor re-export shims into interfaces.go"
```

---

## Final Verification

- [ ] **Run full test suite**

```bash
go test -race -timeout 10m ./...
```

- [ ] **Run quality checks**

```bash
make check
```
