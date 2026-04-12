# DH Enterprise Instagram Generation API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate the DH Enterprise Instagram Generation API to produce up to 4 Instagram carousel draft posts daily from `own_inventory` strategies, feeding them into the existing publish pipeline.

**Architecture:** A new `DHSocialScheduler` calls two new `dh.Client` methods to generate posts and poll for slide readiness. Once slides are ready, it creates `SocialPost` records with `PostType = "dh_instagram"` and pre-populated `SlideURLs`. The existing `SocialPublishScheduler` skips the Puppeteer render step for posts that already have slides, then publishes to Instagram unchanged.

**Tech Stack:** Go 1.26, `internal/adapters/clients/dh`, `internal/adapters/scheduler`, `internal/domain/social`, `internal/platform/config`, `net/http/httptest` for tests.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/adapters/clients/dh/types_v2.go` | Modify | Add `DHInstagramGenerateRequest`, `DHInstagramGenerateResponse`, `DHInstagramStatusResponse` types |
| `internal/adapters/clients/dh/client.go` | Modify | Add `GenerateInstagramPost` and `PollInstagramPostStatus` methods |
| `internal/adapters/clients/dh/client_test.go` | Modify | Add tests for both new client methods |
| `internal/domain/social/types.go` | Modify | Add `PostTypeDHInstagram` constant |
| `internal/adapters/scheduler/dh_social.go` | Create | `DHSocialScheduler` struct, `DHInstagramClient` interface, `buildDHCaption`, tick logic |
| `internal/adapters/scheduler/dh_social_test.go` | Create | Unit tests for `DHSocialScheduler.tick()` |
| `internal/adapters/scheduler/social_publish.go` | Modify | Skip render step when `post.SlideURLs` is already populated |
| `internal/adapters/scheduler/social_publish_test.go` | Modify | Add test case for pre-rendered (DH) post that skips render |
| `internal/platform/config/types.go` | Modify | Add `SocialEnabled`, `SocialHour`, `SocialPollInterval`, `SocialPollTimeout` to `DHConfig` |
| `internal/platform/config/loader.go` | Modify | Read `DH_SOCIAL_*` env vars |
| `internal/adapters/scheduler/builder.go` | Modify | Wire `DHSocialScheduler` in `BuildGroup` |
| `.env.example` | Modify | Document new `DH_SOCIAL_*` variables |

---

## Task 1: Add Response Types to `dh/types_v2.go`

**Files:**
- Modify: `internal/adapters/clients/dh/types_v2.go` (append at end of file)

- [ ] **Step 1: Add types**

Open `internal/adapters/clients/dh/types_v2.go` and append at the end of the file:

```go
// DHInstagramGenerateRequest is the request body for POST /api/v1/enterprise/instagram/generate.
type DHInstagramGenerateRequest struct {
	Scope    string `json:"scope"`
	Strategy string `json:"strategy"`
	Headline string `json:"headline,omitempty"`
}

// DHInstagramGenerateResponse is the response from POST /api/v1/enterprise/instagram/generate.
type DHInstagramGenerateResponse struct {
	PostID int64 `json:"post_id"`
}

// DHInstagramStatusResponse is the response from GET /api/v1/enterprise/instagram/posts/:id/status.
type DHInstagramStatusResponse struct {
	RenderStatus   string   `json:"render_status"`    // "generating" | "ready" | "failed"
	SlideImageURLs []string `json:"slide_image_urls"` // non-nil when render_status == "ready"
}
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/adapters/clients/dh/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/dh/types_v2.go
git commit -m "feat(dh): add Instagram generation response types"
```

---

## Task 2: Add Client Methods to `dh/client.go`

**Files:**
- Modify: `internal/adapters/clients/dh/client.go` (add two methods at end of file)

- [ ] **Step 1: Add `GenerateInstagramPost` method**

Append to `internal/adapters/clients/dh/client.go`:

```go
// GenerateInstagramPost initiates a DH-side Instagram post generation.
// Returns the numeric post_id for use with PollInstagramPostStatus.
// Requires EnterpriseAvailable() == true.
func (c *Client) GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error) {
	url := c.baseURL + "/api/v1/enterprise/instagram/generate"
	req := DHInstagramGenerateRequest{
		Scope:    scope,
		Strategy: strategy,
		Headline: headline,
	}
	var resp DHInstagramGenerateResponse
	if err := c.postEnterprise(ctx, url, req, &resp); err != nil {
		return 0, fmt.Errorf("dh GenerateInstagramPost: %w", err)
	}
	return resp.PostID, nil
}

