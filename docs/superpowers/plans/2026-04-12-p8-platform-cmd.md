# P8 — platform+cmd Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix internal error exposure in HTTP responses, add configurable shutdown timeout, and improve code quality in `internal/platform/` and `cmd/`.

**Architecture:** Changes are confined to `internal/platform/` and `cmd/slabledger/`. The `admin_analyze.go` spec item refers to the CLI admin command which writes to stdout/stderr (not HTTP). The actual HTTP error exposure is in `internal/adapters/httpserver/handlers/cardladder_sync.go:106` — but that file is in P5 scope. For P8, focus on: admin error formatting, cache TTL enforcement, shutdown timeout, config refactoring, handler registration order.

**Tech Stack:** Go 1.21+, environment variable configuration.

---

## Task 1: Replace raw err.Error() with sanitized HTTP error response in cardladder_sync handler

> **Note:** The spec attribute this to `cmd/slabledger/admin_analyze.go` but after code inspection, `admin_analyze.go` is a CLI tool (no HTTP). The actual HTTP `err.Error()` exposure is in `internal/adapters/httpserver/handlers/cardladder_sync.go:106`. Since P5 covers `internal/adapters/httpserver/`, coordinate: if P5 is already implemented, this task may be done. If not, implement it here.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/cardladder_sync.go` (line 106)

- [ ] **Step 1: Read cardladder_sync.go around line 106**

```go
// Find the HTTP 500 with raw error:
writeError(w, http.StatusInternalServerError, err.Error())
```

- [ ] **Step 2: Check P5 plan to avoid duplication**

If P5 is already done, skip this task. Otherwise replace raw error:

```go
// Before (exposes internal error details):
writeError(w, http.StatusInternalServerError, err.Error())

// After (logs internally, returns generic message):
h.logger.Error(r.Context(), "cardladder sync failed", observability.Err(err))
writeError(w, http.StatusInternalServerError, "sync operation failed")
```

- [ ] **Step 3: Build and test**

```bash
go build ./internal/adapters/httpserver/...
go test -race ./internal/adapters/httpserver/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/cardladder_sync.go
git commit -m "fix: replace raw error message in cardladder sync HTTP response with generic message"
```

---

## Task 2: Add configurable shutdown timeout via SHUTDOWN_TIMEOUT_SECONDS

**Why:** `shutdown.go:36` hardcodes `30 * time.Second` as the scheduler shutdown timeout. In production, this may need to be tuned.

**Files:**
- Modify: `cmd/slabledger/shutdown.go`
- Modify: `internal/platform/config/loader.go`
- Modify: `.env.example`

- [ ] **Step 1: Read current shutdown.go**

Read `cmd/slabledger/shutdown.go`. Confirm the timeout at line 36:

```go
case <-time.After(30 * time.Second):
    logger.Warn(ctx, "scheduler shutdown timed out after 30s")
