package pricecharting

import (
	"testing"
)

func TestUPCDatabase(t *testing.T) {
	db := NewUPCDatabase()

	t.Run("Add", func(t *testing.T) {
		mapping := &UPCMapping{
			UPC:         "820650558726",
			ProductID:   "surging-sparks-250",
			ProductName: "Pikachu ex",
			SetName:     "Surging Sparks",
			CardNumber:  "250",
			Language:    "English",
			Confidence:  1.0,
		}

		db.Add(mapping)

		// Verify via FindByCardInfo
		results := db.FindByCardInfo("Surging Sparks", "250")
		if len(results) != 1 {
			t.Errorf("Expected 1 mapping, got %d", len(results))
		}
		if results[0].ProductID != "surging-sparks-250" {
			t.Errorf("Expected product ID surging-sparks-250, got %s", results[0].ProductID)
		}
	})

	t.Run("AddBatch", func(t *testing.T) {
		mappings := []*UPCMapping{
			{
				UPC:         "820650558733",
				ProductID:   "surging-sparks-251",
				ProductName: "Alolan Exeggutor ex",
				SetName:     "Surging Sparks",
				CardNumber:  "251",
			},
			{
				UPC:         "820650559876",
				ProductID:   "prismatic-evolutions-001",
				ProductName: "Eevee",
				SetName:     "Prismatic Evolutions",
				CardNumber:  "001",
			},
		}

		db.AddBatch(mappings)

		// Verify all were added via FindByCardInfo
		results := db.FindByCardInfo("Surging Sparks", "251")
		if len(results) != 1 {
			t.Error("Expected to find first batch mapping")
		}
		results = db.FindByCardInfo("Prismatic Evolutions", "001")
		if len(results) != 1 {
			t.Error("Expected to find second batch mapping")
		}
	})

	t.Run("FindByCardInfo", func(t *testing.T) {
		db.Add(&UPCMapping{
			UPC:         "4521329385426",
			ProductID:   "vmax-climax-003",
			ProductName: "Charizard VMAX",
			SetName:     "VMAX Climax",
			CardNumber:  "003",
			Language:    "Japanese",
		})

		results := db.FindByCardInfo("VMAX Climax", "003")
		if len(results) != 1 {
			t.Errorf("Expected 1 mapping for card info, got %d", len(results))
		}

		// Test case insensitive search
		results = db.FindByCardInfo("vmax climax", "003")
		if len(results) != 1 {
			t.Errorf("Expected case-insensitive search to work, got %d results", len(results))
		}
	})

	t.Run("PopulateCommonMappings", func(t *testing.T) {
		freshDB := NewUPCDatabase()
		freshDB.PopulateCommonMappings()

		// Verify common mappings were loaded by checking for known card
		results := freshDB.FindByCardInfo("Surging Sparks", "250")
		if len(results) == 0 {
			t.Error("Expected to find Pikachu ex in common mappings")
		}
	})
}

func TestUPCDatabase_ConcurrentAccess(t *testing.T) {
	db := NewUPCDatabase()

	// Test concurrent Add and FindByCardInfo operations
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			db.Add(&UPCMapping{
				UPC:        "concurrent-upc-1",
				ProductID:  "concurrent-product-1",
				SetName:    "Concurrent Set",
				CardNumber: "001",
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = db.FindByCardInfo("Concurrent Set", "001")
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify final state
	results := db.FindByCardInfo("Concurrent Set", "001")
	if len(results) != 1 {
		t.Errorf("Expected 1 mapping after concurrent operations, got %d", len(results))
	}
	if results[0].ProductID != "concurrent-product-1" {
		t.Errorf("Expected product ID concurrent-product-1, got %s", results[0].ProductID)
	}
}
