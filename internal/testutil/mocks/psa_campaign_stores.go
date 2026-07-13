package mocks

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// SnapshotStoreMock implements psacampaign.SnapshotStore with the Fn-field pattern.
type SnapshotStoreMock struct {
	SaveSnapshotFn func(ctx context.Context, campaigns []psacampaign.PortalCampaign) error
	GetSnapshotFn  func(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error)
}

var _ psacampaign.SnapshotStore = (*SnapshotStoreMock)(nil)

func (m *SnapshotStoreMock) SaveSnapshot(ctx context.Context, campaigns []psacampaign.PortalCampaign) error {
	if m.SaveSnapshotFn != nil {
		return m.SaveSnapshotFn(ctx, campaigns)
	}
	return nil
}

func (m *SnapshotStoreMock) GetSnapshot(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
	if m.GetSnapshotFn != nil {
		return m.GetSnapshotFn(ctx)
	}
	return []psacampaign.PortalCampaign{}, time.Time{}, nil
}

// PushQueueStoreMock implements psacampaign.PushQueueStore with the Fn-field pattern.
type PushQueueStoreMock struct {
	EnqueueFn      func(ctx context.Context, p psacampaign.PushRow) error
	ApproveFn      func(ctx context.Context, id, approvedBy string) error
	ListByStatusFn func(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error)
	MarkResultFn   func(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error
	ClaimFn        func(ctx context.Context, id string) (bool, error)
}

var _ psacampaign.PushQueueStore = (*PushQueueStoreMock)(nil)

func (m *PushQueueStoreMock) Enqueue(ctx context.Context, p psacampaign.PushRow) error {
	if m.EnqueueFn != nil {
		return m.EnqueueFn(ctx, p)
	}
	return nil
}

func (m *PushQueueStoreMock) Approve(ctx context.Context, id, approvedBy string) error {
	if m.ApproveFn != nil {
		return m.ApproveFn(ctx, id, approvedBy)
	}
	return nil
}

func (m *PushQueueStoreMock) ListByStatus(ctx context.Context, status psacampaign.PushStatus) ([]psacampaign.PushRow, error) {
	if m.ListByStatusFn != nil {
		return m.ListByStatusFn(ctx, status)
	}
	return []psacampaign.PushRow{}, nil
}

func (m *PushQueueStoreMock) MarkResult(ctx context.Context, id string, status psacampaign.PushStatus, resultJSON, errMsg string) error {
	if m.MarkResultFn != nil {
		return m.MarkResultFn(ctx, id, status, resultJSON, errMsg)
	}
	return nil
}

func (m *PushQueueStoreMock) Claim(ctx context.Context, id string) (bool, error) {
	if m.ClaimFn != nil {
		return m.ClaimFn(ctx, id)
	}
	return true, nil
}
