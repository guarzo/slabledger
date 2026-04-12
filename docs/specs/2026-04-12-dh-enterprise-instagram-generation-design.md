# Design: DH Enterprise Instagram Generation API Integration

**Date:** 2026-04-12  
**Status:** Approved

---

## Overview

Integrate the DH Enterprise Instagram Generation API as a second, parallel post-generation pipeline alongside the existing LLM + Puppeteer sidecar pipeline. DH handles card selection and slide rendering on their side; we receive ready-to-publish image URLs and store them as `SocialPost` records in the existing draft→published lifecycle.

---

## Goals

- Generate Instagram carousel posts daily using DH's `own_inventory` strategies
- Run all 4 `own_inventory` strategies per daily tick (up to 4 new draft posts per day)
- Reuse the existing publish scheduler, caption LLM, and Instagram publish path without modification
- Keep the existing LLM+Puppeteer pipeline fully intact and unchanged

## Non-Goals

- `global_market` scope strategies (not needed)
- Manual/on-demand generation UI triggers
- Strategy configuration per post (round-robin or user-selectable); strategies run fixed set daily
- Changes to the Instagram OAuth, token refresh, or insights systems

---

## Architecture

### Approach

New `DHSocialScheduler` in `internal/adapters/scheduler/` + two new methods on the existing `dh.Client`. No changes to `social.Service`, `social.Repository` interface, or `SocialPublishScheduler`.

### Component Map

```
internal/adapters/clients/dh/
  client.go                     ← add GenerateInstagramPost, PollInstagramPostStatus

internal/adapters/scheduler/
  dh_social.go                  ← new DHSocialScheduler
  group.go                      ← wire DHSocialScheduler when DH_SOCIAL_ENABLED

internal/domain/social/
  types.go                      ← add PostType constant "dh_instagram"

internal/platform/config/
  types.go                      ← add SocialEnabled, SocialHour, SocialPollInterval, SocialPollTimeout to DHConfig
  loader.go                     ← read DH_SOCIAL_* env vars

internal/adapters/scheduler/
  social_publish.go             ← skip render step when SlideURLs already populated

.env.example                    ← document new DH_SOCIAL_* variables
```

---

## Section 1: DH Client Extensions

Two new methods added to `dh.Client` in `internal/adapters/clients/dh/client.go`.

Both use the existing `doEnterprise` helper (which sets `X-Api-Key: <key>`), and are guarded by `EnterpriseAvailable()`.

```go
// GenerateInstagramPost initiates a DH-side post generation.
// Returns the numeric post_id for subsequent status polling.
// Requires EnterpriseAvailable() == true.
GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error)

// PollInstagramPostStatus returns the current render status and,
// when ready, the public slide image URLs.
PollInstagramPostStatus(ctx context.Context, postID int64) (*DHInstagramPostStatus, error)
```

```go
type DHInstagramPostStatus struct {
    RenderStatus   string   // "generating" | "ready" | "failed"
    SlideImageURLs []string // non-nil and non-empty when RenderStatus == "ready"
}
```

**API mapping:**

| Method | HTTP | Path |
|--------|------|------|
| `GenerateInstagramPost` | POST | `/api/v1/enterprise/instagram/generate` |
| `PollInstagramPostStatus` | GET | `/api/v1/enterprise/instagram/posts/:id/status` |

Request body for generate:
```json
{ "scope": "<scope>", "strategy": "<strategy>", "headline": "<headline>" }
```

---

## Section 2: DHSocialScheduler

**File:** `internal/adapters/scheduler/dh_social.go`

The scheduler fires once daily at `DH_SOCIAL_HOUR` (UTC). On each tick it runs all 4 `own_inventory` strategies sequentially.

### Strategies (own_inventory scope)

| Strategy | Description |
|----------|-------------|
| `inventory_top_expensive` | Highest-value unsold cards |
| `inventory_top_gainers_week` | Biggest weekly price gainers in inventory |
| `inventory_top_gainers_month` | Biggest monthly price gainers in inventory |
| `inventory_pokemon_top_cards` | Top Pokémon cards in current inventory |

