package psaexchange

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CategoryPokemon is the catalog category we filter on in v1.
const CategoryPokemon = "POKEMON CARDS"

// Filter thresholds for v1 (admin-configurable in v2).
const (
	minConfidence      = 3
	minQuarterVelocity = 1
)

// DefaultCardLadderTTL is the default TTL for cached cardladder responses.
// 24h is conservative because PSA-Exchange has not published rate limits.
const DefaultCardLadderTTL = 24 * time.Hour

// Service is the domain entry point for PSA-Exchange opportunities.
type Service interface {
	Opportunities(ctx context.Context) (OpportunitiesResult, error)
}

type cardLadderEntry struct {
	cl        CardLadder
	expiresAt time.Time
}

type service struct {
	client   CatalogClient
	logger   observability.Logger
	clock    func() time.Time
	cacheTTL time.Duration

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

// withClock is package-private; used by tests to inject a deterministic clock.
func withClock(fn func() time.Time) Option {
	return func(s *service) { s.clock = fn }
}

// NewService constructs a Service.
func NewService(client CatalogClient, opts ...Option) Service {
	s := &service{
		client:   client,
		clock:    time.Now,
		cacheTTL: DefaultCardLadderTTL,
		cache:    map[string]cardLadderEntry{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *service) Opportunities(ctx context.Context) (OpportunitiesResult, error) {
	cat, err := s.client.FetchCatalog(ctx)
	if err != nil {
		return OpportunitiesResult{}, fmt.Errorf("psaexchange: fetch catalog: %w", err)
	}

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
		if cl.Confidence < minConfidence || cl.OneQuarterData.Velocity < minQuarterVelocity {
			continue
		}
		listings = append(listings, buildListing(c, cl))
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
func buildListing(c CatalogCard, cl CardLadder) Listing {
	listCents := dollarsToCents(c.Price)
	compCents := dollarsToCents(cl.EstimatedValue)
	score := ScoreListing(ScoreInputs{
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
