package pricecharting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAPIResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *PCMatch
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid complete response",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Charizard",
				"console-name": "Pokemon Card Game",
				"status": "success",
				"loose-price": 5099,
				"graded-price": 8000,
				"box-only-price": 12050,
				"manual-only-price": 15050,
				"bgs-10-price": 20000,
				"upc": "123456789"
			}`,
			want: &PCMatch{
				ID:           "12345",
				ProductName:  "Pokemon Charizard",
				LooseCents:   5099,
				Grade9Cents:  8000,
				Grade95Cents: 12050,
				PSA10Cents:   15050,
				BGS10Cents:   20000,
				UPC:          "123456789",
			},
		},
		{
			name: "minimal valid response",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Pikachu"
			}`,
			want: &PCMatch{
				ID:          "12345",
				ProductName: "Pokemon Pikachu",
				// All prices should be 0, Language should be empty
			},
		},
		{
			name: "null UPC",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Pikachu",
				"loose-price": 5000,
				"upc": null
			}`,
			want: &PCMatch{
				ID:          "12345",
				ProductName: "Pokemon Pikachu",
				LooseCents:  5000,
				UPC:         "",
			},
		},
		{
			name: "Japanese card",
			input: `{
				"id": "67890",
				"product-name": "Pokemon Charizard",
				"console-name": "Pokemon Card Game (Japanese)",
				"loose-price": 10000
			}`,
			want: &PCMatch{
				ID:          "67890",
				ProductName: "Pokemon Charizard",
				LooseCents:  10000,
			},
		},
		{
			name: "with sales data",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Charizard",
				"sales-volume": 10,
				"sales-data": [
					{
						"sale-price": 100.00,
						"sale-date": "2024-01-15",
						"grade": "PSA 10",
						"source": "eBay"
					},
					{
						"sale-price": 110.00,
						"sale-date": "2024-01-16",
						"grade": "PSA 10"
					}
				]
			}`,
			want: &PCMatch{
				ID:          "12345",
				ProductName: "Pokemon Charizard",
				SalesVolume: 10,
				SalesCount:  2,
				RecentSales: []SaleData{
					{
						PriceCents: 10000,
						Date:       "2024-01-15",
						Grade:      "PSA 10",
						Source:     "eBay",
					},
					{
						PriceCents: 11000,
						Date:       "2024-01-16",
						Grade:      "PSA 10",
						Source:     "eBay", // Default
					},
				},
				AvgSalePrice: 10500, // (10000 + 11000) / 2
			},
		},
		{
			name: "with marketplace data",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Charizard",
				"active-listings": 25,
				"lowest-listing": 9550,
				"average-listing-price": 10500,
				"listing-velocity": 2.5,
				"competition-level": "HIGH"
			}`,
			want: &PCMatch{
				ID:                  "12345",
				ProductName:         "Pokemon Charizard",
				ActiveListings:      25,
				LowestListing:       9550,
				AverageListingPrice: 10500,
				ListingVelocity:     2.5,
				CompetitionLevel:    "HIGH",
			},
		},
		{
			name: "all price fields",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Charizard",
				"loose-price": 5000,
				"graded-price": 8000,
				"box-only-price": 10000,
				"manual-only-price": 15000,
				"bgs-10-price": 20000,
				"new-price": 30000,
				"cib-price": 25000,
				"manual-price": 17500,
				"box-price": 12500,
				"retail-buy-price": 4000,
				"retail-sell-price": 6000
			}`,
			want: &PCMatch{
				ID:               "12345",
				ProductName:      "Pokemon Charizard",
				LooseCents:       5000,
				Grade9Cents:      8000,
				Grade95Cents:     10000,
				PSA10Cents:       15000,
				BGS10Cents:       20000,
				NewPriceCents:    30000,
				CIBPriceCents:    25000,
				ManualPriceCents: 17500,
				BoxPriceCents:    12500,
			},
		},
		{
			name: "price rounding - now obsolete since API returns integers",
			input: `{
				"id": "12345",
				"product-name": "Pokemon Charizard",
				"loose-price": 5100,
				"graded-price": 8000
			}`,
			want: &PCMatch{
				ID:          "12345",
				ProductName: "Pokemon Charizard",
				LooseCents:  5100,
				Grade9Cents: 8000,
			},
		},
		{
			name: "missing required field: id",
			input: `{
				"product-name": "Pokemon Charizard"
			}`,
			wantErr: true,
			errMsg:  "missing required field: id",
		},
		{
			name: "missing required field: product-name",
			input: `{
				"id": "12345"
			}`,
			wantErr: true,
			errMsg:  "missing required field: product-name",
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
			errMsg:  "Invalid response from provider 'PriceCharting'",
		},
		{
			name:    "empty JSON",
			input:   `{}`,
			wantErr: true,
			errMsg:  "missing required field: id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAPIResponseWithLogger([]byte(tt.input), nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.want.ID, got.ID)
			assert.Equal(t, tt.want.ProductName, got.ProductName)
			assert.Equal(t, tt.want.LooseCents, got.LooseCents)
			assert.Equal(t, tt.want.Grade9Cents, got.Grade9Cents)
			assert.Equal(t, tt.want.Grade95Cents, got.Grade95Cents)
			assert.Equal(t, tt.want.PSA10Cents, got.PSA10Cents)
			assert.Equal(t, tt.want.BGS10Cents, got.BGS10Cents)
			assert.Equal(t, tt.want.UPC, got.UPC)

			// Check additional fields if set
			if tt.want.SalesVolume > 0 {
				assert.Equal(t, tt.want.SalesVolume, got.SalesVolume)
			}
			if tt.want.SalesCount > 0 {
				assert.Equal(t, tt.want.SalesCount, got.SalesCount)
				assert.Equal(t, len(tt.want.RecentSales), len(got.RecentSales))
			}
			if tt.want.AvgSalePrice > 0 {
				assert.Equal(t, tt.want.AvgSalePrice, got.AvgSalePrice)
			}
			if tt.want.ActiveListings > 0 {
				assert.Equal(t, tt.want.ActiveListings, got.ActiveListings)
			}
			if tt.want.NewPriceCents > 0 {
				assert.Equal(t, tt.want.NewPriceCents, got.NewPriceCents)
			}
		})
	}
}