// PollInstagramPostStatus returns the current render status and, when ready,
// the public slide image URLs for the given post_id.
// Requires EnterpriseAvailable() == true.
func (c *Client) PollInstagramPostStatus(ctx context.Context, postID int64) (*DHInstagramStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/enterprise/instagram/posts/%d/status", c.baseURL, postID)
	var resp DHInstagramStatusResponse
	if err := c.getEnterprise(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("dh PollInstagramPostStatus: %w", err)
	}
	return &resp, nil
}
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/adapters/clients/dh/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/dh/client.go
git commit -m "feat(dh): add GenerateInstagramPost and PollInstagramPostStatus methods"
```

---

## Task 3: Test the New DH Client Methods

**Files:**
- Modify: `internal/adapters/clients/dh/client_test.go` (add two test functions)

- [ ] **Step 1: Write the failing tests**

Append to `internal/adapters/clients/dh/client_test.go`:

```go
func TestClient_GenerateInstagramPost(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		strategy   string
		headline   string
		serverResp string
		statusCode int
		wantPostID int64
		wantErr    bool
	}{
		{
			name:       "success",
			scope:      "own_inventory",
			strategy:   "inventory_top_expensive",
			headline:   "",
			serverResp: `{"post_id": 42}`,
			statusCode: http.StatusOK,
			wantPostID: 42,
		},
		{
			name:       "server error",
			scope:      "own_inventory",
			strategy:   "inventory_top_expensive",
			serverResp: `{"error":"internal"}`,
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/api/v1/enterprise/instagram/generate", r.URL.Path)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

				var req DHInstagramGenerateRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				require.Equal(t, tc.scope, req.Scope)
				require.Equal(t, tc.strategy, req.Strategy)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.serverResp))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			postID, err := c.GenerateInstagramPost(context.Background(), tc.scope, tc.strategy, tc.headline)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantPostID, postID)
		})
	}
}

func TestClient_PollInstagramPostStatus(t *testing.T) {
	tests := []struct {
		name           string
		postID         int64
		serverResp     string
		statusCode     int
		wantStatus     string
		wantSlideCount int
		wantErr        bool
	}{
		{
			name:       "generating",
			postID:     42,
			serverResp: `{"render_status":"generating","slide_image_urls":null}`,
			statusCode: http.StatusOK,
			wantStatus: "generating",
		},
		{
			name:           "ready with slides",
			postID:         42,
			serverResp:     `{"render_status":"ready","slide_image_urls":["https://cdn.example.com/1.jpg","https://cdn.example.com/2.jpg"]}`,
			statusCode:     http.StatusOK,
			wantStatus:     "ready",
			wantSlideCount: 2,
		},
		{
			name:       "failed",
			postID:     42,
			serverResp: `{"render_status":"failed","slide_image_urls":null}`,
			statusCode: http.StatusOK,
			wantStatus: "failed",
		},
		{
			name:       "server error",
			postID:     42,
			serverResp: `{"error":"not found"}`,
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, fmt.Sprintf("/api/v1/enterprise/instagram/posts/%d/status", tc.postID), r.URL.Path)
				require.Equal(t, "Bearer test_api_key", r.Header.Get("Authorization"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.serverResp))
			}))
			defer server.Close()

			c := newTestClient(server.URL)
			resp, err := c.PollInstagramPostStatus(context.Background(), tc.postID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, resp.RenderStatus)
			require.Len(t, resp.SlideImageURLs, tc.wantSlideCount)
		})
	}
}
```

- [ ] **Step 2: Ensure the test file imports `net/http/httptest`**

Check that `internal/adapters/clients/dh/client_test.go` imports section includes:
```go
"net/http"
"net/http/httptest"
```
If not already present, add them to the existing import block.

- [ ] **Step 3: Run the tests to confirm they pass**

```bash
go test ./internal/adapters/clients/dh/... -run "TestClient_GenerateInstagramPost|TestClient_PollInstagramPostStatus" -v
```

Expected: both test functions pass (PASS).

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/clients/dh/client_test.go
git commit -m "test(dh): add tests for GenerateInstagramPost and PollInstagramPostStatus"
```

