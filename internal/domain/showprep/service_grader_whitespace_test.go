package showprep_test

import (
	"context"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
)

func TestConfirmationGraderWhitespace(t *testing.T) {
	for _, tt := range []struct {
		name, grader string
		matches      bool
	}{
		{"surrounding whitespace and case", " \tpsa\n ", true},
		{"different grader remains rejected", " BGS ", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("scan", func(t *testing.T) {
				p := scanPurchase()
				p.Grader = tt.grader
				items := []sp.Item{scanMember()}
				receipts := []sp.CommandReceipt{}
				store := scanStore(p, &items, &receipts)
				result, err := sp.NewService(store, nil, func() time.Time { return fixedScanNow }).ScanConfirm(context.Background(), showListID, scanCommand())
				require.NoError(t, err)
				require.Len(t, receipts, 1)
				if tt.matches {
					require.Equal(t, "already_confirmed", result.Original.Outcome)
					require.Empty(t, result.Original.Code)
				} else {
					require.Equal(t, "rejected", result.Original.Outcome)
					require.Equal(t, "identity_changed", result.Original.Code)
				}
				require.Equal(t, scanMember(), items[0])
			})
			t.Run("existing cohort", func(t *testing.T) {
				list, items, purchases := transitionFixture()
				p := purchases[scanPurchaseID]
				p.Grader = tt.grader
				purchases[scanPurchaseID] = p
				receipts := []sp.CommandReceipt{}
				store := transitionMock(list, &items, purchases, &receipts)
				result, err := sp.NewService(store, nil, func() time.Time { return fixedScanNow }).ConfirmExisting(context.Background(), showListID, transitionCommand(list, items))
				if tt.matches {
					require.NoError(t, err)
					require.Len(t, result.ConfirmedItemIDs, 2)
					require.Len(t, receipts, 1)
				} else {
					require.ErrorIs(t, err, sp.ErrConflict)
					require.Empty(t, receipts)
				}
			})
		})
	}
}
