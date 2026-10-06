package inventory

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var ErrReturnConflict = errors.New("confirmed return conflict")

type ReturnConflict struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewReturnConflict(code, message string) *ReturnConflict {
	return &ReturnConflict{Code: code, Message: message}
}
func (e *ReturnConflict) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
func (e *ReturnConflict) Unwrap() error { return ErrReturnConflict }

// ReturnTargetIdentity is an operator-observed precondition for NEW external
// returns. The server still captures the provider target from its own purchase.
type ReturnTargetIdentity struct {
	DHInventoryID int    `json:"dhInventoryId"`
	CertNumber    string `json:"certNumber"`
	Grader        string `json:"grader"`
}

// The HTTP boundary must independently require explicit expectedSaleId presence
// and literal confirmation; nil represents explicit JSON null.
type ConfirmReturnRequest struct {
	ReturnConfirmed bool                  `json:"returnConfirmed"`
	ExpectedSaleID  *string               `json:"expectedSaleId"`
	ExpectedTarget  *ReturnTargetIdentity `json:"expectedTarget,omitempty"`
	OperationID     string                `json:"operationId,omitempty"`
}
type ReturnFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
}
type ConfirmedReturnEpisode struct {
	ID                  string          `json:"id"`
	Key                 string          `json:"-"`
	PurchaseID          *string         `json:"purchaseId"`
	CapturedPurchaseID  string          `json:"capturedPurchaseId"`
	DHInventoryID       int             `json:"dhInventoryId"`
	CertNumber          string          `json:"certNumber"`
	Grader              string          `json:"grader"`
	ExpectedSaleID      *string         `json:"expectedSaleId"`
	CapturedOrderID     string          `json:"capturedOrderId"`
	ReturnedOrderID     string          `json:"returnedOrderId"`
	State               string          `json:"state"`
	CreatedAt           time.Time       `json:"createdAt"`
	CompletedAt         *time.Time      `json:"completedAt,omitempty"`
	ListingAuthorizedAt *time.Time      `json:"listingAuthorizedAt,omitempty"`
	LastError           *ReturnFailure  `json:"lastError,omitempty"`
	ObservedReceipt     *DHReturnResult `json:"observedReceipt,omitempty"`
}
type ConfirmedReturnState struct {
	Operation        *ConfirmedReturnEpisode `json:"operation"`
	ExpectedSaleID   *string                 `json:"expectedSaleId"`
	AwaitingListing  bool                    `json:"awaitingListing"`
	PrecedingAttempt *DHMutationAttempt      `json:"precedingAttempt"`
	Purchase         *Purchase               `json:"purchase"`
	Sale             *Sale                   `json:"sale"`
	Outcome          string                  `json:"outcome,omitempty"`
}
type ConfirmedReturnRepository interface {
	DHMutationRepository
	GetReturnState(context.Context, string) (*ConfirmedReturnState, error)
	ResolveReturn(context.Context, string, ConfirmReturnRequest) (*ConfirmedReturnState, error)
	AssertMutationAllowed(context.Context, string) error
	DeleteLocalConfirmedSale(context.Context, string, string) error
	PrepareReturn(context.Context, string, ConfirmReturnRequest, string, string) (*ConfirmedReturnState, error)
	CompleteReturn(context.Context, string, *ConfirmedReturnEpisode, *DHReturnResult) error
	RecordReturnFailure(context.Context, string, string, ReturnFailure, *DHReturnResult, bool) error
}

// SelectReturnSource runs only AFTER episode/precondition resolution.
func SelectReturnSource(p *Purchase, s *Sale) (string, error) {
	if s != nil && s.OrderID != "" && (s.DHSaleID != "" || s.DHIdempotencyKey != "") {
		return "", NewReturnConflict("identity_conflict", "imported order and off-platform DH identity disagree")
	}
	if p.DHInventoryID == 0 {
		if s == nil {
			return "none", nil
		}
		return "local", nil
	}
	if s == nil {
		return "external", nil
	}
	if s.OrderID != "" {
		if s.SaleChannel != SaleChannelEbay || !IsCanonicalExternalOrder(s.OrderID) {
			return "", NewReturnConflict("unsupported_return_source", "only attributed external eBay orders support confirmed return")
		}
		return "external", nil
	}
	if s.DHSaleID != "" || s.DHIdempotencyKey != "" {
		return "legacy_void", nil
	}
	return "external", nil
}
func IsCanonicalExternalOrder(order string) bool {
	if len(order) < 5 || order[:4] != "ext-" {
		return false
	}
	n, e := strconv.ParseInt(order[4:], 10, 64)
	return e == nil && n > 0 && order == fmt.Sprintf("ext-%d", n)
}
