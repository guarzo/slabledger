# Social Media Content Generation — Implementation Plan

## Context

Card Yeti acquires 8-10 PSA-graded Pokemon cards/day through PSA. The business wants to build an Instagram presence to drive traffic to cardyeti.com. Rather than a standalone system (as described in the PRD at `tmp/prd/card-yeti-instagram-prd.md`), we'll build an in-app content generation tool that detects post-worthy events across inventory, auto-generates themed multi-card posts with AI captions, and presents them in a review queue.

**Phase 1 (this plan):** Generate + review queue + download/copy
**Phase 2 (future):** Instagram API publishing integration

## Feature Design

### Post Types (event-driven, multi-card)
1. **New Arrivals** — Cards imported in last 48h, batched together
2. **Price Movers** — Cards with significant price changes (>15%) since last snapshot
3. **Hot Deals** — Unsold cards priced notably below market value

### User Flow
1. Scheduler runs daily (+ manual "Generate Posts" button) → detects post-worthy card groups
2. AI generates Instagram caption + hashtags for each post
3. Posts land in review queue as drafts on the **Content** page
4. User previews carousel (cover slide + card slides), edits caption, approves/rejects
5. Approved posts → download slide images as PNGs + copy caption to clipboard

### Visual Format
- **Carousel**: Cover slide (themed) + individual card slides
- **Cover slide**: Card Yeti branding, post type title, date, card count
- **Card slide**: Slab image (frontImageURL), grade badge, card name, set, market price, CY branding
- Rendered client-side via `html-to-image` (React components → PNG at 1080x1080)

---

## Implementation Steps

### Step 1: Domain Layer — `internal/domain/social/`

**New files:**

`types.go` — Domain types:
- `PostType` enum: `new_arrivals`, `price_movers`, `hot_deals`
- `PostStatus` enum: `draft`, `approved`, `rejected`
- `SocialPost` struct: ID, UserID, PostType, Status, Caption, Hashtags, CoverTitle, CardCount, CreatedAt, UpdatedAt
- `PostCard` struct: PostID, PurchaseID, SlideOrder
- `PostCardDetail` struct: PostCard + enriched purchase fields (CardName, SetName, GradeValue, CertNumber, FrontImageURL, BuyCostCents, MedianCents, Trend30d)

`repository.go` — Repository interface:
- CRUD for posts + cards
- `GetRecentPurchaseIDs(ctx, since)` for new arrivals detection
- `GetPurchaseIDsInExistingPosts(ctx, ids, postType)` for duplicate prevention

`service.go` — Service interface:
- `DetectAndGenerate(ctx) (int, error)` — runs all detection types, returns count of posts created
- `ListPosts(ctx, status, limit, offset)` — filtered list
- `GetPost(ctx, id)` — post + card details
- `UpdateCaption(ctx, id, caption, hashtags)` — edit draft
- `Approve(ctx, id)` / `Reject(ctx, id)` / `Delete(ctx, id)`
- `RegenerateCaption(ctx, id, stream func(StreamEvent))` — re-run AI caption with SSE streaming

`service_impl.go` — Implementation with detection logic + caption generation:
- Constructor takes `Repository`, `advisor.LLMProvider` (optional, via functional option), `observability.Logger`
- Detection: queries purchases via repository, groups by post type, deduplicates against existing posts
- Caption generation: builds prompt with card data, calls `LLMProvider.StreamCompletion()` directly (no tool-calling needed — all card data is in the prompt)
- Minimum 2 cards per post, maximum 10

`prompts.go` — AI prompts for caption generation:
- System prompt: Instagram marketing expert for PSA-graded Pokemon card business, Card Yeti brand voice
- Per-post-type user prompts with serialized card metadata (name, grade, set, era, price, trend)
- Output format: caption text + hashtag suggestions (structured so we can parse them)

### Step 2: Database Migration

**New files:**

`internal/adapters/storage/sqlite/migrations/000011_social_posts.up.sql`:
```sql
CREATE TABLE social_posts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    post_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    caption TEXT NOT NULL DEFAULT '',
    hashtags TEXT NOT NULL DEFAULT '',
    cover_title TEXT NOT NULL DEFAULT '',
    card_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_social_posts_status ON social_posts(status);
CREATE INDEX idx_social_posts_type ON social_posts(post_type);

CREATE TABLE social_post_cards (
    post_id TEXT NOT NULL REFERENCES social_posts(id) ON DELETE CASCADE,
    purchase_id TEXT NOT NULL,
    slide_order INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (post_id, purchase_id)
);
CREATE INDEX idx_social_post_cards_purchase ON social_post_cards(purchase_id);
```

`000011_social_posts.down.sql`: Drop both tables.

### Step 3: SQLite Repository

**New file:** `internal/adapters/storage/sqlite/social_repository.go`

