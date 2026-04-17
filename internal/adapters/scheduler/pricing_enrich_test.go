package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePricer is a thread-safe SinglePurchasePricer stub.
type fakePricer struct {
	name     string
	called   chan string
	err      error
	failMode bool
}

func newFakePricer(name string, bufferSize int) *fakePricer {
	return &fakePricer{name: name, called: make(chan string, bufferSize)}
}

func (f *fakePricer) PriceSinglePurchase(_ context.Context, p *inventory.Purchase) error {
	f.called <- p.CertNumber
	if f.failMode {
		return errors.New(f.name + " failed")
	}
	return f.err
}

// recvCerts drains up to n cert numbers from a pricer's call channel. Fails the
// test if they don't arrive within the timeout.
func recvCerts(t *testing.T, ch <-chan string, n int, timeout time.Duration) []string {
	t.Helper()
	out := make([]string, 0, n)
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case c := <-ch:
			out = append(out, c)
		case <-deadline:
			t.Fatalf("timed out waiting for %d pricer calls (got %d)", n, len(out))
		}
	}
	return out
}

// fakePurchaseRepo is a PurchaseRepository stub that only implements the one
// method PricingEnrichJob needs.
type fakePurchaseRepo struct {
	inventory.PurchaseRepository
	mu    sync.Mutex
	byKey map[string]*inventory.Purchase
}

func newFakePurchaseRepo() *fakePurchaseRepo {
	return &fakePurchaseRepo{byKey: map[string]*inventory.Purchase{}}
}

func (r *fakePurchaseRepo) put(cert string, p *inventory.Purchase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byKey[cert] = p
}

func (r *fakePurchaseRepo) GetPurchaseByCertNumber(_ context.Context, _ string, cert string) (*inventory.Purchase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byKey[cert], nil
}

func TestPricingEnrichJob_EnqueueFansOutToAllPricers(t *testing.T) {
	repo := newFakePurchaseRepo()
	repo.put("C1", &inventory.Purchase{ID: "p1", CertNumber: "C1", CardName: "Charizard"})

	cl := newFakePricer("cl", 2)
	mm := newFakePricer("mm", 2)
	job := NewPricingEnrichJob(repo, mocks.NewMockLogger(), cl, mm)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.Start(ctx)
	defer func() {
		cancel()
		job.Stop()
		job.WG().Wait()
	}()

	job.Enqueue("C1")

	assert.Equal(t, []string{"C1"}, recvCerts(t, cl.called, 1, 2*time.Second))
	assert.Equal(t, []string{"C1"}, recvCerts(t, mm.called, 1, 2*time.Second))
}

func TestPricingEnrichJob_EnqueueBeforeSetPricersIsNoOp(t *testing.T) {
	repo := newFakePurchaseRepo()
	repo.put("C1", &inventory.Purchase{ID: "p1", CertNumber: "C1"})

	job := NewPricingEnrichJob(repo, mocks.NewMockLogger())

	// No pricers attached — Enqueue should silently drop the cert.
	job.Enqueue("C1")

	// Starting the worker with nothing queued must still exit cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	job.Start(ctx)
	cancel()
	job.Stop()

	done := make(chan struct{})
	go func() { job.WG().Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exit after Stop")
	}
}

func TestPricingEnrichJob_OnePricerFailureDoesNotBlockOther(t *testing.T) {
	repo := newFakePurchaseRepo()
	repo.put("C2", &inventory.Purchase{ID: "p2", CertNumber: "C2"})

	failing := newFakePricer("cl", 1)
	failing.failMode = true
	working := newFakePricer("mm", 1)

	job := NewPricingEnrichJob(repo, mocks.NewMockLogger(), failing, working)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.Start(ctx)
	defer func() {
		cancel()
		job.Stop()
		job.WG().Wait()
	}()

	job.Enqueue("C2")

	require.Equal(t, []string{"C2"}, recvCerts(t, failing.called, 1, 2*time.Second))
	require.Equal(t, []string{"C2"}, recvCerts(t, working.called, 1, 2*time.Second),
		"second pricer must still run after first returns an error")
}

func TestPricingEnrichJob_SetPricersLateUnblocksEnqueue(t *testing.T) {
	repo := newFakePurchaseRepo()
	repo.put("C3", &inventory.Purchase{ID: "p3", CertNumber: "C3"})

	job := NewPricingEnrichJob(repo, mocks.NewMockLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.Start(ctx)
	defer func() {
		cancel()
		job.Stop()
		job.WG().Wait()
	}()

	// No pricers → Enqueue drops.
	job.Enqueue("C3")

	pricer := newFakePricer("cl", 1)
	job.SetPricers(pricer)
	job.Enqueue("C3")

	assert.Equal(t, []string{"C3"}, recvCerts(t, pricer.called, 1, 2*time.Second))
}
