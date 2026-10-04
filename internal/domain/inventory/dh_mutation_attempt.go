package inventory

import (
	"context"
	"errors"
	"time"
)

// DHMutationRequest describes ONE immutable outbound orchestration. The payload
// identity must cover every request field and phase (including sync/revert).
// Only receipt-backed sale/return operations can replay an open attempt.
type DHMutationRequest struct {
	Kind              string
	Phase             string
	Key               string
	PayloadIdentity   string
	ReturnOperationID string
}
type DHMutationAttempt struct {
	ID                 string     `json:"id"`
	CapturedPurchaseID string     `json:"capturedPurchaseId"`
	DHInventoryID      int        `json:"dhInventoryId"`
	CertNumber         string     `json:"certNumber"`
	Grader             string     `json:"grader"`
	Kind               string     `json:"kind"`
	Phase              string     `json:"phase"`
	Key                string     `json:"-"`
	PayloadIdentity    string     `json:"-"`
	ReturnOperationID  string     `json:"operationId,omitempty"`
	StartedAt          time.Time  `json:"startedAt"`
	SettledAt          *time.Time `json:"settledAt,omitempty"`
	Outcome            string     `json:"outcome"`
	Replaying          bool       `json:"-"`
}

// DHMutationSettlement is a caller-verified terminal provider response, NOT
// desired state observed by GET. Receipt names the endpoint-specific evidence.
// For unkeyed calls, success must cover ALL authorized phases and local writes.
type DHMutationSettlement struct {
	Outcome string
	Receipt string
}

// DHMutationGuards is the narrow shared integration port. Authorization is
// called only by authenticated explicit List after received/price/pause checks.
// Observation callbacks must only perform local persistence with scoped ctx.
type DHMutationGuards interface {
	AssertMutationAllowed(context.Context, string) error
	AuthorizeReturnedListing(context.Context, string, string) error
	IsReturnedOrder(context.Context, string, string) (bool, error)
	ObservationTime(context.Context) (time.Time, error)
	ApplyDHObservation(context.Context, string, int, time.Time, func(context.Context) error) (bool, error)
	ApplyDHSoldObservation(context.Context, string, string, int, time.Time, func(context.Context) error) (bool, error)
}

type DHMutationRepository interface {
	AssertDHPreparationAllowed(context.Context) error
	PrepareDHMutation(context.Context, string, DHMutationRequest) (*DHMutationAttempt, error)
	OwnDHMutation(context.Context, string, *DHMutationAttempt) error
	SettleDHMutation(context.Context, string, *DHMutationAttempt, DHMutationSettlement) error
}

// DHNonMutationError must be constructed ONLY at a source-verified pre-mutation
// provider guard covering the ENTIRE orchestration (not a sync rejection after
// a successful PATCH). Generic HTTP status classification is deliberately
// absent. Preserve any earlier uncertain transport evidence in Uncertain.
// It cannot acknowledge an older open request on replay.
type DHNonMutationError struct {
	Code      string
	Message   string
	Cause     error
	Uncertain bool
}

func NewDHNonMutationError(code, message string, cause error) *DHNonMutationError {
	return &DHNonMutationError{Code: code, Message: message, Cause: cause}
}
func (e *DHNonMutationError) Error() string { return e.Code + ": " + e.Message }
func (e *DHNonMutationError) Unwrap() error { return e.Cause }
func IsDefinitiveDHNonMutation(err error) bool {
	var e *DHNonMutationError
	return errors.As(err, &e) && !e.Uncertain && e.Code != "" && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

type DHMutationCoordinator struct {
	scope PurchaseMutationScope
	repo  DHMutationRepository
}

func NewDHMutationCoordinator(scope PurchaseMutationScope, repo DHMutationRepository) *DHMutationCoordinator {
	return &DHMutationCoordinator{scope: scope, repo: repo}
}

// Run commits prepare's local sale/key AND the attempt before execute. Call Run
// outside any owning scope; nested preparation is refused by the repository.
// Execute reloads and uses exactly a.Key; local rollback cannot erase prepare.
func (c *DHMutationCoordinator) Run(ctx context.Context, id string, prepare func(context.Context) (DHMutationRequest, error), execute func(context.Context, *DHMutationAttempt) (*DHMutationSettlement, error)) error {
	if c == nil || !DHDependenciesPresent(c.scope, c.repo, prepare, execute) {
		return NewReturnConflict("coordination_unavailable", "DH mutation coordination is required")
	}
	if err := c.repo.AssertDHPreparationAllowed(ctx); err != nil {
		return err
	}
	var attempt *DHMutationAttempt
	err := c.scope.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		request, e := prepare(owned)
		if e != nil {
			return e
		}
		attempt, e = c.repo.PrepareDHMutation(owned, id, request)
		return e
	})
	if err != nil {
		return err
	}
	var rejection *DHNonMutationError
	err = c.scope.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		if e := c.repo.OwnDHMutation(owned, id, attempt); e != nil {
			return e
		}
		result, e := execute(owned, attempt)
		if e != nil {
			errors.As(e, &rejection)
			return e
		}
		if result == nil {
			return NewReturnConflict("terminal_receipt_required", "no verified terminal response")
		}
		return c.repo.SettleDHMutation(owned, id, attempt, *result)
	})
	// Discard execute's local writes on rejection; settle its known nonmutation
	// in a new short transaction. Cancellation is not a reason to lose evidence.
	if err != nil && rejection != nil && IsDefinitiveDHNonMutation(err) && !attempt.Replaying {
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		e := c.scope.WithPurchaseMutation(persist, id, func(owned context.Context) error {
			if e := c.repo.OwnDHMutation(owned, id, attempt); e != nil {
				return e
			}
			return c.repo.SettleDHMutation(owned, id, attempt, DHMutationSettlement{Outcome: "rejected", Receipt: rejection.Code})
		})
		if e != nil {
			return errors.Join(err, e)
		}
	}
	return err
}
