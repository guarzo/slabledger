package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/social"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSocialPost(id string, postType social.PostType, status social.PostStatus) *social.SocialPost {
	now := time.Now().Truncate(time.Second)
	return &social.SocialPost{
		ID:         id,
		PostType:   postType,
		Status:     status,
		Caption:    "Test caption for " + id,
		Hashtags:   "#test #cards",
		CoverTitle: "Cover " + id,
		CardCount:  3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// createSocialPurchase inserts a campaign and a purchase that has a front_image_url,
// which is needed for social queries that filter on front_image_url != ”.
func createSocialPurchase(t *testing.T, db *DB, campaignID, purchaseID, certNumber string) {
	t.Helper()
	createTestCampaign(t, db, campaignID, "Social Campaign "+campaignID)
	now := time.Now().Truncate(time.Second)
	p := &campaigns.Purchase{
		ID:            purchaseID,
		CampaignID:    campaignID,
		CardName:      "Charizard",
		CertNumber:    certNumber,
		SetName:       "Base Set",
		CardNumber:    "4",
		Grader:        "PSA",
		GradeValue:    9.0,
		BuyCostCents:  80000,
		FrontImageURL: "https://example.com/" + certNumber + ".jpg",
		PurchaseDate:  "2026-01-15",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	repo := NewCampaignsRepository(db.DB)
	require.NoError(t, repo.CreatePurchase(context.Background(), p))
}

func TestSocialRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	t.Run("create and retrieve post", func(t *testing.T) {
		post := newTestSocialPost("soc-create-1", social.PostTypeNewArrivals, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		got, err := repo.GetPost(ctx, "soc-create-1")
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Equal(t, post.ID, got.ID)
		assert.Equal(t, social.PostTypeNewArrivals, got.PostType)
		assert.Equal(t, social.PostStatusDraft, got.Status)
		assert.Equal(t, post.Caption, got.Caption)
		assert.Equal(t, post.Hashtags, got.Hashtags)
		assert.Equal(t, post.CoverTitle, got.CoverTitle)
		assert.Equal(t, 3, got.CardCount)
		assert.Equal(t, post.CreatedAt.Unix(), got.CreatedAt.Unix())
		assert.Equal(t, post.UpdatedAt.Unix(), got.UpdatedAt.Unix())
	})

	t.Run("get non-existent returns nil nil", func(t *testing.T) {
		got, err := repo.GetPost(ctx, "soc-does-not-exist")
		assert.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("create duplicate ID fails", func(t *testing.T) {
		post := newTestSocialPost("soc-dup", social.PostTypePriceMovers, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.CreatePost(ctx, post)
		assert.Error(t, err)
	})
}

func TestSocialRepository_ListPosts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	// Create posts with different statuses and staggered times.
	for i, st := range []social.PostStatus{social.PostStatusDraft, social.PostStatusDraft, social.PostStatusPublished} {
		p := newTestSocialPost(fmt.Sprintf("soc-list-%c", 'a'+i), social.PostTypeNewArrivals, st)
		// Stagger CreatedAt so ordering is deterministic.
		p.CreatedAt = p.CreatedAt.Add(time.Duration(i) * time.Second)
		p.UpdatedAt = p.CreatedAt
		require.NoError(t, repo.CreatePost(ctx, p))
	}

	t.Run("list all posts", func(t *testing.T) {
		posts, err := repo.ListPosts(ctx, nil, 0, 0)
		require.NoError(t, err)
		assert.Len(t, posts, 3)
		// Most recent first.
		assert.Equal(t, "soc-list-c", posts[0].ID)
	})

	t.Run("filter by status", func(t *testing.T) {
		draft := social.PostStatusDraft
		posts, err := repo.ListPosts(ctx, &draft, 0, 0)
		require.NoError(t, err)
		assert.Len(t, posts, 2)
		for _, p := range posts {
			assert.Equal(t, social.PostStatusDraft, p.Status)
		}
	})

	t.Run("limit", func(t *testing.T) {
		posts, err := repo.ListPosts(ctx, nil, 2, 0)
		require.NoError(t, err)
		assert.Len(t, posts, 2)
	})

	t.Run("offset", func(t *testing.T) {
		posts, err := repo.ListPosts(ctx, nil, 1, 2)
		require.NoError(t, err)
		assert.Len(t, posts, 1)
		assert.Equal(t, "soc-list-a", posts[0].ID)
	})

	t.Run("offset without limit", func(t *testing.T) {
		posts, err := repo.ListPosts(ctx, nil, 0, 1)
		require.NoError(t, err)
		assert.Len(t, posts, 2)
	})
}

func TestSocialRepository_DeletePost(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	t.Run("delete existing post", func(t *testing.T) {
		post := newTestSocialPost("soc-del-1", social.PostTypeNewArrivals, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.DeletePost(ctx, "soc-del-1")
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-del-1")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("delete non-existent returns error", func(t *testing.T) {
		err := repo.DeletePost(ctx, "soc-del-missing")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_UpdatePostStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	t.Run("update status", func(t *testing.T) {
		post := newTestSocialPost("soc-status-1", social.PostTypeNewArrivals, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.UpdatePostStatus(ctx, "soc-status-1", social.PostStatusPublished)
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-status-1")
		require.NoError(t, err)
		assert.Equal(t, social.PostStatusPublished, got.Status)
		assert.True(t, got.UpdatedAt.After(post.UpdatedAt) || got.UpdatedAt.Equal(post.UpdatedAt))
	})

	t.Run("update non-existent returns error", func(t *testing.T) {
		err := repo.UpdatePostStatus(ctx, "soc-status-missing", social.PostStatusDraft)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_UpdatePostCaption(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	t.Run("update caption and hashtags", func(t *testing.T) {
		post := newTestSocialPost("soc-caption-1", social.PostTypeNewArrivals, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.UpdatePostCaption(ctx, "soc-caption-1", "New caption!", "#new #hashtags")
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-caption-1")
		require.NoError(t, err)
		assert.Equal(t, "New caption!", got.Caption)
		assert.Equal(t, "#new #hashtags", got.Hashtags)
	})

	t.Run("update non-existent returns error", func(t *testing.T) {
		err := repo.UpdatePostCaption(ctx, "soc-caption-missing", "x", "y")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_SetPublished(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	t.Run("sets status to published with instagram ID", func(t *testing.T) {
		post := newTestSocialPost("soc-pub-1", social.PostTypeNewArrivals, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.SetPublished(ctx, "soc-pub-1", "ig-12345")
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-pub-1")
		require.NoError(t, err)
		assert.Equal(t, social.PostStatusPublished, got.Status)
		assert.Equal(t, "ig-12345", got.InstagramPostID)
	})

	t.Run("non-existent returns error", func(t *testing.T) {
		err := repo.SetPublished(ctx, "soc-pub-missing", "ig-xxx")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_SetPublishing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	transitionTests := []struct {
		name        string
		id          string
		startStatus social.PostStatus
		setFailed   bool // if true, set to failed before calling SetPublishing
	}{
		{"from draft", "soc-pbing-1", social.PostStatusDraft, false},
		{"from failed (clears error)", "soc-pbing-2", social.PostStatusDraft, true},
		{"from approved", "soc-pbing-3", social.PostStatusApproved, false},
	}
	for _, tc := range transitionTests {
		t.Run(tc.name, func(t *testing.T) {
			post := newTestSocialPost(tc.id, social.PostTypeNewArrivals, tc.startStatus)
			require.NoError(t, repo.CreatePost(ctx, post))
			if tc.setFailed {
				require.NoError(t, repo.SetError(ctx, tc.id, "some error"))
			}

			err := repo.SetPublishing(ctx, tc.id)
			require.NoError(t, err)

			got, err := repo.GetPost(ctx, tc.id)
			require.NoError(t, err)
			assert.Equal(t, social.PostStatusPublishing, got.Status)
			assert.Empty(t, got.ErrorMessage)
		})
	}

	t.Run("rejects transition from published", func(t *testing.T) {
		post := newTestSocialPost("soc-pbing-4", social.PostTypeNewArrivals, social.PostStatusPublished)
		require.NoError(t, repo.CreatePost(ctx, post))

		err := repo.SetPublishing(ctx, "soc-pbing-4")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in publishable state")
	})

	t.Run("non-existent returns error", func(t *testing.T) {
		err := repo.SetPublishing(ctx, "soc-pbing-missing")
		assert.ErrorIs(t, err, social.ErrPostNotFound)
	})
}

func TestSocialRepository_SetError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	post := newTestSocialPost("soc-err-1", social.PostTypeNewArrivals, social.PostStatusPublishing)
	require.NoError(t, repo.CreatePost(ctx, post))

	err := repo.SetError(ctx, "soc-err-1", "upload failed: timeout")
	require.NoError(t, err)

	got, err := repo.GetPost(ctx, "soc-err-1")
	require.NoError(t, err)
	assert.Equal(t, social.PostStatusFailed, got.Status)
	assert.Equal(t, "upload failed: timeout", got.ErrorMessage)
}

func TestSocialRepository_AddPostCards(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	// Set up campaign and purchases needed for FK.
	createSocialPurchase(t, db, "soc-camp-cards", "soc-purch-1", "CARD001")
	// Second purchase in same campaign — use direct SQL to avoid duplicate campaign.
	now := time.Now().Truncate(time.Second)
	campRepo := NewCampaignsRepository(db.DB)
	p2 := &campaigns.Purchase{
		ID: "soc-purch-2", CampaignID: "soc-camp-cards", CardName: "Pikachu",
		CertNumber: "CARD002", Grader: "PSA", GradeValue: 10, BuyCostCents: 30000,
		FrontImageURL: "https://example.com/CARD002.jpg", PurchaseDate: "2026-01-16",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p2))

	post := newTestSocialPost("soc-cards-1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	t.Run("add multiple cards transactionally", func(t *testing.T) {
		cards := []social.PostCard{
			{PostID: "soc-cards-1", PurchaseID: "soc-purch-1", SlideOrder: 1},
			{PostID: "soc-cards-1", PurchaseID: "soc-purch-2", SlideOrder: 2},
		}
		err := repo.AddPostCards(ctx, "soc-cards-1", cards)
		require.NoError(t, err)
	})

	t.Run("duplicate card fails", func(t *testing.T) {
		cards := []social.PostCard{
			{PostID: "soc-cards-1", PurchaseID: "soc-purch-1", SlideOrder: 1},
		}
		err := repo.AddPostCards(ctx, "soc-cards-1", cards)
		assert.Error(t, err)
	})
}

func TestSocialRepository_ListPostCards(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	campRepo := NewCampaignsRepository(db.DB)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	createTestCampaign(t, db, "soc-camp-lpc", "List Post Cards")

	// Create purchases with different data.
	purchases := []*campaigns.Purchase{
		{
			ID: "soc-lpc-p1", CampaignID: "soc-camp-lpc", CardName: "Charizard",
			CertNumber: "LPC001", SetName: "Base Set", CardNumber: "4", Grader: "PSA",
			GradeValue: 9, BuyCostCents: 80000, FrontImageURL: "https://example.com/LPC001.jpg",
			ReviewedPriceCents: 95000, CLValueCents: 90000, PurchaseDate: "2026-01-15",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "soc-lpc-p2", CampaignID: "soc-camp-lpc", CardName: "Pikachu",
			CertNumber: "LPC002", SetName: "Jungle", CardNumber: "60", Grader: "PSA",
			GradeValue: 10, BuyCostCents: 30000, FrontImageURL: "https://example.com/LPC002.jpg",
			ReviewedPriceCents: 35000, CLValueCents: 32000, PurchaseDate: "2026-01-16",
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, p := range purchases {
		require.NoError(t, campRepo.CreatePurchase(ctx, p))
	}

	post := newTestSocialPost("soc-lpc-post", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	cards := []social.PostCard{
		{PostID: "soc-lpc-post", PurchaseID: "soc-lpc-p2", SlideOrder: 1},
		{PostID: "soc-lpc-post", PurchaseID: "soc-lpc-p1", SlideOrder: 2},
	}
	require.NoError(t, repo.AddPostCards(ctx, "soc-lpc-post", cards))

	t.Run("returns enriched card details ordered by slide_order", func(t *testing.T) {
		details, err := repo.ListPostCards(ctx, "soc-lpc-post")
		require.NoError(t, err)
		require.Len(t, details, 2)

		// First card by slide_order is Pikachu.
		assert.Equal(t, "soc-lpc-p2", details[0].PurchaseID)
		assert.Equal(t, 1, details[0].SlideOrder)
		assert.Equal(t, "Pikachu", details[0].CardName)
		assert.Equal(t, "Jungle", details[0].SetName)
		assert.Equal(t, 35000, details[0].AskingPriceCents)
		assert.False(t, details[0].Sold)

		// Second card is Charizard.
		assert.Equal(t, "soc-lpc-p1", details[1].PurchaseID)
		assert.Equal(t, 2, details[1].SlideOrder)
		assert.Equal(t, "Charizard", details[1].CardName)
	})

	t.Run("marks sold cards", func(t *testing.T) {
		// Sell Pikachu.
		sale := &campaigns.Sale{
			ID: "soc-lpc-sale-1", PurchaseID: "soc-lpc-p2",
			SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 40000,
			SaleFeeCents: 4940, SaleDate: "2026-02-01", DaysToSell: 16,
			NetProfitCents: 5060, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, campRepo.CreateSale(ctx, sale))

		details, err := repo.ListPostCards(ctx, "soc-lpc-post")
		require.NoError(t, err)
		require.Len(t, details, 2)

		// Pikachu is sold.
		assert.True(t, details[0].Sold)
		// Charizard is not.
		assert.False(t, details[1].Sold)
	})

	t.Run("empty for non-existent post", func(t *testing.T) {
		details, err := repo.ListPostCards(ctx, "soc-lpc-no-post")
		require.NoError(t, err)
		assert.Empty(t, details)
	})
}

func TestSocialRepository_UpdateSlideURLs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	post := newTestSocialPost("soc-slides-1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	t.Run("stores and retrieves JSON array of URLs", func(t *testing.T) {
		urls := []string{"https://cdn.example.com/slide1.png", "https://cdn.example.com/slide2.png"}
		err := repo.UpdateSlideURLs(ctx, "soc-slides-1", urls)
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-slides-1")
		require.NoError(t, err)
		assert.Equal(t, urls, got.SlideURLs)
	})

	t.Run("overwrite with new URLs", func(t *testing.T) {
		urls := []string{"https://cdn.example.com/slide3.png"}
		err := repo.UpdateSlideURLs(ctx, "soc-slides-1", urls)
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-slides-1")
		require.NoError(t, err)
		assert.Equal(t, urls, got.SlideURLs)
	})

	t.Run("non-existent post returns error", func(t *testing.T) {
		err := repo.UpdateSlideURLs(ctx, "soc-slides-missing", []string{"x"})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_UpdateBackgroundURLs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	post := newTestSocialPost("soc-bg-1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	t.Run("stores and retrieves JSON array of URLs", func(t *testing.T) {
		urls := []string{"https://cdn.example.com/bg1.png", "https://cdn.example.com/bg2.png"}
		err := repo.UpdateBackgroundURLs(ctx, "soc-bg-1", urls)
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-bg-1")
		require.NoError(t, err)
		assert.Equal(t, urls, got.BackgroundURLs)
	})

	t.Run("non-existent post returns error", func(t *testing.T) {
		err := repo.UpdateBackgroundURLs(ctx, "soc-bg-missing", []string{"x"})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_UpdateCoverTitle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	post := newTestSocialPost("soc-cover-1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	t.Run("updates cover title", func(t *testing.T) {
		err := repo.UpdateCoverTitle(ctx, "soc-cover-1", "Updated Title")
		require.NoError(t, err)

		got, err := repo.GetPost(ctx, "soc-cover-1")
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", got.CoverTitle)
	})

	t.Run("non-existent returns error", func(t *testing.T) {
		err := repo.UpdateCoverTitle(ctx, "soc-cover-missing", "x")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, social.ErrPostNotFound))
	})
}

func TestSocialRepository_GetRecentPurchaseIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	campRepo := NewCampaignsRepository(db.DB)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	createTestCampaign(t, db, "soc-camp-recent", "Recent Purchases")

	// Purchase with image, unsold.
	p1 := &campaigns.Purchase{
		ID: "soc-recent-p1", CampaignID: "soc-camp-recent", CardName: "Charizard",
		CertNumber: "REC001", GradeValue: 9, BuyCostCents: 80000,
		FrontImageURL: "https://example.com/REC001.jpg", PurchaseDate: "2026-01-15",
		CreatedAt: now, UpdatedAt: now,
	}
	// Purchase without image — should be excluded.
	p2 := &campaigns.Purchase{
		ID: "soc-recent-p2", CampaignID: "soc-camp-recent", CardName: "Pikachu",
		CertNumber: "REC002", GradeValue: 10, BuyCostCents: 30000,
		FrontImageURL: "", PurchaseDate: "2026-01-16",
		CreatedAt: now, UpdatedAt: now,
	}
	// Purchase with image, will be sold — should be excluded.
	p3 := &campaigns.Purchase{
		ID: "soc-recent-p3", CampaignID: "soc-camp-recent", CardName: "Blastoise",
		CertNumber: "REC003", GradeValue: 8, BuyCostCents: 40000,
		FrontImageURL: "https://example.com/REC003.jpg", PurchaseDate: "2026-01-17",
		CreatedAt: now, UpdatedAt: now,
	}
	// Old purchase — before "since" date.
	p4 := &campaigns.Purchase{
		ID: "soc-recent-p4", CampaignID: "soc-camp-recent", CardName: "Venusaur",
		CertNumber: "REC004", GradeValue: 9, BuyCostCents: 35000,
		FrontImageURL: "https://example.com/REC004.jpg", PurchaseDate: "2025-12-01",
		CreatedAt: now.Add(-90 * 24 * time.Hour), UpdatedAt: now.Add(-90 * 24 * time.Hour),
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p1))
	require.NoError(t, campRepo.CreatePurchase(ctx, p2))
	require.NoError(t, campRepo.CreatePurchase(ctx, p3))
	require.NoError(t, campRepo.CreatePurchase(ctx, p4))

	// Sell p3.
	sale := &campaigns.Sale{
		ID: "soc-recent-s1", PurchaseID: "soc-recent-p3",
		SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 50000,
		SaleFeeCents: 6175, SaleDate: "2026-02-01", DaysToSell: 15,
		NetProfitCents: 3825, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreateSale(ctx, sale))

	// "since" = 30 days ago from now.
	since := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	ids, err := repo.GetRecentPurchaseIDs(ctx, since)
	require.NoError(t, err)

	// Should only include p1 (has image, unsold, recent).
	assert.Contains(t, ids, "soc-recent-p1")
	assert.NotContains(t, ids, "soc-recent-p2") // no image
	assert.NotContains(t, ids, "soc-recent-p3") // sold
	assert.NotContains(t, ids, "soc-recent-p4") // too old
}

func TestSocialRepository_GetPurchaseIDsInExistingPosts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	createSocialPurchase(t, db, "soc-camp-exist", "soc-exist-p1", "EXIST001")
	campRepo := NewCampaignsRepository(db.DB)
	now := time.Now().Truncate(time.Second)
	p2 := &campaigns.Purchase{
		ID: "soc-exist-p2", CampaignID: "soc-camp-exist", CardName: "Pikachu",
		CertNumber: "EXIST002", GradeValue: 10, BuyCostCents: 30000,
		FrontImageURL: "https://example.com/EXIST002.jpg", PurchaseDate: "2026-01-16",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p2))

	// Create a draft post with p1.
	draftPost := newTestSocialPost("soc-exist-post1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, draftPost))
	require.NoError(t, repo.AddPostCards(ctx, "soc-exist-post1", []social.PostCard{
		{PostID: "soc-exist-post1", PurchaseID: "soc-exist-p1", SlideOrder: 1},
	}))

	// Create a rejected post with p2 — should be excluded from results.
	rejectedPost := newTestSocialPost("soc-exist-post2", social.PostTypeNewArrivals, social.PostStatusRejected)
	require.NoError(t, repo.CreatePost(ctx, rejectedPost))
	require.NoError(t, repo.AddPostCards(ctx, "soc-exist-post2", []social.PostCard{
		{PostID: "soc-exist-post2", PurchaseID: "soc-exist-p2", SlideOrder: 1},
	}))

	t.Run("returns only non-rejected post purchase IDs", func(t *testing.T) {
		result, err := repo.GetPurchaseIDsInExistingPosts(ctx, []string{"soc-exist-p1", "soc-exist-p2"}, social.PostTypeNewArrivals)
		require.NoError(t, err)
		assert.True(t, result["soc-exist-p1"])
		assert.False(t, result["soc-exist-p2"]) // rejected post excluded
	})

	t.Run("different post type returns empty", func(t *testing.T) {
		result, err := repo.GetPurchaseIDsInExistingPosts(ctx, []string{"soc-exist-p1"}, social.PostTypePriceMovers)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	t.Run("empty input returns empty map", func(t *testing.T) {
		result, err := repo.GetPurchaseIDsInExistingPosts(ctx, []string{}, social.PostTypeNewArrivals)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

func TestSocialRepository_GetAvailableCardsForPosts(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	campRepo := NewCampaignsRepository(db.DB)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	createTestCampaign(t, db, "soc-camp-avail", "Available Cards")

	// Available card: has image, unsold, not in any post.
	p1 := &campaigns.Purchase{
		ID: "soc-avail-p1", CampaignID: "soc-camp-avail", CardName: "Charizard",
		CertNumber: "AVAIL001", SetName: "Base Set", CardNumber: "4", Grader: "PSA",
		GradeValue: 9, BuyCostCents: 80000, FrontImageURL: "https://example.com/AVAIL001.jpg",
		PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now,
	}
	// No image — excluded.
	p2 := &campaigns.Purchase{
		ID: "soc-avail-p2", CampaignID: "soc-camp-avail", CardName: "Pikachu",
		CertNumber: "AVAIL002", GradeValue: 10, BuyCostCents: 30000,
		FrontImageURL: "", PurchaseDate: "2026-01-16",
		CreatedAt: now, UpdatedAt: now,
	}
	// Sold — excluded.
	p3 := &campaigns.Purchase{
		ID: "soc-avail-p3", CampaignID: "soc-camp-avail", CardName: "Blastoise",
		CertNumber: "AVAIL003", GradeValue: 8, BuyCostCents: 40000,
		FrontImageURL: "https://example.com/AVAIL003.jpg", PurchaseDate: "2026-01-17",
		CreatedAt: now, UpdatedAt: now,
	}
	// In a draft post — excluded (not rejected/failed).
	p4 := &campaigns.Purchase{
		ID: "soc-avail-p4", CampaignID: "soc-camp-avail", CardName: "Venusaur",
		CertNumber: "AVAIL004", Grader: "PSA", GradeValue: 9, BuyCostCents: 35000,
		FrontImageURL: "https://example.com/AVAIL004.jpg", PurchaseDate: "2026-01-18",
		CreatedAt: now, UpdatedAt: now,
	}
	// In a rejected post — should be available.
	p5 := &campaigns.Purchase{
		ID: "soc-avail-p5", CampaignID: "soc-camp-avail", CardName: "Mewtwo",
		CertNumber: "AVAIL005", Grader: "PSA", GradeValue: 10, BuyCostCents: 100000,
		FrontImageURL: "https://example.com/AVAIL005.jpg", PurchaseDate: "2026-01-19",
		CreatedAt: now, UpdatedAt: now,
	}
	for _, p := range []*campaigns.Purchase{p1, p2, p3, p4, p5} {
		require.NoError(t, campRepo.CreatePurchase(ctx, p))
	}

	// Sell p3.
	sale := &campaigns.Sale{
		ID: "soc-avail-s1", PurchaseID: "soc-avail-p3",
		SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 50000,
		SaleFeeCents: 6175, SaleDate: "2026-02-01", DaysToSell: 15,
		NetProfitCents: 3825, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreateSale(ctx, sale))

	// Add p4 to a draft post.
	draftPost := newTestSocialPost("soc-avail-draft", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, draftPost))
	require.NoError(t, repo.AddPostCards(ctx, "soc-avail-draft", []social.PostCard{
		{PostID: "soc-avail-draft", PurchaseID: "soc-avail-p4", SlideOrder: 1},
	}))

	// Add p5 to a rejected post (should still be available).
	rejPost := newTestSocialPost("soc-avail-rej", social.PostTypeNewArrivals, social.PostStatusRejected)
	require.NoError(t, repo.CreatePost(ctx, rejPost))
	require.NoError(t, repo.AddPostCards(ctx, "soc-avail-rej", []social.PostCard{
		{PostID: "soc-avail-rej", PurchaseID: "soc-avail-p5", SlideOrder: 1},
	}))

	cards, err := repo.GetAvailableCardsForPosts(ctx)
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, c := range cards {
		ids[c.PurchaseID] = true
	}
	assert.True(t, ids["soc-avail-p1"], "p1 should be available (unsold, has image, not in post)")
	assert.False(t, ids["soc-avail-p2"], "p2 should be excluded (no image)")
	assert.False(t, ids["soc-avail-p3"], "p3 should be excluded (sold)")
	assert.False(t, ids["soc-avail-p4"], "p4 should be excluded (in draft post)")
	assert.True(t, ids["soc-avail-p5"], "p5 should be available (only in rejected post)")
}

func TestSocialRepository_GetUnsoldPurchasesWithSnapshots(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	campRepo := NewCampaignsRepository(db.DB)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	createTestCampaign(t, db, "soc-camp-snap", "Snapshot Test")

	// Purchase with snapshot data and image.
	p1 := &campaigns.Purchase{
		ID: "soc-snap-p1", CampaignID: "soc-camp-snap", CardName: "Charizard",
		CertNumber: "SNAP001", GradeValue: 9, BuyCostCents: 80000,
		FrontImageURL: "https://example.com/SNAP001.jpg", PurchaseDate: "2026-01-15",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p1))

	// Update snapshot fields via SQL since Purchase struct doesn't set them on create.
	_, err := db.Exec(
		`UPDATE campaign_purchases SET median_cents = 90000, trend_30d = 0.15, snapshot_date = '2026-03-01' WHERE id = ?`,
		"soc-snap-p1",
	)
	require.NoError(t, err)

	// Purchase without snapshot data — excluded (median_cents=0 and mm_trend_pct=0).
	p2 := &campaigns.Purchase{
		ID: "soc-snap-p2", CampaignID: "soc-camp-snap", CardName: "Pikachu",
		CertNumber: "SNAP002", GradeValue: 10, BuyCostCents: 30000,
		FrontImageURL: "https://example.com/SNAP002.jpg", PurchaseDate: "2026-01-16",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p2))

	// Sold purchase with snapshots — excluded.
	p3 := &campaigns.Purchase{
		ID: "soc-snap-p3", CampaignID: "soc-camp-snap", CardName: "Blastoise",
		CertNumber: "SNAP003", GradeValue: 8, BuyCostCents: 40000,
		FrontImageURL: "https://example.com/SNAP003.jpg", PurchaseDate: "2026-01-17",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p3))
	_, err = db.Exec(
		`UPDATE campaign_purchases SET median_cents = 50000, trend_30d = -0.05, snapshot_date = '2026-03-01' WHERE id = ?`,
		"soc-snap-p3",
	)
	require.NoError(t, err)
	sale := &campaigns.Sale{
		ID: "soc-snap-s1", PurchaseID: "soc-snap-p3",
		SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 60000,
		SaleFeeCents: 7410, SaleDate: "2026-02-01", DaysToSell: 15,
		NetProfitCents: 12590, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreateSale(ctx, sale))

	// No image — excluded.
	p4 := &campaigns.Purchase{
		ID: "soc-snap-p4", CampaignID: "soc-camp-snap", CardName: "Venusaur",
		CertNumber: "SNAP004", GradeValue: 9, BuyCostCents: 35000,
		FrontImageURL: "", PurchaseDate: "2026-01-18",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, campRepo.CreatePurchase(ctx, p4))
	_, err = db.Exec(
		`UPDATE campaign_purchases SET median_cents = 40000 WHERE id = ?`, "soc-snap-p4",
	)
	require.NoError(t, err)

	snapshots, err := repo.GetUnsoldPurchasesWithSnapshots(ctx)
	require.NoError(t, err)

	ids := make(map[string]social.PurchaseSnapshot)
	for _, s := range snapshots {
		ids[s.PurchaseID] = s
	}

	require.Contains(t, ids, "soc-snap-p1")
	assert.Equal(t, 80000, ids["soc-snap-p1"].BuyCostCents)
	assert.Equal(t, 90000, ids["soc-snap-p1"].MedianCents)
	assert.InDelta(t, 0.15, ids["soc-snap-p1"].Trend30d, 0.001)
	assert.Equal(t, "2026-03-01", ids["soc-snap-p1"].SnapshotDate)

	assert.NotContains(t, ids, "soc-snap-p2") // no snapshot data
	assert.NotContains(t, ids, "soc-snap-p3") // sold
	assert.NotContains(t, ids, "soc-snap-p4") // no image
}

func TestSocialRepository_SlideURLsNilWhenEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	post := newTestSocialPost("soc-nilslide", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))

	got, err := repo.GetPost(ctx, "soc-nilslide")
	require.NoError(t, err)
	assert.Nil(t, got.SlideURLs, "slide URLs should be nil when not set")
	assert.Nil(t, got.BackgroundURLs, "background URLs should be nil when not set")
}

func TestSocialRepository_AllPostTypes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	types := []social.PostType{social.PostTypeNewArrivals, social.PostTypePriceMovers, social.PostTypeHotDeals}
	for i, pt := range types {
		post := newTestSocialPost(fmt.Sprintf("soc-type-%c", 'a'+i), pt, social.PostStatusDraft)
		require.NoError(t, repo.CreatePost(ctx, post))

		got, err := repo.GetPost(ctx, post.ID)
		require.NoError(t, err)
		assert.Equal(t, pt, got.PostType)
	}
}

func TestSocialRepository_CascadeDeletePostCards(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewSocialRepository(db.DB)
	ctx := context.Background()

	createSocialPurchase(t, db, "soc-camp-cascade", "soc-cascade-p1", "CASCADE001")

	post := newTestSocialPost("soc-cascade-1", social.PostTypeNewArrivals, social.PostStatusDraft)
	require.NoError(t, repo.CreatePost(ctx, post))
	require.NoError(t, repo.AddPostCards(ctx, "soc-cascade-1", []social.PostCard{
		{PostID: "soc-cascade-1", PurchaseID: "soc-cascade-p1", SlideOrder: 1},
	}))

	// Deleting the post should cascade-delete the cards.
	require.NoError(t, repo.DeletePost(ctx, "soc-cascade-1"))

	cards, err := repo.ListPostCards(ctx, "soc-cascade-1")
	require.NoError(t, err)
	assert.Empty(t, cards)
}