```

- [ ] **Step 2: Update shutdownGracefully to accept a timeout parameter**

```go
// shutdownGracefully stops schedulers and waits for in-flight background
// goroutines before the database is closed.
func shutdownGracefully(
	ctx context.Context,
	logger observability.Logger,
	cancelScheduler context.CancelFunc,
	schedulerResult *scheduler.BuildResult,
	hOut handlerOutputs,
	socialService social.Service,
	campaignsService inventory.Service,
	shutdownTimeout time.Duration,  // NEW
) {
	logger.Info(ctx, "shutting down schedulers")
	cancelScheduler()
	schedulerResult.Group.StopAll()

	waitDone := make(chan struct{})
	go func() {
		schedulerResult.Group.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		// Schedulers shut down cleanly
	case <-time.After(shutdownTimeout):
		logger.Warn(ctx, "scheduler shutdown timed out",
			observability.String("timeout", shutdownTimeout.String()))
	}

	// Wait for any in-flight background DH bulk match to finish
	if hOut.DHHandler != nil {
		hOut.DHHandler.Wait()
	}

	// Wait for any in-flight background advisor analyses to finish
	if hOut.AdvisorHandler != nil {
		hOut.AdvisorHandler.Wait()
	}

	// Wait for in-flight social caption generation goroutines
	socialService.Wait()

	// Shut down campaign service background workers
	if campaignsService != nil {
		campaignsService.Close()
	}
}
```

- [ ] **Step 3: Add ShutdownTimeoutSeconds to config**

In `internal/platform/config/loader.go`, in the `Config` struct or the server config section, add:

```go
// In Config or ServerConfig struct:
ShutdownTimeout time.Duration
```

In `FromEnv` or equivalent, parse it:

```go
if v := os.Getenv("SHUTDOWN_TIMEOUT_SECONDS"); v != "" {
    if n, err := strconv.Atoi(v); err == nil && n > 0 {
        cfg.ShutdownTimeout = time.Duration(n) * time.Second
    }
}
```

Default:

```go
cfg.ShutdownTimeout = 30 * time.Second
```

Check the exact config struct path — use `grep -n "ShutdownTimeout\|ServerConfig\|type Config" internal/platform/config/` to find the right location.

- [ ] **Step 4: Update the caller in server.go or main.go**

Find where `shutdownGracefully` is called:

```bash
grep -n "shutdownGracefully" cmd/slabledger/*.go
```

Update the call to pass `cfg.ShutdownTimeout`.

- [ ] **Step 5: Add to .env.example**

```bash
# Graceful shutdown timeout in seconds (default: 30)
# SHUTDOWN_TIMEOUT_SECONDS=30
```

- [ ] **Step 6: Build**

```bash
go build ./cmd/slabledger/...
```

Expected: no errors.

- [ ] **Step 7: Run tests**

```bash
go test -race ./cmd/slabledger/...
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add cmd/slabledger/shutdown.go internal/platform/config/ .env.example
git commit -m "feat: add SHUTDOWN_TIMEOUT_SECONDS env var for configurable graceful shutdown"
```

---

## Task 3: Deduplicate FromEnv/FromFlags parse logic in config

**Why:** The spec notes `FromEnv`/`FromFlags` may have duplicate parse logic. After code inspection, they use different mechanisms (env helpers vs `flag.*Var`) with minimal duplication. The main refactoring opportunity is if any field has similar parse-and-validate logic in both places.

**Files:**
- Modify: `internal/platform/config/loader.go`

- [ ] **Step 1: Read the env helpers and FromEnv**

Read `internal/platform/config/loader.go` lines 43–160. Note the `envString`, `envInt`, `envBool`, `envDuration` helper functions. Confirm they're reusable.

- [ ] **Step 2: Identify any duplication**

```bash
grep -n "envString\|envInt\|envBool\|envDuration" internal/platform/config/loader.go | wc -l
grep -n "flag\." internal/platform/config/loader.go | wc -l
```

If FromFlags calls `flag.StringVar`, `flag.IntVar`, etc. with no shared validation logic, there's minimal duplication. In that case, add a clarifying comment:

```go
// FromEnv reads configuration from environment variables using the env* helpers above.
// FromFlags reads configuration from CLI flags using the standard flag package.
// Both set the same Config fields; FromFlags takes precedence (applied after FromEnv).
```

- [ ] **Step 3: If there IS duplication, extract shared validation**

If both `FromEnv` and `FromFlags` have duplicate range checks (e.g., "port must be 1–65535"):

```go
// validatePort validates a port number is in the valid range.
func validatePort(port int) error {
    if port < 1 || port > 65535 {
        return fmt.Errorf("port must be between 1 and 65535, got %d", port)
    }
    return nil
}
```

- [ ] **Step 4: Build and test**

```bash
go build ./internal/platform/config/...
go test -race ./internal/platform/config/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config/loader.go
git commit -m "refactor: clarify FromEnv/FromFlags relationship; extract shared port validation"
```

---

## Task 4: Verify init_services.go file size is acceptable

**Why:** Spec item 4 says `cmd/init_services.go` is "still 307 lines" and may need splitting if natural boundaries exist.

**Files:**
- Assess: `cmd/slabledger/init_services.go`

- [ ] **Step 1: Check current line count**

```bash
wc -l cmd/slabledger/init_services.go
```

Note: After the Phase 9 refactor (split to `init_inventory_services.go`), init_services.go is ~308 lines.

- [ ] **Step 2: Review the file structure**

Read `cmd/slabledger/init_services.go` in full. Identify what functions remain:

- `initializePriceProviders` — DH price provider wiring
- `initializeAdvisorService` — Azure AI + advisor service wiring
- `initializeSocialService` — social service wiring
- Other optional service initializers

- [ ] **Step 3: Decision: split or leave**

Per spec constraint: "do not split if no natural boundary exists. 307 lines is borderline — acceptable if the file has a single cohesive responsibility."

**If the functions share cohesive responsibility (all "initialize optional AI/social services"):** Leave the file and add a comment at the top:

```go
// init_services.go initializes optional AI-powered and social services.
// The core campaign/inventory services are initialized in init_inventory_services.go.
// Scheduler initialization is in init_schedulers.go.
```

**If natural split exists (e.g., social vs AI):** Create `init_ai_services.go` and move advisor-related functions there.

- [ ] **Step 4: Build**

```bash
go build ./cmd/slabledger/...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/slabledger/
git commit -m "docs: add file-level comment to init_services.go documenting responsibility boundary"
```

---

## Task 5: Align handler registration order with docs/API.md

**Why:** Spec item 8 says handler registration order in `cmd/handlers.go` should match `docs/API.md` order. This is a documentation/consistency improvement.

**Files:**
- Modify: `cmd/slabledger/handlers.go` (or wherever routes are registered)
- Reference: `docs/API.md`

- [ ] **Step 1: Read docs/API.md endpoint order**

```bash
grep -n "^##\|^###\|^GET\|^POST\|^PUT\|^DELETE\|^PATCH" docs/API.md | head -60
```

Note the documented endpoint order.

- [ ] **Step 2: Read cmd/handlers.go to find route registration**

Read `cmd/slabledger/handlers.go` in full to find the route wiring structure.

- [ ] **Step 3: Check if routes are registered in httpserver router instead**

The actual route registration is likely in `internal/adapters/httpserver/router.go` (not in `cmd/handlers.go`). If so, note this in the plan and check:

```bash
grep -rn "Handle\|mux\|router\|GET\|POST" internal/adapters/httpserver/router.go | head -40
```

- [ ] **Step 4: If reordering is needed, align registration with docs**

Only reorder if the mismatch is significant. If handlers.go is just a DI struct, not a router, skip this task.

Add a comment to `handlers.go`:

```go
// Handler registration follows the route order defined in docs/API.md.
// When adding new routes, update docs/API.md in the same commit.
```

- [ ] **Step 5: Build and test**

```bash
go build ./cmd/slabledger/...
go test -race ./cmd/slabledger/...
```

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/handlers.go
git commit -m "docs: align handler registration comment with docs/API.md order"
```

---

## Verification

After all tasks:

```bash
go build ./...
go test -race -timeout 10m ./...
make check
```

Expected: all pass, no regressions.
