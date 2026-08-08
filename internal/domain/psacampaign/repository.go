package psacampaign

import (
	"context"
	"errors"
	"time"
)

// ErrPushNotPending is returned when Approve is called on a push-queue row
// that is not currently in the pending state.
var ErrPushNotPending = errors.New("psacampaign: push row is not pending")

// ErrPushNotClaimed is returned when MarkResult is called for a row the caller
// does not hold a claim on — it is missing, or its status is not pushing.
// Recording an outcome is only valid for the drain run that claimed the row;
// without this, a slow or stale drain could overwrite a row another run had
// already claimed and pushed.
var ErrPushNotClaimed = errors.New("psacampaign: push row is not claimed for pushing")

// ErrDuplicateCreate is returned by Enqueue when an unresolved create proposal
// (pending/approved/pushing) already exists for the same internal campaign,
// enforced atomically by a partial unique index.
var ErrDuplicateCreate = errors.New("psacampaign: a create is already queued for this campaign")

// ErrPushNotFound is returned by Get when no push row has the requested id.
var ErrPushNotFound = errors.New("psacampaign: push row not found")

// PushRow is one queued edit awaiting approval/push to the PSA portal.
type PushRow struct {
	ID                 string
	Operation          Operation
	PSACampaignID      string
	InternalCampaignID string
	RequestedBy        string
	ApprovedBy         string
	Diff               ProposedDiff
	Status             PushStatus
	Error              string    // last push error, set when Status is failed
	UpdatedAt          time.Time // last lifecycle transition

	// Approval signature (see approval.go). These are written together by
	// Approve and are all empty/zero on a row that has not been approved, or on
	// one approved before signing existed.
	ApprovedAt        time.Time // when the approval was signed
	PayloadDigest     string    // hex SHA-256 of the canonical Diff at approval time
	ApprovalSignature string    // hex HMAC over the envelope from SigningInput
	SignatureKeyID    string    // which key signed, so rotation does not strand in-flight rows
}

// SnapshotStore persists the most recent portal campaign snapshot.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, campaigns []PortalCampaign) error
	GetSnapshot(ctx context.Context) ([]PortalCampaign, time.Time, error)
}

// PushQueueStore persists queued edits and their approval/push lifecycle.
type PushQueueStore interface {
	Enqueue(ctx context.Context, p PushRow) error
	// Get returns a single row by id, or ErrPushNotFound. The approve path
	// reads the row before approving so it can digest and sign the exact
	// payload being authorized.
	Get(ctx context.Context, id string) (PushRow, error)
	// Approve marks a pending row approved and records the signed approval
	// atomically with the status transition. It returns ErrPushNotPending if
	// the row is not currently pending.
	Approve(ctx context.Context, id string, a Approval) error
	ListByStatus(ctx context.Context, status PushStatus) ([]PushRow, error)
	// MarkResult records the outcome of a push attempt. Only valid for a row
	// the caller has claimed: it returns ErrPushNotClaimed unless the row is
	// still in the pushing state.
	MarkResult(ctx context.Context, id string, status PushStatus, resultJSON, errMsg string) error
	// Claim atomically transitions row id from approved to pushing, returning
	// true if the claim succeeded (i.e. the row was still approved). Used to
	// prevent double-push from concurrent DrainPushQueue runs.
	Claim(ctx context.Context, id string) (bool, error)
	// LatestPerCampaign returns the most recent push row for each internal
	// campaign (any status), for the UI's pending/in-flight/failed indicators.
	LatestPerCampaign(ctx context.Context) ([]PushRow, error)
}

// CampaignLinker writes the portal campaign id onto an internal campaign
// after a successful create push, and reads it back so a retried create can
// detect that the portal campaign already exists (idempotency guard).
type CampaignLinker interface {
	LinkPSACampaign(ctx context.Context, internalCampaignID, psaCampaignRequestID string) error
	// LinkedPSACampaignID returns the portal campaign id currently linked to
	// the internal campaign, or "" if none. Used by the drain to avoid
	// re-creating a portal campaign for a row whose prior create already
	// succeeded but failed to record its result.
	LinkedPSACampaignID(ctx context.Context, internalCampaignID string) (string, error)
}
