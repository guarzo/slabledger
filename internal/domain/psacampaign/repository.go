package psacampaign

import (
	"context"
	"errors"
	"time"
)

// ErrPushNotPending is returned when Approve is called on a push-queue row
// that is not currently in the pending state.
var ErrPushNotPending = errors.New("psacampaign: push row is not pending")

// ErrDuplicateCreate is returned by Enqueue when an unresolved create proposal
// (pending/approved/pushing) already exists for the same internal campaign,
// enforced atomically by a partial unique index.
var ErrDuplicateCreate = errors.New("psacampaign: a create is already queued for this campaign")

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
}

// SnapshotStore persists the most recent portal campaign snapshot.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, campaigns []PortalCampaign) error
	GetSnapshot(ctx context.Context) ([]PortalCampaign, time.Time, error)
}

// PushQueueStore persists queued edits and their approval/push lifecycle.
type PushQueueStore interface {
	Enqueue(ctx context.Context, p PushRow) error
	Approve(ctx context.Context, id, approvedBy string) error
	ListByStatus(ctx context.Context, status PushStatus) ([]PushRow, error)
	MarkResult(ctx context.Context, id string, status PushStatus, resultJSON, errMsg string) error
	// Claim atomically transitions row id from approved to pushing, returning
	// true if the claim succeeded (i.e. the row was still approved). Used to
	// prevent double-push from concurrent DrainPushQueue runs.
	Claim(ctx context.Context, id string) (bool, error)
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
