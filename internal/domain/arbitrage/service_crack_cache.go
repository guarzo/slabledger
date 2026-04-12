package arbitrage

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// StartCrackCacheWorker launches the background crack cache refresh goroutine.
func (s *service) StartCrackCacheWorker(ctx context.Context) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go s.crackCacheWorker(cctx)
	return cancel
}

// BuildCrackCandidateSet returns the cached set of crack candidate purchase IDs.
func (s *service) BuildCrackCandidateSet(ctx context.Context) map[string]bool {
	s.crackCacheMu.RLock()
	set := s.crackCacheSet
	s.crackCacheMu.RUnlock()
	if set == nil && s.logger != nil {
		s.logger.Info(ctx, "crack cache not yet populated, signals will be incomplete")
	}
	return set
}

// computeCrackOpportunitiesLive computes crack opportunities by making live API calls.
func (s *service) computeCrackOpportunitiesLive(ctx context.Context) ([]CrackAnalysis, error) {
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list active campaigns: %w", err)
	}
	var allResults []CrackAnalysis
	for _, campaign := range allCampaigns {
		results, err := s.crackCandidatesForCampaign(ctx, &campaign)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "crack candidates failed for campaign",
					observability.String("campaignId", campaign.ID),
					observability.Err(err))
			}
			continue
		}
		allResults = append(allResults, results...)
	}
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].CrackAdvantage > allResults[j].CrackAdvantage
	})
	return allResults, nil
}

// refreshCrackCandidates recomputes the crack candidate set and stores it in the cache.
func (s *service) refreshCrackCandidates(ctx context.Context) error {
	cracks, err := s.computeCrackOpportunitiesLive(ctx)
	if err != nil {
		return err
	}
	set := make(map[string]bool, len(cracks))
	for _, c := range cracks {
		if c.IsCrackCandidate {
			set[c.PurchaseID] = true
		}
	}
	s.crackCacheMu.Lock()
	s.crackCacheSet = set
	s.crackCacheAll = cracks
	s.crackCacheMu.Unlock()
	return nil
}

// crackCacheWorker periodically refreshes the crack candidate cache.
func (s *service) crackCacheWorker(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := s.refreshCrackCandidates(ctx); err != nil && s.logger != nil {
		s.logger.Error(ctx, "initial crack cache refresh failed — inventory signals unavailable", observability.Err(err))
	}
	ticker := time.NewTicker(crackCacheRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshCrackCandidates(ctx); err != nil && s.logger != nil {
				s.logger.Warn(ctx, "crack cache refresh failed", observability.Err(err))
			}
		}
	}
}
