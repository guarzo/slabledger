package psacampaign

import (
	"context"
	"errors"
	"time"
)

// ErrPushNotPending is returned when Approve is called on a push-queue row
// that is not currently in the pending state.
var ErrPushNotPending = errors.New("psacampaign: push row is not pending")

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
