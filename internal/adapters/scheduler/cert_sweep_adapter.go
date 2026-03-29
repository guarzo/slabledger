package scheduler

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// PurchaseCertLister lists cert numbers of purchases missing external ID mappings.
type PurchaseCertLister interface {
	ListUnmappedPurchaseCerts(ctx context.Context, provider string) ([]string, error)
}

// CertSweepAdapter implements CertSweeper by querying unmapped purchases
// and resolving their certs via the CardIDResolver (DetailsByCerts).
type CertSweepAdapter struct {
	repo     PurchaseCertLister
	resolver campaigns.CardIDResolver
	logger   observability.Logger
}

// NewCertSweepAdapter creates a new CertSweepAdapter.
func NewCertSweepAdapter(repo PurchaseCertLister, resolver campaigns.CardIDResolver, logger observability.Logger) *CertSweepAdapter {
	return &CertSweepAdapter{repo: repo, resolver: resolver, logger: logger}
}

// SweepUnmappedCerts finds purchases without a CardHedger card_id mapping
// and resolves their cert numbers via DetailsByCerts.
func (a *CertSweepAdapter) SweepUnmappedCerts(ctx context.Context) (int, error) {
	certs, err := a.repo.ListUnmappedPurchaseCerts(ctx, "cardhedger")
	if err != nil {
		return 0, err
	}
	if len(certs) == 0 {
		return 0, nil
	}

	if a.logger != nil {
		a.logger.Info(ctx, "cert sweep: resolving unmapped certs",
			observability.Int("count", len(certs)))
	}

	resolved, err := a.resolver.ResolveCardIDsByCerts(ctx, certs, "PSA")
	if err != nil {
		return len(resolved), err
	}
	return len(resolved), nil
}