---

## Task 4: Add `PostTypeDHInstagram` Constant

**Files:**
- Modify: `internal/domain/social/types.go`

- [ ] **Step 1: Add the constant**

In `internal/domain/social/types.go`, locate the `PostType` constants block:

```go
const (
	PostTypeNewArrivals PostType = "new_arrivals"
	PostTypePriceMovers PostType = "price_movers"
	PostTypeHotDeals    PostType = "hot_deals"
)
```

Replace with:

```go
const (
	PostTypeNewArrivals  PostType = "new_arrivals"
	PostTypePriceMovers  PostType = "price_movers"
	PostTypeHotDeals     PostType = "hot_deals"
	PostTypeDHInstagram  PostType = "dh_instagram"
)
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/domain/social/...
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/social/types.go
git commit -m "feat(social): add PostTypeDHInstagram post type constant"
```

---

## Task 5: Create `DHSocialScheduler`

**Files:**
- Create: `internal/adapters/scheduler/dh_social.go`

- [ ] **Step 1: Write the scheduler file**

Create `internal/adapters/scheduler/dh_social.go`:

```go
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// DHInstagramClient is a narrow interface for DH Instagram generation methods.
// Implemented by *dh.Client.
type DHInstagramClient interface {
	EnterpriseAvailable() bool
	GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error)
	PollInstagramPostStatus(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
}

// dhInstagramStrategy holds strategy metadata for dh_instagram post generation.
type dhInstagramStrategy struct {
	key   string // DH API strategy identifier
	title string // Human-readable cover title stored on the post
}

// dhInstagramStrategies is the fixed set of own_inventory strategies run each daily tick.
var dhInstagramStrategies = []dhInstagramStrategy{
	{key: "inventory_top_expensive", title: "Top Expensive Cards"},
	{key: "inventory_top_gainers_week", title: "Top Weekly Gainers"},
	{key: "inventory_top_gainers_month", title: "Top Monthly Gainers"},
	{key: "inventory_pokemon_top_cards", title: "Top Pokémon Cards"},
}

// defaultDHInstagramHashtags is written to all DH-generated posts.
const defaultDHInstagramHashtags = "#pokemon #pokemoncards #pokemontcg #tradingcards #cardcollector"

// DHSocialSchedulerConfig holds runtime parameters for DHSocialScheduler.
type DHSocialSchedulerConfig struct {
	Hour         int           // UTC hour to fire (0–23)
	PollInterval time.Duration // How often to poll DH for render status
	PollTimeout  time.Duration // Max wait before abandoning a DH post
}

// DHSocialScheduler generates Instagram posts via the DH Enterprise Instagram API
// and stores them as dh_instagram draft SocialPosts for the existing publish pipeline.
type DHSocialScheduler struct {
	StopHandle
	dhClient   DHInstagramClient
	socialRepo DHSocialRepo
	logger     observability.Logger
	cfg        DHSocialSchedulerConfig
}

// DHSocialRepo is the minimal subset of social.Repository needed by DHSocialScheduler.
type DHSocialRepo interface {
	CreatePost(ctx context.Context, post *social.SocialPost) error
	UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error
	UpdateSlideURLs(ctx context.Context, id string, urls []string) error
}

// NewDHSocialScheduler constructs a DHSocialScheduler.
func NewDHSocialScheduler(
	dhClient DHInstagramClient,
	socialRepo DHSocialRepo,
	logger observability.Logger,
	cfg DHSocialSchedulerConfig,
) *DHSocialScheduler {
	return &DHSocialScheduler{
		dhClient:   dhClient,
		socialRepo: socialRepo,
		logger:     logger,
		cfg:        cfg,
	}
}

// Start runs the scheduler loop, firing once daily at cfg.Hour (UTC).
func (s *DHSocialScheduler) Start(ctx context.Context) {
	s.StopHandle.Init()
	go func() {
		defer s.StopHandle.Done()
		loop(ctx, s.StopHandle.Stop, time.Hour, func(ctx context.Context) {
			if time.Now().UTC().Hour() == s.cfg.Hour {
				s.tick(ctx)
			}
		})
	}()
}

// Stop signals the scheduler to stop.
func (s *DHSocialScheduler) Stop() {
	s.StopHandle.Signal()
}

func (s *DHSocialScheduler) tick(ctx context.Context) {
	if !s.dhClient.EnterpriseAvailable() {
		s.logger.Info(ctx, "dh social: enterprise key not configured, skipping")
		return
	}

	for _, strategy := range dhInstagramStrategies {
		if err := s.generatePost(ctx, strategy); err != nil {
			s.logger.Warn(ctx, "dh social: failed to generate post",
				observability.String("strategy", strategy.key),
				observability.Err(err),
			)
		}
	}
}

func (s *DHSocialScheduler) generatePost(ctx context.Context, strategy dhInstagramStrategy) error {
	postID, err := s.dhClient.GenerateInstagramPost(ctx, "own_inventory", strategy.key, "")
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	slideURLs, err := s.pollUntilReady(ctx, postID)
	if err != nil {
		return fmt.Errorf("poll post %d: %w", postID, err)
	}

	post := &social.SocialPost{
		PostType:   social.PostTypeDHInstagram,
		Status:     social.PostStatusDraft,
		CoverTitle: strategy.title,
		SlideURLs:  slideURLs,
	}
	if err := s.socialRepo.CreatePost(ctx, post); err != nil {
		return fmt.Errorf("create post: %w", err)
	}

	caption := buildDHCaption(strategy)
	if err := s.socialRepo.UpdatePostCaption(ctx, post.ID, caption, defaultDHInstagramHashtags); err != nil {
		// Non-fatal: post is created, caption update failure just means default empty caption.
		s.logger.Warn(ctx, "dh social: failed to set caption",
			observability.String("post_id", post.ID),
			observability.Err(err),
		)
	}

	s.logger.Info(ctx, "dh social: created draft post",
		observability.String("strategy", strategy.key),
		observability.String("post_id", post.ID),
		observability.Int("slides", len(slideURLs)),
	)
	return nil
}

func (s *DHSocialScheduler) pollUntilReady(ctx context.Context, dhPostID int64) ([]string, error) {
	deadline := time.Now().Add(s.cfg.PollTimeout)
	for time.Now().Before(deadline) {
		status, err := s.dhClient.PollInstagramPostStatus(ctx, dhPostID)
		if err != nil {
			return nil, fmt.Errorf("poll status: %w", err)
		}
		switch status.RenderStatus {
		case "ready":
			return status.SlideImageURLs, nil
		case "failed":
			return nil, fmt.Errorf("DH render failed for post_id %d", dhPostID)
		}
		// Still generating — wait before next poll.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.cfg.PollInterval):
		}
	}
	return nil, fmt.Errorf("timed out waiting for DH post %d to render", dhPostID)
}

// buildDHCaption returns a human-readable caption for a DH Instagram post.
func buildDHCaption(strategy dhInstagramStrategy) string {
	return fmt.Sprintf("%s — check out what's hot in our collection!", strategy.title)
}
```

