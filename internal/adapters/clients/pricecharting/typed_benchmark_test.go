package pricecharting

import (
	"encoding/json"
	"testing"
)

// BenchmarkParseAPIResponse_Typed benchmarks the typed API response parsing
func BenchmarkParseAPIResponse_Typed(b *testing.B) {
	// Create sample API response JSON
	apiResponse := PriceChartingAPIResponse{
		ID:           "12345",
		ProductName:  "Pokemon Base Set Charizard #004",
		ConsoleName:  "Pokemon Card Game",
		Status:       "success",
		LoosePrice:   intPtr(5000),   // $50.00 in cents
		GradedPrice:  intPtr(15000),  // $150.00 in cents
		BoxOnlyPrice: intPtr(25000),  // $250.00 in cents
		ManualPrice:  intPtr(50000),  // $500.00 in cents
		BGS10Price:   intPtr(100000), // $1000.00 in cents
		UPC:          stringPtr("12345678901"),
		SalesVolume:  &FlexibleInt{Value: 100, IsSet: true},
		LastSoldDate: stringPtr("2024-01-01"),
		SalesData: []APISaleData{
			{
				SalePrice: floatPtr(500.00),
				SaleDate:  stringPtr("2024-01-01"),
				Grade:     stringPtr("PSA 10"),
				Source:    stringPtr("eBay"),
			},
			{
				SalePrice: floatPtr(480.00),
				SaleDate:  stringPtr("2023-12-28"),
				Grade:     stringPtr("PSA 10"),
				Source:    stringPtr("eBay"),
			},
		},
	}

	// Marshal to JSON bytes once
	jsonBytes, err := json.Marshal(apiResponse)
	if err != nil {
		b.Fatalf("Failed to marshal test data: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parseAPIResponseWithLogger(jsonBytes, nil)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkParseSaleData_Typed benchmarks the typed sale data parsing
func BenchmarkParseSaleData_Typed(b *testing.B) {
	apiSale := APISaleData{
		SalePrice: floatPtr(500.00),
		SaleDate:  stringPtr("2024-01-01"),
		Grade:     stringPtr("PSA 10"),
		Source:    stringPtr("eBay"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = parseSaleData(apiSale)
	}
}

// BenchmarkTypedVsMap_APIResponse compares typed struct vs map[string]interface{} unmarshaling
func BenchmarkTypedVsMap_APIResponse(b *testing.B) {
	jsonData := []byte(`{
		"id": "12345",
		"product-name": "Pokemon Base Set Charizard #004",
		"console-name": "Pokemon Card Game",
		"status": "success",
		"loose-price": 50.00,
		"graded-price": 150.00,
		"box-only-price": 250.00,
		"manual-only-price": 500.00,
		"bgs-10-price": 1000.00,
		"upc": "12345678901",
		"sales-volume": 100
	}`)

	b.Run("Typed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var resp PriceChartingAPIResponse
			if err := json.Unmarshal(jsonData, &resp); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Map", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var resp map[string]interface{}
			if err := json.Unmarshal(jsonData, &resp); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkHasPriceKeysTyped benchmarks typed price key checking
func BenchmarkHasPriceKeysTyped(b *testing.B) {
	typedResp := &PriceChartingAPIResponse{
		ID:          "12345",
		ProductName: "Test Card",
		Status:      "success",
		LoosePrice:  intPtr(5000),  // $50.00 in cents
		ManualPrice: intPtr(50000), // $500.00 in cents
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hasPriceKeysTyped(typedResp)
	}
}
