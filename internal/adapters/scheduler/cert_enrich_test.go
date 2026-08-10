package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
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

// A cert PSA cannot serve images for must be asked exactly once per process.
// The BACKFILL_IMAGES sweep re-enqueues the same rows on every restart, so
// without this the doomed certs drain the daily quota indefinitely (SLA-108).
func TestEnrichImages_FailedCertNotRetried(t *testing.T) {
	cases := []struct {
		name           string
		lookupImagesFn func(context.Context, string) (string, string, error)
	}{
		{
			name: "per-cert error",
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				return "", "", errors.New("PSA API request: status 500")
			},
		},
		{
			name: "PSA has no images",
			lookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
				return "", "", nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var calls int
			certLookup := &mocks.MockCertLookup{
				LookupImagesFn: func(c context.Context, cert string) (string, string, error) {
					calls++
					return tc.lookupImagesFn(c, cert)
				},
			}
			repo := &mocks.PurchaseRepositoryMock{}
			job := newEnrichJob(certLookup, repo)

			for range 3 {
				job.enrichImages(ctx, &inventory.Purchase{ID: "p1", CertNumber: "C1"}, "C1")
			}

			if calls != 1 {
				t.Errorf("LookupImages called %d times across 3 sweeps, want 1", calls)
			}
		})
	}
}

// TestEnrichImages_PoolWideFailureDoesNotRetireCert is the counterweight to
// TestEnrichImages_FailedCertNotRetried. Quota exhaustion is the ordinary
// end-of-day state for a two-token PSA pool, and it says nothing about the cert
// being looked up. Marking on it would retire every cert the sweep touched and
// kill image backfill for the life of the process, which — with deploy-on-push
// — can be days (SLA-108).
func TestEnrichImages_PoolWideFailureDoesNotRetireCert(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "quota exhausted", err: apperrors.ProviderRateLimited("PSA", "")},
		{name: "every token rejected", err: apperrors.ProviderAuthFailed("PSA", errors.New("all keys rejected"))},
		{name: "circuit breaker open", err: apperrors.ProviderCircuitOpen("PSA")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var calls int
			certLookup := &mocks.MockCertLookup{
				LookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
					calls++
					// Recover once PSA is usable again, as a real outage would.
					if calls == 1 {
						return "", "", tc.err
					}
					return "https://psa.example/f.jpg", "https://psa.example/b.jpg", nil
				},
			}
			var updated string
			repo := &mocks.PurchaseRepositoryMock{
				UpdatePurchaseImagesFn: func(_ context.Context, id, _, _ string) error {
					updated = id
					return nil
				},
			}
			job := newEnrichJob(certLookup, repo)

			job.enrichImages(ctx, &inventory.Purchase{ID: "p1", CertNumber: "C1"}, "C1")
			job.enrichImages(ctx, &inventory.Purchase{ID: "p1", CertNumber: "C1"}, "C1")

			if calls != 2 {
				t.Errorf("LookupImages called %d times, want 2 (the cert must stay eligible)", calls)
			}
			if updated != "p1" {
				t.Errorf("images were not backfilled after PSA recovered; updated = %q", updated)
			}
		})
	}
}

// The skip is per-cert, not global: one bad cert must not stop the sweep.
func TestEnrichImages_SkipIsPerCert(t *testing.T) {
	ctx := context.Background()
	certLookup := &mocks.MockCertLookup{
		LookupImagesFn: func(_ context.Context, cert string) (string, string, error) {
			if cert == "BAD" {
				return "", "", errors.New("status 500")
			}
			return "https://psa.example/f.jpg", "https://psa.example/b.jpg", nil
		},
	}
	var updated string
	repo := &mocks.PurchaseRepositoryMock{
		UpdatePurchaseImagesFn: func(_ context.Context, id, _, _ string) error {
			updated = id
			return nil
		},
	}
	job := newEnrichJob(certLookup, repo)

	job.enrichImages(ctx, &inventory.Purchase{ID: "bad", CertNumber: "BAD"}, "BAD")
	job.enrichImages(ctx, &inventory.Purchase{ID: "good", CertNumber: "GOOD"}, "GOOD")

	if updated != "good" {
		t.Errorf("healthy cert was not backfilled; UpdatePurchaseImages id = %q, want %q", updated, "good")
	}
}

// The backfill sweep only needs images. Spending a PSA cert lookup on rows
// that already carry their metadata doubles the sweep's quota cost.
func TestEnrichImagesOnly_SkipsCertLookup(t *testing.T) {
	ctx := context.Background()
	certLookup := &mocks.MockCertLookup{
		LookupCertFn: func(_ context.Context, _ string) (*inventory.CertInfo, error) {
			t.Fatal("LookupCert must not be called on the images-only path")
			return nil, nil
		},
		LookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
			return "https://psa.example/f.jpg", "https://psa.example/b.jpg", nil
		},
	}
	var updated string
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseByCertNumberFn: func(_ context.Context, _, cert string) (*inventory.Purchase, error) {
			return &inventory.Purchase{ID: "p1", CertNumber: cert}, nil
		},
		UpdatePurchaseImagesFn: func(_ context.Context, id, _, _ string) error {
			updated = id
			return nil
		},
	}

	newEnrichJob(certLookup, repo).enrichImagesOnly(ctx, "C1")

	if updated != "p1" {
		t.Errorf("images-only backfill did not update the purchase; got id %q", updated)
	}
}

// EnqueueImagesOnly must route to the images-only path, not the full enrich.
func TestEnqueueImagesOnly_RoutesToImagesOnlyPath(t *testing.T) {
	job := newEnrichJob(&mocks.MockCertLookup{}, &mocks.PurchaseRepositoryMock{})

	job.Enqueue("A")
	job.EnqueueImagesOnly("B")

	got := []enrichRequest{<-job.ch, <-job.ch}
	want := []enrichRequest{
		{certNumber: "A", imagesOnly: false},
		{certNumber: "B", imagesOnly: true},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("queued request %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
