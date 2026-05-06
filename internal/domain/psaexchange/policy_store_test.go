package psaexchange_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psaexchange"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newSvc returns a Service wired with the supplied seed and (optional) store.
// A short policy cache TTL exercises the post-Set reseed path while still
// allowing the cache to satisfy the immediate next EffectivePolicy call —
// the behavior we actually want the tests to validate.
func newSvc(seed psaexchange.Policy, store psaexchange.PolicyStore) psaexchange.Service {
	opts := []psaexchange.Option{
		psaexchange.WithPolicy(seed),
		psaexchange.WithPolicyCacheTTL(30 * time.Second),
	}
	if store != nil {
		opts = append(opts, psaexchange.WithPolicyStore(store))
	}
	return psaexchange.NewService(nil, opts...)
}

func TestEffectivePolicy(t *testing.T) {
	seed := psaexchange.DefaultPolicy()
	seed.HighLiquidityOfferPct = 0.8 // distinguishable from default

	cases := []struct {
		name    string
		store   *mocks.MockPSAExchangePolicyStore
		wantPct float64
	}{
		{
			name:    "no store falls back to seed",
			store:   nil,
			wantPct: 0.8,
		},
		{
			name: "empty store falls back to seed",
			store: &mocks.MockPSAExchangePolicyStore{
				GetFn: func(_ context.Context) (psaexchange.Policy, bool, error) {
					return psaexchange.Policy{}, false, nil
				},
			},
			wantPct: 0.8,
		},
		{
			name: "stored row overrides seed",
			store: &mocks.MockPSAExchangePolicyStore{
				GetFn: func(_ context.Context) (psaexchange.Policy, bool, error) {
					stored := psaexchange.DefaultPolicy()
					stored.HighLiquidityOfferPct = 0.9
					return stored, true, nil
				},
			},
			wantPct: 0.9,
		},
		{
			name: "store error falls back to seed",
			store: &mocks.MockPSAExchangePolicyStore{
				GetFn: func(_ context.Context) (psaexchange.Policy, bool, error) {
					return psaexchange.Policy{}, false, errors.New("boom")
				},
			},
			wantPct: 0.8,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var store psaexchange.PolicyStore
			if tc.store != nil {
				store = tc.store
			}
			svc := newSvc(seed, store)
			got := svc.EffectivePolicy(context.Background()).HighLiquidityOfferPct
			if got != tc.wantPct {
				t.Fatalf("HighLiquidityOfferPct = %v, want %v", got, tc.wantPct)
			}
		})
	}
}

func TestSetPolicy(t *testing.T) {
	updated := psaexchange.DefaultPolicy()
	updated.HighLiquidityOfferPct = 0.85

	invalid := psaexchange.DefaultPolicy()
	invalid.DefaultOfferPct = 0.9 // > HighLiquidityOfferPct (0.75) → tier inversion

	cases := []struct {
		name           string
		store          *mocks.MockPSAExchangePolicyStore
		input          psaexchange.Policy
		wantErr        error
		wantPersisted  int
		wantEffective  float64 // -1 to skip post-Set EffectivePolicy assertion
	}{
		{
			name:          "persists and invalidates cache",
			store:         &mocks.MockPSAExchangePolicyStore{},
			input:         updated,
			wantPersisted: 1,
			wantEffective: 0.85,
		},
		{
			name:          "rejects invalid policy without persisting",
			store:         &mocks.MockPSAExchangePolicyStore{},
			input:         invalid,
			wantErr:       psaexchange.ErrInvalidPolicy,
			wantPersisted: 0,
			wantEffective: -1,
		},
		{
			name:          "no store returns ErrPolicyStoreUnavailable",
			store:         nil,
			input:         updated,
			wantErr:       psaexchange.ErrPolicyStoreUnavailable,
			wantPersisted: 0,
			wantEffective: -1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var store psaexchange.PolicyStore
			if tc.store != nil {
				store = tc.store
			}
			svc := newSvc(psaexchange.DefaultPolicy(), store)
			ctx := context.Background()

			// Prime the cache so we can verify SetPolicy invalidates it.
			_ = svc.EffectivePolicy(ctx)

			err := svc.SetPolicy(ctx, tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("SetPolicy: %v", err)
			}

			if tc.store != nil && len(tc.store.SetCalls) != tc.wantPersisted {
				t.Fatalf("SetCalls = %d, want %d", len(tc.store.SetCalls), tc.wantPersisted)
			}

			if tc.wantEffective >= 0 {
				got := svc.EffectivePolicy(ctx).HighLiquidityOfferPct
				if got != tc.wantEffective {
					t.Fatalf("post-Set EffectivePolicy hi pct = %v, want %v", got, tc.wantEffective)
				}
			}
		})
	}
}

func TestValidatePolicy(t *testing.T) {
	cases := []struct {
		name   string
		mut    func(*psaexchange.Policy)
		wantOK bool
	}{
		{"default", func(p *psaexchange.Policy) {}, true},
		{"hi pct zero", func(p *psaexchange.Policy) { p.HighLiquidityOfferPct = 0 }, false},
		{"hi pct over 1", func(p *psaexchange.Policy) { p.HighLiquidityOfferPct = 1.2 }, false},
		{"default pct over hi", func(p *psaexchange.Policy) { p.DefaultOfferPct = 0.9 }, false},
		{"min confidence too high", func(p *psaexchange.Policy) { p.MinConfidence = 11 }, false},
		{"negative velocity", func(p *psaexchange.Policy) { p.MinQuarterVelocity = -1 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := psaexchange.DefaultPolicy()
			tc.mut(&p)
			err := psaexchange.ValidatePolicy(p)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				return
			}
			if !errors.Is(err, psaexchange.ErrInvalidPolicy) {
				t.Fatalf("expected ErrInvalidPolicy, got %v", err)
			}
		})
	}
}
