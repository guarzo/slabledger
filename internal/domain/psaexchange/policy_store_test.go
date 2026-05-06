package psaexchange

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubPolicyStore struct {
	policy   Policy
	have     bool
	getErr   error
	setCalls []Policy
	setErr   error
}

func (s *stubPolicyStore) Get(_ context.Context) (Policy, bool, error) {
	if s.getErr != nil {
		return Policy{}, false, s.getErr
	}
	return s.policy, s.have, nil
}

func (s *stubPolicyStore) Set(_ context.Context, p Policy) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.setCalls = append(s.setCalls, p)
	s.policy = p
	s.have = true
	return nil
}

func TestEffectivePolicy_FallsBackToSeedWhenStoreEmpty(t *testing.T) {
	seed := DefaultPolicy()
	seed.HighLiquidityOfferPct = 0.8 // distinguish from default
	store := &stubPolicyStore{}
	svc := newServiceForTest(t, seed, store)

	got := svc.EffectivePolicy(context.Background())
	if got.HighLiquidityOfferPct != 0.8 {
		t.Fatalf("expected seed (0.8), got %v", got.HighLiquidityOfferPct)
	}
}

func TestEffectivePolicy_PrefersStoreRowOverSeed(t *testing.T) {
	seed := DefaultPolicy()
	stored := DefaultPolicy()
	stored.HighLiquidityOfferPct = 0.9
	store := &stubPolicyStore{policy: stored, have: true}
	svc := newServiceForTest(t, seed, store)

	got := svc.EffectivePolicy(context.Background())
	if got.HighLiquidityOfferPct != 0.9 {
		t.Fatalf("expected stored (0.9), got %v", got.HighLiquidityOfferPct)
	}
}

func TestEffectivePolicy_FallsBackOnStoreError(t *testing.T) {
	seed := DefaultPolicy()
	store := &stubPolicyStore{getErr: errors.New("boom")}
	svc := newServiceForTest(t, seed, store)

	got := svc.EffectivePolicy(context.Background())
	if got != seed {
		t.Fatalf("expected seed on store error, got %+v", got)
	}
}

func TestSetPolicy_PersistsAndInvalidatesCache(t *testing.T) {
	store := &stubPolicyStore{}
	svc := newServiceForTest(t, DefaultPolicy(), store)
	ctx := context.Background()

	// Prime the cache with the seed (no store row yet).
	_ = svc.EffectivePolicy(ctx)

	updated := DefaultPolicy()
	updated.HighLiquidityOfferPct = 0.85
	if err := svc.SetPolicy(ctx, updated); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if len(store.setCalls) != 1 {
		t.Fatalf("expected 1 set call, got %d", len(store.setCalls))
	}
	got := svc.EffectivePolicy(ctx)
	if got.HighLiquidityOfferPct != 0.85 {
		t.Fatalf("post-set EffectivePolicy returned stale value: %v", got.HighLiquidityOfferPct)
	}
}

func TestSetPolicy_RejectsInvalid(t *testing.T) {
	svc := newServiceForTest(t, DefaultPolicy(), &stubPolicyStore{})
	bad := DefaultPolicy()
	bad.DefaultOfferPct = 0.9 // > HighLiquidityOfferPct (0.75)
	err := svc.SetPolicy(context.Background(), bad)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("expected ErrInvalidPolicy, got %v", err)
	}
}

func TestSetPolicy_NoStoreReturnsErr(t *testing.T) {
	svc := newServiceForTest(t, DefaultPolicy(), nil)
	err := svc.SetPolicy(context.Background(), DefaultPolicy())
	if !errors.Is(err, ErrPolicyStoreUnavailable) {
		t.Fatalf("expected ErrPolicyStoreUnavailable, got %v", err)
	}
}

func TestValidatePolicy(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(*Policy)
		wantOK bool
	}{
		{"default", func(p *Policy) {}, true},
		{"hi pct zero", func(p *Policy) { p.HighLiquidityOfferPct = 0 }, false},
		{"hi pct over 1", func(p *Policy) { p.HighLiquidityOfferPct = 1.2 }, false},
		{"default pct over hi", func(p *Policy) { p.DefaultOfferPct = 0.9 }, false},
		{"min confidence too high", func(p *Policy) { p.MinConfidence = 11 }, false},
		{"negative velocity", func(p *Policy) { p.MinQuarterVelocity = -1 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultPolicy()
			tc.mut(&p)
			err := ValidatePolicy(p)
			if tc.wantOK && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.wantOK && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// newServiceForTest returns the concrete *service so tests can exercise the
// non-interface methods that take Service receivers.
func newServiceForTest(_ *testing.T, seed Policy, store PolicyStore) *service {
	s := &service{
		client:         nil,
		clock:          time.Now,
		cacheTTL:       0,
		policy:         seed,
		policyCacheTTL: 50 * time.Millisecond,
		cache:          map[string]cardLadderEntry{},
	}
	if store != nil {
		s.store = store
	}
	return s
}
