package campaigns

import (
	"strings"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/domain/errors"
)

const (
	MaxCampaignNameLength = 100
)

var (
	ErrCampaignNameRequired   = errors.NewAppError(ErrCodeCampaignValidation, "campaign name is required")
	ErrCampaignNameTooLong    = errors.NewAppError(ErrCodeCampaignValidation, "campaign name exceeds maximum length")
	ErrInvalidBuyTermsPct     = errors.NewAppError(ErrCodeCampaignValidation, "buyTermsCLPct must be between 0 and 1")
	ErrInvalidEbayFeePct      = errors.NewAppError(ErrCodeCampaignValidation, "ebayFeePct must be between 0 and 1")
	ErrInvalidPhase           = errors.NewAppError(ErrCodeCampaignValidation, "invalid campaign phase")
	ErrInvalidSaleChannel     = errors.NewAppError(ErrCodeCampaignValidation, "invalid sale channel")
	ErrCardNameRequired       = errors.NewAppError(ErrCodeCampaignValidation, "card name is required")
	ErrCertNumberRequired     = errors.NewAppError(ErrCodeCampaignValidation, "cert number is required")
	ErrInvalidGrade           = errors.NewAppError(ErrCodeCampaignValidation, "PSA grade must be between 1 and 10")
	ErrInvalidGradeValue      = errors.NewAppError(ErrCodeCampaignValidation, "grade value must be between 1 and 10")
	ErrInvalidAmount          = errors.NewAppError(ErrCodeCampaignValidation, "amount must be positive")
	ErrPurchaseDateRequired   = errors.NewAppError(ErrCodeCampaignValidation, "purchase date is required")
	ErrSaleDateRequired       = errors.NewAppError(ErrCodeCampaignValidation, "sale date is required")
	ErrPurchaseIDRequired     = errors.NewAppError(ErrCodeCampaignValidation, "purchase ID is required")
	ErrSaleDateBeforePurchase = errors.NewAppError(ErrCodeCampaignValidation, "sale date cannot be before purchase date")
	ErrInvalidDailySpend      = errors.NewAppError(ErrCodeCampaignValidation, "daily spend cap must be non-negative")
	ErrInvalidYearRange       = errors.NewAppError(ErrCodeCampaignValidation, "yearRange must be empty or in format 'startYear-endYear' (e.g. 1999-2003)")
	ErrInvalidPriceRange      = errors.NewAppError(ErrCodeCampaignValidation, "priceRange must be empty or in format 'min-max' (e.g. 50-500)")
	ErrInvalidGradeRange      = errors.NewAppError(ErrCodeCampaignValidation, "gradeRange must be empty or in format 'min-max' (e.g. 7-10)")
)

var validPhases = map[Phase]bool{
	PhasePending: true,
	PhaseActive:  true,
	PhaseClosed:  true,
}

var validSaleChannels = map[SaleChannel]bool{
	SaleChannelEbay:     true,
	SaleChannelWebsite:  true,
	SaleChannelInPerson: true,
	// Legacy channels accepted for backward compatibility with existing DB records.
	SaleChannelTCGPlayer:  true,
	SaleChannelLocal:      true,
	SaleChannelOther:      true,
	SaleChannelGameStop:   true,
	SaleChannelCardShow:   true,
	SaleChannelDoubleHolo: true,
}

// ValidateAndNormalizeCampaign validates and normalizes campaign fields (trims whitespace).
func ValidateAndNormalizeCampaign(c *Campaign) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return ErrCampaignNameRequired
	}
	if utf8.RuneCountInString(c.Name) > MaxCampaignNameLength {
		return ErrCampaignNameTooLong
	}
	if c.BuyTermsCLPct < 0 || c.BuyTermsCLPct > 1 {
		return ErrInvalidBuyTermsPct
	}
	if c.EbayFeePct < 0 || c.EbayFeePct > 1 {
		return ErrInvalidEbayFeePct
	}
	if c.DailySpendCapCents < 0 {
		return ErrInvalidDailySpend
	}
	if c.Phase != "" && !validPhases[c.Phase] {
		return ErrInvalidPhase
	}
	c.GradeRange = strings.TrimSpace(c.GradeRange)
	if err := validateRange(c.GradeRange, 1, 10, ErrInvalidGradeRange); err != nil {
		return err
	}
	c.YearRange = strings.TrimSpace(c.YearRange)
	if err := validateRange(c.YearRange, 1900, 2100, ErrInvalidYearRange); err != nil {
		return err
	}
	c.PriceRange = strings.TrimSpace(c.PriceRange)
	if err := validateRange(c.PriceRange, 0, 1_000_000, ErrInvalidPriceRange); err != nil {
		return err
	}
	return nil
}

// validateRange checks that s is empty or matches "min-max" where both are integers
// within [floor, ceil] and min <= max.
func validateRange(s string, floor, ceil int, errVal error) error {
	if s == "" {
		return nil
	}
	lo, hi, ok := ParseRange(s)
	if !ok || lo < floor || hi > ceil {
		return errVal
	}
	return nil
}

// ValidateAndNormalizePurchase validates and normalizes purchase fields (trims whitespace).
func ValidateAndNormalizePurchase(p *Purchase) error {
	p.CardName = strings.TrimSpace(p.CardName)
	p.CertNumber = strings.TrimSpace(p.CertNumber)

	if p.CardName == "" {
		return ErrCardNameRequired
	}
	if p.CertNumber == "" {
		return ErrCertNumberRequired
	}
	if p.GradeValue < 1 || p.GradeValue > 10 {
		return ErrInvalidGradeValue
	}
	if p.BuyCostCents <= 0 {
		return ErrInvalidAmount
	}
	if p.PurchaseDate == "" {
		return ErrPurchaseDateRequired
	}
	return nil
}

// ValidateAndNormalizeExternalPurchase validates purchases from external sources (e.g. Shopify).
// Unlike ValidateAndNormalizePurchase, it allows BuyCostCents=0,
// but requires CardName, CertNumber, GradeValue in range 1-10 (allows half-grades),
// and PurchaseDate.
func ValidateAndNormalizeExternalPurchase(p *Purchase) error {
	p.CardName = strings.TrimSpace(p.CardName)
	p.CertNumber = strings.TrimSpace(p.CertNumber)

	if p.CardName == "" {
		return ErrCardNameRequired
	}
	if p.CertNumber == "" {
		return ErrCertNumberRequired
	}
	if p.GradeValue < 1 || p.GradeValue > 10 {
		return ErrInvalidGradeValue
	}
	if p.PurchaseDate == "" {
		return ErrPurchaseDateRequired
	}
	return nil
}

// ValidateSale validates sale fields.
func ValidateSale(s *Sale) error {
	if s.PurchaseID == "" {
		return ErrPurchaseIDRequired
	}
	if !validSaleChannels[s.SaleChannel] {
		return ErrInvalidSaleChannel
	}
	if s.SalePriceCents <= 0 {
		return ErrInvalidAmount
	}
	if s.SaleDate == "" {
		return ErrSaleDateRequired
	}
	return nil
}