- [ ] **Step 2: Verify the file compiles**

```bash
go build ./internal/adapters/scheduler/...
```

Expected: no output. If there are errors about `observability.Int` or `observability.Err` not existing, check what logging helpers are actually exported from `internal/domain/observability` and use the correct ones (e.g., `observability.String`, `observability.Int64`, `observability.ErrorField` — whichever is present).

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/scheduler/dh_social.go
git commit -m "feat(scheduler): add DHSocialScheduler for DH Enterprise Instagram generation"
```

---

## Task 6: Test `DHSocialScheduler`

**Files:**
- Create: `internal/adapters/scheduler/dh_social_test.go`

- [ ] **Step 1: Write the test file**

Create `internal/adapters/scheduler/dh_social_test.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

// mockDHInstagramClient implements DHInstagramClient for testing.
type mockDHInstagramClient struct {
	EnterpriseAvailableFn     func() bool
	GenerateInstagramPostFn   func(ctx context.Context, scope, strategy, headline string) (int64, error)
	PollInstagramPostStatusFn func(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
}

func (m *mockDHInstagramClient) EnterpriseAvailable() bool {
	if m.EnterpriseAvailableFn != nil {
		return m.EnterpriseAvailableFn()
	}
	return true
}

func (m *mockDHInstagramClient) GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error) {
	return m.GenerateInstagramPostFn(ctx, scope, strategy, headline)
}

