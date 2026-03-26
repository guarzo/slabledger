package campaigns

import (
	"github.com/guarzo/slabledger/internal/domain/errors"
)

// Domain-specific error codes for campaigns
const (
	ErrCodeCampaignNotFound       errors.ErrorCode = "ERR_CAMPAIGN_NOT_FOUND"
	ErrCodePurchaseNotFound       errors.ErrorCode = "ERR_PURCHASE_NOT_FOUND"
	ErrCodeSaleNotFound           errors.ErrorCode = "ERR_SALE_NOT_FOUND"
	ErrCodeDuplicateCertNumber    errors.ErrorCode = "ERR_DUPLICATE_CERT_NUMBER"
	ErrCodeDuplicateSale          errors.ErrorCode = "ERR_DUPLICATE_SALE"
	ErrCodeCampaignValidation     errors.ErrorCode = "ERR_CAMPAIGN_VALIDATION"
	ErrCodeInvoiceNotFound        errors.ErrorCode = "ERR_INVOICE_NOT_FOUND"
	ErrCodeRevocationTooSoon      errors.ErrorCode = "ERR_REVOCATION_TOO_SOON"
	ErrCodeRevocationFlagNotFound errors.ErrorCode = "ERR_REVOCATION_FLAG_NOT_FOUND"
	ErrCodeNoAISuggestion         errors.ErrorCode = "ERR_NO_AI_SUGGESTION"
	ErrCodePriceFlagNotFound      errors.ErrorCode = "ERR_PRICE_FLAG_NOT_FOUND"
)

// Sentinel errors for campaign operations
var (
	ErrCampaignNotFound       = errors.NewAppError(ErrCodeCampaignNotFound, "campaign not found")
	ErrPurchaseNotFound       = errors.NewAppError(ErrCodePurchaseNotFound, "purchase not found")
	ErrSaleNotFound           = errors.NewAppError(ErrCodeSaleNotFound, "sale not found")
	ErrDuplicateCertNumber    = errors.NewAppError(ErrCodeDuplicateCertNumber, "certificate number already exists")
	ErrDuplicateSale          = errors.NewAppError(ErrCodeDuplicateSale, "sale already exists for this purchase")
	ErrInvoiceNotFound        = errors.NewAppError(ErrCodeInvoiceNotFound, "invoice not found")
	ErrRevocationTooSoon      = errors.NewAppError(ErrCodeRevocationTooSoon, "revocation already submitted within the past 7 days")
	ErrRevocationFlagNotFound = errors.NewAppError(ErrCodeRevocationFlagNotFound, "revocation flag not found")
	ErrNoAISuggestion         = errors.NewAppError(ErrCodeNoAISuggestion, "no AI suggestion to accept or suggestion has changed")
	ErrPriceFlagNotFound      = errors.NewAppError(ErrCodePriceFlagNotFound, "price flag not found or already resolved")
)

// IsCampaignNotFound checks if the error is a "campaign not found" error.
func IsCampaignNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeCampaignNotFound) }

// IsDuplicateCertNumber checks if the error is a "duplicate cert number" error.
func IsDuplicateCertNumber(err error) bool {
	return errors.HasErrorCode(err, ErrCodeDuplicateCertNumber)
}

// IsSaleNotFound checks if the error is a "sale not found" error.
func IsSaleNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeSaleNotFound) }

// IsDuplicateSale checks if the error is a "duplicate sale" error.
func IsDuplicateSale(err error) bool { return errors.HasErrorCode(err, ErrCodeDuplicateSale) }

// IsPurchaseNotFound checks if the error is a "purchase not found" error.
func IsPurchaseNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodePurchaseNotFound) }

// IsValidationError checks if the error is a campaign validation error.
func IsValidationError(err error) bool { return errors.HasErrorCode(err, ErrCodeCampaignValidation) }

// IsNoAISuggestion checks if the error indicates a missing or stale AI suggestion.
func IsNoAISuggestion(err error) bool { return errors.HasErrorCode(err, ErrCodeNoAISuggestion) }

// IsPriceFlagNotFound checks if the error is a "price flag not found" error.
func IsPriceFlagNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodePriceFlagNotFound) }