Implements `social.Repository`. Pattern follows `advisor_cache.go`:
- Struct with `*sql.DB`
- `ListPostCards` joins `social_post_cards` with `purchases` table to get enriched card details
- `GetRecentPurchaseIDs` queries purchases by `created_at > ?`
- `GetPurchaseIDsInExistingPosts` checks existing non-rejected posts

### Step 4: HTTP Handlers

**New file:** `internal/adapters/httpserver/handlers/social.go`

Endpoints (all auth-required):

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/social/posts` | List posts (query: `status`) |
| `GET` | `/api/social/posts/{id}` | Get post + card details |
| `POST` | `/api/social/posts/generate` | Manual trigger: run detection |
| `PATCH` | `/api/social/posts/{id}/caption` | Update caption + hashtags |
| `POST` | `/api/social/posts/{id}/approve` | Approve draft |
| `POST` | `/api/social/posts/{id}/reject` | Reject draft |
| `DELETE` | `/api/social/posts/{id}` | Delete post |
| `POST` | `/api/social/posts/{id}/regenerate-caption` | SSE: regenerate AI caption |

SSE streaming for `regenerate-caption` follows the exact pattern in `handlers/advisor.go:streamAnalysis()`.

**Modify:** `internal/adapters/httpserver/router.go`
- Add `socialHandler *handlers.SocialHandler` field to `Router` and `RouterConfig`
- Register routes in `Setup()` (gated on `rt.socialHandler != nil && rt.authMW != nil`)
- Add SPA route: `mux.HandleFunc("/content", rt.spaHandler.HandleIndex)`

### Step 5: Scheduler Job

**New file:** `internal/adapters/scheduler/social_content.go`

Follows `advisor_refresh.go` pattern:
- `SocialContentScheduler` with `StopHandle`
- Runs at configurable interval (default: 24h)
- Initial delay: 5min (let price refresh run first)
- Calls `service.DetectAndGenerate()`
- Gated by `SOCIAL_CONTENT_ENABLED` env var (default: false)

**Modify:** `internal/platform/config/types.go` — Add `SocialContentConfig` struct
**Modify:** `internal/platform/config/defaults.go` — Add defaults
**Modify:** `internal/adapters/scheduler/builder.go` — Add `SocialContentDetector` interface + `SocialContentService` to `BuildDeps`, wire scheduler

### Step 6: App Wiring

**Modify:** `cmd/slabledger/main.go` (around line 430-505):
- Create `social.Repository` (SQLite)
- Create `social.Service` with LLMProvider from Azure AI client (if configured)
- Create `handlers.SocialHandler`
- Add to `BuildDeps` for scheduler
- Add to `ServerDependencies`

**Modify:** `cmd/slabledger/server.go`:
- Add `SocialHandler` to `ServerDependencies`
- Pass to `RouterConfig`

### Step 7: Frontend Types + API

**New file:** `web/src/types/social.ts`
- `PostType`, `PostStatus` unions
- `SocialPost`, `PostCardDetail`, `SocialPostDetail` interfaces

**Modify:** `web/src/js/api.ts` — Add methods:
- `getSocialPosts(status?)`, `getSocialPost(id)`
- `generateSocialPosts()`, `updateSocialCaption(id, caption, hashtags)`
- `approveSocialPost(id)`, `rejectSocialPost(id)`, `deleteSocialPost(id)`

### Step 8: Frontend Queries

**New file:** `web/src/react/queries/useSocialQueries.ts`
- React Query hooks following `useCampaignQueries.ts` pattern
- Query keys in `queryKeys.ts`

### Step 9: Content Page + Components

**New file:** `web/src/react/pages/ContentPage.tsx`
- Header with "Content" title + "Generate Posts" button (calls `generateSocialPosts()`)
- Tab navigation: Drafts | Approved | Rejected (filters by status)
- Post list using `CardShell` component pattern

**New file:** `web/src/react/components/social/PostCard.tsx`
- Post type badge (color-coded: New Arrivals=blue, Price Movers=amber, Hot Deals=green)
- Card count, creation date, caption preview (truncated)
- Action buttons: Preview, Approve, Reject, Delete

**New file:** `web/src/react/components/social/PostPreview.tsx`
- Full carousel viewer: slide navigation (cover + card slides)
- Caption display with edit mode (textarea + character count for Instagram 2200 limit)
- Hashtag editor
- "Regenerate Caption" button → SSE streaming via `useAdvisorStream` hook (reuse)
- "Download Slides" + "Copy Caption" buttons

**New file:** `web/src/react/components/social/CaptionEditor.tsx`
- Textarea with live character count
- Hashtag display/edit
- Regenerate button wired to SSE endpoint

### Step 10: Slide Templates + Image Export

**New dependency:** `html-to-image` in `web/package.json`

**New file:** `web/src/react/components/social/slides/CoverSlide.tsx`
- 1080x1080 React component
- Card Yeti logo, post type title, date, card count
- Themed gradient background per post type

**New file:** `web/src/react/components/social/slides/CardSlide.tsx`
- 1080x1080 React component
- Slab image (`frontImageURL` with fallback placeholder if missing)
- Grade badge, card name, set name, market price
- Card Yeti branding footer

**New file:** `web/src/react/components/social/slides/SlideRenderer.tsx`
- Renders slides into offscreen DOM, captures via `html-to-image` `toPng()`
- `downloadAllSlides()` — downloads each slide as PNG
- `copyCaption()` — copies caption + hashtags to clipboard

### Step 11: Navigation + Routing

**Modify:** `web/src/react/App.tsx` — Add lazy-loaded `/content` route
**Modify:** `web/src/react/components/Navigation.tsx` — Add "Content" nav item

---

## Files Summary

### New Files (19)
| File | Purpose |
|------|---------|
| `internal/domain/social/types.go` | Domain types |
| `internal/domain/social/repository.go` | Repository interface |
| `internal/domain/social/service.go` | Service interface |
| `internal/domain/social/service_impl.go` | Service impl + detection logic |
| `internal/domain/social/prompts.go` | AI caption prompts |
| `internal/adapters/storage/sqlite/social_repository.go` | SQLite repository |
| `internal/adapters/storage/sqlite/migrations/000011_social_posts.up.sql` | Schema |
| `internal/adapters/storage/sqlite/migrations/000011_social_posts.down.sql` | Rollback |
| `internal/adapters/httpserver/handlers/social.go` | HTTP handlers |
| `internal/adapters/scheduler/social_content.go` | Scheduler job |
| `web/src/types/social.ts` | TypeScript types |
| `web/src/react/queries/useSocialQueries.ts` | React Query hooks |
| `web/src/react/pages/ContentPage.tsx` | Content queue page |
| `web/src/react/components/social/PostCard.tsx` | Post list item |
| `web/src/react/components/social/PostPreview.tsx` | Full post preview |
| `web/src/react/components/social/CaptionEditor.tsx` | Caption editor |
| `web/src/react/components/social/slides/CoverSlide.tsx` | Cover slide template |
| `web/src/react/components/social/slides/CardSlide.tsx` | Card slide template |
| `web/src/react/components/social/slides/SlideRenderer.tsx` | Image export |

### Modified Files (10)
| File | Change |
|------|--------|
| `internal/adapters/httpserver/router.go` | Add SocialHandler, routes, SPA route |
| `internal/adapters/scheduler/builder.go` | Add social content scheduler |
| `internal/platform/config/types.go` | Add SocialContentConfig |
| `internal/platform/config/defaults.go` | Add defaults |
| `cmd/slabledger/main.go` | Wire social repo, service, handler, scheduler |
| `cmd/slabledger/server.go` | Add SocialHandler to deps |
| `web/src/js/api.ts` | Add social API methods |
| `web/src/react/App.tsx` | Add /content route |
| `web/src/react/components/Navigation.tsx` | Add Content nav item |
| `web/package.json` | Add html-to-image |

---

## Key Patterns to Reuse

- **Advisor SSE streaming**: `handlers/advisor.go:streamAnalysis()` → `handlers/social.go` for caption regeneration
- **`useAdvisorStream` hook**: Reuse for streaming caption regeneration on frontend
- **Scheduler pattern**: `scheduler/advisor_refresh.go` → `scheduler/social_content.go`
- **Repository pattern**: `sqlite/advisor_cache.go` → `sqlite/social_repository.go`
- **Sell sheet helpers**: `sellSheetHelpers.tsx` `formatCardName()`, `gradeDisplay()` for slide rendering
- **CardShell component**: Consistent card UI on Content page
- **Blob download pattern**: `OperationsTab.tsx` for image download

## Edge Cases

- **No card images**: CardSlide renders a styled placeholder with card name/grade prominently displayed
- **Duplicate detection**: Purchases already in a non-rejected post of the same type are excluded
- **Empty detection**: Returns 0 posts created, no error. UI shows "No new content detected" toast
- **AI unavailable**: Post created with empty caption + "(Caption generation unavailable)" placeholder; user can type manually
- **Market data stale**: Price Movers skips cards with snapshots older than 24h

## Verification

1. `go test ./internal/domain/social/...` — Unit tests for detection logic + service
2. `go test ./internal/adapters/storage/sqlite/...` — Repository tests with migration
3. `go build ./cmd/slabledger` — Build succeeds
4. Start server, navigate to `/content` page — renders empty state
5. Click "Generate Posts" → posts appear in draft queue
6. Preview a post → carousel slides render with card data
7. Edit caption → save succeeds
8. Regenerate caption → SSE stream displays live AI text
9. Download slides → PNGs save at 1080x1080
10. Copy caption → clipboard contains caption + hashtags
11. `cd web && npm run build && npm test` — Frontend builds and tests pass
12. `golangci-lint run ./...` — No lint errors
