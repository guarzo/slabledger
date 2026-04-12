# P5 — adapters/httpserver Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use **superpowers:subagent-driven-development** to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. See Setup section below for worktree creation.

**Goal:** Fix 10 HTTP handler issues in `internal/adapters/httpserver/handlers/` — including post-200 error handling, missing error logging, goroutine lifecycle management, and HTTP status code correctness.

**Architecture:** All changes are in `internal/adapters/httpserver/handlers/`. No new interfaces. The goroutine lifecycle fix for `HandleGenerate` needs to match the existing shutdown pattern — check if a `context.Context` with cancellation is already threaded through the handler struct.

**Tech Stack:** Go 1.26, `net/http`.

**Worktree:** `.worktrees/plan-p5-httpserver`

---

## Setup

```bash
# Create worktree from the main repo root (not from within another worktree)
git -C /workspace worktree add /workspace/.worktrees/plan-p5-httpserver -b feature/polish-p5-httpserver
cd /workspace/.worktrees/plan-p5-httpserver
```

---

## Task 1: Buffer CSV before writing response — `campaigns_imports.go:105-107` (HIGH)

**Problem:** The CSV is written to the `http.ResponseWriter` while being generated. At line 104-107, `writer.Flush()` and `writer.Error()` are called after the response header (and some content) has already been committed. The HTTP 200 is sent before the flush error is detected — the client receives a partial CSV with no error indication.

**Fix strategy:** Buffer the entire CSV in a `bytes.Buffer` first. Only write to the response on success.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go:83-108`

- [ ] **Step 1: Read the current CSV write block**

```bash
sed -n '80,112p' internal/adapters/httpserver/handlers/campaigns_imports.go
```

- [ ] **Step 2: Replace with buffered approach**

```go
// Replace direct-to-ResponseWriter pattern:
var buf bytes.Buffer
writer := csv.NewWriter(&buf)
if err := writer.Write([]string{"Date Purchased", "Cert #", "Grader", "Investment", "Estimated Value", "Notes", "Date Sold", "Sold Price"}); err != nil {
    h.logger.Error(r.Context(), "csv header write failed", observability.Err(err))
    writeError(w, http.StatusInternalServerError, "failed to generate CSV")
    return
}
for _, e := range entries {
    if err := writer.Write([]string{
        e.DatePurchased, e.CertNumber, e.Grader,
        fmt.Sprintf("%.2f", e.Investment),
        fmt.Sprintf("%.2f", e.EstimatedValue),
        "", "", "",
    }); err != nil {
        h.logger.Error(r.Context(), "csv row write failed", observability.Err(err))
        writeError(w, http.StatusInternalServerError, "failed to generate CSV")
        return
    }
}
writer.Flush()
if err := writer.Error(); err != nil {
    h.logger.Error(r.Context(), "csv flush failed", observability.Err(err))
    writeError(w, http.StatusInternalServerError, "failed to generate CSV")
    return
}
// Only write headers and body after all errors are checked
w.Header().Set("Content-Type", "text/csv")
w.Header().Set("Content-Disposition", `attachment; filename="card_ladder_import.csv"`)
w.WriteHeader(http.StatusOK)
_, _ = w.Write(buf.Bytes())
```

Add `"bytes"` to imports.

- [ ] **Step 3: Apply the same fix to `campaigns_imports_mm.go:32`**

```bash
sed -n '25,75p' internal/adapters/httpserver/handlers/campaigns_imports_mm.go
```

Apply the same buffered CSV pattern there.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_imports.go internal/adapters/httpserver/handlers/campaigns_imports_mm.go
git commit -m "fix: buffer CSV in memory before writing response to prevent partial writes after 200"
```

---

## Task 2: Fix error handling in `HandleCreateSale` — `GetPurchase` failure (HIGH)

**Problem:** At lines ~84-88, `GetPurchase` failure maps all errors to 404 with no logging. If it's a DB error (500), the client silently gets a 404.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_purchases.go:84-98`

- [ ] **Step 1: Read the current block**

```go
// ~line 84-98
purchase, err := h.service.GetPurchase(r.Context(), s.PurchaseID)
if err != nil {
    writeError(w, http.StatusNotFound, "Purchase not found")
    return
}
...
campaign, err := h.service.GetCampaign(r.Context(), id)
if err != nil {
    writeError(w, http.StatusNotFound, "Campaign not found")
    return
}
```

- [ ] **Step 2: Add error logging and use 500 for non-not-found errors**

