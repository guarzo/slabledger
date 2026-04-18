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

func TestEnrichImages_BackfillsMissingImages(t *testing.T) {
	ctx := context.Background()
	purchase := &inventory.Purchase{ID: "p1", CertNumber: "C1"}

	certLookup := &mocks.MockCertLookup{
		LookupImagesFn: func(_ context.Context, cert string) (string, string, error) {
			if cert != "C1" {
				t.Fatalf("unexpected cert: %q", cert)
			}
			return "https://psa.example/front.jpg", "https://psa.example/back.jpg", nil
		},
	}

	var gotID, gotFront, gotBack string
	repo := &mocks.PurchaseRepositoryMock{
		UpdatePurchaseImagesFn: func(_ context.Context, id, front, back string) error {
			gotID, gotFront, gotBack = id, front, back
			return nil
		},
	}

	newEnrichJob(certLookup, repo).enrichImages(ctx, purchase, "C1")

	if gotID != "p1" || gotFront != "https://psa.example/front.jpg" || gotBack != "https://psa.example/back.jpg" {
		t.Fatalf("UpdatePurchaseImages got (%q, %q, %q), want (p1, front, back)", gotID, gotFront, gotBack)
	}
}

func TestEnrichImages_SkipsWhenImageAlreadySet(t *testing.T) {
	ctx := context.Background()
	purchase := &inventory.Purchase{ID: "p1", CertNumber: "C1", FrontImageURL: "existing"}

	certLookup := &mocks.MockCertLookup{
		LookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
			t.Fatal("LookupImages should not be called when image already set")
			return "", "", nil
		},
	}
	repo := &mocks.PurchaseRepositoryMock{
		UpdatePurchaseImagesFn: func(_ context.Context, _, _, _ string) error {
			t.Fatal("UpdatePurchaseImages should not be called")
			return nil
		},
	}

	newEnrichJob(certLookup, repo).enrichImages(ctx, purchase, "C1")
}

func TestEnrichImages_SwallowsLookupError(t *testing.T) {
	ctx := context.Background()
	purchase := &inventory.Purchase{ID: "p1", CertNumber: "C1"}

	certLookup := &mocks.MockCertLookup{
		LookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", errors.New("PSA rate limited")
		},
	}
	repo := &mocks.PurchaseRepositoryMock{
		UpdatePurchaseImagesFn: func(_ context.Context, _, _, _ string) error {
			t.Fatal("UpdatePurchaseImages should not be called on lookup error")
			return nil
		},
	}

	newEnrichJob(certLookup, repo).enrichImages(ctx, purchase, "C1")
}

func TestEnrichImages_SkipsWhenPSAReturnsNoImages(t *testing.T) {
	ctx := context.Background()
	purchase := &inventory.Purchase{ID: "p1", CertNumber: "C1"}

	certLookup := &mocks.MockCertLookup{
		LookupImagesFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", nil
		},
	}
	repo := &mocks.PurchaseRepositoryMock{
		UpdatePurchaseImagesFn: func(_ context.Context, _, _, _ string) error {
			t.Fatal("UpdatePurchaseImages should not be called when both URLs empty")
			return nil
		},
	}

	newEnrichJob(certLookup, repo).enrichImages(ctx, purchase, "C1")
}
