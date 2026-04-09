package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// GetPurchaseByCertNumber
// ---------------------------------------------------------------------------

func TestPurchasesQueries_GetPurchaseByCertNumber(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-1", "PQ Campaign")

	t.Run("found", func(t *testing.T) {
		p := newTestPurchase("pq-camp-1", "PQ100001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		got, err := repo.GetPurchaseByCertNumber(ctx, "PSA", "PQ100001")
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, "PQ100001", got.CertNumber)
		assert.Equal(t, "PSA", got.Grader)
	})

	t.Run("not found returns ErrPurchaseNotFound", func(t *testing.T) {
		_, err := repo.GetPurchaseByCertNumber(ctx, "PSA", "NONEXISTENT")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})

	t.Run("wrong grader returns not found", func(t *testing.T) {
		p := newTestPurchase("pq-camp-1", "PQ100002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		_, err := repo.GetPurchaseByCertNumber(ctx, "CGC", "PQ100002")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------------------------------------------------------------------------
// GetPurchasesByGraderAndCertNumbers
// ---------------------------------------------------------------------------

func TestPurchasesQueries_GetPurchasesByGraderAndCertNumbers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-2", "PQ Campaign 2")

	t.Run("empty input returns empty map", func(t *testing.T) {
		result, err := repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("single cert", func(t *testing.T) {
		p := newTestPurchase("pq-camp-2", "PQ200001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		result, err := repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", []string{"PQ200001"})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, p.ID, result["PQ200001"].ID)
	})

	t.Run("multiple certs with some missing", func(t *testing.T) {
		p := newTestPurchase("pq-camp-2", "PQ200002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		result, err := repo.GetPurchasesByGraderAndCertNumbers(ctx, "PSA", []string{"PQ200002", "MISSING1"})
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["PQ200002"])
	})

	t.Run("wrong grader returns empty", func(t *testing.T) {
		p := newTestPurchase("pq-camp-2", "PQ200003")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		result, err := repo.GetPurchasesByGraderAndCertNumbers(ctx, "BGS", []string{"PQ200003"})
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

// ---------------------------------------------------------------------------
// GetPurchasesByCertNumbers (cross-grader, chunked)
// ---------------------------------------------------------------------------

func TestPurchasesQueries_GetPurchasesByCertNumbers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-3", "PQ Campaign 3")

	t.Run("empty input returns empty map", func(t *testing.T) {
		result, err := repo.GetPurchasesByCertNumbers(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns purchases across graders", func(t *testing.T) {
		p1 := newTestPurchase("pq-camp-3", "PQ300001")
		p1.Grader = "PSA"
		require.NoError(t, repo.CreatePurchase(ctx, p1))

		p2 := newTestPurchase("pq-camp-3", "PQ300002")
		p2.ID = "purch-PQ300002-cgc"
		p2.Grader = "CGC"
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		result, err := repo.GetPurchasesByCertNumbers(ctx, []string{"PQ300001", "PQ300002"})
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("chunked batch with 501 items", func(t *testing.T) {
		const count = 501
		certs := make([]string, count)
		for i := 0; i < count; i++ {
			cert := fmt.Sprintf("PQ3B%05d", i)
			certs[i] = cert
			p := newTestPurchase("pq-camp-3", cert)
			p.ID = fmt.Sprintf("purch-batch3-%05d", i)
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		result, err := repo.GetPurchasesByCertNumbers(ctx, certs)
		require.NoError(t, err)
		assert.Len(t, result, count)
	})
}

// ---------------------------------------------------------------------------
// GetPurchasesByIDs (chunked)
// ---------------------------------------------------------------------------

func TestPurchasesQueries_GetPurchasesByIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-4", "PQ Campaign 4")

	t.Run("empty input returns empty map", func(t *testing.T) {
		result, err := repo.GetPurchasesByIDs(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns matching purchases", func(t *testing.T) {
		p1 := newTestPurchase("pq-camp-4", "PQ400001")
		p2 := newTestPurchase("pq-camp-4", "PQ400002")
		require.NoError(t, repo.CreatePurchase(ctx, p1))
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		result, err := repo.GetPurchasesByIDs(ctx, []string{p1.ID, p2.ID})
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, p1.CertNumber, result[p1.ID].CertNumber)
		assert.Equal(t, p2.CertNumber, result[p2.ID].CertNumber)
	})

	t.Run("ignores nonexistent IDs", func(t *testing.T) {
		p := newTestPurchase("pq-camp-4", "PQ400003")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		result, err := repo.GetPurchasesByIDs(ctx, []string{p.ID, "nonexistent-id"})
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("chunked batch with 502 items", func(t *testing.T) {
		const count = 502
		ids := make([]string, count)
		for i := 0; i < count; i++ {
			cert := fmt.Sprintf("PQ4B%05d", i)
			id := fmt.Sprintf("purch-batch4-%05d", i)
			ids[i] = id
			p := newTestPurchase("pq-camp-4", cert)
			p.ID = id
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		result, err := repo.GetPurchasesByIDs(ctx, ids)
		require.NoError(t, err)
		assert.Len(t, result, count)
	})
}

// ---------------------------------------------------------------------------
// UpdateExternalPurchaseFields
// ---------------------------------------------------------------------------

func TestPurchasesQueries_UpdateExternalPurchaseFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-5", "PQ Campaign 5")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("pq-camp-5", "PQ500001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		updated := &campaigns.Purchase{
			CardName:      "Blastoise",
			CardNumber:    "2/102",
			SetName:       "Base Set",
			Grader:        "CGC",
			GradeValue:    9.5,
			BuyCostCents:  50000,
			CLValueCents:  120000,
			FrontImageURL: "https://example.com/front.jpg",
			BackImageURL:  "https://example.com/back.jpg",
		}

		err := repo.UpdateExternalPurchaseFields(ctx, p.ID, updated)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Blastoise", got.CardName)
		assert.Equal(t, "2/102", got.CardNumber)
		assert.Equal(t, "Base Set", got.SetName)
		assert.Equal(t, "CGC", got.Grader)
		assert.Equal(t, 9.5, got.GradeValue)
		assert.Equal(t, 50000, got.BuyCostCents)
		assert.Equal(t, 120000, got.CLValueCents)
		assert.Equal(t, "https://example.com/front.jpg", got.FrontImageURL)
		assert.Equal(t, "https://example.com/back.jpg", got.BackImageURL)
	})

	t.Run("not found", func(t *testing.T) {
		updated := &campaigns.Purchase{CardName: "X"}
		err := repo.UpdateExternalPurchaseFields(ctx, "nonexistent", updated)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------------------------------------------------------------------------
// UpdatePurchaseMarketSnapshot
// ---------------------------------------------------------------------------

func TestPurchasesQueries_UpdatePurchaseMarketSnapshot(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-6", "PQ Campaign 6")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("pq-camp-6", "PQ600001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		snap := campaigns.MarketSnapshotData{
			LastSoldCents:     9500,
			LowestListCents:   8800,
			ConservativeCents: 8000,
			MedianCents:       9200,
			ActiveListings:    15,
			SalesLast30d:      7,
			Trend30d:          0.05,
			SnapshotDate:      "2026-03-01",
			SnapshotJSON:      `{"test":"data"}`,
		}

		err := repo.UpdatePurchaseMarketSnapshot(ctx, p.ID, snap)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 9500, got.LastSoldCents)
		assert.Equal(t, 8800, got.LowestListCents)
		assert.Equal(t, 8000, got.ConservativeCents)
		assert.Equal(t, 9200, got.MedianCents)
		assert.Equal(t, 15, got.ActiveListings)
		assert.Equal(t, 7, got.SalesLast30d)
		assert.InDelta(t, 0.05, got.Trend30d, 0.001)
		assert.Equal(t, "2026-03-01", got.SnapshotDate)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseMarketSnapshot(ctx, "nonexistent", campaigns.MarketSnapshotData{})
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------------------------------------------------------------------------
// ListSnapshotPurchasesByStatus + UpdatePurchaseSnapshotStatus
// ---------------------------------------------------------------------------

func TestPurchasesQueries_SnapshotStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-7", "PQ Campaign 7")

	t.Run("list pending snapshots", func(t *testing.T) {
		// Create a purchase then set it to pending via direct SQL (CreatePurchase defaults to "")
		p := newTestPurchase("pq-camp-7", "PQ700001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, campaigns.SnapshotStatusPending, 0)
		require.NoError(t, err)

		list, err := repo.ListSnapshotPurchasesByStatus(ctx, campaigns.SnapshotStatusPending, 10)
		require.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, p.ID, list[0].ID)
		assert.Equal(t, campaigns.SnapshotStatusPending, list[0].SnapshotStatus)
	})

	t.Run("update status to failed with retry count", func(t *testing.T) {
		p := newTestPurchase("pq-camp-7", "PQ700002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, campaigns.SnapshotStatusFailed, 2)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, campaigns.SnapshotStatusFailed, got.SnapshotStatus)
		assert.Equal(t, 2, got.SnapshotRetryCount)
	})

	t.Run("list respects limit", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			cert := fmt.Sprintf("PQ700L%02d", i)
			p := newTestPurchase("pq-camp-7", cert)
			require.NoError(t, repo.CreatePurchase(ctx, p))
			require.NoError(t, repo.UpdatePurchaseSnapshotStatus(ctx, p.ID, campaigns.SnapshotStatusExhausted, 3))
		}

		list, err := repo.ListSnapshotPurchasesByStatus(ctx, campaigns.SnapshotStatusExhausted, 3)
		require.NoError(t, err)
		assert.Len(t, list, 3)
	})

	t.Run("empty when no matches", func(t *testing.T) {
		// SnapshotStatusNone ("") is the default; use a status we haven't set
		list, err := repo.ListSnapshotPurchasesByStatus(ctx, "imaginary", 100)
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("update not found", func(t *testing.T) {
		err := repo.UpdatePurchaseSnapshotStatus(ctx, "nonexistent", campaigns.SnapshotStatusPending, 0)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------------------------------------------------------------------------
// UpdatePurchasePSAFields
// ---------------------------------------------------------------------------

func TestPurchasesQueries_UpdatePurchasePSAFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-8", "PQ Campaign 8")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("pq-camp-8", "PQ800001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		fields := campaigns.PSAUpdateFields{
			VaultStatus:     "in_vault",
			InvoiceDate:     "2026-02-15",
			WasRefunded:     true,
			FrontImageURL:   "https://example.com/psa-front.jpg",
			BackImageURL:    "https://example.com/psa-back.jpg",
			PurchaseSource:  "direct_buy",
			PSAListingTitle: "2023 Pokemon Charizard #4 PSA 9",
		}

		err := repo.UpdatePurchasePSAFields(ctx, p.ID, fields)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "in_vault", got.VaultStatus)
		assert.Equal(t, "2026-02-15", got.InvoiceDate)
		assert.True(t, got.WasRefunded)
		assert.Equal(t, "https://example.com/psa-front.jpg", got.FrontImageURL)
		assert.Equal(t, "https://example.com/psa-back.jpg", got.BackImageURL)
		assert.Equal(t, "direct_buy", got.PurchaseSource)
		assert.Equal(t, "2023 Pokemon Charizard #4 PSA 9", got.PSAListingTitle)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchasePSAFields(ctx, "nonexistent", campaigns.PSAUpdateFields{})
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

// ---------------------------------------------------------------------------
// ListPurchasesMissingImages
// ---------------------------------------------------------------------------

func TestPurchasesQueries_ListPurchasesMissingImages(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-9", "PQ Campaign 9")

	t.Run("returns purchases with empty front_image_url and non-empty cert", func(t *testing.T) {
		// Purchase with no images, has cert number → should appear
		p1 := newTestPurchase("pq-camp-9", "PQ900001")
		require.NoError(t, repo.CreatePurchase(ctx, p1))

		// Purchase with front image → should NOT appear
		p2 := newTestPurchase("pq-camp-9", "PQ900002")
		p2.FrontImageURL = "https://example.com/exists.jpg"
		require.NoError(t, repo.CreatePurchase(ctx, p2))

		// Purchase with no cert number → should NOT appear
		p3 := newTestPurchase("pq-camp-9", "")
		p3.ID = "purch-PQ900003-nocert"
		require.NoError(t, repo.CreatePurchase(ctx, p3))

		list, err := repo.ListPurchasesMissingImages(ctx, 100)
		require.NoError(t, err)

		// Only p1 should match
		ids := make([]string, len(list))
		for i, item := range list {
			ids[i] = item.ID
		}
		assert.Contains(t, ids, p1.ID)
		for _, item := range list {
			if item.ID == p2.ID {
				t.Error("should not include purchase with front_image_url set")
			}
			if item.ID == p3.ID {
				t.Error("should not include purchase with empty cert_number")
			}
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			cert := fmt.Sprintf("PQ900L%02d", i)
			p := newTestPurchase("pq-camp-9", cert)
			p.ID = fmt.Sprintf("purch-limit9-%02d", i)
			require.NoError(t, repo.CreatePurchase(ctx, p))
		}

		list, err := repo.ListPurchasesMissingImages(ctx, 2)
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})
}

// ---------------------------------------------------------------------------
// UpdatePurchaseImageURLs — conditional branches
// ---------------------------------------------------------------------------

func TestPurchasesQueries_UpdatePurchaseImageURLs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "pq-camp-10", "PQ Campaign 10")

	t.Run("both front and back", func(t *testing.T) {
		p := newTestPurchase("pq-camp-10", "PQA00001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseImageURLs(ctx, p.ID, "https://front.jpg", "https://back.jpg")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://front.jpg", got.FrontImageURL)
		assert.Equal(t, "https://back.jpg", got.BackImageURL)
	})

	t.Run("front only", func(t *testing.T) {
		p := newTestPurchase("pq-camp-10", "PQA00002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseImageURLs(ctx, p.ID, "https://front-only.jpg", "")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://front-only.jpg", got.FrontImageURL)
		assert.Empty(t, got.BackImageURL)
	})

	t.Run("back only", func(t *testing.T) {
		p := newTestPurchase("pq-camp-10", "PQA00003")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseImageURLs(ctx, p.ID, "", "https://back-only.jpg")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Empty(t, got.FrontImageURL)
		assert.Equal(t, "https://back-only.jpg", got.BackImageURL)
	})

	t.Run("both empty is no-op", func(t *testing.T) {
		p := newTestPurchase("pq-camp-10", "PQA00004")
		p.FrontImageURL = "https://original.jpg"
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseImageURLs(ctx, p.ID, "", "")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://original.jpg", got.FrontImageURL)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseImageURLs(ctx, "nonexistent", "https://x.jpg", "")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}
