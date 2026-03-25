package campaigns

import (
	"testing"
)

func TestValidateAndNormalizeCampaign(t *testing.T) {
	tests := []struct {
		name    string
		c       Campaign
		wantErr error
	}{
		{
			name:    "valid campaign",
			c:       Campaign{Name: "Test Campaign", BuyTermsCLPct: 0.8, EbayFeePct: 0.12},
			wantErr: nil,
		},
		{
			name:    "empty name",
			c:       Campaign{Name: ""},
			wantErr: ErrCampaignNameRequired,
		},
		{
			name:    "whitespace-only name",
			c:       Campaign{Name: "   "},
			wantErr: ErrCampaignNameRequired,
		},
		{
			name:    "buyTermsCLPct too high",
			c:       Campaign{Name: "Test", BuyTermsCLPct: 1.5},
			wantErr: ErrInvalidBuyTermsPct,
		},
		{
			name:    "buyTermsCLPct negative",
			c:       Campaign{Name: "Test", BuyTermsCLPct: -0.1},
			wantErr: ErrInvalidBuyTermsPct,
		},
		{
			name:    "ebayFeePct too high",
			c:       Campaign{Name: "Test", EbayFeePct: 1.5},
			wantErr: ErrInvalidEbayFeePct,
		},
		{
			name:    "invalid phase",
			c:       Campaign{Name: "Test", Phase: "invalid"},
			wantErr: ErrInvalidPhase,
		},
		{
			name:    "valid phase",
			c:       Campaign{Name: "Test", Phase: PhaseActive},
			wantErr: nil,
		},
		{
			name:    "negative daily spend",
			c:       Campaign{Name: "Test", DailySpendCapCents: -100},
			wantErr: ErrInvalidDailySpend,
		},
		// YearRange validation
		{
			name:    "valid year range",
			c:       Campaign{Name: "Test", YearRange: "1999-2003"},
			wantErr: nil,
		},
		{
			name:    "empty year range is valid",
			c:       Campaign{Name: "Test", YearRange: ""},
			wantErr: nil,
		},
		{
			name:    "year range missing dash",
			c:       Campaign{Name: "Test", YearRange: "1999"},
			wantErr: ErrInvalidYearRange,
		},
		{
			name:    "year range non-numeric",
			c:       Campaign{Name: "Test", YearRange: "abc-def"},
			wantErr: ErrInvalidYearRange,
		},
		{
			name:    "year range inverted",
			c:       Campaign{Name: "Test", YearRange: "2003-1999"},
			wantErr: ErrInvalidYearRange,
		},
		// PriceRange validation
		{
			name:    "valid price range",
			c:       Campaign{Name: "Test", PriceRange: "50-500"},
			wantErr: nil,
		},
		{
			name:    "empty price range is valid",
			c:       Campaign{Name: "Test", PriceRange: ""},
			wantErr: nil,
		},
		{
			name:    "price range missing dash",
			c:       Campaign{Name: "Test", PriceRange: "500"},
			wantErr: ErrInvalidPriceRange,
		},
		{
			name:    "price range non-numeric",
			c:       Campaign{Name: "Test", PriceRange: "low-high"},
			wantErr: ErrInvalidPriceRange,
		},
		{
			name:    "price range inverted",
			c:       Campaign{Name: "Test", PriceRange: "500-50"},
			wantErr: ErrInvalidPriceRange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAndNormalizeCampaign(&tt.c)
			if err != tt.wantErr {
				t.Errorf("ValidateAndNormalizeCampaign() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAndNormalizePurchase(t *testing.T) {
	valid := Purchase{
		CardName:     "Charizard",
		CertNumber:   "12345678",
		GradeValue:   9,
		BuyCostCents: 50000,
		PurchaseDate: "2026-01-15",
	}

	t.Run("valid purchase", func(t *testing.T) {
		p := valid
		if err := ValidateAndNormalizePurchase(&p); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid half-grade purchase", func(t *testing.T) {
		p := valid
		p.GradeValue = 8.5
		if err := ValidateAndNormalizePurchase(&p); err != nil {
			t.Errorf("unexpected error for half-grade 8.5: %v", err)
		}
	})

	t.Run("missing card name", func(t *testing.T) {
		p := valid
		p.CardName = ""
		if err := ValidateAndNormalizePurchase(&p); err != ErrCardNameRequired {
			t.Errorf("got %v, want ErrCardNameRequired", err)
		}
	})

	t.Run("missing cert number", func(t *testing.T) {
		p := valid
		p.CertNumber = ""
		if err := ValidateAndNormalizePurchase(&p); err != ErrCertNumberRequired {
			t.Errorf("got %v, want ErrCertNumberRequired", err)
		}
	})

	t.Run("grade too low", func(t *testing.T) {
		p := valid
		p.GradeValue = 0
		if err := ValidateAndNormalizePurchase(&p); err != ErrInvalidGradeValue {
			t.Errorf("got %v, want ErrInvalidGradeValue", err)
		}
	})

	t.Run("grade too high", func(t *testing.T) {
		p := valid
		p.GradeValue = 11
		if err := ValidateAndNormalizePurchase(&p); err != ErrInvalidGradeValue {
			t.Errorf("got %v, want ErrInvalidGradeValue", err)
		}
	})

	t.Run("zero buy cost", func(t *testing.T) {
		p := valid
		p.BuyCostCents = 0
		if err := ValidateAndNormalizePurchase(&p); err != ErrInvalidAmount {
			t.Errorf("got %v, want ErrInvalidAmount", err)
		}
	})

	t.Run("missing purchase date", func(t *testing.T) {
		p := valid
		p.PurchaseDate = ""
		if err := ValidateAndNormalizePurchase(&p); err != ErrPurchaseDateRequired {
			t.Errorf("got %v, want ErrPurchaseDateRequired", err)
		}
	})
}

func TestValidateSale(t *testing.T) {
	valid := Sale{
		PurchaseID:     "abc-123",
		SaleChannel:    SaleChannelEbay,
		SalePriceCents: 75000,
		SaleDate:       "2026-02-01",
	}

	t.Run("valid sale", func(t *testing.T) {
		s := valid
		if err := ValidateSale(&s); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing purchase ID", func(t *testing.T) {
		s := valid
		s.PurchaseID = ""
		if err := ValidateSale(&s); err != ErrPurchaseIDRequired {
			t.Errorf("got %v, want ErrPurchaseIDRequired", err)
		}
	})

	t.Run("invalid sale channel", func(t *testing.T) {
		s := valid
		s.SaleChannel = "invalid"
		if err := ValidateSale(&s); err != ErrInvalidSaleChannel {
			t.Errorf("got %v, want ErrInvalidSaleChannel", err)
		}
	})

	t.Run("zero sale price", func(t *testing.T) {
		s := valid
		s.SalePriceCents = 0
		if err := ValidateSale(&s); err != ErrInvalidAmount {
			t.Errorf("got %v, want ErrInvalidAmount", err)
		}
	})

	t.Run("missing sale date", func(t *testing.T) {
		s := valid
		s.SaleDate = ""
		if err := ValidateSale(&s); err != ErrSaleDateRequired {
			t.Errorf("got %v, want ErrSaleDateRequired", err)
		}
	})
}
