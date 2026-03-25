package pricecharting

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/cache"
)

// TestPriceCharting_EnrichWithHistoricalData tests historical data enrichment
func TestPriceCharting_EnrichWithHistoricalData(t *testing.T) {
	tmpDir := t.TempDir()
	c, _ := cache.New(cache.Config{Type: "file", FilePath: filepath.Join(tmpDir, "cache.json")})
	pc, _ := NewPriceCharting(DefaultConfig("test-token"), c, nil)
	ctx := context.Background()

	// Test with nil match
	err := pc.EnrichWithHistoricalData(ctx, nil)
	if err == nil {
		t.Error("Expected error for nil match")
	}

	// Test with empty ID
	match := &PCMatch{
		ID:          "",
		ProductName: "Test Card",
	}
	err = pc.EnrichWithHistoricalData(ctx, match)
	if err == nil {
		t.Error("Expected error for empty ID")
	}

	// Test with valid match but no data available
	match = &PCMatch{
		ID:          "test-id",
		ProductName: "Test Card",
		PSA10Cents:  15000,
	}
	err = pc.EnrichWithHistoricalData(ctx, match)
	if err != nil {
		t.Errorf("Enrichment should not fail when historical data unavailable: %v", err)
	}
}