func TestParseSaleData(t *testing.T) {
	tests := []struct {
		name     string
		input    APISaleData
		expected *SaleData
	}{
		{
			name: "complete sale data",
			input: APISaleData{
				SalePrice: floatPtr(100.50),
				SaleDate:  stringPtr("2024-01-15"),
				Grade:     stringPtr("PSA 10"),
				Source:    stringPtr("eBay"),
			},
			expected: &SaleData{
				PriceCents: 10050,
				Date:       "2024-01-15",
				Grade:      "PSA 10",
				Source:     "eBay",
			},
		},
		{
			name: "default source",
			input: APISaleData{
				SalePrice: floatPtr(100.00),
				SaleDate:  stringPtr("2024-01-15"),
				Grade:     stringPtr("PSA 10"),
				Source:    nil, // No source provided
			},
			expected: &SaleData{
				PriceCents: 10000,
				Date:       "2024-01-15",
				Grade:      "PSA 10",
				Source:     "eBay", // Default
			},
		},
		{
			name: "minimal data",
			input: APISaleData{
				SalePrice: floatPtr(100.00),
			},
			expected: &SaleData{
				PriceCents: 10000,
				Source:     "eBay",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSaleData(tt.input)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected.PriceCents, result.PriceCents)
			assert.Equal(t, tt.expected.Date, result.Date)
			assert.Equal(t, tt.expected.Grade, result.Grade)
			assert.Equal(t, tt.expected.Source, result.Source)
		})
	}
}

// Helper functions for tests
func floatPtr(f float64) *float64 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func TestPriceChartingAPIResponse_Validate(t *testing.T) {
	tests := []struct {
		name    string
		resp    PriceChartingAPIResponse
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid response",
			resp: PriceChartingAPIResponse{
				ID:          "12345",
				ProductName: "Pokemon Charizard",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			resp: PriceChartingAPIResponse{
				ProductName: "Pokemon Charizard",
			},
			wantErr: true,
			errMsg:  "missing required field: id",
		},
		{
			name: "missing ProductName",
			resp: PriceChartingAPIResponse{
				ID: "12345",
			},
			wantErr: true,
			errMsg:  "missing required field: product-name",
		},
		{
			name: "valid with nil prices",
			resp: PriceChartingAPIResponse{
				ID:          "12345",
				ProductName: "Pokemon Charizard",
				LoosePrice:  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