```go
purchase, err := h.service.GetPurchase(r.Context(), s.PurchaseID)
if err != nil {
    if inventory.IsPurchaseNotFound(err) {
        writeError(w, http.StatusNotFound, "Purchase not found")
    } else {
        h.logger.Error(r.Context(), "HandleCreateSale: GetPurchase failed",
            observability.String("purchaseID", s.PurchaseID),
            observability.Err(err))
        writeError(w, http.StatusInternalServerError, "Internal server error")
    }
    return
}

campaign, err := h.service.GetCampaign(r.Context(), id)
if err != nil {
    if inventory.IsCampaignNotFound(err) {
        writeError(w, http.StatusNotFound, "Campaign not found")
    } else {
        h.logger.Error(r.Context(), "HandleCreateSale: GetCampaign failed",
            observability.String("campaignID", id),
            observability.Err(err))
        writeError(w, http.StatusInternalServerError, "Internal server error")
    }
    return
}
```

Check if `inventory.IsPurchaseNotFound` exists:

```bash
grep -n "IsPurchaseNotFound\|func Is.*Purchase" internal/domain/inventory/errors.go 2>/dev/null | head
```

If it doesn't exist, use `errors.Is(err, inventory.ErrPurchaseNotFound)` directly, or check `inventory.ErrCampaignNotFound` pattern and follow it.

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_purchases.go
git commit -m "fix: add error logging and correct 500/404 status in HandleCreateSale GetPurchase/GetCampaign errors"
```

---

## Task 3: Upgrade analytics GetCampaign log level — `campaigns_analytics.go:62-76` (MEDIUM)

**Problem:** `GetCampaign` failure in analytics handler is logged at Debug level, returns partial 200. Should be Error level with a 500 or 404.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_analytics.go:62-76`

- [ ] **Step 1: Find the log call**

```bash
grep -n "Debug\|GetCampaign\|campaign not found\|404\|500" internal/adapters/httpserver/handlers/campaigns_analytics.go | head -15
```

- [ ] **Step 2: Fix**

```go
campaign, err := h.service.GetCampaign(r.Context(), id)
if err != nil {
    if inventory.IsCampaignNotFound(err) {
        writeError(w, http.StatusNotFound, "Campaign not found")
    } else {
        h.logger.Error(r.Context(), "campaigns analytics: GetCampaign failed",
            observability.String("campaignID", id),
            observability.Err(err))
        writeError(w, http.StatusInternalServerError, "Internal server error")
    }
    return
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_analytics.go
git commit -m "fix: upgrade analytics GetCampaign error from Debug to Error, return 404/500 instead of partial 200"
```

---

## Task 4: Log errors in DH listing goroutine — `campaigns_dh_listing.go:29` (MEDIUM)