func (m *mockDHInstagramClient) PollInstagramPostStatus(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
	return m.PollInstagramPostStatusFn(ctx, postID)
}

// mockDHSocialRepo implements DHSocialRepo for testing.
type mockDHSocialRepo struct {
	CreatePostFn         func(ctx context.Context, post *social.SocialPost) error
	UpdatePostCaptionFn  func(ctx context.Context, id string, caption, hashtags string) error
	UpdateSlideURLsFn    func(ctx context.Context, id string, urls []string) error
}

func (m *mockDHSocialRepo) CreatePost(ctx context.Context, post *social.SocialPost) error {
	if m.CreatePostFn != nil {
		return m.CreatePostFn(ctx, post)
	}
	return nil
}

func (m *mockDHSocialRepo) UpdatePostCaption(ctx context.Context, id string, caption, hashtags string) error {
	if m.UpdatePostCaptionFn != nil {
		return m.UpdatePostCaptionFn(ctx, id, caption, hashtags)
	}
	return nil
}

func (m *mockDHSocialRepo) UpdateSlideURLs(ctx context.Context, id string, urls []string) error {
	if m.UpdateSlideURLsFn != nil {
		return m.UpdateSlideURLsFn(ctx, id, urls)
	}
	return nil
}

func newTestDHSocialScheduler(dhClient DHInstagramClient, repo DHSocialRepo) *DHSocialScheduler {
	logger := telemetry.NewNopLogger()
	cfg := DHSocialSchedulerConfig{
		Hour:         6,
		PollInterval: 1 * time.Millisecond,
		PollTimeout:  100 * time.Millisecond,
	}
	return NewDHSocialScheduler(dhClient, repo, logger, cfg)
}

func TestDHSocialScheduler_Tick(t *testing.T) {
	readySlides := []string{"https://cdn.example.com/slide1.jpg", "https://cdn.example.com/slide2.jpg"}

	tests := []struct {
		name             string
		enterpriseAvail  bool
		generateFn       func(ctx context.Context, scope, strategy, headline string) (int64, error)
		pollFn           func(ctx context.Context, postID int64) (*dh.DHInstagramStatusResponse, error)
		wantPostsCreated int
	}{
		{
			name:            "no enterprise key — skips all",
			enterpriseAvail: false,
			wantPostsCreated: 0,
		},
		{
			name:            "happy path — all 4 strategies succeed",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				return &dh.DHInstagramStatusResponse{
					RenderStatus:   "ready",
					SlideImageURLs: readySlides,
				}, nil
			},
			wantPostsCreated: 4,
		},
		{
			name:            "one strategy fails generate — others still run",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				if strategy == "inventory_top_expensive" {
					return 0, errors.New("DH error")
				}
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				return &dh.DHInstagramStatusResponse{
					RenderStatus:   "ready",
					SlideImageURLs: readySlides,
				}, nil
			},
			wantPostsCreated: 3,
		},
		{
			name:            "one strategy render fails — others still run",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				// First call returns failed, all subsequent return ready.
				// Use a counter via closure.
				return &dh.DHInstagramStatusResponse{RenderStatus: "failed"}, nil
			},
			wantPostsCreated: 0, // all fail because pollFn always returns "failed"
		},
		{
			name:            "poll timeout — skips strategy",
			enterpriseAvail: true,
			generateFn: func(_ context.Context, scope, strategy, headline string) (int64, error) {
				return 42, nil
			},
			pollFn: func(_ context.Context, postID int64) (*dh.DHInstagramStatusResponse, error) {
				// Always "generating" → will timeout
				return &dh.DHInstagramStatusResponse{RenderStatus: "generating"}, nil
			},
			wantPostsCreated: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			postsCreated := 0

			repo := &mockDHSocialRepo{
				CreatePostFn: func(ctx context.Context, post *social.SocialPost) error {
					require.Equal(t, social.PostTypeDHInstagram, post.PostType)
					require.Equal(t, social.PostStatusDraft, post.Status)
					require.NotEmpty(t, post.SlideURLs)
					require.NotEmpty(t, post.CoverTitle)
					postsCreated++
					return nil
				},
			}

			dhClient := &mockDHInstagramClient{
				EnterpriseAvailableFn: func() bool { return tc.enterpriseAvail },
			}
			if tc.generateFn != nil {
				dhClient.GenerateInstagramPostFn = tc.generateFn
			}
			if tc.pollFn != nil {
				dhClient.PollInstagramPostStatusFn = tc.pollFn
			}

			s := newTestDHSocialScheduler(dhClient, repo)
			s.tick(context.Background())

			require.Equal(t, tc.wantPostsCreated, postsCreated)
		})
	}
}

