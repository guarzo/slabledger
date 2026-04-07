package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePurchase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-1", "11111111")
		err := repo.CreatePurchase(ctx, p)
		require.NoError(t, err)
	})

	t.Run("default grader", func(t *testing.T) {
		p := newTestPurchase("camp-1", "22222222")
		p.Grader = ""
		err := repo.CreatePurchase(ctx, p)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "PSA", got.Grader)
	})

	t.Run("duplicate cert number", func(t *testing.T) {
		p := newTestPurchase("camp-1", "33333333")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		dup := newTestPurchase("camp-1", "33333333")
		dup.ID = "purch-dup"
		err := repo.CreatePurchase(ctx, dup)
		assert.ErrorIs(t, err, campaigns.ErrDuplicateCertNumber)
		assert.True(t, campaigns.IsDuplicateCertNumber(err))
	})
}

func TestGetPurchase(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("found", func(t *testing.T) {
		p := newTestPurchase("camp-1", "44444444")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.CardName, got.CardName)
		assert.Equal(t, p.CertNumber, got.CertNumber)
		assert.Equal(t, p.GradeValue, got.GradeValue)
		assert.Equal(t, p.BuyCostCents, got.BuyCostCents)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetPurchase(ctx, "nonexistent")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
		assert.True(t, campaigns.IsPurchaseNotFound(err))
	})
}

func TestListPurchasesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Campaign One")
	createTestCampaign(t, db, "camp-2", "Campaign Two")

	for i, cert := range []string{"50000001", "50000002", "50000003"} {
		p := newTestPurchase("camp-1", cert)
		p.PurchaseDate = time.Date(2026, 1, 10+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		require.NoError(t, repo.CreatePurchase(ctx, p))
	}

	p := newTestPurchase("camp-2", "60000001")
	require.NoError(t, repo.CreatePurchase(ctx, p))

	t.Run("filters by campaign", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "camp-1", 100, 0)
		require.NoError(t, err)
		assert.Len(t, list, 3)
		for _, item := range list {
			assert.Equal(t, "camp-1", item.CampaignID)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "camp-1", 2, 0)
		require.NoError(t, err)
		assert.Len(t, list, 2)

		list2, err := repo.ListPurchasesByCampaign(ctx, "camp-1", 2, 2)
		require.NoError(t, err)
		assert.Len(t, list2, 1)
	})

	t.Run("empty campaign", func(t *testing.T) {
		list, err := repo.ListPurchasesByCampaign(ctx, "camp-empty", 100, 0)
		require.NoError(t, err)
		assert.Empty(t, list)
	})
}

func TestCountPurchasesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	count, err := repo.CountPurchasesByCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	for _, cert := range []string{"70000001", "70000002"} {
		require.NoError(t, repo.CreatePurchase(ctx, newTestPurchase("camp-1", cert)))
	}

	count, err = repo.CountPurchasesByCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestUpdatePurchaseBuyCost(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-1", "80000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseBuyCost(ctx, p.ID, 95000)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 95000, got.BuyCostCents)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseBuyCost(ctx, "nonexistent", 95000)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchaseGrade(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-1", "81000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseGrade(ctx, p.ID, 10.0)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 10.0, got.GradeValue)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseGrade(ctx, "nonexistent", 10.0)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchaseCardMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-1", "82000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseCardMetadata(ctx, p.ID, "Blastoise", "2/102", "Base Set")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Blastoise", got.CardName)
		assert.Equal(t, "2/102", got.CardNumber)
		assert.Equal(t, "Base Set", got.SetName)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseCardMetadata(ctx, "nonexistent", "Blastoise", "2/102", "Base Set")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchaseCampaign(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-src", "Source")
	createTestCampaign(t, db, "camp-dst", "Destination")

	t.Run("move between campaigns", func(t *testing.T) {
		p := newTestPurchase("camp-src", "83000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseCampaign(ctx, p.ID, "camp-dst", 500)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "camp-dst", got.CampaignID)
		assert.Equal(t, 500, got.PSASourcingFeeCents)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseCampaign(ctx, "nonexistent", "camp-dst", 500)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestGetPurchaseIDByCertNumber(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-1", "Test Campaign")

	t.Run("found", func(t *testing.T) {
		p := newTestPurchase("camp-1", "84000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		id, err := repo.GetPurchaseIDByCertNumber(ctx, "84000001")
		require.NoError(t, err)
		assert.Equal(t, p.ID, id)
	})

	t.Run("not found returns empty string", func(t *testing.T) {
		id, err := repo.GetPurchaseIDByCertNumber(ctx, "99999999")
		require.NoError(t, err)
		assert.Empty(t, id)
	})
}

func TestUpdatePurchaseCLValue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-cl", "CL Value Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-cl", "CL000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseCLValue(ctx, p.ID, 125000, 42)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 125000, got.CLValueCents)
		assert.Equal(t, 42, got.Population)
	})

	t.Run("update to zero", func(t *testing.T) {
		p := newTestPurchase("camp-cl", "CL000002")
		p.CLValueCents = 100000
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseCLValue(ctx, p.ID, 0, 0)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, got.CLValueCents)
		assert.Equal(t, 0, got.Population)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseCLValue(ctx, "nonexistent", 100000, 10)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

