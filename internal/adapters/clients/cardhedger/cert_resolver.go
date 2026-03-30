package cardhedger

import (
	"context"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// MappingSaver persists card ID mappings discovered during cert resolution.
type MappingSaver interface {
	SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error
}

// MissingCardTracker records certs whose cards are not yet linked in CardHedger.
type MissingCardTracker interface {
	TrackMissingCert(ctx context.Context, cert, grader, grade, description string) error
}

// CertResolverOption configures a CertResolver after construction.
type CertResolverOption func(*CertResolver)

// WithMissingCardTracker sets a tracker that records unlinked certs.
func WithMissingCardTracker(t MissingCardTracker) CertResolverOption {
	return func(r *CertResolver) { r.missingCardTracker = t }
}

// CertResolver resolves cert numbers to CardHedger card_ids via the details-by-certs endpoint.
// Implements campaigns.CardIDResolver.
type CertResolver struct {
	client             *Client
	mappingSaver       MappingSaver
	missingCardTracker MissingCardTracker
	logger             observability.Logger
}

// NewCertResolver creates a CertResolver that calls details-by-certs and caches mappings.
func NewCertResolver(client *Client, mappingSaver MappingSaver, logger observability.Logger, opts ...CertResolverOption) *CertResolver {
	r := &CertResolver{
		client:       client,
		mappingSaver: mappingSaver,
		logger:       logger,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolveCardIDsByCerts resolves cert numbers to CardHedger card_ids in batch.
// Returns a map of certNumber -> cardID for successfully resolved certs.
func (r *CertResolver) ResolveCardIDsByCerts(ctx context.Context, certs []string, grader string) (map[string]string, error) {
	if r.client == nil || !r.client.Available() {
		return nil, fmt.Errorf("cardhedger client not available")
	}
	if len(certs) == 0 {
		return nil, nil
	}

	result := make(map[string]string)
	var errs []error

	// Process in chunks of 100 (API limit)
	for i := 0; i < len(certs); i += 100 {
		end := min(i+100, len(certs))
		chunk := certs[i:end]

		resp, _, _, err := r.client.DetailsByCerts(ctx, chunk, grader)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn(ctx, "details-by-certs failed",
					observability.Int("chunk_start", i),
					observability.Int("chunk_size", len(chunk)),
					observability.Err(err))
			}
			errs = append(errs, fmt.Errorf("chunk %d-%d: %w", i, end, err))
			continue
		}
		if resp == nil {
			if r.logger != nil {
				r.logger.Warn(ctx, "details-by-certs returned nil response",
					observability.Int("chunk_start", i),
					observability.Int("chunk_size", len(chunk)))
			}
			errs = append(errs, fmt.Errorf("chunk %d-%d: nil response", i, end))
			continue
		}

		for _, detail := range resp.Results {
			if detail.Card == nil || detail.Card.CardID == "" {
				if r.missingCardTracker != nil && detail.CertInfo.Cert != "" {
					if trackErr := r.missingCardTracker.TrackMissingCert(ctx, detail.CertInfo.Cert, grader, detail.CertInfo.Grade, detail.CertInfo.Description); trackErr != nil && r.logger != nil {
						r.logger.Warn(ctx, "failed to track missing cert",
							observability.String("cert", detail.CertInfo.Cert),
							observability.String("grader", grader),
							observability.Err(trackErr))
					}
				}
				continue
			}
			cert := detail.CertInfo.Cert
			if cert == "" {
				continue
			}
			result[cert] = detail.Card.CardID

			// Cache the mapping using CardHedger's canonical card identity
			if r.mappingSaver != nil {
				cardName := detail.Card.Player
				if cardName == "" {
					cardName = detail.Card.Description
				}
				if err := r.mappingSaver.SaveExternalID(ctx, cardName, detail.Card.Set, detail.Card.Number, pricing.SourceCardHedger, detail.Card.CardID); err != nil {
					if r.logger != nil {
						r.logger.Debug(ctx, "failed to cache cert mapping",
							observability.String("cert", cert),
							observability.Err(err))
					}
				}
			}
		}

		if r.logger != nil {
			r.logger.Info(ctx, "details-by-certs chunk resolved",
				observability.Int("requested", len(chunk)),
				observability.Int("found", len(resp.Results)))
		}
	}

	if len(errs) > 0 {
		return result, fmt.Errorf("details-by-certs: %d/%d chunks failed: %w",
			len(errs), (len(certs)+99)/100, errors.Join(errs...))
	}
	return result, nil
}
