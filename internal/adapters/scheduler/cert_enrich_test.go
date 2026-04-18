package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newEnrichJob(certLookup inventory.CertLookup, repo inventory.PurchaseRepository) *CertEnrichJob {
	return NewCertEnrichJob(certLookup, repo, mocks.NewMockLogger())
}

func TestEnrichImages(t *testing.T) {
	type updateCall struct {
		id, front, back string
	}

	cases := []struct {
		name           string
		purchase       *inventory.Purchase
		lookupImagesFn func(context.Context, string) (string, string, error)
		wantUpdate     *updateCall // nil = UpdatePurchaseImages must NOT be called
	}{
		{
			name:     "backfills when both image fields empty",
			purchase: &inventory.Purchase{ID: "p1", CertNumber: "C1"},
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				return "https://psa.example/front.jpg", "https://psa.example/back.jpg", nil
			},
			wantUpdate: &updateCall{id: "p1", front: "https://psa.example/front.jpg", back: "https://psa.example/back.jpg"},
		},
		{
			name:     "skips when front image already set",
			purchase: &inventory.Purchase{ID: "p1", CertNumber: "C1", FrontImageURL: "existing"},
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				t.Fatal("LookupImages should not be called when image already set")
				return "", "", nil
			},
			wantUpdate: nil,
		},
		{
			name:     "swallows lookup error",
			purchase: &inventory.Purchase{ID: "p1", CertNumber: "C1"},
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				return "", "", errors.New("PSA rate limited")
			},
			wantUpdate: nil,
		},
		{
			name:     "skips update when PSA returns no images",
			purchase: &inventory.Purchase{ID: "p1", CertNumber: "C1"},
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				return "", "", nil
			},
			wantUpdate: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			certLookup := &mocks.MockCertLookup{LookupImagesFn: tc.lookupImagesFn}

			var got *updateCall
			repo := &mocks.PurchaseRepositoryMock{
				UpdatePurchaseImagesFn: func(_ context.Context, id, front, back string) error {
					if tc.wantUpdate == nil {
						t.Fatalf("UpdatePurchaseImages unexpectedly called with (%q, %q, %q)", id, front, back)
					}
					got = &updateCall{id: id, front: front, back: back}
					return nil
				},
			}

			newEnrichJob(certLookup, repo).enrichImages(ctx, tc.purchase, tc.purchase.CertNumber)

			if tc.wantUpdate == nil {
				return
			}
			if got == nil {
				t.Fatal("UpdatePurchaseImages was not called")
			}
			if *got != *tc.wantUpdate {
				t.Errorf("UpdatePurchaseImages = %+v, want %+v", *got, *tc.wantUpdate)
			}
		})
	}
}
