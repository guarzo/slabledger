package psa

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// CertAdapter wraps a PSA Client to implement inventory.CertLookup.
//
// A single adapter instance is shared by the HTTP handler and the background
// enrichment job (see cmd/slabledger/init_inventory_services.go), so its cache
// spans the whole scan → import → enrich path.
type CertAdapter struct {
	client *Client
	cache  *certCache
}

var _ inventory.CertLookup = (*CertAdapter)(nil)

// NewCertAdapter creates a CertLookup adapter around a PSA client.
func NewCertAdapter(client *Client) *CertAdapter {
	return &CertAdapter{client: client, cache: newCertCache()}
}

// LookupCert resolves a PSA certificate number to card details, serving
// repeat lookups of the same cert from a short-lived cache.
func (a *CertAdapter) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if cached, ok := a.cache.get(certNumber); ok {
		return cached, nil
	}

	info, err := a.client.GetCert(ctx, certNumber)
	if err != nil {
		return nil, fmt.Errorf("PSA cert lookup %s: %w", certNumber, err)
	}

	grade := ParseGrade(info.CardGrade)
	if grade == 0 {
		grade = ParseGrade(info.GradeDescription)
	}

	result := &inventory.CertInfo{
		CertNumber: info.CertNumber,
		CardName:   BuildCardName(info),
		Grade:      grade, // float64 from ParseGrade
		Year:       info.Year,
		Brand:      info.Brand,
		Category:   info.Category,
		Subject:    info.Subject,
		Variety:    info.Variety,
		CardNumber: info.CardNumber,
		Population: info.TotalPopulation,
		PopHigher:  info.PopulationHigher,
	}
	a.cache.put(certNumber, result)
	return result, nil
}

// LookupImages resolves a PSA certificate number to front/back slab image URLs.
func (a *CertAdapter) LookupImages(ctx context.Context, certNumber string) (string, string, error) {
	images, err := a.client.GetImages(ctx, certNumber)
	if err != nil {
		return "", "", fmt.Errorf("PSA image lookup %s: %w", certNumber, err)
	}

	var front, back string
	for _, img := range images {
		if img.IsFrontImage {
			front = img.ImageURL
		} else {
			back = img.ImageURL
		}
	}
	return front, back, nil
}
