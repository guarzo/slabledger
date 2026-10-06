package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func saleCheckState() *inventory.ConfirmedReturnState {
	return &inventory.ConfirmedReturnState{Purchase: &inventory.Purchase{ID: "p1", DHInventoryID: 147840, CertNumber: "160944741", Grader: "PSA"}}
}

func TestDHSaleCheckReadOnlyExactStoredTarget(t *testing.T) {
	reads := 0
	repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(_ context.Context, id string) (*inventory.ConfirmedReturnState, error) {
		require.Equal(t, "p1", id)
		reads++
		return saleCheckState(), nil
	}}
	remote := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(_ context.Context, id int, cert string) (string, error) {
		require.Equal(t, 147840, id)
		require.Equal(t, "160944741", cert)
		return "sold", nil
	}, ReturnInventoryToStockFn: func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		t.Fatal("GET must not return inventory")
		return nil, nil
	}}
	result, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
	require.NoError(t, err)
	require.Equal(t, &inventory.DHSaleCheck{Status: "sold", Resolvable: true, Target: inventory.ReturnTargetIdentity{DHInventoryID: 147840, CertNumber: "160944741", Grader: "PSA"}}, result)
	require.Equal(t, 2, reads, "re-read durable state after provider read")
}

func TestDHSaleCheckBlocksUnsafeStatesWithoutProviderCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*inventory.ConfirmedReturnState)
	}{
		{"local sale", func(s *inventory.ConfirmedReturnState) { s.Sale = &inventory.Sale{ID: "sale"} }},
		{"unlinked purchase", func(s *inventory.ConfirmedReturnState) { s.Purchase.DHInventoryID = 0 }},
		{"missing cert", func(s *inventory.ConfirmedReturnState) { s.Purchase.CertNumber = "" }},
		{"open attempt", func(s *inventory.ConfirmedReturnState) {
			s.PrecedingAttempt = &inventory.DHMutationAttempt{ID: "attempt", Kind: "list"}
		}},
		{"pending return", func(s *inventory.ConfirmedReturnState) {
			s.Operation = &inventory.ConfirmedReturnEpisode{ID: "ep", State: "pending"}
		}},
		{"completed null-sale return", func(s *inventory.ConfirmedReturnState) {
			s.Operation = &inventory.ConfirmedReturnEpisode{ID: "ep", State: "completed"}
		}},
		{"completed non-null-sale return", func(s *inventory.ConfirmedReturnState) {
			sale := "old"
			s.Operation = &inventory.ConfirmedReturnEpisode{ID: "ep", State: "completed", ExpectedSaleID: &sale}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := saleCheckState()
			tc.alter(s)
			repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return s, nil }}
			remote := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) {
				t.Fatal("unsafe state must not read provider")
				return "sold", nil
			}, ReturnInventoryToStockFn: func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				t.Fatal("GET must not mutate provider")
				return nil, nil
			}}
			result, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
			require.NoError(t, err)
			require.False(t, result.Resolvable)
			require.NotEmpty(t, result.Reason)
		})
	}
}

func TestDHSaleCheckProviderStatusAndFailures(t *testing.T) {
	for _, tc := range []struct {
		status  string
		wantErr bool
	}{
		{"in_stock", false}, {"listed", false}, {"sold", false}, {"", true}, {"future_status", true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return saleCheckState(), nil }}
			remote := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) { return tc.status, nil }}
			result, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.status == "sold", result.Resolvable)
			require.Equal(t, tc.status, result.Status)
		})
	}
	for _, remote := range []inventory.DHReturner{nil, (*mocks.DHReturnerMock)(nil)} {
		repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return saleCheckState(), nil }}
		_, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
		var conflict *inventory.ReturnConflict
		require.ErrorAs(t, err, &conflict)
		require.Equal(t, "coordination_unavailable", conflict.Code)
	}
}

func TestDHSaleCheckRejectsChangesDuringRemoteRead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*inventory.ConfirmedReturnState)
	}{
		{"target", func(s *inventory.ConfirmedReturnState) { s.Purchase.DHInventoryID++ }},
		{"sale", func(s *inventory.ConfirmedReturnState) { s.Sale = &inventory.Sale{ID: "new"} }},
		{"episode", func(s *inventory.ConfirmedReturnState) {
			s.Operation = &inventory.ConfirmedReturnEpisode{ID: "ep", State: "completed"}
		}},
		{"attempt", func(s *inventory.ConfirmedReturnState) {
			s.PrecedingAttempt = &inventory.DHMutationAttempt{ID: "attempt"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
				reads++
				s := saleCheckState()
				if reads == 2 {
					tc.alter(s)
				}
				return s, nil
			}}
			remote := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) { return "sold", nil }}
			result, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
			require.NoError(t, err)
			require.False(t, result.Resolvable)
			require.NotEmpty(t, result.Reason)
			require.Equal(t, 2, reads)
		})
	}
}

func TestDHSaleCheckReadErrorDoesNotOfferResolution(t *testing.T) {
	repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return saleCheckState(), nil }}
	remote := &mocks.DHReturnerMock{GetReturnInventoryStatusFn: func(context.Context, int, string) (string, error) { return "", errors.New("provider unavailable") }}
	result, err := inventory.NewConfirmedReturnService(nil, repo, remote, nil, nil).CheckDHSale(context.Background(), "p1")
	require.ErrorContains(t, err, "provider unavailable")
	require.Nil(t, result)
}
