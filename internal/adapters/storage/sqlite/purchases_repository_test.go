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

func TestUpdatePurchaseDHFields(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-dh", "DH Fields Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-dh", "DH000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		update := campaigns.DHFieldsUpdate{
			CardID:            12345,
			InventoryID:       67890,
			CertStatus:        "matched",
			ListingPriceCents: 95000,
			ChannelsJSON:      `["ebay","tcgplayer"]`,
			DHStatus:          campaigns.DHStatusListed,
		}
		err := repo.UpdatePurchaseDHFields(ctx, p.ID, update)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 12345, got.DHCardID)
		assert.Equal(t, 67890, got.DHInventoryID)
		assert.Equal(t, "matched", got.DHCertStatus)
		assert.Equal(t, 95000, got.DHListingPriceCents)
		assert.Equal(t, `["ebay","tcgplayer"]`, got.DHChannelsJSON)
		assert.Equal(t, campaigns.DHStatusListed, got.DHStatus)
	})

	t.Run("overwrite existing fields", func(t *testing.T) {
		p := newTestPurchase("camp-dh", "DH000002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		// First update
		err := repo.UpdatePurchaseDHFields(ctx, p.ID, campaigns.DHFieldsUpdate{
			CardID: 111, InventoryID: 222, CertStatus: "pending",
			ListingPriceCents: 50000, DHStatus: campaigns.DHStatusInStock,
		})
		require.NoError(t, err)

		// Second update overwrites
		err = repo.UpdatePurchaseDHFields(ctx, p.ID, campaigns.DHFieldsUpdate{
			CardID: 333, InventoryID: 444, CertStatus: "matched",
			ListingPriceCents: 75000, ChannelsJSON: `["ebay"]`, DHStatus: campaigns.DHStatusListed,
		})
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 333, got.DHCardID)
		assert.Equal(t, 444, got.DHInventoryID)
		assert.Equal(t, "matched", got.DHCertStatus)
		assert.Equal(t, 75000, got.DHListingPriceCents)
		assert.Equal(t, `["ebay"]`, got.DHChannelsJSON)
		assert.Equal(t, campaigns.DHStatusListed, got.DHStatus)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseDHFields(ctx, "nonexistent", campaigns.DHFieldsUpdate{CardID: 1})
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestGetPurchasesByDHCertStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-dhcert", "DH Cert Status Test")

	// Create purchases with different cert statuses
	certStatuses := []struct {
		cert, status string
	}{
		{"DC000001", "pending"},
		{"DC000002", "pending"},
		{"DC000003", "matched"},
		{"DC000004", "ambiguous"},
		{"DC000005", "pending"},
	}
	for _, cs := range certStatuses {
		p := newTestPurchase("camp-dhcert", cs.cert)
		require.NoError(t, repo.CreatePurchase(ctx, p))
		err := repo.UpdatePurchaseDHFields(ctx, p.ID, campaigns.DHFieldsUpdate{CertStatus: cs.status})
		require.NoError(t, err)
	}

	tests := []struct {
		name          string
		status        string
		limit         int
		expectedCount int
		checkStatus   bool
	}{
		{"pending returns 3", "pending", 100, 3, false},
		{"matched returns 1", "matched", 100, 1, true},
		{"ambiguous returns 1", "ambiguous", 100, 1, false},
		{"respects limit", "pending", 2, 2, false},
		{"no matches returns empty", "nonexistent_status", 100, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.GetPurchasesByDHCertStatus(ctx, tt.status, tt.limit)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			if tt.checkStatus && len(results) > 0 {
				assert.Equal(t, tt.status, results[0].DHCertStatus)
			}
		})
	}
}