func TestBuildDHCaption(t *testing.T) {
	for _, strategy := range dhInstagramStrategies {
		caption := buildDHCaption(strategy)
		require.NotEmpty(t, caption, "caption for strategy %s should not be empty", strategy.key)
		require.Contains(t, caption, strategy.title, "caption should include strategy title")
	}
}
```

- [ ] **Step 2: Run the tests**

```bash
go test ./internal/adapters/scheduler/... -run "TestDHSocialScheduler|TestBuildDHCaption" -v
```

Expected: all tests pass (PASS). If `telemetry.NewNopLogger()` does not exist, check what nop logger is available — look at other scheduler tests in the same package for the correct import and usage.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/scheduler/dh_social_test.go
git commit -m "test(scheduler): add DHSocialScheduler unit tests"
```

---

## Task 7: Skip Render Step for Pre-Rendered Posts

**Files:**
- Modify: `internal/adapters/scheduler/social_publish.go`

The `tick()` method currently always calls `renderClient.Render()` unconditionally (after the health check). We need to skip the render, slide-save, and `UpdateSlideURLs` steps when `post.SlideURLs` is already populated (i.e., DH posts).

- [ ] **Step 1: Modify `tick()` to skip render when slides exist**

In `social_publish.go`, locate lines approximately 121–159 (the health check through UpdateSlideURLs). Replace the section from the health check and render call with:

Find this code (the render section within tick(), approximately lines 121–159):
```go
	// Verify render sidecar is healthy
	if err := s.renderClient.Health(ctx); err != nil {
```

The full block to replace (from the health check through UpdateSlideURLs) should look like:

```go
	// If slides are not yet generated, render them via the sidecar.
	// DH-generated posts already have SlideURLs set; skip this step for them.
	if len(post.SlideURLs) == 0 {
		if err := s.renderClient.Health(ctx); err != nil {
			s.logger.Warn(ctx, "render service unhealthy, skipping publish tick",
				observability.Err(err),
			)
			return
		}

		blobs, err := s.renderClient.Render(ctx, post.ID, *post)
		if err != nil {
			s.logger.Error(ctx, "render failed",
				observability.String("post_id", post.ID),
				observability.Err(err),
			)
			return
		}

		urls, err := s.saveSlides(post.ID, blobs)
		if err != nil {
			s.logger.Error(ctx, "save slides failed",
				observability.String("post_id", post.ID),
				observability.Err(err),
			)
			return
		}

		if err := s.repo.UpdateSlideURLs(ctx, post.ID, urls); err != nil {
			s.logger.Error(ctx, "update slide urls failed",
				observability.String("post_id", post.ID),
				observability.Err(err),
			)
			return
		}
	}
```

**Important:** Read `social_publish.go` carefully before editing. Wrap the existing render/save/UpdateSlideURLs block in `if len(post.SlideURLs) == 0 { ... }`. The `s.publisher.Publish(ctx, post.ID)` call that follows must remain outside this conditional — it always runs regardless.

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/adapters/scheduler/...
```

Expected: no output.

- [ ] **Step 3: Run existing publish scheduler tests to confirm no regression**

```bash
go test ./internal/adapters/scheduler/... -run "TestSocialPublishScheduler" -v
```

Expected: all existing test cases pass.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/scheduler/social_publish.go
git commit -m "feat(scheduler): skip render step for posts with pre-populated SlideURLs"
```

---

## Task 8: Add Test for Pre-Rendered Post in Publish Scheduler

**Files:**
- Modify: `internal/adapters/scheduler/social_publish_test.go`

- [ ] **Step 1: Add test case for DH post (pre-rendered slides)**

Read `social_publish_test.go` to understand the exact struct field names used in the test table (they are local `fn`-field mocks, not from `internal/testutil/mocks/`). Then add a new test case that:

1. Sets up a `*social.PostDetail` with `SlideURLs` already populated:
```go
preRenderedPost := &social.PostDetail{
    SocialPost: social.SocialPost{
        ID:        "post-dh-1",
        PostType:  social.PostTypeDHInstagram,
        Status:    social.PostStatusDraft,
        Caption:   "Top cards this week!",
        Hashtags:  "#pokemon",
        SlideURLs: []string{
            "https://cdn.example.com/slide1.jpg",
            "https://cdn.example.com/slide2.jpg",
        },
    },
}
```

2. Configures the repo mock's `fetchEligibleDraft` fn to return this post.

3. Adds a boolean `renderCalled := false` guard to the render client mock's `Render` fn:
```go
mockRender.renderFn = func(ctx context.Context, id string, post social.PostDetail) ([][]byte, error) {
    renderCalled = true
    return nil, nil
}
```

4. Asserts after `s.tick(context.Background())` that `renderCalled == false` and that `publishCalled == true`.

The key invariant: when `post.SlideURLs` is already set, `renderClient.Render` must NOT be called, but `publisher.Publish` must still be called.

- [ ] **Step 2: Run all publish scheduler tests**

```bash
go test ./internal/adapters/scheduler/... -run "TestSocialPublishScheduler" -v
```

Expected: all test cases including the new one pass.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/scheduler/social_publish_test.go
git commit -m "test(scheduler): add pre-rendered post test case for publish scheduler"
```

---

## Task 9: Config Changes

**Files:**
- Modify: `internal/platform/config/types.go`
- Modify: `internal/platform/config/loader.go`

- [ ] **Step 1: Add fields to `DHConfig`**

In `internal/platform/config/types.go`, find the `DHConfig` struct (currently lines 122–129):

```go
type DHConfig struct {
	Enabled               bool
	CacheTTLHours         int
	RateLimitRPS          int
	OrdersPollInterval    time.Duration
	InventoryPollInterval time.Duration
	PushInterval          time.Duration
}
```

Replace with:

```go
type DHConfig struct {
	Enabled               bool
	CacheTTLHours         int
	RateLimitRPS          int
	OrdersPollInterval    time.Duration
	InventoryPollInterval time.Duration
	PushInterval          time.Duration
	SocialEnabled         bool
	SocialHour            int
	SocialPollInterval    time.Duration
	SocialPollTimeout     time.Duration
}
```

- [ ] **Step 2: Load new env vars in `loader.go`**

In `internal/platform/config/loader.go`, find the DH config loading block (around lines 244–249). After the existing DH lines, add:

```go
envBool("DH_SOCIAL_ENABLED", &cfg.DH.SocialEnabled, false)
envIntRange("DH_SOCIAL_HOUR", &cfg.DH.SocialHour, 0, 23)
envDurationPositive("DH_SOCIAL_POLL_INTERVAL", &cfg.DH.SocialPollInterval)
envDurationPositive("DH_SOCIAL_POLL_TIMEOUT", &cfg.DH.SocialPollTimeout)
```

- [ ] **Step 3: Set defaults in `ApplyDefaults()` or equivalent**

Check if `DHConfig` has an `ApplyDefaults()` method or if defaults are set elsewhere in `loader.go`. If there is a `Default()` function that sets `DHConfig` defaults, add:

```go
cfg.DH.SocialHour         = 6
cfg.DH.SocialPollInterval = 5 * time.Second
cfg.DH.SocialPollTimeout  = 3 * time.Minute
```

If defaults are set inline in `loader.go` (e.g., `envIntRange` with a default), set the struct field before the env-loading block.

- [ ] **Step 4: Verify compilation**

```bash
go build ./internal/platform/config/...
```

Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add internal/platform/config/types.go internal/platform/config/loader.go
git commit -m "feat(config): add DH_SOCIAL_* config fields for DHSocialScheduler"
```

---

## Task 10: Wire `DHSocialScheduler` in `BuildGroup`

**Files:**
- Modify: `internal/adapters/scheduler/builder.go`

- [ ] **Step 1: Add wiring in `BuildGroup`**

In `internal/adapters/scheduler/builder.go`, find the `BuildGroup` function. After the `SocialPublishScheduler` construction block (around line 273), add:

```go
if cfg.DH.SocialEnabled && deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() {
	dhSocialCfg := DHSocialSchedulerConfig{
		Hour:         cfg.DH.SocialHour,
		PollInterval: cfg.DH.SocialPollInterval,
		PollTimeout:  cfg.DH.SocialPollTimeout,
	}
	schedulers = append(schedulers, NewDHSocialScheduler(
		deps.DHClient,
		deps.SocialPublishRepo,
		deps.Logger,
		dhSocialCfg,
	))
}
```

**Note:** `deps.SocialPublishRepo` implements `SocialPublishRepo` which has `UpdateSlideURLs` and `FetchEligibleDraft`, but `DHSocialRepo` requires `CreatePost` and `UpdatePostCaption` which are not on `SocialPublishRepo`. You need to pass a repository that satisfies `DHSocialRepo`. Look at `BuildDeps` — there may be a separate social repository dep. If `SocialPublishRepo` does not implement `DHSocialRepo`, you must either:
- Add a `DHSocialRepo` field to `BuildDeps` (pass the full social SQLite repository)
- Or widen the `SocialPublishRepo` interface

The cleanest approach: add a `DHSocialRepo DHSocialRepo` field to `BuildDeps` (it will be wired with the same SQLite social repository that already implements `social.Repository`). Then use `deps.DHSocialRepo` in the wiring block above.

- [ ] **Step 2: Add `DHSocialRepo` to `BuildDeps` if needed**

If the `DHSocialRepo` interface is not satisfied by an existing dep, add to `BuildDeps` struct in `builder.go`:

```go
DHSocialRepo DHSocialRepo // optional: required when DH_SOCIAL_ENABLED=true
```

Then the wiring guard becomes:
```go
if cfg.DH.SocialEnabled && deps.DHClient != nil && deps.DHClient.EnterpriseAvailable() && deps.DHSocialRepo != nil {
```

- [ ] **Step 3: Wire `DHSocialRepo` at the call site**

Find where `BuildGroup` is called (likely in `cmd/slabledger/main.go` or a server setup file). The SQLite social repository that already implements `social.Repository` also satisfies `DHSocialRepo` (since `DHSocialRepo` is a subset of `social.Repository`). Pass it as `DHSocialRepo`.

To find the call site:
```bash
grep -r "BuildGroup\|BuildDeps{" /workspace/internal /workspace/cmd --include="*.go" -l
```

Then open that file and add `DHSocialRepo: <existing-social-repo-variable>` to the `BuildDeps` struct literal.

- [ ] **Step 4: Verify full build**

```bash
go build ./...
```

Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/scheduler/builder.go
git add $(git diff --name-only)  # pick up any other modified files (main.go, etc.)
git commit -m "feat(scheduler): wire DHSocialScheduler in BuildGroup"
```

---

## Task 11: Document New Config Variables in `.env.example`

**Files:**
- Modify: `.env.example`

- [ ] **Step 1: Add DH social config documentation**

Find the DH-related section in `.env.example` (search for `DH_API_BASE_URL` or `DH_ENTERPRISE_API_KEY`). After the existing DH variables, add:

```
# DH Enterprise Instagram Generation
# Enables DHSocialScheduler which creates up to 4 Instagram draft posts per day
# using DH's own_inventory strategies. Requires DH_ENTERPRISE_API_KEY to be set.
DH_SOCIAL_ENABLED=false
DH_SOCIAL_HOUR=6              # UTC hour to run daily (0-23)
DH_SOCIAL_POLL_INTERVAL=5s    # How often to poll DH while waiting for slide renders
DH_SOCIAL_POLL_TIMEOUT=3m     # Max time to wait for DH to finish rendering slides
```

- [ ] **Step 2: Commit**

```bash
git add .env.example
git commit -m "docs: document DH_SOCIAL_* environment variables in .env.example"
```

---

## Task 12: Full Test Suite

- [ ] **Step 1: Run all tests with race detection**

```bash
go test -race -timeout 10m ./...
```

Expected: all tests pass with no race conditions detected. Fix any failures before proceeding.

- [ ] **Step 2: Run architecture import check**

```bash
make check
```

Expected: lint passes, import check passes, file size check passes. If `dh_social.go` exceeds limits, split `buildDHCaption` and `dhInstagramStrategies` into a separate `dh_social_captions.go` file.

- [ ] **Step 3: Final commit if any fixes were needed**

If any fixes were made in this step:
```bash
git add -A
git commit -m "fix: address test/lint issues from full suite run"
```