**Problem:** `ListPurchases` return value (which contains an error field per P2's fix) is ignored in a fire-and-forget goroutine.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_dh_listing.go`

- [ ] **Step 1: Find the goroutine**

```bash
grep -n "go func\|ListPurchases\|goroutine" internal/adapters/httpserver/handlers/campaigns_dh_listing.go | head
```

- [ ] **Step 2: Add error logging**

```go
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    result := h.service.ListPurchases(ctx, certNumbers)
    if result.Error != nil {
        h.logger.Error(ctx, "dh listing goroutine: ListPurchases failed",
            observability.Err(result.Error))
    } else {
        h.logger.Info(ctx, "dh listing goroutine completed",
            observability.Int("listed", result.Listed))
    }
}()
```

Adapt to match the actual `DHListingResult` type (check what error field was added in P2).

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_dh_listing.go
git commit -m "fix: log errors from ListPurchases in DH listing fire-and-forget goroutine"
```

---

## Task 5: Surface error count in bulk DH match response — `dh_match_handler.go:56` (MEDIUM)

**Problem:** Bulk match per-purchase failures are only in server logs — the HTTP response doesn't indicate partial failure.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_match_handler.go`

- [ ] **Step 1: Read the handler**

```bash
cat internal/adapters/httpserver/handlers/dh_match_handler.go
```

- [ ] **Step 2: Add error count to response**

Find the response struct or map being written. Add an `errors_count` or `failed` field:

```go
// If writing a map:
writeJSON(w, http.StatusOK, map[string]interface{}{
    "matched": matchedCount,
    "failed":  failedCount,
    "total":   totalCount,
})

// Or if a struct exists, add a Failed int field
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_match_handler.go
git commit -m "fix: include failed count in bulk DH match response body"
```

---

## Task 6: Add error field to DH status handler — `dh_status_handler.go:151-174` (MEDIUM)

**Problem:** Zero counts in DH status response are indistinguishable from DB error.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/dh_status_handler.go:151-174`

- [ ] **Step 1: Read the relevant block**

```bash
sed -n '145,180p' internal/adapters/httpserver/handlers/dh_status_handler.go
```

- [ ] **Step 2: Add partial error signal**

```go
// If a DB call fails, return the partial result with an error field:
type dhStatusResponse struct {
    InStock  int    `json:"in_stock"`
    Listed   int    `json:"listed"`
    Synced   int    `json:"synced"`
    Error    string `json:"error,omitempty"`
}

// In the handler:
result := dhStatusResponse{...}
if err != nil {
    result.Error = "partial data: DB query failed"
    h.logger.Error(r.Context(), "dh status: DB error", observability.Err(err))
}
writeJSON(w, http.StatusOK, result)
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/dh_status_handler.go
git commit -m "fix: add error field to DH status response to distinguish zero-count from DB error"
```

---

## Task 7: Replace `TrimPrefix` with `PathValue` in admin handler (MEDIUM)

**Problem:** `HandleRemoveAllowedEmail` at line 81 uses `strings.TrimPrefix(r.URL.Path, "/api/admin/allowlist/")` instead of the standard `r.PathValue("email")` from Go 1.22+ `net/http`.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/admin.go:78-85`

- [ ] **Step 1: Read the current code**

```bash
sed -n '77,90p' internal/adapters/httpserver/handlers/admin.go
```

- [ ] **Step 2: Check the route definition**

```bash
grep -n "HandleRemoveAllowedEmail\|allowlist" internal/adapters/httpserver/routes.go
```

Expected route pattern like: `DELETE /api/admin/allowlist/{email}`

- [ ] **Step 3: Replace with PathValue**

```go
// Before:
email := strings.TrimPrefix(r.URL.Path, "/api/admin/allowlist/")

// After:
email := r.PathValue("email")
if email == "" {
    writeError(w, http.StatusBadRequest, "missing email parameter")
    return
}
```

Remove `strings` import if no longer used elsewhere in admin.go:

```bash
grep -c "strings\." internal/adapters/httpserver/handlers/admin.go
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/admin.go
git commit -m "fix: use r.PathValue instead of strings.TrimPrefix in HandleRemoveAllowedEmail"
```

---

## Task 8: Fix goroutine lifecycle in `HandleGenerate` — `social.go:111-120` (MEDIUM)

**Problem:** `HandleGenerate` starts a goroutine without any lifecycle management — it races against server shutdown.

The current code:
```go
go func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    created, err := h.service.DetectAndGenerate(ctx)
    ...
}()
```

The goroutine uses `context.Background()` — not the server's shutdown context — so it doesn't terminate on graceful shutdown.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/social.go:111-120`

- [ ] **Step 1: Check if SocialHandler has a server shutdown context**

```bash
grep -n "type SocialHandler\|shutdownCtx\|serverCtx\|done\b" internal/adapters/httpserver/handlers/social.go | head
```

- [ ] **Step 2: Check how other handlers manage goroutines**

```bash
grep -rn "go func\|WaitGroup\|shutdownCtx\|serverCtx" internal/adapters/httpserver/handlers/ | grep -v "_test.go" | head -20
```

- [ ] **Step 3: Add shutdown context awareness**

Option A — if handler has a shutdown context field:
```go
go func() {
    ctx, cancel := context.WithTimeout(h.shutdownCtx, 5*time.Minute)
    defer cancel()
    ...
}()
```

Option B — if no shutdown context, derive from request:
```go
// Use a detached context with a deadline, but log when server signals shutdown
go func() {
    // Create context that respects server shutdown
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    // ... existing logic ...
}()
// Note: server shutdown can't interrupt this goroutine. For proper lifecycle,
// add a shutdown context to SocialHandler (out of scope for this plan).
```

Option C (preferred if feasible) — add a `WaitGroup` to track background goroutines:

Check if there's a server-level `WaitGroup`:
```bash
grep -rn "WaitGroup\|wg\b" internal/adapters/httpserver/ | grep -v "_test.go" | head -10
```

If a `WaitGroup` exists at the server/handler level, use it:
```go
h.wg.Add(1)
go func() {
    defer h.wg.Done()
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    ...
}()
```

Pick the approach that matches existing patterns in the codebase.

- [ ] **Step 4: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/handlers/social.go
git commit -m "fix: add lifecycle management to HandleGenerate goroutine for graceful server shutdown"
```

---

## Task 9: Fix 400→404 for CampaignNotFound in `campaigns_purchases.go:43-44` (LOW)

**Problem:** `IsCampaignNotFound` is mapped to 400 (Bad Request) when it should be 404 (Not Found).

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_purchases.go:43-44`

- [ ] **Step 1: Find the mapping**

```bash
grep -n "IsCampaignNotFound\|StatusBadRequest\|400" internal/adapters/httpserver/handlers/campaigns_purchases.go | head
```

- [ ] **Step 2: Fix the status code**

```go
// Before:
if inventory.IsCampaignNotFound(err) {
    writeError(w, http.StatusBadRequest, "invalid purchase data")
    return
}

// After:
if inventory.IsCampaignNotFound(err) {
    writeError(w, http.StatusNotFound, "Campaign not found")
    return
}
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/handlers/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_purchases.go
git commit -m "fix: return 404 instead of 400 for IsCampaignNotFound in purchases handler"
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
