package psaexchange

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CategoryPokemon is the catalog category we filter on in v1.
const CategoryPokemon = "POKEMON CARDS"

// DefaultCardLadderTTL is the default TTL for cached cardladder responses.
// 24h is conservative because PSA-Exchange has not published rate limits.
const DefaultCardLadderTTL = 24 * time.Hour

// Service is the domain entry point for PSA-Exchange opportunities.
type Service interface {
	Opportunities(ctx context.Context) (OpportunitiesResult, error)
	// Policy returns the seed policy (env / DefaultPolicy) — the fallback used
	// when no row is persisted. Use EffectivePolicy for what the next
	// Opportunities() call will actually apply.
	Policy() Policy
	// EffectivePolicy returns the policy that will be applied to the next
	// Opportunities() call: the persisted row if any, else the seed.
	EffectivePolicy(ctx context.Context) Policy
	// SetPolicy persists a new policy. Validates the inputs before writing.
	// Returns ErrPolicyStoreUnavailable if no PolicyStore was configured.
	SetPolicy(ctx context.Context, p Policy) error
}

// PolicyStore persists the active scoring/filter policy. The store always
// represents a single logical row — Get returns (Policy, false, nil) when
// nothing is persisted; the service falls back to the seed policy in that
// case.
type PolicyStore interface {
	Get(ctx context.Context) (Policy, bool, error)
	Set(ctx context.Context, p Policy) error
}

// ErrPolicyStoreUnavailable is returned by SetPolicy when no PolicyStore was
// wired (e.g. test setups, integrations disabled).
var ErrPolicyStoreUnavailable = errors.New("psaexchange: policy store unavailable")

type cardLadderEntry struct {
	cl        CardLadder
	expiresAt time.Time
}

type service struct {
	client   CatalogClient
	logger   observability.Logger
	clock    func() time.Time
	cacheTTL time.Duration
	policy   Policy // seed (env / DefaultPolicy) — fallback when no row is stored
	store    PolicyStore

	policyMu       sync.RWMutex
	policyCache    Policy
	policyCacheSet bool
	policyCacheExp time.Time
	policyCacheTTL time.Duration

	mu    sync.Mutex
	cache map[string]cardLadderEntry
}

// Option configures a Service.
type Option func(*service)

