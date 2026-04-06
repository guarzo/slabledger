# Instagram Content Generation — Design Document

A system that automatically detects interesting inventory, generates AI captions and background images, and publishes Instagram carousel posts. Built with hexagonal architecture: domain logic defines interfaces, adapters implement them.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture](#architecture)
3. [Domain Layer](#domain-layer)
4. [Content Detection](#content-detection)
5. [AI Caption Generation](#ai-caption-generation)
6. [AI Background Image Generation](#ai-background-image-generation)
7. [Instagram Publishing](#instagram-publishing)
8. [Instagram OAuth & Token Management](#instagram-oauth--token-management)
9. [Metrics Collection](#metrics-collection)
10. [Scheduling](#scheduling)
11. [HTTP API](#http-api)
12. [Frontend Slide Rendering](#frontend-slide-rendering)
13. [Database Schema](#database-schema)
14. [Configuration](#configuration)
15. [Error Handling & Resilience](#error-handling--resilience)
16. [Key Design Decisions](#key-design-decisions)

---

## System Overview

The content generation pipeline runs as a background process (scheduled or on-demand) and follows this flow:

```
Inventory Detection  →  Post Creation  →  AI Caption (async)  →  Slide Rendering (frontend)  →  Publishing (async)
                                        →  AI Backgrounds (async)                              →  Metrics Polling
```

**Post lifecycle**: `draft` → `publishing` → `published` | `failed`

Three post types drive content strategy:
- **new_arrivals** — Recently acquired inventory (last 7 days)
- **price_movers** — Cards with significant 30-day price changes (>=15%)
- **hot_deals** — Cards priced below 70% of market median value

---

## Architecture

Hexagonal (Clean Architecture) with strict dependency rules:

```
┌─────────────────────────────────────────────────────────┐
│  Domain Layer (internal/domain/)                        │
│  ┌─────────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │  social/     │  │  ai/     │  │  observability/    │ │
│  │  - Service   │  │  - LLM   │  │  - Logger          │ │
│  │  - Repo      │  │  - Image │  │                    │ │
│  │  - Publisher │  │  - Track │  │                    │ │
│  └─────────────┘  └──────────┘  └────────────────────┘ │
└─────────────────────────────────────────────────────────┘
         ▲ interfaces only            ▲
         │                            │
┌────────┴────────────────────────────┴───────────────────┐
│  Adapter Layer (internal/adapters/)                      │
│  ┌─────────────┐  ┌──────────┐  ┌────────────────────┐ │
│  │  instagram/  │  │  azureai/│  │  sqlite/           │ │
│  │  - Client    │  │  - LLM   │  │  - SocialRepo      │ │
│  │  - Publisher │  │  - Image │  │  - MetricsRepo     │ │
│  │  - Token*    │  │          │  │  - InstagramConfig │ │
│  └─────────────┘  └──────────┘  └────────────────────┘ │
│  ┌─────────────┐  ┌──────────────────────────────────┐ │
│  │  scheduler/  │  │  httpserver/handlers/            │ │
│  │  - Social    │  │  - social.go (CRUD + generate)   │ │
│  │  - Metrics   │  │  - social_media.go (slide upload)│ │
│  └─────────────┘  │  - instagram.go (OAuth + publish) │ │
│                    └──────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

**Key principle**: Domain code (`internal/domain/social/`) depends only on interfaces. External services (Instagram API, Azure AI, SQLite) are injected via adapters.

---

## Domain Layer

### Core Types

```go
// internal/domain/social/types.go

type PostType string   // "new_arrivals", "price_movers", "hot_deals"
type PostStatus string // "draft", "publishing", "published", "failed"

type SocialPost struct {
    ID              string     // UUID v4
    PostType        PostType
    Status          PostStatus
    Caption         string     // AI-generated, max 300 runes
    Hashtags        string     // Separate from caption for editing
    CoverTitle      string     // Short title for cover slide (max ~40 chars)
    CardCount       int
    InstagramPostID string     // Set after successful publish
    ErrorMessage    string     // Set on publish failure
    SlideURLs       []string   // Frontend-rendered slide images (JPEG)
    BackgroundURLs  []string   // AI-generated background images (PNG)
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type PostCard struct {
    PostID     string
    PurchaseID string
    SlideOrder int  // 1-indexed ordering within the carousel
}

// Enriched card detail with purchase data for API responses and prompt building
type PostCardDetail struct {
    PurchaseID       string
    SlideOrder       int
    CardName         string
    SetName          string
    CardNumber       string
    GradeValue       float64   // e.g. 10.0, 9.0
    Grader           string    // e.g. "PSA"
    CertNumber       string
    FrontImageURL    string
    AskingPriceCents int
    CLValueCents     int       // Card Ladder market value
    Trend30d         float64   // 30-day price trend as decimal (0.15 = +15%)
    CreatedAt        time.Time
    Sold             bool
}
```

### Service Interface

```go
// internal/domain/social/service.go

type Service interface {
    DetectAndGenerate(ctx context.Context) (int, error)        // Main entry: detect + create drafts
    ListPosts(ctx context.Context, status *PostStatus, limit, offset int) ([]SocialPost, error)
    GetPost(ctx context.Context, id string) (*PostDetail, error)
    UpdateCaption(ctx context.Context, id, caption, hashtags string) error
    Delete(ctx context.Context, id string) error
    Publish(ctx context.Context, id string) error              // Async Instagram publish
    RegenerateCaption(ctx context.Context, id string, stream func(ai.StreamEvent)) error
    Wait()                                                     // Wait for background goroutines
}
```

Dependencies are injected via functional options:

```go
NewService(repo Repository,
    WithLLM(llm),                                    // AI caption + post suggestion
    WithPublisher(publisher, tokenProvider),          // Instagram publishing
    WithImageGenerator(imageGen, quality, dir, url),  // AI background images
    WithMediaStore(mediaStore),                       // File storage abstraction
    WithAITracker(tracker),                           // Metrics recording
    WithLogger(logger),
)
```

### Domain Interfaces

```go
// Publishing abstraction (domain does not depend on Instagram client)
type Publisher interface {
    PublishCarousel(ctx context.Context, token, igUserID string, imageURLs []string, caption string) (*PublishResultInfo, error)
}

type InstagramTokenProvider interface {
    GetToken(ctx context.Context) (token, igUserID string, err error)
}

// File storage abstraction (domain does not depend on os package)
type MediaStore interface {
    EnsureDir(ctx context.Context, path string) error
    WriteFile(ctx context.Context, path string, data []byte) error
}

// Persistence
type Repository interface {
    CreatePost(ctx, post) error
    GetPost(ctx, id) (*SocialPost, error)
    ListPosts(ctx, status, limit, offset) ([]SocialPost, error)
    UpdatePostCaption(ctx, id, caption, hashtags) error
    UpdateCoverTitle(ctx, id, title) error
    SetPublishing(ctx, id) error          // Atomic status transition
    SetPublished(ctx, id, igPostID) error
    SetError(ctx, id, message) error
    DeletePost(ctx, id) error
    AddPostCards(ctx, postID, cards) error
    ListPostCards(ctx, postID) ([]PostCardDetail, error)
    GetRecentPurchaseIDs(ctx, since) ([]string, error)
    GetPurchaseIDsInExistingPosts(ctx, ids, postType) (map[string]bool, error)
    GetUnsoldPurchasesWithSnapshots(ctx) ([]PurchaseSnapshot, error)
    GetAvailableCardsForPosts(ctx) ([]PostCardDetail, error)
    UpdateSlideURLs(ctx, id, urls) error
    UpdateBackgroundURLs(ctx, id, urls) error
}
```

### Error Types

```go
// internal/domain/social/errors.go
ErrPostNotFound   // Post doesn't exist
ErrNotConfigured  // Publisher/token provider not injected
ErrNotPublishable // Caption is placeholder or empty — blocks publishing
```

---

## Content Detection

Detection uses a **two-tier strategy**: LLM-powered (primary) with rule-based fallback.

### Tier 1: LLM-Powered Detection

When an LLM provider is configured, it receives the full available inventory and returns suggested post groupings.

**Flow**:
1. Fetch all available cards via `repo.GetAvailableCardsForPosts()` (unsold, with images, not in existing posts)
2. Cap at 100 cards to prevent prompt bloat
3. Send to LLM with `postSuggestionSystemPrompt` — includes grouping strategies, post type classification criteria, and output format
4. Parse JSON response containing 2-5 post suggestions
5. Validate each suggestion:
   - Check purchase IDs exist in available cards
   - Deduplicate by purchase ID across suggestions
   - Deduplicate by **card identity** `(name, set, grade)` — prevents the same logical card appearing in multiple posts
   - Enforce min/max card count (1-9)
6. Create posts in DB, spawn async caption + background generation

**Card identity deduplication** resolves duplicates by preferring:
1. Cards with a front image URL
2. Cards with higher market value (CLValueCents)

**LLM grouping prompt** guides toward strong themes:
- Evolution lines, era stories, set collections
- Grade-tier groupings, price-tier themes
- Character spotlights, artist/art style connections
- Post type classification using real data (asking price vs. CL value, 30-day trend, acquisition date)

### Tier 2: Rule-Based Fallback

If LLM fails or isn't configured, three deterministic detectors run:

| Post Type | Detection Logic |
|-----------|----------------|
| **new_arrivals** | Purchases created within 7 days, unsold, with images |
| **price_movers** | `\|trend_30d\| >= 15%` in last 7 days of snapshot data |
| **hot_deals** | `buy_cost < 70% * median_value` in last 7 days of snapshot data |

Each detector:
1. Generates candidate purchase IDs
2. Deduplicates against existing posts of the same type
3. Deduplicates by card identity
4. Enforces min (1) / max (9) card count
5. Creates post + spawns async caption/background generation

---

## AI Caption Generation

Captions are generated asynchronously in a background goroutine after post creation.

### Flow

1. 3-minute timeout context (independent of caller)
2. Fetch post's card details from repository
3. Build user prompt based on post type (different prompts for new_arrivals, price_movers, hot_deals)
4. Stream LLM completion with `captionSystemPrompt`
5. Parse response as JSON `{title, caption, hashtags}` — fallback to text splitting if JSON fails
6. Truncate caption to 300 runes at word boundary (Instagram limit)
7. Save to DB using a **fresh context** (prevents timeout race with LLM context)
8. On failure: set `placeholderCaption` — this blocks publishing until regeneration

### Caption System Prompt

The prompt defines brand voice (knowledgeable curator, collector vocabulary) and enforces:
- **Varied openings**: Rotates between question hooks, bold claims, story/context, data hooks
- **Post-type CTAs**: hot_deals ("Priced to move"), new_arrivals ("Fresh in the case"), price_movers ("Track these")
- **Anti-patterns**: No hype language, emoji spam, fake urgency
- **Output format**: JSON with `title` (cover slide, max 40 chars), `caption` (max 300 chars), `hashtags`

### Card Data in Prompts

Each card includes: name, set, card number, grader + grade (with human label like "gem mint"), CL market value, cert number. Price movers also include 30-day trend percentage.

### Regeneration

Users can regenerate captions on demand via `RegenerateCaption()`, which streams the result back via SSE (Server-Sent Events) for real-time UI feedback.

---

## AI Background Image Generation

Background images are generated asynchronously alongside captions.

### Flow

1. 10-minute timeout (image generation is slower than text)
2. Generate **cover background** — mood-based prompt derived from post type
3. Generate **per-card backgrounds** — sequentially, themed by Pokemon type detection
4. Store as PNG files: `social/{postID}/bg-cover.png`, `bg-1.png`, `bg-2.png`, ...
5. Filter empty URLs (failed generations) before saving to DB
6. Graceful degradation: posts are publishable without backgrounds (falls back to card front images)

### Theme Detection

Pokemon-aware theme matching scans card names and set names for keywords:

| Theme | Keywords | Visual Direction |
|-------|----------|-----------------|
| Fire | charizard, arcanine, flareon, ... | Volcanic, flame accents, warm oranges and reds |
| Water | blastoise, gyarados, vaporeon, ... | Ocean depths, aquatic blues, flowing water |
| Electric | pikachu, raichu, jolteon, ... | Electric arcs, lightning, bright yellows |
| Dark/Cosmic | umbreon, mewtwo, gengar, ... | Cosmic, nebula, deep purples and blacks |
| Grass | venusaur, leafeon, sceptile, ... | Lush forest, emerald tones, natural light |
| Default | (no match) | Abstract energy, prismatic light, rich dark tones |

### Post-Type Moods

| Post Type | Mood |
|-----------|------|
| hot_deals | Energetic, warm, urgency — glowing embers, molten energy |
| new_arrivals | Fresh, premium, discovery — crystalline light, subtle sparkle |
| price_movers | Dynamic, momentum, velocity — data streams, motion blur |

### Image Prompt Structure

```
Generate a 1024x1024 background image for a social media post about collectible graded Pokemon cards.
Mood: {post type mood}
Theme: {detected card theme}
Style: Abstract, atmospheric. No text, no cards, no characters, no logos.
Dark base tones suitable for white text overlay. Rich but not overwhelming.
Leave the center relatively clear — cards will be composited on top.
```

---

## Instagram Publishing

### Flow

1. **Validation**: Check caption is not placeholder/empty
2. **Atomic status transition**: `SetPublishing(id)` — only `draft` and `failed` posts can be published, prevents double-publish
3. **Background goroutine** (5-minute timeout):
   a. Fetch post + card details
   b. Prefer slide URLs (frontend-rendered); fall back to card front images
   c. Append hashtags to caption
   d. Get OAuth token from `InstagramTokenProvider`
   e. Call `Publisher.PublishCarousel()`
   f. On success: set status to `published` with Instagram post ID
   g. On failure: set status to `failed` with error message

### Carousel API Protocol

The Instagram Graph API requires a multi-step process:

```
For each image:
  1. POST /{igUserID}/media  →  item container ID
     {image_url, is_carousel_item=true}
  2. Poll GET /{containerID}?fields=status_code
     until FINISHED (max 30s, 1s intervals)

Then:
  3. POST /{igUserID}/media  →  carousel container ID
     {media_type=CAROUSEL, children=[id1,id2,...], caption}
  4. Poll carousel container until FINISHED
  5. POST /{igUserID}/media_publish  →  published media ID
     {creation_id=carouselContainerID}
```

Single-image fallback: When only 1 image exists, publishes directly without carousel container step.

### Critical Safety: Publish-but-DB-Fail

If the Instagram publish succeeds but the DB update fails, the error message is set to:
> "published to Instagram (post X) but failed to update DB — do NOT re-publish"

This prevents accidental duplicate Instagram posts.

---

## Instagram OAuth & Token Management

Uses the **Instagram Login** OAuth path (`graph.instagram.com`) — does not require a Facebook Page.

### OAuth Flow

1. `GET /instagram/connect` → Returns authorization URL with CSRF state token
2. User authorizes on Instagram → Redirected to callback
3. `GET /instagram/callback?code=...&state=...`:
   - Validate CSRF state
   - Exchange code → short-lived token (`POST api.instagram.com/oauth/access_token`)
   - Exchange short-lived → long-lived token (`GET graph.instagram.com/access_token`)
   - Fetch username via `GET graph.instagram.com/{userID}?fields=username`
   - Store in `instagram_config` singleton table

### Token Refresh

- Long-lived tokens expire in ~60 days
- `TokenRefresher` auto-refreshes tokens expiring within 7 days
- Called by the social content scheduler before each detection run
- Refresh endpoint: `GET graph.instagram.com/refresh_access_token?grant_type=ig_refresh_token`

### Scopes

```
instagram_business_basic,instagram_business_content_publish
```

---

## Metrics Collection

After publishing, engagement metrics are polled from the Instagram Insights API.

### Types

```go
type PostMetrics struct {
    PostID      string
    Impressions int
    Reach       int
    Likes       int
    Comments    int
    Saves       int
    Shares      int
    PolledAt    time.Time
}
```

### Polling

The `InsightsPollerAdapter` fetches from two endpoints per post:
1. **Insights API** (`/{mediaID}/insights?metric=impressions,reach,saved,shares`) — impressions, reach, saves, shares
2. **Media API** (`/{mediaID}?fields=like_count,comments_count`) — likes, comments

Partial data is returned if one endpoint fails. Error only when both fail.

---

## Scheduling

### Social Content Scheduler

```go
type SocialContentConfig struct {
    Enabled      bool          // default: true
    Interval     time.Duration // default: 24h
    InitialDelay time.Duration // default: 5m
    ContentHour  int           // 0-23 UTC; -1 = use InitialDelay; default: 5 (5am UTC)
}
```

**Tick behavior**:
1. Refresh Instagram token if expiring soon
2. Call `DetectAndGenerate()` with 5-minute timeout
3. Log number of posts created

The scheduler computes initial delay from `ContentHour` (time until next occurrence of that UTC hour), then runs every `Interval`.

---

## HTTP API

### Social Content Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/social/posts?status=&limit=&offset=` | List posts (filtered) |
| `GET` | `/social/posts/{id}` | Get post with card details |
| `POST` | `/social/posts/generate` | Trigger detection (202 Accepted) |
| `PATCH` | `/social/posts/{id}/caption` | Update caption/hashtags |
| `DELETE` | `/social/posts/{id}` | Delete post + cleanup media |
| `POST` | `/social/posts/{id}/regenerate-caption` | Stream caption via SSE |
| `GET` | `/social/posts/{id}/metrics` | Fetch metrics snapshots |
| `GET` | `/social/metrics/summary` | Latest metrics for all published posts |

### Slide Upload

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/social/posts/{id}/upload-slides` | Multipart JPEG upload (max 10 files x 8MB) |
| `GET` | `/api/media/social/{postID}/{filename}` | Serve slide images (unauthenticated for Instagram API access) |

Slide upload validates JPEG magic bytes (`0xFF 0xD8`), removes old slides, and stores URLs in DB.

### Instagram OAuth

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/instagram/status` | Connection status + token expiry |
| `POST` | `/instagram/connect` | Get OAuth login URL |
| `GET` | `/instagram/callback` | OAuth callback (exchange code) |
| `POST` | `/instagram/disconnect` | Remove connection |
| `POST` | `/social/posts/{id}/publish` | Start async publishing (202 Accepted) |

### SSE Streaming (Caption Regeneration)

```
data: {"type":"done","content":"{\"caption\":\"...\",\"hashtags\":\"...\",\"title\":\"...\"}"}
```

---

## Frontend Slide Rendering

The frontend renders Instagram-ready slide images in the browser, then uploads them as JPEGs.

### Component Hierarchy

```
SlideRenderer (coordinator)
├── CoverSlide
│   ├── AIBackground (AI-generated background image)
│   ├── PostTypeBadge
│   ├── Branding (Card Yeti logo)
│   └── FanSpread (card fan effect) or cover variants:
│       ├── NewArrivalsCover
│       ├── PriceMoversCover
│       └── HotDealsCover
└── CardSlide (one per card)
    ├── AIBackground
    ├── CardArtHero (card front image)
    ├── GradeBadge (PSA grade display)
    ├── CardInfoPanel (name, set, price, trend)
    ├── TrendLines (price trend visualization)
    └── Visual effects: Sparkles, Flames, DynamicScatter
```

### Visual Theming

Color schemes are per-post-type, defined in `theme.ts`. Each slide type has distinct visual treatments (e.g., hot deals use warm/urgent colors, new arrivals use clean/premium aesthetics).

---

## Database Schema

### Tables

```sql
-- Core post storage
CREATE TABLE social_posts (
    id TEXT PRIMARY KEY,
    post_type TEXT,                    -- new_arrivals | price_movers | hot_deals
    status TEXT DEFAULT 'draft',       -- draft | publishing | published | failed
    caption TEXT DEFAULT '',
    hashtags TEXT DEFAULT '',
    cover_title TEXT DEFAULT '',
    card_count INTEGER,
    instagram_post_id TEXT DEFAULT '',
    error_message TEXT DEFAULT '',
    slide_urls JSON,                   -- ["url1", "url2", ...] rendered slides
    background_urls JSON,              -- ["url1", ...] AI backgrounds
    created_at DATETIME,
    updated_at DATETIME
);
CREATE INDEX idx_social_posts_status ON social_posts(status);
CREATE INDEX idx_social_posts_type ON social_posts(post_type);

-- Card-to-post associations with slide ordering
CREATE TABLE social_post_cards (
    post_id TEXT,
    purchase_id TEXT,
    slide_order INTEGER,
    PRIMARY KEY (post_id, purchase_id)
);
CREATE INDEX idx_social_post_cards_purchase ON social_post_cards(purchase_id);

-- Instagram OAuth credentials (singleton row)
CREATE TABLE instagram_config (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    access_token TEXT,
    ig_user_id TEXT,
    username TEXT,
    token_expires_at DATETIME,
    connected_at DATETIME,
    updated_at DATETIME
);

-- Engagement metrics time series (see SCHEMA.md for current definition)
-- Table: instagram_post_metrics
```

---

## Configuration

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `INSTAGRAM_APP_ID` | Instagram OAuth app ID | — |
| `INSTAGRAM_APP_SECRET` | Instagram OAuth app secret | — |
| `INSTAGRAM_REDIRECT_URI` | OAuth callback URL | `http://localhost:8081/auth/instagram/callback` |
| `AZURE_AI_ENDPOINT` | Azure AI base URL | — |
| `AZURE_AI_API_KEY` | Azure AI API key | — |
| `AZURE_AI_DEPLOYMENT` | Default AI model deployment | — |
| `SOCIAL_AI_DEPLOYMENT` | Separate model for social content (optional, falls back to main) | — |
| `IMAGE_AI_DEPLOYMENT` | Image generation model deployment | — |
| `IMAGE_AI_ENABLED` | Enable AI image generation | `false` |
| `IMAGE_AI_QUALITY` | Image quality: low, medium, high | `medium` |
| `SOCIAL_CONTENT_ENABLED` | Enable scheduler | `true` |
| `SOCIAL_CONTENT_HOUR` | UTC hour to run (0-23, -1 for delay-based) | `5` |
| `PRICE_REFRESH_ENABLED` | Enable price refresh (needed for trend data) | — |

---

## Error Handling & Resilience

### Async Context Management

Background goroutines create **independent contexts** (not derived from the HTTP request):
- Caption generation: 3-minute timeout
- Background image generation: 10-minute timeout
- Publishing: 5-minute timeout
- DB writes after async work: fresh 30-second context (prevents timeout race)

### Panic Recovery

All background goroutines use `safeGo()` which wraps with `defer recover()` and logs panics.

### Graceful Degradation

| Failure | Behavior |
|---------|----------|
| LLM unavailable | Falls back to rule-based detection |
| Caption generation fails | Sets placeholder caption; blocks publishing; user can regenerate |
| Image generation fails (single card) | Empty URL filtered out; other cards still get backgrounds |
| Image generation fails (all) | Post still publishable with card front images |
| Instagram publish fails | Error captured; status set to `failed`; user can retry |
| Instagram publish succeeds + DB fails | Error message explicitly warns "do NOT re-publish" |
| Token expired | Error returned; user prompted to reconnect |

### Deduplication Safety

- **Purchase ID dedup**: Same physical card can't appear in multiple posts
- **Card identity dedup**: Same logical card `(name, set, grade)` can't appear in multiple posts — prevents duplicates when inventory has multiple copies
- **Existing post check**: Rule-based detection cross-references against all non-rejected posts

### AI Call Tracking

Every LLM and image generation call is recorded with:
- Operation type (`social_caption`, `social_suggestion`)
- Success/error/rate-limited status
- Latency, token counts, estimated cost
- Detached context with 5-second timeout (telemetry doesn't block business logic)

---

## Key Design Decisions

1. **Hexagonal architecture** — Domain defines `Publisher`, `LLMProvider`, `ImageGenerator`, `MediaStore` interfaces. Adapters (Instagram API, Azure AI, filesystem) are injected. This makes the domain testable without external services and allows swapping providers.

2. **Two-tier detection** — LLM generates themed groupings when available; rule-based fallback ensures the system works without AI. This provides graceful degradation.

3. **Async everything** — Caption generation, background generation, and publishing all run in background goroutines. HTTP handlers return 202 Accepted immediately. `Wait()` supports graceful shutdown.

4. **Separate caption and hashtags** — Stored and edited independently. Hashtags are appended at publish time. This lets users edit captions without touching hashtag strategy.

5. **Slide rendering in frontend** — The frontend renders Instagram-ready slides (canvas → JPEG) with card images composited over AI backgrounds, then uploads them. This avoids server-side image compositing complexity.

6. **Token singleton** — One Instagram connection per instance (singleton `instagram_config` row). Token refresh is automatic via scheduler.

7. **Placeholder caption guard** — When AI caption generation fails, a placeholder string is stored. `Publish()` explicitly checks for this and returns `ErrNotPublishable`, preventing posts with broken captions from going live.

8. **Card identity deduplication** — Beyond purchase ID uniqueness, the system deduplicates by `(name, set, grade)` tuple. When duplicates exist, it prefers cards with images, then higher market value. This prevents redundant content when inventory has multiple copies of the same card.

9. **LLM JSON sanitization** — LLM output goes through `stripMarkdownFences()` and `sanitizeLLMJSON()` (fixes literal newlines/tabs inside JSON strings) before parsing. Falls back to text-based splitting if JSON fails.

10. **Fresh contexts for terminal writes** — After async operations, DB writes use a new `context.WithTimeout(context.Background(), 30s)` to prevent the main operation's timeout from blocking the final status update.
