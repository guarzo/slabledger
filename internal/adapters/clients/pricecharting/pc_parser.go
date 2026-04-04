package pricecharting

import (
	"context"
	"encoding/json"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// parseAPIResponseWithLogger parses JSON bytes from PriceCharting API to PCMatch using typed structs.
// It accepts an optional logger for diagnostics and optional context for log correlation.
func parseAPIResponseWithLogger(data []byte, log observability.Logger, ctx ...context.Context) (*PCMatch, error) {
	var apiResp PriceChartingAPIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, apperrors.ProviderInvalidResponse("PriceCharting", err)
	}
	return convertAPIResponse(&apiResp, log, ctx...)
}

// convertAPIResponse validates and converts a typed API response to PCMatch directly,
// avoiding a marshal/unmarshal roundtrip when the response is already deserialized.
func convertAPIResponse(apiResp *PriceChartingAPIResponse, log observability.Logger, ctx ...context.Context) (*PCMatch, error) {
	if err := apiResp.Validate(); err != nil {
		return nil, apperrors.ProviderInvalidResponse("PriceCharting", err)
	}

	match := &PCMatch{
		ID:          apiResp.ID,
		ProductName: apiResp.ProductName,
		ConsoleName: apiResp.ConsoleName,
	}

	// PriceCharting API returns prices already in CENTS (as integers)
	// NO conversion needed - just copy the values directly
	if apiResp.LoosePrice != nil {
		match.LooseCents = *apiResp.LoosePrice
	}
	if apiResp.PSA8Price != nil {
		match.PSA8Cents = *apiResp.PSA8Price
	} else if apiResp.CIBPrice != nil {
		// For Pokemon cards, PriceCharting maps PSA 8 to cib-price
		match.PSA8Cents = *apiResp.CIBPrice
	}

	if apiResp.GradedPrice != nil {
		match.Grade9Cents = *apiResp.GradedPrice // graded-price = Grade 9 (Condition 5)
	}
	if apiResp.BoxOnlyPrice != nil {
		match.Grade95Cents = *apiResp.BoxOnlyPrice // box-only-price = Grade 9.5 (Condition 6)
	}
	if apiResp.ManualPrice != nil {
		match.PSA10Cents = *apiResp.ManualPrice // manual-only-price = Grade 10 (Condition 7)
	}

	if (apiResp.GradedPrice != nil && *apiResp.GradedPrice > 0) ||
		(apiResp.ManualPrice != nil && *apiResp.ManualPrice > 0) {
		gradedVal := 0
		manualVal := 0
		if apiResp.GradedPrice != nil {
			gradedVal = *apiResp.GradedPrice
		}
		if apiResp.ManualPrice != nil {
			manualVal = *apiResp.ManualPrice
		}

		// Log when PSA10 < PSA9 (suspicious - should investigate)
		if match.PSA10Cents > 0 && match.Grade9Cents > 0 && match.PSA10Cents < match.Grade9Cents {
			if log != nil {
				logCtx := context.Background()
				if len(ctx) > 0 && ctx[0] != nil {
					logCtx = ctx[0]
				}
				log.Warn(logCtx, "PRICE MAPPING WARNING: PSA10 < PSA9 - possible field swap in API",
					observability.String("product", apiResp.ProductName),
					observability.Int("graded_price_api", gradedVal),
					observability.Int("manual_price_api", manualVal),
					observability.Int("psa9_mapped", match.Grade9Cents),
					observability.Int("psa10_mapped", match.PSA10Cents),
				)
			}
		}
	}
	if apiResp.BGS10Price != nil {
		match.BGS10Cents = *apiResp.BGS10Price
	}

	// Additional price fields (already in cents)
	if apiResp.NewPrice != nil {
		match.NewPriceCents = *apiResp.NewPrice
	}
	if apiResp.CIBPrice != nil {
		match.CIBPriceCents = *apiResp.CIBPrice
	}
	if apiResp.ManualPrice2 != nil {
		match.ManualPriceCents = *apiResp.ManualPrice2
	}
	if apiResp.BoxPrice != nil {
		match.BoxPriceCents = *apiResp.BoxPrice
	}

	// Handle optional fields
	if apiResp.UPC != nil {
		match.UPC = *apiResp.UPC
	}

	if apiResp.SalesVolume != nil && apiResp.SalesVolume.IsSet {
		match.SalesVolume = apiResp.SalesVolume.Value
	}

	if apiResp.LastSoldDate != nil {
		match.LastSoldDate = *apiResp.LastSoldDate
	}

	// Marketplace fields (prices already in cents)
	if apiResp.ActiveListings != nil {
		match.ActiveListings = *apiResp.ActiveListings
	}
	if apiResp.LowestListing != nil {
		match.LowestListing = *apiResp.LowestListing
	}
	if apiResp.AverageListingPrice != nil {
		match.AverageListingPrice = *apiResp.AverageListingPrice
	}
	if apiResp.ListingVelocity != nil {
		match.ListingVelocity = *apiResp.ListingVelocity
	}
	if apiResp.CompetitionLevel != nil {
		match.CompetitionLevel = *apiResp.CompetitionLevel
	}

	// Parse sales data if available
	if len(apiResp.SalesData) > 0 {
		for _, apiSale := range apiResp.SalesData {
			saleData := parseSaleData(apiSale)
			if saleData != nil {
				match.RecentSales = append(match.RecentSales, *saleData)
			}
		}
		match.SalesCount = len(match.RecentSales)

		// Calculate average sale price if we have sales
		if len(match.RecentSales) > 0 {
			total := 0
			for _, sale := range match.RecentSales {
				total += sale.PriceCents
			}
			match.AvgSalePrice = total / len(match.RecentSales)
		}

	}

	// If SalesVolume wasn't directly provided but we have sales data, use the count
	if match.SalesVolume == 0 && match.SalesCount > 0 {
		match.SalesVolume = match.SalesCount
	}

	return match, nil
}

// parseSaleData extracts SaleData from typed API sale data
func parseSaleData(apiSale APISaleData) *SaleData {
	saleData := &SaleData{}

	// Extract price (convert from dollars to cents)
	if apiSale.SalePrice != nil {
		saleData.PriceCents = int(mathutil.ToCents(*apiSale.SalePrice))
	}

	// Extract date
	if apiSale.SaleDate != nil {
		saleData.Date = *apiSale.SaleDate
	}

	// Extract grade
	if apiSale.Grade != nil {
		saleData.Grade = *apiSale.Grade
	}

	// Extract source
	if apiSale.Source != nil {
		saleData.Source = *apiSale.Source
	} else {
		// Default source to eBay if not specified
		saleData.Source = "eBay"
	}

	return saleData
}
