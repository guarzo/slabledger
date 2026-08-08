package inventory

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error) {
	if s.certLookup == nil {
		return nil, nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("cert lookup: %w", err)
	}

	var snapshot *MarketSnapshot
	if s.priceProv != nil && info.CardName != "" && info.Grade > 0 {
		resolvedCategory := ResolvePSACategory(info.Category)
		if IsGenericSetName(resolvedCategory) {
			resolvedCategory = info.Category
		}
		snapshot, err = s.priceProv.GetMarketSnapshot(ctx, CardIdentity{CardName: info.CardName, CardNumber: info.CardNumber, SetName: resolvedCategory}, info.Grade)
		if err != nil && s.logger != nil {
			s.logger.Debug(ctx, "GetMarketSnapshot for cert lookup failed",
				observability.String("card", info.CardName),
				observability.Err(err))
		}
	}

	return info, snapshot, nil
}
