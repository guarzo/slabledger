package inventory

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

func internalTestIDGen() func() string {
	var counter atomic.Int64
	return func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }
}

func TestService_ReassignPurchase(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c1 := &Campaign{Name: "Source", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Target", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	p := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "MOVE001", GradeValue: 9, BuyCostCents: 15000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p)

	if err := svc.ReassignPurchase(ctx, p.ID, c2.ID); err != nil {
		t.Fatalf("ReassignPurchase: %v", err)
	}

	// Verify purchase moved
	moved, _ := repo.GetPurchase(ctx, p.ID)
	if moved.CampaignID != c2.ID {
		t.Errorf("CampaignID = %s, want %s", moved.CampaignID, c2.ID)
	}
	if moved.PSASourcingFeeCents != 500 {
		t.Errorf("PSASourcingFeeCents = %d, want 500", moved.PSASourcingFeeCents)
	}
}

func TestService_ReassignPurchase_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Target"}
	_ = svc.CreateCampaign(ctx, c)

	err := svc.ReassignPurchase(ctx, "nonexistent", c.ID)
	if err == nil {
		t.Error("expected error for nonexistent purchase")
	}
}