### Tick Logic

```
DHSocialScheduler.tick()
  IF NOT DHClient.EnterpriseAvailable(): return

  FOR each strategy in [inventory_top_expensive, inventory_top_gainers_week,
                        inventory_top_gainers_month, inventory_pokemon_top_cards]:

    postID, err = DHClient.GenerateInstagramPost(ctx, "own_inventory", strategy, "")
    IF err: log.Warn, continue to next strategy

    poll loop (every DH_SOCIAL_POLL_INTERVAL, max DH_SOCIAL_POLL_TIMEOUT):
      status, err = DHClient.PollInstagramPostStatus(ctx, postID)
      IF err: log.Warn, break → next strategy
      IF status.RenderStatus == "ready": break poll loop
      IF status.RenderStatus == "failed": log.Warn, break → next strategy
      sleep(DH_SOCIAL_POLL_INTERVAL)
    IF timed out: log.Warn, continue to next strategy

    SocialRepository.CreatePost(ctx, SocialPost{
        PostType:   "dh_instagram",
        Status:     "draft",
        SlideURLs:  status.SlideImageURLs,
        CoverTitle: humanReadable(strategy), // e.g. "Top Expensive Cards"
    })

    // DH posts have no linked PostCard rows (no purchase IDs), so the existing
    // card-based generateCaptionAsync cannot be reused directly.
    // Instead: write a strategy-derived caption directly in the scheduler.
    caption = buildDHCaption(strategy)  // e.g. "Check out our top cards this week! 🃏"
    SocialRepository.UpdatePostCaption(ctx, postID, caption, defaultHashtags)
```

**Struct:**
```go
type DHSocialScheduler struct {
    dhClient       DHInstagramClient   // interface: GenerateInstagramPost, PollInstagramPostStatus
    socialRepo     social.Repository
    logger         observability.Logger
    pollInterval   time.Duration
    pollTimeout    time.Duration
    hour           int
}
```

`DHInstagramClient` is a narrow interface defined in the scheduler file (to keep DH adapter out of domain):
```go
type DHInstagramClient interface {
    EnterpriseAvailable() bool
    GenerateInstagramPost(ctx context.Context, scope, strategy, headline string) (int64, error)
    PollInstagramPostStatus(ctx context.Context, postID int64) (*DHInstagramPostStatus, error)
}
```

---

## Section 3: Post Lifecycle & Publish Path

### New post type constant

```go
// internal/domain/social/types.go
const PostTypeDHInstagram PostType = "dh_instagram"
```

### Skip render for pre-rendered posts

`SocialPublishScheduler` currently always calls `renderClient.Render(...)` to generate slide JPEGs. For DH posts, slides are already external public URLs and no local rendering is needed.

**Change in `internal/adapters/scheduler/social_publish.go`:**

```go
// Before calling renderClient.Render, check if slides are already present
if len(post.SlideURLs) > 0 {
    // slides already set (e.g. from DH); skip render, go straight to publish
} else {
    // existing render path
    slides, err = renderClient.Render(ctx, post.ID, *post)
    // ... save slides to disk, update SlideURLs ...
}
```

This is the only change to the publish scheduler.

### Full lifecycle

```
DHSocialScheduler.tick()
  → CreatePost (status=draft, SlideURLs populated)
  → [async] GenerateCaptionAsync → UpdatePostCaption + UpdateCoverTitle

SocialPublishScheduler.tick()  [unchanged entry point]
  → FetchEligibleDraft()        [dh_instagram posts are eligible]
  → IF post.SlideURLs set: skip render
  → ELSE: render via Puppeteer sidecar
  → PublishCarousel (token, igUserID, slideURLs, caption+hashtags)
  → SetPublished(instagramPostID)
```