// WithLogger attaches a structured logger.
func WithLogger(l observability.Logger) Option {
	return func(s *service) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithCardLadderCacheTTL overrides the per-cert cardladder cache TTL.
// Pass a non-positive duration to disable caching entirely.
func WithCardLadderCacheTTL(d time.Duration) Option {
	return func(s *service) { s.cacheTTL = d }
}

// WithClock injects a deterministic clock; intended for tests.
func WithClock(fn func() time.Time) Option {
	return func(s *service) { s.clock = fn }
}

// WithPolicy merges the non-zero fields of p onto the existing policy
// (defaulting to DefaultPolicy()), so callers can override individual levers
// without having to re-specify the rest. Pass DefaultPolicy() with explicit
// overrides for full control. The merged result is stored as the SEED — it is
// the fallback when no PolicyStore row exists.
func WithPolicy(p Policy) Option {
	return func(s *service) {
		base := s.policy
		if base == (Policy{}) {
			base = DefaultPolicy()
		}
		if p.HighLiquidityVelocity != 0 {
			base.HighLiquidityVelocity = p.HighLiquidityVelocity
		}
		if p.HighLiquidityConfidence != 0 {
			base.HighLiquidityConfidence = p.HighLiquidityConfidence
		}
		if p.HighLiquidityOfferPct != 0 {
			base.HighLiquidityOfferPct = p.HighLiquidityOfferPct
		}
		if p.DefaultOfferPct != 0 {
			base.DefaultOfferPct = p.DefaultOfferPct
		}
		if p.MinConfidence != 0 {
			base.MinConfidence = p.MinConfidence
		}
		if p.MinQuarterVelocity != 0 {
			base.MinQuarterVelocity = p.MinQuarterVelocity
		}
		s.policy = base
	}
}

// WithPolicyStore wires a persistent store for the active policy. When set,
// EffectivePolicy reads from the store (with a short TTL cache) and falls back
// to the seed policy when the store has no row.
func WithPolicyStore(s PolicyStore) Option {
	return func(svc *service) { svc.store = s }
}

// WithPolicyCacheTTL overrides how long EffectivePolicy caches the value
// fetched from the PolicyStore. Default: 10s.
func WithPolicyCacheTTL(d time.Duration) Option {
	return func(svc *service) { svc.policyCacheTTL = d }
}

// NewService constructs a Service.
func NewService(client CatalogClient, opts ...Option) Service {
	s := &service{
		client:         client,
		clock:          time.Now,
		cacheTTL:       DefaultCardLadderTTL,
		policy:         DefaultPolicy(),
		policyCacheTTL: 10 * time.Second,
		cache:          map[string]cardLadderEntry{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Policy returns the SEED policy (env / DefaultPolicy). This is the fallback
// when no row is persisted in the PolicyStore. Use EffectivePolicy for the
// values that will be applied to the next Opportunities() call.
func (s *service) Policy() Policy { return s.policy }

// EffectivePolicy returns the policy that will be applied to the next
// Opportunities() call: the row from the PolicyStore if present, else the
// seed. Cached for policyCacheTTL to avoid hammering the DB on every page
// load — Set invalidates the cache so admin saves take effect immediately.
func (s *service) EffectivePolicy(ctx context.Context) Policy {
	if s.store == nil {
		return s.policy
	}
	s.policyMu.RLock()
	if s.policyCacheSet && s.clock().Before(s.policyCacheExp) {
		p := s.policyCache
		s.policyMu.RUnlock()
		return p
	}
	s.policyMu.RUnlock()
	stored, ok, err := s.store.Get(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "psa_exchange.policy_store_read_failed", observability.Err(err))
		}
		return s.policy
	}
	effective := s.policy
	if ok {
		effective = stored
	}
	s.policyMu.Lock()
	s.policyCache = effective
	s.policyCacheSet = true
	s.policyCacheExp = s.clock().Add(s.policyCacheTTL)
	s.policyMu.Unlock()
	return effective
}

// SetPolicy validates and persists a new policy, then invalidates the cache so
// EffectivePolicy reads the new value on the next call.
func (s *service) SetPolicy(ctx context.Context, p Policy) error {
	if s.store == nil {
		return ErrPolicyStoreUnavailable
	}
	if err := ValidatePolicy(p); err != nil {
		return err
	}
	if err := s.store.Set(ctx, p); err != nil {
		return fmt.Errorf("psaexchange: persist policy: %w", err)
	}
	s.policyMu.Lock()
	s.policyCache = p
	s.policyCacheSet = true
	s.policyCacheExp = s.clock().Add(s.policyCacheTTL)
	s.policyMu.Unlock()
	return nil
}

func (s *service) Opportunities(ctx context.Context) (OpportunitiesResult, error) {
	cat, err := s.client.FetchCatalog(ctx)
	if err != nil {
		return OpportunitiesResult{}, fmt.Errorf("psaexchange: fetch catalog: %w", err)
	}

	policy := s.EffectivePolicy(ctx)

	var pokemon []CatalogCard
	for _, c := range cat.Cards {
		if c.Category == CategoryPokemon {
			pokemon = append(pokemon, c)
		}
	}

	listings := make([]Listing, 0, len(pokemon))
	enrichErrs := 0
	for _, c := range pokemon {
		cl, err := s.fetchCardLadderCached(ctx, c.Cert)
		if err != nil {
			enrichErrs++
			if s.logger != nil {
				s.logger.Warn(ctx, "psa_exchange.cardladder_failed",
					observability.String("cert", c.Cert),
					observability.Err(err))
			}
			continue
		}
		if cl.Confidence < policy.MinConfidence || cl.OneQuarterData.Velocity < policy.MinQuarterVelocity {
			continue
		}
		listings = append(listings, buildListing(c, cl, policy))
	}

	sort.SliceStable(listings, func(i, j int) bool {
		if listings[i].Score == listings[j].Score {
			return listings[i].Cert < listings[j].Cert
		}
		return listings[i].Score > listings[j].Score
	})

	return OpportunitiesResult{
		Opportunities:    listings,
		CategoryURL:      s.client.CategoryURL(CategoryPokemon),
		FetchedAt:        s.clock(),
		TotalCatalog:     len(pokemon),
		AfterFilter:      len(listings),
		EnrichmentErrors: enrichErrs,
	}, nil
}

// buildListing merges catalog row + cardladder data + score into a Listing.
func buildListing(c CatalogCard, cl CardLadder, p Policy) Listing {
	listCents := dollarsToCents(c.Price)
	compCents := dollarsToCents(cl.EstimatedValue)
	score := p.Score(ScoreInputs{
		ListPriceCents: listCents,
		CompCents:      compCents,
		VelocityMonth:  cl.OneMonthData.Velocity,
		Confidence:     cl.Confidence,
	})
	return Listing{
		Cert:               c.Cert,
		Name:               c.Name,
		Description:        cl.Description,
		Grade:              c.Grade,
		ListPriceCents:     listCents,
		TargetOfferCents:   score.TargetOfferCents,
		MaxOfferPct:        score.MaxOfferPct,
		CompCents:          compCents,
		LastSalePriceCents: dollarsToCents(cl.LastSalePrice),
		LastSaleDate:       cl.LastSaleDate,
		VelocityMonth:      cl.OneMonthData.Velocity,
		VelocityQuarter:    cl.OneQuarterData.Velocity,
		Confidence:         cl.Confidence,
		Population:         cl.Population,
		EdgeAtOffer:        score.EdgeAtOffer,
		Score:              score.Score,
		ListRunwayPct:      score.ListRunwayPct,
		MayTakeAtList:      score.MayTakeAtList,
		FrontImage:         c.Front,
		BackImage:          c.Back,
		IndexID:            cl.IndexID,
		Tier:               score.Tier.Name,
	}
}

// dollarsToCents converts a USD float (as returned by upstream APIs) to cents.
// Rounds to nearest cent.
func dollarsToCents(d float64) int64 {
	if d <= 0 {
		return 0
	}
	return int64(d*100 + 0.5)
}

// fetchCardLadderCached returns a cached cardladder response if one is fresh,
// otherwise calls the adapter and stores the result. Errors are NOT cached.
func (s *service) fetchCardLadderCached(ctx context.Context, cert string) (CardLadder, error) {
	if s.cacheTTL > 0 {
		s.mu.Lock()
		entry, ok := s.cache[cert]
		s.mu.Unlock()
		if ok && s.clock().Before(entry.expiresAt) {
			return entry.cl, nil
		}
	}
	cl, err := s.client.FetchCardLadder(ctx, cert)
	if err != nil {
		return CardLadder{}, err
	}
	if s.cacheTTL > 0 {
		s.mu.Lock()
		s.cache[cert] = cardLadderEntry{cl: cl, expiresAt: s.clock().Add(s.cacheTTL)}
		s.mu.Unlock()
	}
	return cl, nil
}
