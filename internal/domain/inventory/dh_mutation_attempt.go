package inventory

import (
	"context"
	"errors"
	"time"
)

// DHMutationRequest describes ONE immutable outbound orchestration. The payload
// identity must cover every request field and phase (including sync/revert).
// Keyed sale/return/live-price replay their receipts. Listing can resume only
// with persisted PATCH-retry permission and an explicit same-payload request.
type DHMutationRequest struct {
	AllowListingReplay bool
	Kind               string
	Phase              string
	Key                string
	PayloadIdentity    string
	ReturnOperationID  string
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
	RetryNotBefore     *time.Time `json:"retryNotBefore,omitempty"`
	Outcome            string     `json:"outcome"`
	Replaying          bool       `json:"-"`
	Retryable          bool       `json:"retryable,omitempty"`
}

// DHMutationSettlement is a caller-verified terminal provider response, NOT
// desired state observed by GET. Receipt names the endpoint-specific evidence.
// For unkeyed calls, success must cover ALL authorized phases and local writes.
type DHMutationSettlement struct {
	Outcome string
	Receipt string
	// A price-only execution with no local writes can commit its cooldown
	// under uninterrupted ownership. This is not terminal evidence.
	RetryNotBefore *time.Time
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
	var retry *DHMutationRetryError
	var listingRetry *DHListingPatchRetryError
	executionEntered := false
	err = c.scope.WithPurchaseMutation(ctx, id, func(owned context.Context) error {
		if e := c.repo.OwnDHMutation(owned, id, attempt); e != nil {
			return e
		}
		executionEntered = true
		result, e := execute(owned, attempt)
		if e != nil {
			errors.As(e, &rejection)
			errors.As(e, &retry)
			errors.As(e, &listingRetry)
			return e
		}
		if result == nil {
			return NewReturnConflict("terminal_receipt_required", "no verified terminal response")
		}
		if result.RetryNotBefore != nil {
			if attempt.Kind != "price" || attempt.Phase != "live_price" || attempt.Key == "" || result.Outcome != "" || result.Receipt != "" {
				return NewReturnConflict("invalid_mutation_deferral", "only a keyed price-only response can commit a nonterminal cooldown")
			}
			// Retain the received deadline if cancellation or COMMIT failure
			// prevents the preferred atomic deferral from becoming durable.
			retry = &DHMutationRetryError{NotBefore: *result.RetryNotBefore}
			retries, ok := c.repo.(DHMutationRetryRepository)
			if !ok {
				return NewReturnConflict("coordination_unavailable", "durable DH retry persistence required")
			}
			return retries.DeferDHMutation(owned, id, attempt, *result.RetryNotBefore)
		}
		return c.repo.SettleDHMutation(owned, id, attempt, *result)
	})
	// Rate limits keep uncertainty open. Atomic live-price deferral is preferred;
	// retain its deadline after a failed commit or a caller-cancelled execution.
	if err != nil && retry != nil {
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		e := c.scope.WithPurchaseMutation(persist, id, func(owned context.Context) error {
			if e := c.repo.OwnDHMutation(owned, id, attempt); e != nil {
				return e
			}
			if repo, ok := c.repo.(DHMutationRetryRepository); ok {
				return repo.DeferDHMutation(owned, id, attempt, retry.NotBefore)
			}
			return NewReturnConflict("coordination_unavailable", "durable DH retry persistence required")
		})
		return errors.Join(err, e)
	}
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
		return errors.Join(err, e)
	}
	// A consumed replay that failed before execute never sent a new PATCH.
	// Restore its permission even if ownership/identity guards refused execution.
	if err != nil && (listingRetry != nil || (!executionEntered && attempt.Replaying && attempt.Kind == "list" && attempt.Phase == "intake_patch_sync")) {
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		e := c.scope.WithPurchaseMutation(persist, id, func(owned context.Context) error {
			repo, ok := c.repo.(DHListingRetryRepository)
			if !ok {
				return NewReturnConflict("coordination_unavailable", "durable listing retry persistence required")
			}
			return repo.EnableDHListingRetry(owned, id, attempt)
		})
		return errors.Join(err, e)
	}
	return err
}
