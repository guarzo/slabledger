package inventory

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
	ErrCodeCampaignConflict       errors.ErrorCode = "ERR_CAMPAIGN_CONFLICT"
	ErrCodeInvoiceNotFound        errors.ErrorCode = "ERR_INVOICE_NOT_FOUND"
	ErrCodeRevocationTooSoon      errors.ErrorCode = "ERR_REVOCATION_TOO_SOON"
	ErrCodeRevocationFlagNotFound errors.ErrorCode = "ERR_REVOCATION_FLAG_NOT_FOUND"
	ErrCodeNoAISuggestion         errors.ErrorCode = "ERR_NO_AI_SUGGESTION"
	ErrCodePriceFlagNotFound      errors.ErrorCode = "ERR_PRICE_FLAG_NOT_FOUND"
	ErrCodeCertNotFound           errors.ErrorCode = "ERR_CERT_NOT_FOUND"
	ErrCodePendingItemNotFound    errors.ErrorCode = "ERR_PENDING_ITEM_NOT_FOUND"
	ErrCodeInvalidCashflowConfig  errors.ErrorCode = "ERR_INVALID_CASHFLOW_CONFIG"
	ErrCodePurchaseHasSale        errors.ErrorCode = "ERR_PURCHASE_HAS_SALE"

	// DH sale recording (docs/specs/2026-08-20-dh-record-sale-design.md, §3)
	ErrCodeDHItemSoldOnChannel     errors.ErrorCode = "ERR_DH_ITEM_SOLD_ON_CHANNEL"
	ErrCodeDHItemUnavailable       errors.ErrorCode = "ERR_DH_ITEM_UNAVAILABLE"
	ErrCodeDHIdempotencyInProgress errors.ErrorCode = "ERR_DH_IDEMPOTENCY_IN_PROGRESS"
	ErrCodeDHReversalWouldCollide  errors.ErrorCode = "ERR_DH_REVERSAL_WOULD_COLLIDE"
	ErrCodeDHIdempotencyKeyReused  errors.ErrorCode = "ERR_DH_IDEMPOTENCY_KEY_REUSED"
	ErrCodeDHValidation            errors.ErrorCode = "ERR_DH_VALIDATION"
	ErrCodeDHLockContention        errors.ErrorCode = "ERR_DH_LOCK_CONTENTION"
	ErrCodeDHSaleNotFound          errors.ErrorCode = "ERR_DH_SALE_NOT_FOUND"
)

// Sentinel errors for campaign operations
var (
	ErrCampaignNotFound       = errors.NewAppError(ErrCodeCampaignNotFound, "campaign not found")
	ErrCampaignConflict       = errors.NewAppError(ErrCodeCampaignConflict, "campaign changed since it was read")
	ErrPurchaseNotFound       = errors.NewAppError(ErrCodePurchaseNotFound, "purchase not found")
	ErrSaleNotFound           = errors.NewAppError(ErrCodeSaleNotFound, "sale not found")
	ErrDuplicateCertNumber    = errors.NewAppError(ErrCodeDuplicateCertNumber, "certificate number already exists")
	ErrDuplicateSale          = errors.NewAppError(ErrCodeDuplicateSale, "sale already exists for this purchase")
	ErrInvoiceNotFound        = errors.NewAppError(ErrCodeInvoiceNotFound, "invoice not found")
	ErrRevocationTooSoon      = errors.NewAppError(ErrCodeRevocationTooSoon, "revocation already submitted within the past 7 days")
	ErrRevocationFlagNotFound = errors.NewAppError(ErrCodeRevocationFlagNotFound, "revocation flag not found")
	ErrNoAISuggestion         = errors.NewAppError(ErrCodeNoAISuggestion, "no AI suggestion to accept or suggestion has changed")
	ErrPriceFlagNotFound      = errors.NewAppError(ErrCodePriceFlagNotFound, "price flag not found or already resolved")
	ErrCertNotFound           = errors.NewAppError(ErrCodeCertNotFound, "cert not found")
	ErrPendingItemNotFound    = errors.NewAppError(ErrCodePendingItemNotFound, "pending item not found")
	ErrInvalidCashflowConfig  = errors.NewAppError(ErrCodeInvalidCashflowConfig, "invalid cashflow config")
	ErrPurchaseHasSale        = errors.NewAppError(ErrCodePurchaseHasSale, "cannot reassign purchase with a linked sale")

	// DH sale recording sentinels. Only ErrDHIdempotencyInProgress and
	// ErrDHLockContention are retryable byte-identical (see
	// IsRetryableDHSaleError in dh_sale.go) — every other sentinel here
	// represents a permanent outcome that a caller must flag for review
	// rather than resubmit.
	ErrDHItemSoldOnChannel     = errors.NewAppError(ErrCodeDHItemSoldOnChannel, "item already sold on another channel")
	ErrDHItemUnavailable       = errors.NewAppError(ErrCodeDHItemUnavailable, "item unavailable for sale")
	ErrDHIdempotencyInProgress = errors.NewAppError(ErrCodeDHIdempotencyInProgress, "a request with this idempotency key is already in progress")
	ErrDHReversalWouldCollide  = errors.NewAppError(ErrCodeDHReversalWouldCollide, "voiding this sale would collide with a later reversal")
	ErrDHIdempotencyKeyReused  = errors.NewAppError(ErrCodeDHIdempotencyKeyReused, "idempotency key reused with a different request body")
	ErrDHValidation            = errors.NewAppError(ErrCodeDHValidation, "DH rejected the sale request as invalid")
	ErrDHLockContention        = errors.NewAppError(ErrCodeDHLockContention, "DH inventory row is locked by a concurrent operation")
	ErrDHSaleNotFound          = errors.NewAppError(ErrCodeDHSaleNotFound, "DH sale not found")
)

// IsCampaignNotFound checks if the error is a "campaign not found" error.
func IsCampaignNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeCampaignNotFound) }

// IsCampaignConflict reports whether a conditional campaign update was rejected
// because the row changed after the caller read it.
func IsCampaignConflict(err error) bool { return errors.HasErrorCode(err, ErrCodeCampaignConflict) }

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

// IsCertNotFound checks if the error is a "cert not found" error.
func IsCertNotFound(err error) bool { return errors.HasErrorCode(err, ErrCodeCertNotFound) }

// IsPendingItemNotFound checks if the error is a "pending item not found" error.
func IsPendingItemNotFound(err error) bool {
	return errors.HasErrorCode(err, ErrCodePendingItemNotFound)
}

// IsPurchaseHasSale checks if the error indicates a purchase cannot be modified because it has a linked sale.
func IsPurchaseHasSale(err error) bool { return errors.HasErrorCode(err, ErrCodePurchaseHasSale) }
