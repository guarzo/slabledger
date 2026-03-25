package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func BenchmarkPriceRepository_StorePrice(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &pricing.PriceEntry{
			CardName:   fmt.Sprintf("Card %d", i),
			SetName:    "Test Set",
			Grade:      "PSA 10",
			PriceCents: int64(10000),
			Source:     "pricecharting",
			PriceDate:  time.Now(),
		}
		if err := repo.StorePrice(ctx, entry); err != nil {
			b.Fatalf("StorePrice failed at iteration %d: %v", i, err)
		}
	}
}

func BenchmarkPriceRepository_GetLatestPrice(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Pre-populate
	entry := &pricing.PriceEntry{
		CardName:   "Benchmark Card",
		SetName:    "Test Set",
		Grade:      "PSA 10",
		PriceCents: 10000,
		Source:     "pricecharting",
		PriceDate:  time.Now(),
	}
	if err := repo.StorePrice(ctx, entry); err != nil {
		b.Fatalf("setup StorePrice failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.GetLatestPrice(ctx, pricing.Card{
			Name: "Benchmark Card",
			Set:  "Test Set",
		}, "PSA 10", "pricecharting"); err != nil {
			b.Fatalf("GetLatestPrice failed at iteration %d: %v", i, err)
		}
	}
}

func BenchmarkPriceRepository_GetStalePrices(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Pre-populate 100 prices
	for i := 0; i < 100; i++ {
		entry := &pricing.PriceEntry{
			CardName:   fmt.Sprintf("Card %d", i),
			SetName:    "Test Set",
			Grade:      "PSA 10",
			PriceCents: int64(10000 + i*100),
			Source:     "pricecharting",
			PriceDate:  time.Now(),
		}
		if err := repo.StorePrice(ctx, entry); err != nil {
			b.Fatalf("setup StorePrice failed at index %d: %v", i, err)
		}
	}

	// Make them stale
	if _, err := db.Exec(`UPDATE price_history SET updated_at = DATETIME('now', '-48 hours')`); err != nil {
		b.Fatalf("setup UPDATE failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.GetStalePrices(ctx, "", 50); err != nil {
			b.Fatalf("GetStalePrices failed at iteration %d: %v", i, err)
		}
	}
}

func BenchmarkPriceRepository_RecordAPICall(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		call := &pricing.APICallRecord{
			Provider:   "pricecharting",
			Endpoint:   "/api/prices",
			StatusCode: 200,
			LatencyMS:  150,
			Timestamp:  time.Now(),
		}
		if err := repo.RecordAPICall(ctx, call); err != nil {
			b.Fatalf("RecordAPICall failed at iteration %d: %v", i, err)
		}
	}
}

func BenchmarkPriceRepository_GetAPIUsage(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Pre-populate API calls
	for i := 0; i < 100; i++ {
		call := &pricing.APICallRecord{
			Provider:   "pricecharting",
			Endpoint:   "/api/prices",
			StatusCode: 200,
			LatencyMS:  150,
			Timestamp:  time.Now(),
		}
		if err := repo.RecordAPICall(ctx, call); err != nil {
			b.Fatalf("setup RecordAPICall failed at index %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.GetAPIUsage(ctx, "pricecharting"); err != nil {
			b.Fatalf("GetAPIUsage failed at iteration %d: %v", i, err)
		}
	}
}

func BenchmarkPriceRepository_IsProviderBlocked(b *testing.B) {
	db := setupBenchDB(b)
	defer db.Close()

	repo := NewPriceRepository(db)
	ctx := context.Background()

	// Pre-set rate limit
	if err := repo.UpdateRateLimit(ctx, "pricecharting", time.Now().Add(1*time.Hour)); err != nil {
		b.Fatalf("setup UpdateRateLimit failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := repo.IsProviderBlocked(ctx, "pricecharting"); err != nil {
			b.Fatalf("IsProviderBlocked failed at iteration %d: %v", i, err)
		}
	}
}

func setupBenchDB(b *testing.B) *DB {
	logger := mocks.NewMockLogger()

	db, err := Open(":memory:", logger)
	if err != nil {
		b.Fatal(err)
	}

	err = RunMigrations(db, "migrations")
	if err != nil {
		b.Fatal(err)
	}

	return db
}