func TestUpdatePurchaseCardYear(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-yr", "Card Year Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-yr", "YR000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseCardYear(ctx, p.ID, "1999")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "1999", got.CardYear)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseCardYear(ctx, "nonexistent", "2000")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchaseDHPushStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-push", "DH Push Status Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-push", "PS000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseDHPushStatus(ctx, p.ID, campaigns.DHPushStatusPending)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, campaigns.DHPushStatusPending, got.DHPushStatus)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseDHPushStatus(ctx, "nonexistent", campaigns.DHPushStatusPending)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchaseDHCandidates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-cand", "DH Candidates Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-cand", "CD000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		candidates := `[{"id":1,"name":"Charizard"},{"id":2,"name":"Charizard Holo"}]`
		err := repo.UpdatePurchaseDHCandidates(ctx, p.ID, candidates)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, candidates, got.DHCandidatesJSON)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdatePurchaseDHCandidates(ctx, "nonexistent", "[]")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestGetPurchasesByDHPushStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-pushq", "DH Push Query Test")

	// Create purchases with different push statuses
	certs := []string{"PQ000001", "PQ000002", "PQ000003"}
	for _, cert := range certs {
		p := newTestPurchase("camp-pushq", cert)
		require.NoError(t, repo.CreatePurchase(ctx, p))
	}
	require.NoError(t, repo.UpdatePurchaseDHPushStatus(ctx, "purch-PQ000001", campaigns.DHPushStatusPending))
	require.NoError(t, repo.UpdatePurchaseDHPushStatus(ctx, "purch-PQ000002", campaigns.DHPushStatusPending))
	require.NoError(t, repo.UpdatePurchaseDHPushStatus(ctx, "purch-PQ000003", campaigns.DHPushStatusMatched))

	t.Run("filters by push status", func(t *testing.T) {
		pending, err := repo.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusPending, 100)
		require.NoError(t, err)
		assert.Len(t, pending, 2)

		matched, err := repo.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusMatched, 100)
		require.NoError(t, err)
		assert.Len(t, matched, 1)
	})

	t.Run("respects limit", func(t *testing.T) {
		pending, err := repo.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusPending, 1)
		require.NoError(t, err)
		assert.Len(t, pending, 1)
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		result, err := repo.GetPurchasesByDHPushStatus(ctx, campaigns.DHPushStatusManual, 100)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestListUnsoldPurchases_EmptyCampaign(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	// Unknown campaign ID should return empty, not an error
	unsold, err := repo.ListUnsoldPurchases(ctx, "camp-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, unsold)
}

func TestListAllUnsoldPurchases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	// Create one active campaign and one closed campaign
	createTestCampaign(t, db, "camp-lau-active", "Active Campaign")
	_, err := db.Exec(
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"camp-lau-closed", "Closed Campaign", "closed",
	)
	require.NoError(t, err)

	// Create purchases in both campaigns
	activeP1 := newTestPurchase("camp-lau-active", "LAU00001")
	activeP2 := newTestPurchase("camp-lau-active", "LAU00002")
	closedP1 := newTestPurchase("camp-lau-closed", "LAU00003")
	require.NoError(t, repo.CreatePurchase(ctx, activeP1))
	require.NoError(t, repo.CreatePurchase(ctx, activeP2))
	require.NoError(t, repo.CreatePurchase(ctx, closedP1))

	t.Run("excludes closed campaign purchases", func(t *testing.T) {
		unsold, err := repo.ListAllUnsoldPurchases(ctx)
		require.NoError(t, err)
		// Only active campaign purchases should appear
		assert.Len(t, unsold, 2)
		for _, p := range unsold {
			assert.Equal(t, "camp-lau-active", p.CampaignID)
		}
	})

	t.Run("sold purchase excluded from results", func(t *testing.T) {
		s := newTestSale(activeP1.ID)
		require.NoError(t, repo.CreateSale(ctx, s))

		unsold, err := repo.ListAllUnsoldPurchases(ctx)
		require.NoError(t, err)
		assert.Len(t, unsold, 1)
		assert.Equal(t, activeP2.ID, unsold[0].ID)
	})
}

func TestListUnsoldCards(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-luc", "Unsold Cards Test")

	// Create purchases with distinct card names and one sold card
	p1 := newTestPurchase("camp-luc", "LUC00001")
	p1.CardName = "Charizard"
	p1.SetName = "Base Set"
	p1.CardNumber = "4/102"

	p2 := newTestPurchase("camp-luc", "LUC00002")
	p2.CardName = "Blastoise"
	p2.SetName = "Base Set"
	p2.CardNumber = "2/102"

	p3 := newTestPurchase("camp-luc", "LUC00003")
	p3.CardName = "Pikachu"
	p3.SetName = "Base Set"
	p3.CardNumber = "58/102"

	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))
	require.NoError(t, repo.CreatePurchase(ctx, p3))

	t.Run("returns unsold cards", func(t *testing.T) {
		cards, err := repo.ListUnsoldCards(ctx)
		require.NoError(t, err)
		assert.Len(t, cards, 3)
	})

	t.Run("sold card excluded", func(t *testing.T) {
		s := newTestSale(p1.ID)
		require.NoError(t, repo.CreateSale(ctx, s))

		cards, err := repo.ListUnsoldCards(ctx)
		require.NoError(t, err)
		assert.Len(t, cards, 2)
		for _, c := range cards {
			// Charizard was sold, should not appear
			assert.NotContains(t, c.CardName, "charizard")
		}
	})

	t.Run("deduplication of same card name and set", func(t *testing.T) {
		createTestCampaign(t, db, "camp-luc-dedup", "Dedup Test")

		// Two purchases with the exact same card_name, set_name, card_number
		// SQL DISTINCT will collapse these into one entry
		pd1 := newTestPurchase("camp-luc-dedup", "LUC00010")
		pd1.CardName = "Dark Gyarados"
		pd1.SetName = "Team Rocket"
		pd1.CardNumber = "8/82"

		pd2 := newTestPurchase("camp-luc-dedup", "LUC00011")
		pd2.CardName = "Dark Gyarados"
		pd2.SetName = "Team Rocket"
		pd2.CardNumber = "8/82"

		require.NoError(t, repo.CreatePurchase(ctx, pd1))
		require.NoError(t, repo.CreatePurchase(ctx, pd2))

		cards, err := repo.ListUnsoldCards(ctx)
		require.NoError(t, err)
		// The two identical entries should collapse to one via DISTINCT + normalization
		darkCount := 0
		for _, c := range cards {
			if c.CardNumber == "8/82" {
				darkCount++
			}
		}
		assert.Equal(t, 1, darkCount, "identical card+set+number should appear once")
	})
}

func TestCountUnsoldByDHPushStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-cub", "Count By Push Status")

	// Create purchases with different DH push states
	// 1. push_status='pending'
	pPending := newTestPurchase("camp-cub", "CUB00001")
	require.NoError(t, repo.CreatePurchase(ctx, pPending))
	require.NoError(t, repo.UpdatePurchaseDHPushStatus(ctx, pPending.ID, campaigns.DHPushStatusPending))

	// 2. dh_card_id set but no push status → should count as 'matched'
	pMatched := newTestPurchase("camp-cub", "CUB00002")
	require.NoError(t, repo.CreatePurchase(ctx, pMatched))
	require.NoError(t, repo.UpdatePurchaseDHFields(ctx, pMatched.ID, campaigns.DHFieldsUpdate{
		CardID: 42, CertStatus: "matched",
	}))

	// 3. Neither push_status nor dh_card_id → empty string bucket
	pUnknown := newTestPurchase("camp-cub", "CUB00003")
	require.NoError(t, repo.CreatePurchase(ctx, pUnknown))

	t.Run("groups by push status bucket", func(t *testing.T) {
		counts, err := repo.CountUnsoldByDHPushStatus(ctx)
		require.NoError(t, err)

		assert.Equal(t, 1, counts[campaigns.DHPushStatusPending], "one pending")
		assert.Equal(t, 1, counts["matched"], "one matched via dh_card_id")
		assert.Equal(t, 1, counts[""], "one unknown/empty")
	})

	t.Run("sold purchase excluded from counts", func(t *testing.T) {
		s := newTestSale(pPending.ID)
		require.NoError(t, repo.CreateSale(ctx, s))

		counts, err := repo.CountUnsoldByDHPushStatus(ctx)
		require.NoError(t, err)

		// pending is now sold, should not appear
		assert.Equal(t, 0, counts[campaigns.DHPushStatusPending], "sold pending should not count")
		assert.Equal(t, 1, counts["matched"])
		assert.Equal(t, 1, counts[""])
	})
}

func TestUpdatePurchaseGemRateID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-gr", "GemRateID Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-gr", "GR000001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchaseGemRateID(ctx, p.ID, "gem-abc-123")
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "gem-abc-123", got.GemRateID)
	})

	t.Run("overwrite existing gem_rate_id", func(t *testing.T) {
		p := newTestPurchase("camp-gr", "GR000002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		require.NoError(t, repo.UpdatePurchaseGemRateID(ctx, p.ID, "first-gem-id"))
		require.NoError(t, repo.UpdatePurchaseGemRateID(ctx, p.ID, "second-gem-id"))

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "second-gem-id", got.GemRateID)
	})

	t.Run("not found returns ErrPurchaseNotFound", func(t *testing.T) {
		err := repo.UpdatePurchaseGemRateID(ctx, "nonexistent", "gem-xyz")
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}

func TestUpdatePurchasePSASpecID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-ps", "PSASpecID Test")

	t.Run("success", func(t *testing.T) {
		p := newTestPurchase("camp-ps", "PS100001")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		err := repo.UpdatePurchasePSASpecID(ctx, p.ID, 111222)
		require.NoError(t, err)

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 111222, got.PSASpecID)
	})

	t.Run("overwrite existing psa_spec_id", func(t *testing.T) {
		p := newTestPurchase("camp-ps", "PS100002")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		require.NoError(t, repo.UpdatePurchasePSASpecID(ctx, p.ID, 111111))
		require.NoError(t, repo.UpdatePurchasePSASpecID(ctx, p.ID, 222222))

		got, err := repo.GetPurchase(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, 222222, got.PSASpecID)
	})

	t.Run("not found returns ErrPurchaseNotFound", func(t *testing.T) {
		err := repo.UpdatePurchasePSASpecID(ctx, "nonexistent", 999)
		assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
	})
}