**Caption handling:** DH posts have no linked `PostCard` rows (no purchase IDs), so the existing card-based `generateCaptionAsync` LLM flow cannot be used. Instead, the scheduler writes a strategy-derived caption directly at post creation time via `buildDHCaption(strategy)` — e.g., `"Check out our top cards this week!"` — with a fixed set of default hashtags. No LLM dependency for DH posts. The placeholder caption sentinel is never set, so posts are eligible for auto-publishing as soon as they're created.

---

## Section 4: Config & Wiring

### Config additions

**`internal/platform/config/types.go`** — add to `DHConfig`:
```go
SocialEnabled      bool          // DH_SOCIAL_ENABLED, default false
SocialHour         int           // DH_SOCIAL_HOUR, default 6
SocialPollInterval time.Duration // DH_SOCIAL_POLL_INTERVAL, default 5s
SocialPollTimeout  time.Duration // DH_SOCIAL_POLL_TIMEOUT, default 3m
```

**`internal/platform/config/loader.go`** — read env vars:
```go
cfg.DH.SocialEnabled      = parseBool(getEnv("DH_SOCIAL_ENABLED", "false"))
cfg.DH.SocialHour         = parseInt(getEnv("DH_SOCIAL_HOUR", "6"))
cfg.DH.SocialPollInterval = parseDuration(getEnv("DH_SOCIAL_POLL_INTERVAL", "5s"))
cfg.DH.SocialPollTimeout  = parseDuration(getEnv("DH_SOCIAL_POLL_TIMEOUT", "3m"))
```

**`.env.example`** additions:
```
# DH Enterprise Instagram Generation
DH_SOCIAL_ENABLED=false       # Enable DH-powered Instagram post generation
DH_SOCIAL_HOUR=6              # UTC hour to run daily (0-23)
DH_SOCIAL_POLL_INTERVAL=5s    # Polling interval while waiting for DH to render slides
DH_SOCIAL_POLL_TIMEOUT=3m     # Max wait before giving up on a DH render
```

### Wiring in BuildGroup

**`internal/adapters/scheduler/group.go`:**

```go
if cfg.DH.SocialEnabled && deps.DHClient.EnterpriseAvailable() {
    g.Add(NewDHSocialScheduler(
        deps.DHClient,
        deps.SocialRepository,
        deps.Logger,
        cfg.DH.SocialPollInterval,
        cfg.DH.SocialPollTimeout,
        cfg.DH.SocialHour,
    ))
}
```

### No handler changes

DH posts appear in existing `/api/social/posts` list. Caption editing, deletion, and publish triggers all work through existing endpoints unchanged.

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| DH returns error on generate | Log warning, skip this strategy, continue to next |
| DH render status → "failed" | Log warning, skip this strategy, continue to next |
| Poll timeout (>3 min) | Log warning, skip this strategy, continue to next |
| LLM caption generation fails | Placeholder caption set → post blocked from auto-publish |
| Instagram publish fails | Existing retry / error behavior unchanged |

---

## Testing

- Unit test `dh.Client.GenerateInstagramPost` and `PollInstagramPostStatus` with mock HTTP server responses
- Unit test `DHSocialScheduler.tick()` using a mock `DHInstagramClient` and mock `social.Repository`:
  - All 4 strategies run, posts created with SlideURLs and a non-placeholder caption
  - Strategy that returns "failed" is skipped without affecting others
  - Poll timeout causes skip without affecting others
  - Verify `buildDHCaption` produces correct non-empty captions for each strategy
- Unit test `SocialPublishScheduler` with a `dh_instagram` post that has `SlideURLs` pre-set — verify render client is NOT called
- All mocks via `internal/testutil/mocks/` using the Fn-field pattern

---

## Out of Scope

- `global_market` scope or strategy configuration
- Manual/on-demand UI trigger for DH posts
- Custom headlines per strategy
- Storing DH `post_id` externally (not needed after slides are retrieved)
- Deduplication of DH posts across days (each tick creates new drafts regardless)
