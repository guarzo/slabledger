package showprep_test

import (
	"context"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Catches stale consent deleting changed contents and retries consulting live state.
func TestDeleteShowList(t *testing.T) {
	for _, tt := range []struct {
		name          string
		member, stale bool
	}{
		{"empty exact", false, false}, {"members exact", true, false}, {"stale", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			list := sp.List{ID: showListID, Name: "Show", WithdrawalGeneration: 3}
			items := []sp.Item{}
			if tt.member {
				items = append(items, scanMember())
			}
			cmd := sp.DeleteListCommand{CommandID: commandID, ExpectedState: sp.ListStateFingerprint(list, items)}
			if tt.stale {
				list.Name = "Renamed"
			}
			var retired *sp.RetiredList
			m := &mocks.ShowPrepStoreMock{
				GetListFn:  func(context.Context, string) (sp.List, error) { return list, nil },
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return items, nil },
				GetRetiredListFn: func(context.Context, string) (sp.RetiredList, bool, error) {
					if retired != nil {
						return *retired, true, nil
					}
					return sp.RetiredList{}, false, nil
				},
				RetireListFn: func(_ context.Context, r sp.RetiredList) error { retired = &r; items = nil; return nil },
			}
			svc := sp.NewService(m, nil, func() time.Time { return fixedScanNow })
			result, err := svc.DeleteList(ctx, showListID, cmd)
			if tt.stale {
				require.ErrorIs(t, err, sp.ErrConflict)
				require.Nil(t, retired)
				require.Len(t, items, 1)
				return
			}
			require.NoError(t, err)
			require.Equal(t, sp.DeleteListResult{ListID: showListID, Deleted: true}, result)
			require.NotNil(t, retired)
			require.Equal(t, "2026-10-09T12:00:00Z", retired.RetiredAt)
			require.Equal(t, commandID, retired.DeleteCommandID)
			require.NotEmpty(t, retired.PayloadHash)
			m.GetListFn = func(context.Context, string) (sp.List, error) {
				t.Fatal("tombstone replay must precede live lookup")
				return sp.List{}, sp.ErrNotFound
			}
			m.RetireListFn = func(context.Context, sp.RetiredList) error { t.Fatal("replay must not write"); return nil }
			replay, err := svc.DeleteList(ctx, showListID, cmd)
			require.NoError(t, err)
			require.Equal(t, sp.DeleteListResult{ListID: showListID, Deleted: true, Replay: true}, replay)
			changed := cmd
			changed.ExpectedState = "altered"
			_, err = svc.DeleteList(ctx, showListID, changed)
			require.ErrorIs(t, err, sp.ErrConflict)
			changed = cmd
			changed.CommandID = "55555555-5555-4555-8555-555555555555"
			_, err = svc.DeleteList(ctx, showListID, changed)
			require.ErrorIs(t, err, sp.ErrRetired)
		})
	}
}

func TestDeleteShowListFailures(t *testing.T) {
	failure := errors.New("storage failed")
	for _, tt := range []struct {
		name   string
		want   error
		change func(*mocks.ShowPrepStoreMock)
	}{
		{"missing", sp.ErrNotFound, func(m *mocks.ShowPrepStoreMock) {
			m.GetListFn = func(context.Context, string) (sp.List, error) { return sp.List{}, sp.ErrNotFound }
		}},
		{"tombstone read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetRetiredListFn = func(context.Context, string) (sp.RetiredList, bool, error) { return sp.RetiredList{}, false, failure }
		}},
		{"receipt read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) {
				return sp.CommandReceipt{}, false, failure
			}
		}},
		{"command belongs to scan", sp.ErrConflict, func(m *mocks.ShowPrepStoreMock) {
			m.GetCommandReceiptFn = func(context.Context, string) (sp.CommandReceipt, bool, error) {
				return sp.CommandReceipt{CommandID: commandID, Kind: "scan"}, true, nil
			}
		}},
		{"items read", failure, func(m *mocks.ShowPrepStoreMock) {
			m.GetItemsFn = func(context.Context, string) ([]sp.Item, error) { return nil, failure }
		}},
		{"retirement", failure, func(m *mocks.ShowPrepStoreMock) {
			m.RetireListFn = func(context.Context, sp.RetiredList) error { return failure }
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{}
			tt.change(m)
			_, err := sp.NewService(m, nil, nil).DeleteList(context.Background(), showListID, sp.DeleteListCommand{CommandID: commandID, ExpectedState: sp.ListStateFingerprint(sp.List{ID: showListID}, nil)})
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestLifecycleCommandValidation(t *testing.T) {
	for _, tt := range []struct{ name, list, id, state string }{
		{"invalid list", "bad", commandID, "state"}, {"invalid command", showListID, "bad", "state"}, {"empty state", showListID, commandID, ""}, {"padded state", showListID, commandID, " state "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{WithinFn: func(context.Context, func(sp.Session) error) error {
				t.Fatal("malformed commands cannot transact")
				return nil
			}}
			svc := sp.NewService(m, nil, nil)
			_, err := svc.DeleteList(context.Background(), tt.list, sp.DeleteListCommand{CommandID: tt.id, ExpectedState: tt.state})
			require.ErrorIs(t, err, sp.ErrInvalid)
			_, err = svc.ConfirmExisting(context.Background(), tt.list, sp.ConfirmExistingCommand{CommandID: tt.id, ExpectedState: tt.state, ItemIDs: []string{}})
			require.ErrorIs(t, err, sp.ErrInvalid)
		})
	}
}

func TestScanConfirmRetiredShow(t *testing.T) {
	for _, retired := range []bool{false, true} {
		t.Run(map[bool]string{true: "retired", false: "missing"}[retired], func(t *testing.T) {
			m := &mocks.ShowPrepStoreMock{GetListFn: func(context.Context, string) (sp.List, error) { return sp.List{}, sp.ErrNotFound }, GetRetiredListFn: func(context.Context, string) (sp.RetiredList, bool, error) {
				return sp.RetiredList{ListID: showListID}, retired, nil
			}}
			_, err := sp.NewService(m, nil, nil).ScanConfirm(context.Background(), showListID, scanCommand())
			if retired {
				require.ErrorIs(t, err, sp.ErrRetired)
				require.NotErrorIs(t, err, sp.ErrNotFound)
			} else {
				require.ErrorIs(t, err, sp.ErrNotFound)
			}
		})
	}
}
